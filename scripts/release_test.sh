#!/usr/bin/env bash
# Offline negative-boundary tests. Fixture seals below are NOT verification claims.
set -euo pipefail
python3 scripts/manual_release_test.py
work=$(mktemp -d "${TMPDIR:-/tmp}/sidecar-release-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export RELEASE_SHA=0123456789abcdef0123456789abcdef01234567
export RELEASE_REF=refs/heads/main
export RELEASE_PLATFORM="$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
export RELEASE_STAGE="$work/stage" RELEASE_OUT="$work/assets"
reject() { if "$@" > "$work/output" 2>&1; then echo "unexpected success: $*" >&2; exit 1; fi; }
[[ $(bash scripts/release.sh identity) == "0.0.0-main.$RELEASE_SHA" ]]
RELEASE_REF=refs/tags/v1.2.3; export RELEASE_REF
[[ $(bash scripts/release.sh identity) == 1.2.3 ]]
for ref in refs/tags/v01.2.3 refs/tags/v1.2 refs/tags/v1.2.3-rc.1 refs/heads/topic ''; do
  RELEASE_REF=$ref reject bash scripts/release.sh identity
done
RELEASE_SHA=short reject bash scripts/release.sh identity
reject make require-sidecar-bin SIDECAR_BIN= SIDECAR_CLI_VERSION=dev
reject make require-sidecar-bin SIDECAR_BIN=/unused SIDECAR_CLI_VERSION=
reject bash scripts/release.sh package
RELEASE_PLATFORM=unsupported/arch reject bash scripts/release.sh package
# In this working-tree rehearsal a dirty checkout must fail before writing bytes.
if [[ -n $(git status --porcelain --untracked-files=normal) ]]; then
  RELEASE_SHA=$(git rev-parse HEAD) reject bash scripts/release.sh build
  grep -Fq 'release builds require a clean checkout' "$work/output"
  [[ ! -e $RELEASE_STAGE ]]
fi
mkdir -p "$RELEASE_STAGE"
printf 'fixture executable bytes\n' > "$RELEASE_STAGE/sidecar"
cp README.md LICENSE THIRD_PARTY_NOTICES "$RELEASE_STAGE/"
printf 'version=1.2.3\ncommit=%s\nplatform=%s\n' "$RELEASE_SHA" "$RELEASE_PLATFORM" > "$RELEASE_STAGE/build-info.txt"
(cd "$RELEASE_STAGE" && shasum -a 256 sidecar README.md LICENSE THIRD_PARTY_NOTICES build-info.txt > staged.sha256)
reject bash scripts/release.sh package # no successful-verification seal
cp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
bash scripts/release.sh package > "$work/output"
archive="$RELEASE_OUT/sidecar_${RELEASE_PLATFORM//\//_}.tar.gz"
mkdir "$work/unpacked"
tar -xzf "$archive" -C "$work/unpacked"
for name in sidecar README.md LICENSE THIRD_PARTY_NOTICES build-info.txt; do cmp "$RELEASE_STAGE/$name" "$work/unpacked/$name"; done
(cd "$RELEASE_OUT" && shasum -a 256 -c "${archive##*/}.sha256")
reject bash scripts/release.sh package # never overwrite
# Read-only Git fake exercises provenance transitions without changing real Git.
mkdir "$work/provenance-tools"
cat > "$work/provenance-tools/git" <<'GIT'
#!/usr/bin/env bash
set -eu
case "$*" in
  'rev-parse HEAD')
    if [[ $PROVENANCE_MODE == moved || ( $PROVENANCE_MODE == moved-after && -e $GATE_MARKER ) ]]; then
      printf '%040d\n' 0
    else printf '%s\n' "$RELEASE_SHA"; fi ;;
  'status --porcelain --untracked-files=normal')
    if [[ $PROVENANCE_MODE == dirty || ( $PROVENANCE_MODE == dirty-after && -e $GATE_MARKER ) ]]; then printf '?? unexpected-input\n'; fi ;;
  *) exit 1 ;;
esac
GIT
cat > "$work/provenance-tools/make" <<'MAKE'
#!/usr/bin/env bash
set -eu
[[ $* == cli-gate ]]
touch "$GATE_MARKER"
MAKE
chmod +x "$work/provenance-tools/git" "$work/provenance-tools/make"
export GATE_MARKER="$work/gate-ran"
for mode in dirty moved dirty-after moved-after; do
  rm -f "$GATE_MARKER"
  cp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
  PATH="$work/provenance-tools:$PATH" PROVENANCE_MODE=$mode reject bash scripts/release.sh verify
  [[ ! -e $RELEASE_STAGE/verified.sha256 ]]
  case $mode in *-after) [[ -e $GATE_MARKER ]] ;; *) [[ ! -e $GATE_MARKER ]] ;; esac
done
for early_failure in identity platform; do
  cp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
  if [[ $early_failure == identity ]]; then
    RELEASE_SHA=invalid reject bash scripts/release.sh verify
  else RELEASE_PLATFORM=unsupported/arch reject bash scripts/release.sh verify; fi
  [[ ! -e $RELEASE_STAGE/verified.sha256 ]]
done
PATH="$work/provenance-tools:$PATH" PROVENANCE_MODE=clean bash scripts/release.sh verify > "$work/output"
cmp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
# Effective go env values include persisted settings, not just shell variables.
mkdir "$work/toolchain-tools"
export REAL_GO
REAL_GO=$(command -v go)
cat > "$work/toolchain-tools/go" <<'GO'
#!/usr/bin/env bash
set -eu
case "$*" in
  'env GOVERSION')
    if [[ $TOOLCHAIN_MODE == mismatch ]]; then printf 'go0.0.0\n'; else exec "$REAL_GO" "$@"; fi ;;
  'env GOEXPERIMENT')
    if [[ $TOOLCHAIN_MODE == experiments ]]; then printf 'test-persisted-experiment\n'; else exec "$REAL_GO" "$@"; fi ;;
  *) exec "$REAL_GO" "$@" ;;
esac
GO
chmod +x "$work/toolchain-tools/go"
for mode in mismatch experiments; do
  cp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
  PATH="$work/toolchain-tools:$PATH" TOOLCHAIN_MODE=$mode reject bash scripts/release.sh verify
  [[ ! -e $RELEASE_STAGE/verified.sha256 ]]
  if [[ $mode == mismatch ]]; then reason='audited release toolchain required'; else reason='requires empty effective GOEXPERIMENT'; fi
  grep -Fq "$reason" "$work/output"
  # Even an old valid seal cannot permit packaging with unaudited settings.
  cp "$RELEASE_STAGE/staged.sha256" "$RELEASE_STAGE/verified.sha256"
  PATH="$work/toolchain-tools:$PATH" TOOLCHAIN_MODE=$mode RELEASE_OUT="$work/refused-$mode" reject bash scripts/release.sh package
  grep -Fq "$reason" "$work/output"
  [[ ! -e $work/refused-$mode ]]
  PATH="$work/toolchain-tools:$PATH" TOOLCHAIN_MODE=$mode reject bash scripts/release.sh build
  grep -Fq "$reason" "$work/output"
done
printf 'tamper\n' >> "$RELEASE_STAGE/sidecar"
reject bash scripts/release.sh package
reject bash scripts/release.sh verify
[[ ! -e $RELEASE_STAGE/verified.sha256 ]] # failed verification revokes seal
# Publication boundary fake: no GitHub requests, validate write refusal paths.
mkdir "$work/tools" "$work/publish"
printf 'darwin archive\n' > "$work/publish/sidecar_darwin_arm64.tar.gz"
printf 'linux archive\n' > "$work/publish/sidecar_linux_amd64.tar.gz"
for asset in sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz; do
  (cd "$work/publish" && shasum -a 256 "$asset" > "$asset.sha256")
done
cat > "$work/tools/gh" <<'GH'
#!/usr/bin/env bash
set -eu
printf '%s\n' "$*" >> "$GH_CALLS"
if [[ -n ${GH_FAIL_MATCH:-} && $* == *"$GH_FAIL_MATCH"* ]]; then
  echo 'simulated GitHub API failure' >&2
  exit 1
fi
case "$*" in
  'api repos/example/repo/git/ref/heads/main --jq .object.sha') printf '%s\n' "${REMOTE_SHA:-stale}" ;;
  'api repos/example/repo/commits/v1.2.3 --jq .sha') printf '%s\n' "${REMOTE_SHA:-$RELEASE_SHA}" ;;
  'api --paginate repos/example/repo/releases --jq .[].tag_name') printf '%s\n' "${EXISTING_RELEASE:-}" ;;
  'api --paginate repos/example/repo/git/refs/tags --jq .[].ref')
    if [[ -z ${EXISTING_TAGS:-} ]]; then echo 'gh: Not Found (HTTP 404)' >&2; exit 1; fi
    printf '%s\n' "$EXISTING_TAGS" ;;
  'api --paginate repos/example/repo/git/matching-refs/tags/ --jq .[].ref') printf '%s\n' "${EXISTING_TAGS:-}" ;;
  'api --method PATCH repos/example/repo/git/refs/tags/latest -f sha='*' -F force=true') ;;
  'api --method POST repos/example/repo/git/refs -f ref=refs/tags/latest -f sha='*) ;;
  'release delete latest --yes') ;;
  'release create latest '*) ;;
  'release create v1.2.3 '*) ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 1 ;;
esac
GH
chmod +x "$work/tools/gh"
export PATH="$work/tools:$PATH" GH_CALLS="$work/calls" GH_REPO=example/repo RELEASE_OUT="$work/publish"
RELEASE_REF=refs/heads/main reject bash scripts/publish.sh
grep -Fq 'Refusing stale rolling publication' "$work/output"
EXISTING_RELEASE=v1.2.3 reject bash scripts/publish.sh
grep -Fq 'Stable release already exists' "$work/output"
REMOTE_SHA=wrong reject bash scripts/publish.sh
grep -Fq 'Stable tag changed' "$work/output"
# API failures must not be mistaken for missing releases or refs.
for endpoint in commits/v1.2.3 releases; do
  : > "$GH_CALLS"
  GH_FAIL_MATCH=$endpoint reject bash scripts/publish.sh
  grep -Fq 'simulated GitHub API failure' "$work/output"
  ! grep -Eq 'release create|release delete|--method' "$GH_CALLS"
done
: > "$GH_CALLS"
RELEASE_REF=refs/heads/main GH_FAIL_MATCH=git/ref/heads/main reject bash scripts/publish.sh
grep -Fq 'simulated GitHub API failure' "$work/output"
! grep -Eq 'release create|release delete|--method' "$GH_CALLS"
# A corrupted downloaded asset must fail before even contacting GitHub.
cp "$RELEASE_OUT/sidecar_linux_amd64.tar.gz" "$work/original-archive"
printf 'tampered archive\n' >> "$RELEASE_OUT/sidecar_linux_amd64.tar.gz"
: > "$GH_CALLS"
reject bash scripts/publish.sh
[[ ! -s $GH_CALLS ]]
cp "$work/original-archive" "$RELEASE_OUT/sidecar_linux_amd64.tar.gz"
# An absent stable release creates once, without clobber/delete/tag mutations.
unset REMOTE_SHA
bash scripts/publish.sh > "$work/output"
grep -Fq 'release create v1.2.3 --verify-tag' "$GH_CALLS"
! grep -Eq 'clobber|release delete|--method' "$GH_CALLS"
# Main success traces assert the exact allowed mutations and asset arguments.
for existing in empty absent present; do
  : > "$GH_CALLS"
  if [[ $existing == present ]]; then
    tags=$'refs/tags/v1.2.3\nrefs/tags/latest'; releases=$'v1.2.3\nlatest'
  elif [[ $existing == absent ]]; then
    tags=refs/tags/v1.2.3; releases=v1.2.3
  else tags=; releases=; fi
  RELEASE_REF=refs/heads/main REMOTE_SHA=$RELEASE_SHA EXISTING_TAGS="$tags" EXISTING_RELEASE="$releases" bash scripts/publish.sh > "$work/output"
  {
    printf '%s\n' 'api repos/example/repo/git/ref/heads/main --jq .object.sha' 'api --paginate repos/example/repo/git/matching-refs/tags/ --jq .[].ref'
    if [[ $existing == present ]]; then
      printf 'api --method PATCH repos/example/repo/git/refs/tags/latest -f sha=%s -F force=true\n' "$RELEASE_SHA"
    else printf 'api --method POST repos/example/repo/git/refs -f ref=refs/tags/latest -f sha=%s\n' "$RELEASE_SHA"; fi
    printf '%s\n' 'api --paginate repos/example/repo/releases --jq .[].tag_name'
    if [[ $existing == present ]]; then printf '%s\n' 'release delete latest --yes'; fi
    printf 'release create latest --verify-tag --title Latest build --latest=false --notes CLI-only rolling build 0.0.0-main.%s (%s). Neovim is deferred. sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz checksums.txt\n' "$RELEASE_SHA" "$RELEASE_SHA"
  } > "$work/expected-calls"
  cmp "$work/expected-calls" "$GH_CALLS"
done
for failure in 'git/matching-refs/tags/ --jq' 'releases --jq' 'api --method PATCH' 'release delete' 'release create'; do
  : > "$GH_CALLS"
  RELEASE_REF=refs/heads/main REMOTE_SHA=$RELEASE_SHA EXISTING_TAGS=refs/tags/latest EXISTING_RELEASE=latest GH_FAIL_MATCH="$failure" reject bash scripts/publish.sh
  grep -Fq 'simulated GitHub API failure' "$work/output"
  # The failed command must be the last call: no mutation after any API error.
  [[ $(awk 'END {print}' "$GH_CALLS") == *"$failure"* ]]
done
: > "$GH_CALLS"
RELEASE_REF=refs/heads/main REMOTE_SHA=$RELEASE_SHA GH_FAIL_MATCH='api --method POST' reject bash scripts/publish.sh
grep -Fq 'simulated GitHub API failure' "$work/output"
[[ $(awk 'END {print}' "$GH_CALLS") == *'api --method POST'* ]]
printf '%s\n' 'release identity, gates, archive bytes, checksums, overwrite, tamper and publication checks passed'
