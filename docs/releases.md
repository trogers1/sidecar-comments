# CLI releases

This is a **CLI-only** release gate, not a full v1 ecosystem conformance claim.
The Neovim adapter is deferred. The historical required-red inventory and
`make full-red` remain separate, fail-closed obligations; passing the executable
seed corpus does not imply that all inventory entries have runnable proof.
See [v1 conformance](v1/conformance.md).

## Identity and assets

`cli.Version()` defaults to `dev`. Release builds derive their identity from the
checked-out full Git SHA and committed `VERSION`, never from the executable under
test. CI plans `refs/tags/v<VERSION>` before building; PR verification embeds the
same prospective stable version without creating a tag. Versions are canonical
`MAJOR.MINOR.PATCH`, with nonnegative components and no leading zeroes, suffixes,
or prerelease identifiers. The staging helper also accepts `refs/heads/main` for
local commit-specific snapshots (`0.0.0-main.<40-character commit SHA>`); these
snapshots are not published.

The linker target is
`github.com/trogers1/sidecar-comments/internal/cli.buildVersion`. Malformed linker
identities fail loudly. `SIDECAR_BIN` identifies the exact staged executable;
`SIDECAR_CLI_VERSION` supplies its independently expected identity to the tests
and independent contract runner.

Published assets have fixed names:

- `sidecar_darwin_arm64.tar.gz`
- `sidecar_linux_amd64.tar.gz`
- `checksums.txt` (SHA-256 of both archives)

Each archive contains `sidecar`, `README.md`, `LICENSE`, `THIRD_PARTY_NOTICES`, and `build-info.txt`
(version, full commit/corpus revision, native platform, Go toolchain, format and
protocol). README, MIT license, and third-party notice bytes are captured at build time and
checked alongside the executable. Archives contain precisely the staged, verified
executable bytes, not a post-test rebuild.

## Local native verification

Run all commands from the repository root. Go 1.27.1 and a native supported
host/toolchain (including race support) are required. Build, verification, and
packaging require the effective `go env GOVERSION` to equal `go` plus the main
module's declared `GoVersion` (derived with `go list -m`), and require effective
`go env GOEXPERIMENT` to be empty. This includes persisted `go env -w` settings,
not just shell variables. These restrictions preserve the exact standard-library
and runtime configuration covered by the audited bundled third-party notices;
a toolchain/configuration change requires a fresh notice audit, not an unchecked
release override. For example, on Apple silicon macOS:

```sh
export RELEASE_REF="refs/tags/v$(tr -d '\n' < VERSION)"
export RELEASE_SHA="$(git rev-parse HEAD)"
export RELEASE_PLATFORM=darwin/arm64
export RELEASE_STAGE=.tmp/release-stage
export RELEASE_OUT=.tmp/release-assets
make release
```

Use a fresh stage/output for each attempt. `build`, `verify`, and `package` may
also be run individually via `bash scripts/release.sh <command>`. Builds and verification assert that HEAD matches the supplied SHA and the checkout
is clean. Verification revokes any prior seal before validating any inputs, and
rechecks provenance immediately before issuing a new seal. Builds reject tracked changes and untracked release inputs; ignored stage/output
directories are permitted. Official releases use Actions' clean checkout. A checksum
seal detects any staged changes before/after verification and before packaging;
it is a local sequencing safeguard, not a signature against a malicious caller.

`verify` runs `make cli-gate`: generated contract freshness; gofmt, vet, isolated
module tidy (including `tests/`); release-script negative/packaging checks; all Go
tests against the staged binary; the independent external executable corpus;
all Go tests with the race detector; a separate race-instrumented CLI through
all CLI tests and the independent corpus; and four production fuzz targets
with one worker and five seconds each (override `FUZZ_TIME` explicitly for longer
local runs). The race binary supplements, never replaces, staged-byte testing. Its subprocess
launcher sets only `atexit_sleep_ms=0`, removing the race detector's default
one-second exit delay per CLI invocation while preserving other `GORACE` options.
This also works for corpus children with exact environments. Package-level race
checks are unchanged. A deliberately racy smoke fixture confirms the launcher
still emits race diagnostics and propagates failing exit codes.
The two format targets cover parsing and body round trips; the two domain
targets cover location round trips and deterministic revisions.

For an already staged binary, `make check` / `make cli-gate` requires both
`SIDECAR_BIN` and `SIDECAR_CLI_VERSION`. `make test` also requires them so public
CLI tests never silently use a substitute. `make test-neovim` deliberately
fails as deferred. `make full-red` is not an alias of the CLI gate.

## Homebrew tap

The public [`trogers1/homebrew-tap`](https://github.com/trogers1/homebrew-tap)
repository holds `Formula/sidecar.rb`. Users install stable binaries with
`brew install trogers1/tap/sidecar`; only macOS ARM64 and Linux AMD64 are
supported. There is one release workflow, in this source repository, and no
separate release action in the tap.

### Setup

1. Push the tap's initial README, MIT license, and formula to its default branch
   before activating automation. Its initial formula uses the published `v0.1.0`
   archives, checked against that release's `checksums.txt`.
2. Create a fine-grained GitHub token owned by `trogers1`, restricted to
   **only `homebrew-tap`**, with **Contents: read and write**. Metadata read is
   automatic; no other repository or account permissions are needed.
3. Store it in **sidecar-comments → Settings → Secrets and variables → Actions**
   as the repository secret **`HOMEBREW_TAP_TOKEN`**. Never commit the token.
   Choose an expiration and rotate the secret before it expires.

### Publication and recovery

After stable publication succeeds, the workflow's `homebrew` job downloads the
published archives and checksum manifest, verifies their SHA-256 values, and
updates the formula through GitHub's contents API. The tap token is exposed only
to that job's update step, never to PR verification. Updates are serialized
separately from release publication;
the updater rejects downgrades and checksum changes for an existing version.
An identical rerun makes no commit, and an optimistic file SHA prevents an
intervening tap edit from being overwritten.

A tap failure does **not** roll back the published stable release. Do not rerun
stable publication to repair the tap: existing releases intentionally cannot be
replaced. Instead open **Actions → CLI verification and releases → Run workflow**,
select **main**, and enter the already published tag (for example `v0.1.0`) in
**homebrew_tag**. This is the only manual dispatch mode. Only the tap job runs;
no builds, tags, or releases are created. A missing/expired token, API error,
invalid tag, or checksum mismatch fails the job loudly.

The same updater can be invoked explicitly with `gh` installed and the tap token
in `GH_TOKEN`: `python3 scripts/update_homebrew.py v0.1.0`. This command writes to
the remote tap. Offline tests use a fake GitHub CLI and never publish. After the
initial tap push, validate installation and `brew test trogers1/tap/sidecar` on
both supported native platforms; local macOS checks do not prove Linux behavior.

## Choose a version before pushing

Every push to `main` is a stable release, including documentation and CI fixes.
There is no rolling publication and no manual major/minor/patch release action.
Existing historical `latest` assets are not updated.

Install the tracked hook once per clone (requires Python 3 and Git):

```sh
make install-hooks
```

This sets the repository-local `core.hooksPath` to `.githooks`, refusing to
replace a different custom hooks path. The pre-push hook examines updates whose
**destination** is `refs/heads/main`. It reads `VERSION` from the exact commit
being pushed, queries that destination remote's tags, and requires the version
to be numerically greater than every canonical stable tag. Uncommitted edits
do not count. Branch pushes are not releases and are not version-gated. Deleting
main is rejected. Remote/authentication failures block the push rather than
silently using stale local tags.

For each subsequent release, choose a bump, include it in your reviewed commit,
then push:

```sh
make bump-patch              # or bump-minor / bump-major
git add VERSION             # stage your code changes too
git commit -m "fix(cli): describe the change"
git push origin main
```

The bump commands edit only the working-tree `VERSION`; they do not commit,
tag, or publish. They increment its existing value, not a cached remote version.
If the remote has advanced, choose a newer version and recommit. This migration
already selects `0.1.2`, following locally known `v0.1.1`; confirm that remote
tags have not advanced before the first push.

Hooks can be bypassed and are not installed automatically on clone. The same
version policy is enforced independently by CI before builds. For server-side
protection, require the `plan` and both native `verify` checks on PRs and protect
main from unreviewed/direct pushes. CI cannot undo a push that bypassed the
local hook; it can and does refuse to publish it.

## GitHub publication

Pushes to main and PRs run the same read-only planning and native verification.
The plan reads `VERSION` at the event's checked-out SHA, requires it to exceed
all remote canonical stable tags, and records that SHA, candidate tag, and
highest stable base. Noncanonical tags, prereleases, and historical `latest`
are ignored; an empty stable tag set uses `v0.0.0` as the baseline.

Both macOS ARM64 and Linux AMD64 jobs build and test exactly that SHA and
prospective version. An unexpected hosted-runner architecture fails rather
than silently cross-building. PRs never publish or receive write credentials.
Action references remain pinned to reviewed commit SHAs.

Only successful main pushes proceed to the serialized publisher. It verifies
downloaded archives, refuses an existing release, and rechecks the committed
version, source SHA, highest stable base, and candidate absence before atomically
creating the tag. It then verifies the tag's target and publishes the verified
assets. The Homebrew job updates the tap in the same workflow run. Tag pushes
do not trigger releases, and workflow-created tags do not require a second run.
No script moves stable tags, replaces stable assets, or deletes releases.

Concurrent pushes may select the same base; if the base changes while builds
run, the later publisher fails closed. Push a reviewed commit with a newer
version to release again; do not force a stale publisher through.

### Activation and partial failures

Push the initialized tap first, then this source workflow and its version commit.
Pushing main starts the entire release; no subsequent manual bump is needed.
Configure repository rules protecting `v*` against updates/deletion, while
allowing the Actions token to create tags. Administrators must still preserve
published tag/asset immutability.

If verification fails before tag creation, fix the problem and push; the same
unreserved version may be used if it still exceeds the remote tags. If tag
creation succeeds but publication fails, leave that version reserved, bump
`VERSION`, and push the fix. Neither a workflow rerun nor pushing an existing
tag replaces a reserved/published version. If only the tap fails, use the
tap-only recovery described above instead of making another release.

No remote writes are part of local checks or bump commands. Explicitly invoking
`scripts/publish.sh` or `scripts/version_release.py create` with publication
credentials can write remote tags/releases; the Homebrew updater can write the
remote tap. Offline tests use temporary Git repositories and fake GitHub API
responses. Actual hosted execution and native Homebrew installation on both
platforms still require validation after pushing.
