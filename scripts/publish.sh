#!/usr/bin/env bash
# GitHub Actions only. Called after both native verification jobs succeeded.
set -euo pipefail
: "${GH_REPO:?}" "${RELEASE_REF:?}" "${RELEASE_SHA:?}" "${RELEASE_OUT:?}"
version=$(bash scripts/release.sh identity)
manual_script="$(pwd)/scripts/manual_release.py"
cd "$RELEASE_OUT"
for asset in sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz; do
  test -f "$asset"
  shasum -a 256 -c "$asset.sha256"
done
cat sidecar_darwin_arm64.tar.gz.sha256 sidecar_linux_amd64.tar.gz.sha256 > checksums.txt
assets=(sidecar_darwin_arm64.tar.gz sidecar_linux_amd64.tar.gz checksums.txt)
if [[ $RELEASE_REF == refs/heads/main ]]; then
  # Serialized publishers must recheck freshness: an old queued run cannot win.
  head=$(gh api "repos/$GH_REPO/git/ref/heads/main" --jq .object.sha)
  [[ $head == "$RELEASE_SHA" ]] || { echo 'Refusing stale rolling publication' >&2; exit 1; }
  # Fail on authentication/server errors instead of treating them as absence.
  tags=$(gh api --paginate "repos/$GH_REPO/git/refs/tags" --jq '.[].ref')
  if grep -Fxq refs/tags/latest <<< "$tags"; then
    gh api --method PATCH "repos/$GH_REPO/git/refs/tags/latest" -f sha="$RELEASE_SHA" -F force=true
  else
    gh api --method POST "repos/$GH_REPO/git/refs" -f ref=refs/tags/latest -f sha="$RELEASE_SHA"
  fi
  releases=$(gh api --paginate "repos/$GH_REPO/releases" --jq '.[].tag_name')
  if grep -Fxq latest <<< "$releases"; then gh release delete latest --yes; fi
  gh release create latest --verify-tag --title 'Latest build' --latest=false --notes "CLI-only rolling build $version ($RELEASE_SHA). Neovim is deferred." "${assets[@]}"
else
  tag=${RELEASE_REF#refs/tags/}
  # Existing-tag mode must never create a tag.
  if [[ ${RELEASE_MODE:-existing} == existing ]]; then
    target=$(gh api "repos/$GH_REPO/commits/$tag" --jq .sha)
    [[ $target == "$RELEASE_SHA" ]] || { echo 'Stable tag changed' >&2; exit 1; }
  elif [[ ${RELEASE_MODE:-existing} != manual ]]; then
    echo 'Invalid publication mode' >&2; exit 1
  fi
  releases=$(gh api --paginate "repos/$GH_REPO/releases" --jq '.[].tag_name')
  ! grep -Fxq "$tag" <<< "$releases" || { echo 'Stable release already exists; refusing replacement' >&2; exit 1; }
  if [[ ${RELEASE_MODE:-existing} == manual ]]; then
    python3 "$manual_script" create
    target=$(gh api "repos/$GH_REPO/commits/$tag" --jq .sha)
    [[ $target == "$RELEASE_SHA" ]] || { echo 'Stable tag changed' >&2; exit 1; }
  fi
  gh release create "$tag" --verify-tag --title "$tag" --latest --notes "CLI-only stable build $version ($RELEASE_SHA). Neovim is deferred." "${assets[@]}"
fi
