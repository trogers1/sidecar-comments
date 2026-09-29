# Conformance, fixtures, and release gates

## Executable contract corpus

Every externally visible behavior is specified first, then introduced as a failing black-box fixture, then implemented minimally. A fixture contains only initial filesystem state, command argv, stdin bytes, expected exit status, exact stdout/stderr bytes, and final filesystem state. The runner invokes `SIDECAR_BIN` (default: documented locally built binary), never imports `internal/`, and can run unchanged against any claimed v1 executable. JSON schemas, fixtures, and generated indexes are checked in and freshness-checked.

Required make targets: `make build`, `make test-contract`, `make test`, `make test-race`, `make test-fuzz`, `make test-neovim`, `make preimplementation-check`, `make full-red`, and `make release`. `preimplementation-check` is the green harness-only gate. `full-red` and `release` are final-release gates and remain nonzero while an explicit required-red fixture pack is unavailable; they must never be relabeled as a green partial-coverage check.

## Mandatory coverage

The behavior families below are mandatory for a future v1 claim: grammar and metadata; repair/fix/check/retry; canonical/noncanonical and empty state; source/location/Unicode; UUID and sequential apply; input/output/exit modes; target naming; locks, links, modes, races, and durability; and Neovim popup/extmark/fail-closure behavior. Each implementation change MUST first add a concrete black-box fixture with a stable `BHV-` behavior ID, then make that fixture pass against the compiled executable.

This pre-implementation repository does **not** claim those families are covered. `contracts/behavior-inventory-v1.json` is a machine-readable seed inventory: its fixture mapping records only seeded direct proof, while its `required_red` entries explicitly name mandatory packs that have no fixture yet. `contracts/COVERAGE.md` is a readable seed view, not exhaustive coverage. An unavailable capability is a nonzero `UNAVAILABLE` result and cannot be counted as proof. The existing fixtures are specification artifacts and deliberate future executable obligations, not evidence that a production Sidecar exists. The checked-in shadow `contracts/packs/obligations-v2.json` now reconciles every v1 red entry and registered pack scenario and records 25 individually pending omissions across architecture, compatibility, Neovim, transport, output, storage, and source behavior. Its closed schema and `contracts/packs/capabilities-v2.json` are artifact-checked, but v2 has no fixture promotion or execution path: all applicable entries remain pending, capabilities remain unavailable, and the 25 omission cases still require runnable fixtures and fuller behavior matrices. No shadow entry is proof of v1 behavior.

A future executable corpus MUST add an explicit error-precedence matrix spanning sidecar, source, body, metadata, and operations paths for missing, permission denied, symlink, nonregular, oversized, malformed, concurrent change, and durability failure. It must assert the precedence in [cli](cli.md), exact code/details, no unintended byte change, and limit-plus-one bounded reads for every bounded input.

When production parser/domain code exists, property/fuzz tests MUST cover arbitrary parser bytes without panics; parse → canonical serialize → parse semantic equivalence; canonical serialization idempotence; deterministic revisions; and exact body round-trip including trailing logical LFs and blanks. Seed them from canonical, formatter-only noncanonical, repair, and malformed fixtures and bound input/time. No current harness test is counted as production fuzz proof.

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

A phase may advance only when its listed executable commands and prior fixtures are green. No writer is introduced before phase 4. Package tests supplement, never replace, executable behavior tests.

## Conformance identity

A release records CLI version, format/protocol 1, Go 1.27.1, supported platform, and fixture corpus revision. Once production NFC verification exists it additionally records the exact direct `x/text` version/checksum and Unicode version; this contracts-only checkout has no NFC implementation, though `x/text` is already recorded transitively for the contracts-only JSON Schema harness. Cross-language Unicode fixtures are mandatory. A binary must not claim v1 compatibility if any schema, golden, architecture, race, or release-gate check is skipped.
