#!/usr/bin/env bash
# Offline negative-boundary tests. Fixture seals below are NOT verification claims.
set -euo pipefail
python3 scripts/version_release_test.py
python3 scripts/update_homebrew_test.py
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
# Publication tests run the actual publisher and version gate in a disposable
# committed repository. Only gh is fake; no network or real remote writes.
mkdir -p "$work/tools" "$work/publish" "$work/repo/scripts"
cp scripts/publish.sh scripts/release.sh scripts/version_release.py "$work/repo/scripts/"
printf '1.2.3\n' > "$work/repo/VERSION"
git -C "$work/repo" init -q
git -C "$work/repo" add VERSION scripts
git -C "$work/repo" -c user.name=Fixture -c user.email=fixture@example.invalid commit -qm fixture
export RELEASE_SHA
RELEASE_SHA=$(git -C "$work/repo" rev-parse HEAD)
export GITHUB_SHA="$RELEASE_SHA" GITHUB_EVENT_NAME=push GITHUB_REF=refs/heads/main
export RELEASE_REF=refs/tags/v1.2.3 RELEASE_BASE=refs/tags/v1.2.2
for platform in darwin_arm64 linux_amd64; do
  printf '%s archive\n' "$platform" > "$work/publish/sidecar_$platform.tar.gz"
  (cd "$work/publish" && shasum -a 256 "sidecar_$platform.tar.gz" > "sidecar_$platform.tar.gz.sha256")
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
  'api --paginate repos/example/repo/releases --jq .[].tag_name') printf '%s\n' "${EXISTING_RELEASE:-}" ;;
  'api --paginate --slurp repos/example/repo/git/matching-refs/tags/')
    printf '[[{"ref":"%s"}]]\n' "${REMOTE_BASE:-$RELEASE_BASE}" ;;
  'api --method POST repos/example/repo/git/refs -f ref=refs/tags/v1.2.3 -f sha='*)
    touch "$TAG_CREATED"
    if [[ ${PARTIAL_TAG_FAILURE:-} == yes ]]; then echo 'tag created but response failed' >&2; exit 1; fi
    printf '{"ref":"%s","object":{"sha":"%s"}}\n' "$RELEASE_REF" "$RELEASE_SHA" ;;
  'api repos/example/repo/commits/v1.2.3 --jq .sha') printf '%s\n' "${REMOTE_SHA:-$RELEASE_SHA}" ;;
  'release create v1.2.3 --verify-tag '*) touch "$RELEASE_CREATED" ;;
  *) echo "unexpected gh invocation: $*" >&2; exit 1 ;;
esac
GH
chmod +x "$work/tools/gh"
export PATH="$work/tools:$PATH" GH_CALLS="$work/calls" GH_REPO=example/repo GH_TOKEN=offline-fixture
export RELEASE_OUT="$work/publish" TAG_CREATED="$work/tag-created" RELEASE_CREATED="$work/release-created"
cd "$work/repo"
reset_publication() { : > "$GH_CALLS"; rm -f "$TAG_CREATED" "$RELEASE_CREATED"; }
# Corrupt/missing checksums must stop before even read-only API requests.
cp "$RELEASE_OUT/sidecar_linux_amd64.tar.gz" "$work/original-archive"
printf 'tampered\n' >> "$RELEASE_OUT/sidecar_linux_amd64.tar.gz"
reset_publication
reject bash scripts/publish.sh
[[ ! -s $GH_CALLS && ! -e $TAG_CREATED && ! -e $RELEASE_CREATED ]]
cp "$work/original-archive" "$RELEASE_OUT/sidecar_linux_amd64.tar.gz"
mv "$RELEASE_OUT/sidecar_linux_amd64.tar.gz.sha256" "$work/checksum"
reset_publication
reject bash scripts/publish.sh
[[ ! -s $GH_CALLS ]]
mv "$work/checksum" "$RELEASE_OUT/sidecar_linux_amd64.tar.gz.sha256"
# Unsupported triggers cannot publish, even when handed stable identities.
for event in workflow_dispatch release; do
  reset_publication
  GITHUB_EVENT_NAME=$event reject bash scripts/publish.sh
  [[ ! -s $GH_CALLS ]]
done
reset_publication
GITHUB_REF=refs/tags/v1.2.3 reject bash scripts/publish.sh
[[ ! -s $GH_CALLS ]]
reset_publication
EXISTING_RELEASE=v1.2.3 reject bash scripts/publish.sh
grep -Fq 'Stable release already exists' "$work/output"
[[ $(wc -l < "$GH_CALLS") -eq 1 && ! -e $TAG_CREATED ]]
# A changed remote base or a VERSION/ref mismatch never creates a tag.
reset_publication
REMOTE_BASE=refs/tags/v1.2.4 reject bash scripts/publish.sh
[[ ! -e $TAG_CREATED && ! -e $RELEASE_CREATED ]]
reset_publication
RELEASE_REF=refs/tags/v1.2.4 reject bash scripts/publish.sh
[[ ! -e $TAG_CREATED && ! -e $RELEASE_CREATED ]]
# Every API failure is terminal, including failures after atomic tag creation.
for failure in 'releases --jq' 'git/matching-refs/tags/' 'api --method POST' 'commits/v1.2.3' 'release create'; do
  reset_publication
  GH_FAIL_MATCH="$failure" reject bash scripts/publish.sh
  grep -Fq 'simulated GitHub API failure' "$work/output"
  [[ $(awk 'END {print}' "$GH_CALLS") == *"$failure"* ]]
  [[ ! -e $RELEASE_CREATED ]]
  case $failure in commits/*|'release create') [[ -e $TAG_CREATED ]] ;; *) [[ ! -e $TAG_CREATED ]] ;; esac
done
reset_publication
PARTIAL_TAG_FAILURE=yes reject bash scripts/publish.sh
[[ -e $TAG_CREATED && ! -e $RELEASE_CREATED ]]
[[ $(awk 'END {print}' "$GH_CALLS") == *'api --method POST'* ]]
reset_publication
REMOTE_SHA=0000000000000000000000000000000000000000 reject bash scripts/publish.sh
grep -Fq 'Stable tag changed' "$work/output"
[[ -e $TAG_CREATED && ! -e $RELEASE_CREATED ]]
[[ $(awk 'END {print}' "$GH_CALLS") == *'commits/v1.2.3 --jq .sha' ]]
# Successful trace permits exactly one create-only tag mutation and release.
reset_publication
bash scripts/publish.sh > "$work/output"
{
  printf '%s\n' 'api --paginate repos/example/repo/releases --jq .[].tag_name' 'api --paginate --slurp repos/example/repo/git/matching-refs/tags/'
  printf 'api --method POST repos/example/repo/git/refs -f ref=refs/tags/v1.2.3 -f sha=%s\n' "$RELEASE_SHA"
  printf '%s\n' 'api repos/example/repo/commits/v1.2.3 --jq .sha'
  printf 'release create v1.2.3 --verify-tag --title v1.2.3 --latest --notes CLI-only stable build 1.2.3 (%s). Neovim is deferred. sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz checksums.txt\n' "$RELEASE_SHA"
} > "$work/expected-calls"
cmp "$work/expected-calls" "$GH_CALLS"
[[ -e $TAG_CREATED && -e $RELEASE_CREATED ]]
(cd "$RELEASE_OUT" && shasum -a 256 -c checksums.txt)
printf '%s\n' 'release identity, gates, archive bytes, checksums, overwrite, tamper and publication checks passed'
