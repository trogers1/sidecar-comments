# Conformance, fixtures, and release gates

## Executable contract corpus

Every externally visible behavior is specified first, then introduced as a failing black-box fixture, then implemented minimally. A fixture contains only initial filesystem state, command argv, stdin bytes, expected exit status, exact stdout/stderr bytes, and final filesystem state. The runner invokes `SIDECAR_BIN` (default: documented locally built binary), never imports `internal/`, and can run unchanged against any claimed v1 executable. JSON schemas, fixtures, and generated indexes are checked in and freshness-checked.

The current release scope is **CLI-only**, with Neovim explicitly deferred. `make release` natively builds, verifies, and packages the same staged bytes; `make check` / `make cli-gate` verifies an explicitly supplied `SIDECAR_BIN` and independent `SIDECAR_CLI_VERSION`. This includes all Go tests, the executable seed corpus, race-instrumented CLI execution, static checks, module freshness including `tests/`, and bounded production fuzzing. See [release mechanics](../releases.md). `make full-red` retains the separate historical required-red inventory gate and remains nonzero while required packs are unavailable. Neither a CLI release nor a green seed corpus is a claim of complete v1 ecosystem conformance. `make test-neovim` still fails as deferred; it is not silently skipped into a full-v1 claim.

## Mandatory coverage

The behavior families below are mandatory for a future v1 claim: grammar and metadata; repair/fix/check/retry; canonical/noncanonical and empty state; source/location/Unicode; UUID and sequential apply; input/output/exit modes; target naming; locks, links, modes, races, and durability; and Neovim popup/extmark/fail-closure behavior. Each implementation change MUST first add a concrete black-box fixture with a stable `BHV-` behavior ID, then make that fixture pass against the compiled executable.

The CLI implementation and executable tests do **not** claim every historical family is fully covered. `contracts/behavior-inventory-v1.json` is a machine-readable seed inventory: its fixture mapping records only seeded direct proof, while its `required_red` entries explicitly name mandatory packs that have no fixture yet. `contracts/COVERAGE.md` is a readable seed view, not exhaustive coverage. An unavailable capability is a nonzero `UNAVAILABLE` result and cannot be counted as proof. The existing executable seed fixtures prove only the behaviors they actually exercise against the supplied CLI; unimplemented inventory obligations remain separate. The checked-in shadow `contracts/packs/obligations-v2.json` now reconciles every v1 red entry and registered pack scenario and records 25 individually pending omissions across architecture, compatibility, Neovim, transport, output, storage, and source behavior. Its closed schema and `contracts/packs/capabilities-v2.json` are artifact-checked, but v2 has no fixture promotion or execution path: all applicable entries remain pending, capabilities remain unavailable, and the 25 omission cases still require runnable fixtures and fuller behavior matrices. No shadow entry is proof of v1 behavior.

A future executable corpus MUST add an explicit error-precedence matrix spanning sidecar, source, body, metadata, and operations paths for missing, permission denied, symlink, nonregular, oversized, malformed, concurrent change, and durability failure. It must assert the precedence in [cli](cli.md), exact code/details, no unintended byte change, and limit-plus-one bounded reads for every bounded input.

Production parser/domain property/fuzz tests MUST cover arbitrary parser bytes without panics; parse → canonical serialize → parse semantic equivalence; canonical serialization idempotence; deterministic revisions; and exact body round-trip including trailing logical LFs and blanks. Seed them from canonical, formatter-only noncanonical, repair, and malformed fixtures and bound input/time. No harness test is counted as production fuzz proof. The CLI gate now runs `FuzzParseDocument`, `FuzzDocumentBodyRoundTrip`, `FuzzLocationRoundTrip`, and `FuzzRevisionDeterministic` with one worker, a five-second budget each, and a 60-second process timeout; these checks do not complete the historical red inventory.

## Phase gates

| Phase | Prerequisite / deliverable | Gate |
|---|---|---|
| 1 | docs, binary skeleton, contract harness | build; basic version/usage contracts |
| 2 | phase 1; constructors and token JSON validation | domain/Unicode/schema contracts |
| 3 | phase 2; parser/read/format | format goldens, properties, fuzz seeds |
| 4 | phase 3; secure store before any writer | failure injection, descriptor/lock tests, adversarial independent subprocess races |
| 5 | phase 4; mutations, apply, fix | all operation/revision/atomicity contracts |
| 6 | phase 5; real binary Neovim adapter | headless 0.12 dirty buffers, stale async, selections, extmarks, external changes, negotiation, fail closure |
| 7 | phase 6; release docs and provisioning | complete `make check`, fresh-clone provisioning proof |

Cutover execution and any replacement skill/runbook are caller-owned operational work outside this repository's v1 conformance gate. The historical `plans/initial.md` phase-7 cutover deliverables are superseded by [compatibility](compatibility.md); no runtime or test access to `../config` is permitted.

For the historical full-v1 phase model, a phase may advance only when its listed executable commands and prior fixtures are green. The separately scoped CLI release does not assert completion of phases 6–7. No writer is introduced before phase 4. Package tests supplement, never replace, executable behavior tests.

## Conformance identity

A release records CLI version, format/protocol 1, Go 1.27.1, supported platform, and fixture corpus revision. Production NFC verification uses the direct `x/text` dependency recorded in `go.mod`/`go.sum`; Go module build information records that dependency identity. The release build metadata records its commit-specific corpus identity. A full cross-language conformance claim additionally requires recorded Unicode-version evidence. Cross-language Unicode fixtures are mandatory. A binary must not claim full v1 ecosystem compatibility if any required schema, golden, architecture, race, adapter, or historical full-v1 release-gate check is skipped.
