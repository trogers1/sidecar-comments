# Red-suite completion roadmap

Status: **pre-implementation**. `make preimplementation-check` is the harness-only gate; `make release`/`make check` remain intentionally red. A passing artifact check is not executable proof. The historical cutover runbook and skill are caller-owned outside this repository (`docs/v1/compatibility.md`).

## Completed foundation

- Corrected the five obsolete text-error goldens and the invalid P6 error expectation against the normative protocol.
- Hardened literal byte/template handling, child-process timeout/output bounds, permission observations, output legality, pack-manifest and pack-golden artifact validation, architecture rules, and Makefile failure propagation.
- `contracts/packs/obligations-v2.json` is **shadow traceability only**: 152 v1 red entries and 72 pack scenarios are losslessly reconciled; 25 additional cases are pending; four specifically named nonexistent semantic-precondition demands are reviewed `not_applicable`. The other 245 entries are pending. Artifact validation verifies the ledger and declarative v2 capability catalog, not execution or proof.

## Gates in dependency order

1. **Close the requirements inventory.** Split remaining broad v1 placeholders and the 25 shadow omissions into individual input/outcome cases. Give each a normative clause, stable `BHV-*` ID, and a concrete fixture obligation or named supplemental test. Complete the command/output, body transport, parser/framing, Unicode, source/location, storage object/fault/precedence, apply, and Neovim matrices. Do not remove an existing red entry merely because a neighboring happy-path test exists.
2. **Design proof promotion.** Version the executable corpus and ledger state machine (`pending`, direct corpus proof, reviewed exclusion). Link each promoted obligation to a runnable corpus fixture and exact assertions; ensure zero unresolved obligations is valid only if all required proofs and release stages run. Replace v1's hard-coded nonempty-red and 72-case assumptions only after bidirectional migration tests prove no obligation disappeared. Shadow v2 currently has no promotion mechanism.
3. **Implement controlled runner adapters.** Turn each `unavailable` v2 capability into an independently tested adapter before using it for proof: deterministic blob creation; independent actor/barrier scheduling and lock/identity replacement; controlled filesystem, UUID clock and CSPRNG faults; platform/object/umask setup; bounded-I/O observation; and Neovim 0.12 headless driving. Keep unavailable mandatory cases nonzero. Current v2 inputs only describe designs and do not execute.
4. **Promote black-box fixtures in phases.** Start with domain/token JSON, parser/read/format and properties; complete descriptor-relative secure store and independent-process race/failure contracts **before any writer**; then mutation/apply/fix and the real-binary Neovim flows. Resolve fixture/document disagreement against `docs/v1/` before promoting a case. Run the same public corpus against any executable claiming v1 compatibility.
5. **Finish release wiring.** Build `cmd/sidecar`; make `make build`, `test-contract`, `test-race`, `test-fuzz`, and `test-neovim` run their actual production subjects. Expand gofmt/tidy/race/architecture scans to production roots and ensure release/check require the external binary, all executable proofs, property/fuzz/race/editor suites, and fresh-clone provisioning. Verify the complete gate on supported macOS arm64 and Linux amd64. Nothing in the shadow ledger may substitute for observed execution.

## Current verification

`make preimplementation-check` passes on this checkout (artifact/schema consistency, Go tests, runner race tests, formatting, vet, and tidy drift). `full-red` continues to report missing implementation/proofs. No production CLI or Neovim adapter exists, and no v2 controlled adapter is currently available.
