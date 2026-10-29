#!/usr/bin/env bash
# Run from the repository root. No remote writes; CI owns publication.
set -euo pipefail
fail() { printf '%s\n' "$*" >&2; exit 1; }
identity() {
  [[ ${RELEASE_SHA:-} =~ ^[0-9a-f]{40}$ ]] || fail 'RELEASE_SHA must be a full lowercase Git SHA'
  case ${RELEASE_REF:-} in
    refs/heads/main) printf '0.0.0-main.%s\n' "$RELEASE_SHA" ;;
    refs/tags/v*)
      local version=${RELEASE_REF#refs/tags/v}
      [[ $version =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'stable tags must be canonical vMAJOR.MINOR.PATCH'
      printf '%s\n' "$version" ;;
    *) fail 'release identity requires main or a stable v* tag' ;;
  esac
}
platform() {
  local actual
  actual="$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
  [[ $actual == "${RELEASE_PLATFORM:?RELEASE_PLATFORM required}" ]] || fail "native runtime required: expected $RELEASE_PLATFORM, got $actual"
  case $actual in darwin/arm64|linux/amd64) ;; *) fail "unsupported platform: $actual" ;; esac
  [[ $(go env GOOS)/$(go env GOARCH) == "$actual" ]] || fail 'cross compilation is not native verification'
  local toolchain module_version experiments
  toolchain=$(go env GOVERSION) || fail 'cannot inspect Go toolchain version'
  module_version=$(go list -m -f '{{.GoVersion}}') || fail 'cannot inspect main module Go version'
  [[ $toolchain == "go$module_version" ]] || fail "audited release toolchain required: expected go$module_version, got $toolchain"
  experiments=$(go env GOEXPERIMENT) || fail 'cannot inspect effective Go experiments'
  [[ -z $experiments ]] || fail 'audited release configuration requires empty effective GOEXPERIMENT'
}
provenance() {
  local head status
  head=$(git rev-parse HEAD) || fail 'cannot resolve checkout HEAD'
  [[ $head == "$RELEASE_SHA" ]] || fail 'checkout does not match release SHA'
  status=$(git status --porcelain --untracked-files=normal) || fail 'cannot inspect checkout status'
  [[ -z $status ]] || fail 'release builds require a clean checkout (including untracked inputs)'
}
stage=${RELEASE_STAGE:-.tmp/release-stage}
command=${1:-}
case $command in
  identity) identity ;;
  build)
    version=$(identity); platform
    provenance
    [[ ! -e $stage ]] || fail "stage already exists: $stage"
    mkdir -p "$stage"
    go build -trimpath -ldflags "-X github.com/trogers1/sidecar-comments/internal/cli.buildVersion=$version" -o "$stage/sidecar" ./cmd/sidecar
    cp README.md LICENSE THIRD_PARTY_NOTICES "$stage/"
    printf 'version=%s\ncommit=%s\nplatform=%s\nformat=1\nprotocol=1\ngo=%s\ncorpus=%s\n' "$version" "$RELEASE_SHA" "$RELEASE_PLATFORM" "$(go env GOVERSION)" "$RELEASE_SHA" > "$stage/build-info.txt"
    (cd "$stage" && shasum -a 256 sidecar README.md LICENSE THIRD_PARTY_NOTICES build-info.txt > staged.sha256)
    ;;
  verify)
    # Any failed attempt revokes the old seal, including invalid early inputs.
    rm -f "$stage/verified.sha256"
    version=$(identity); platform; provenance
    [[ -f $stage/staged.sha256 ]] || fail 'build stage missing'
    (cd "$stage" && shasum -a 256 -c staged.sha256)
    grep -Fx "version=$version" "$stage/build-info.txt"
    grep -Fx "commit=$RELEASE_SHA" "$stage/build-info.txt"
    grep -Fx "platform=$RELEASE_PLATFORM" "$stage/build-info.txt"
    export SIDECAR_BIN="$(cd "$stage" && pwd)/sidecar" SIDECAR_CLI_VERSION="$version"
    make cli-gate
    (cd "$stage" && shasum -a 256 -c staged.sha256)
    provenance
    cp "$stage/staged.sha256" "$stage/verified.sha256"
    ;;
  package)
    version=$(identity); platform
    [[ -f $stage/verified.sha256 ]] || fail 'stage has not passed verification'
    cmp "$stage/staged.sha256" "$stage/verified.sha256"
    (cd "$stage" && shasum -a 256 -c verified.sha256)
    grep -Fx "version=$version" "$stage/build-info.txt"
    grep -Fx "commit=$RELEASE_SHA" "$stage/build-info.txt"
    grep -Fx "platform=$RELEASE_PLATFORM" "$stage/build-info.txt"
    out=${RELEASE_OUT:-.tmp/release-assets}
    mkdir -p "$out"
    asset="sidecar_${RELEASE_PLATFORM//\//_}.tar.gz"
    [[ ! -e $out/$asset ]] || fail "asset exists: $out/$asset"
    tar -czf "$out/$asset" -C "$stage" sidecar README.md LICENSE THIRD_PARTY_NOTICES build-info.txt
    (cd "$out" && shasum -a 256 "$asset" > "$asset.sha256")
    ;;
  *) fail 'usage: scripts/release.sh identity|build|verify|package' ;;
esac
