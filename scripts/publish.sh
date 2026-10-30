#!/usr/bin/env bash
# GitHub Actions only. Called after both native verification jobs succeeded.
set -euo pipefail
: "${GH_REPO:?}" "${RELEASE_REF:?}" "${RELEASE_SHA:?}" "${RELEASE_BASE:?}" "${RELEASE_OUT:?}"
[[ ${GITHUB_EVENT_NAME:-} == push && ${GITHUB_REF:-} == refs/heads/main ]] || { echo 'Publication requires push on main' >&2; exit 1; }
[[ $RELEASE_REF == refs/tags/v* ]] || { echo 'Publication requires a stable version tag' >&2; exit 1; }
version=$(bash scripts/release.sh identity)
version_script="$(pwd)/scripts/version_release.py"
# Validate downloaded bytes before even read-only GitHub requests.
(
  cd "$RELEASE_OUT"
  for asset in sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz; do
    test -f "$asset"
    shasum -a 256 -c "$asset.sha256"
  done
  cat sidecar_darwin_arm64.tar.gz.sha256 sidecar_linux_amd64.tar.gz.sha256 > checksums.txt
)
tag=${RELEASE_REF#refs/tags/}
releases=$(gh api --paginate "repos/$GH_REPO/releases" --jq '.[].tag_name')
! grep -Fxq "$tag" <<< "$releases" || { echo 'Stable release already exists; refusing replacement' >&2; exit 1; }
# Keep the checkout as cwd: create rechecks committed VERSION and HEAD, and
# creates the tag atomically only if the planned remote base is unchanged.
python3 "$version_script" create
target=$(gh api "repos/$GH_REPO/commits/$tag" --jq .sha)
[[ $target == "$RELEASE_SHA" ]] || { echo 'Stable tag changed' >&2; exit 1; }
cd "$RELEASE_OUT"
assets=(sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz checksums.txt)
gh release create "$tag" --verify-tag --title "$tag" --latest --notes "CLI-only stable build $version ($RELEASE_SHA). Neovim is deferred." "${assets[@]}"
