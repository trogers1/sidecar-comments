# CLI releases

This is a **CLI-only** release gate, not a full v1 ecosystem conformance claim.
The Neovim adapter is deferred. The historical required-red inventory and
`make full-red` remain separate, fail-closed obligations; passing the executable
seed corpus does not imply that all inventory entries have runnable proof.
See [v1 conformance](v1/conformance.md).

## Identity and assets

`cli.Version()` defaults to `dev`. Release builds derive their identity from the
checked-out full Git SHA and event ref, never from the executable under test:

- `main`: `0.0.0-main.<40-character commit SHA>`;
- stable tag `vMAJOR.MINOR.PATCH`: `MAJOR.MINOR.PATCH`, with canonical nonnegative
  components, no leading zeroes, suffixes or prerelease identifiers.

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
export RELEASE_REF=refs/heads/main
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
local runs). The race binary supplements, never replaces, staged-byte testing.
The two format targets cover parsing and body round trips; the two domain
targets cover location round trips and deterministic revisions.

For an already staged binary, `make check` / `make cli-gate` requires both
`SIDECAR_BIN` and `SIDECAR_CLI_VERSION`. `make test` also requires them so public
CLI tests never silently use a substitute. `make test-neovim` deliberately
fails as deferred. `make full-red` is not an alias of the CLI gate.

## GitHub publication

The workflow runs read-only verification on pull requests, and on main/tag
pushes, and manual stable dispatches. It builds and executes natively on macOS ARM64 and Linux AMD64; an
unexpected hosted-runner architecture fails rather than silently cross-building.
Both platform jobs must finish all gates before publication receives write
permission. Action references are pinned to commit SHAs; dependency updates
must review and revalidate those pins.

Push events and explicit manual dispatches publish. A single non-cancelling publication concurrency group
serializes rolling and stable publication. A queued rolling run rechecks the
current main SHA before writing, refusing stale overwrites. The movable `latest`
tag and release titled **Latest build** are replaced only after both platforms
pass. Explicit `v*` tag pushes require the existing tag to still resolve to the
verified SHA and never create or move that tag. Manual dispatches create their
prospective tag only after both native gates succeed and the serialized publisher
rechecks that the highest stable base has not changed and the candidate is absent.
Both paths reject an existing release and never replace stable assets. Configure
repository tag rules to prevent deletion or retargeting of `v*`, while allowing
the workflow token to create new tags; the workflow cannot enforce
immutability against repository administrators. Rolling replacement is not
transactional: an API failure can temporarily leave no rolling release, and a
newer main commit may arrive during publication; subsequent successful runs
advance it. No stale queued run can overwrite a newer published main build.

### Operator activation

Publication starts only after this workflow is pushed to GitHub. After local
verification and review, pushing `main` triggers both native gates and, only on
success, the rolling release.

For automatic stable version selection, open **Actions → CLI verification and
releases → Run workflow**, select **main**, choose **patch**, **minor**, or
**major**, and run. Any other selected branch/tag is rejected. The read-only
planning job snapshots remote main when it starts and selects the highest
canonical stable `vMAJOR.MINOR.PATCH` tag using numeric component comparison.
Noncanonical tags, `latest`, and prereleases are ignored. With no stable tags,
the baseline is `v0.0.0`: patch yields `v0.0.1`, minor `v0.1.0`, major `v1.0.0`.
There is no version bump commit. Both builds and publication check out the same
immutable selected SHA; if main advances meanwhile, the old snapshot remains
the intended release, not the new main. The prospective version is embedded in
the exact staged binaries before native verification. Only then does publication
create the new tag and release directly in this run: tags created with the
standard `GITHUB_TOKEN` do not trigger another tag workflow. A base conflict
requires a fresh dispatch. API errors fail loudly; if tag creation succeeds but
release creation fails, the tag stays immutable and is not removed or reused by
a later manual dispatch. To recover, dispatch a new version and leave the failed
version reserved. Pushing the same existing remote tag is a no-op, not a retry.
Publishing that exact reserved version would require a separately validated
existing-tag recovery workflow; no such recovery entry point is provided here.

Alternatively, create an annotated
canonical version tag on the intended reviewed commit and push that tag, for
example (operator commands; not executed by these scripts):

```sh
git push origin main
git tag -a v0.1.0 <reviewed-commit> -m 'Release v0.1.0'
git push origin v0.1.0
```

Configure repository rules protecting `v*` against updates/deletion before the
first stable publication, allowing the Actions workflow token to create new
`v*` tags without permission to move or delete existing ones. Never reuse a published stable version; fixes require
a new version tag.

The scripts never push or publish locally unless `scripts/publish.sh` is invoked
explicitly with GitHub credentials. No remote publication is part of local
verification. Workflow behavior, hosted native runtime availability, and actual
release API operations require validation after pushing; cross-compilation or
emulation alone is not native Linux runtime proof.
