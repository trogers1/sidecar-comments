# Simple Go behavioral CLI red-suite roadmap

Status: **Stage A and the first Stage B lifecycle remain independently APPROVED on `red-suite/simple-go` at `a222e59`. The user subsequently authorized broad simple public-CLI expansion; both independent reviewers agreed on six targeted corrections. Those corrections are implemented and independently APPROVED by BOTH reviewers with no remaining blocking findings. Constructed cases are not exhaustive coverage or executed conformance; exact mapping/injection blockers remain isolated.** This replaces the experimental v3 completion roadmap. It is a readable test-construction plan, not a proof ledger, executable scenario DSL, migration-cardinality requirement, or release plan. Do **not repair the experimental v3 framework**.

Normative behavior remains defined by [`docs/v1/index.md`](../docs/v1/index.md), especially `protocol.md`, `cli.md`, `storage-security.md`, `schema/v1.json`, and `conformance.md`. This roadmap changes no normative specification. [`initial.md`](initial.md) is historical only.

## Starting policy

The fresh start is complete. Continue in the same directory on `red-suite/simple-go` at `a222e59`, based on verified baseline `9e45668`. Preserve the uncommitted Stage A changes and untracked `tests/cli` files. The experiment is retained on `archive/red-suite-experiment` at `fe67620`; do not use it or ignored scratch/build artifacts as implementation inputs. Do not reset, clean, create another worktree, or restore archived code. Reverify branch, HEAD, and full status on entry. See [the handoff](red-suite-handoff.md) for the verified file list and restart instructions. No production Sidecar binary is known to exist.

## Current Stage A result

The branch/HEAD and narrow changed paths were verified; the historical experimental snapshot above is not the current status. Direct gofmt, helper/schema tests, their race run, CLI vet, and the narrow `TestArchitectureBoundaries` check passed. An explicitly empty `SIDECAR_BIN` failed the availability test with the required diagnostic (exit 1), not a skip. No compile/test repair or dependency version change was needed. Exact commands/results are in [the handoff](red-suite-handoff.md#verification-already-performed).

Current size: 209 lines of helper plumbing; 410 lines / 14,271 bytes across all three CLI test files after formatting. Independent review APPROVED this boundary with no blockers; do not expand it. Green self-tests prove plumbing/schema validation only, not Sidecar. Stage B now has the first genuine, directly written public CLI formatting lifecycle in `tests/cli/format_test.go`; its exact oracles have been approved by two independent parallel reviewers. The user approved a precreated empty `0600` lock fixture for that lifecycle. Resolve `SIDECAR_BIN` absolutely before changing child working directory. No production implementation has been performed; the only old-framework edit remains the approved Stage A narrow schema authorization check adjustment.

## Current Stage B result — independently approved

One direct `TestFormatLifecycle` hand-constructs noncanonical metadata (whitespace and reversed name/id keys), checks exit 3 without change, formats to independently declared canonical bytes, validates with the real `--source` flag, then checks canonical state with exit 0. All four invocations assert exact status/stdout/stderr, external normative JSON Schema validity, and complete immediate file names/modes/bytes including unchanged source and the approved empty `0600` persistent lock. Revisions are SHA-256 hashes of independently declared bytes, never subject output. The supplied binary is resolved absolutely before using the child fixture cwd. Existing helpers are reused unchanged; no abstraction/platform expansion.

`test-cli-subject` now selects availability plus this lifecycle; green helper/schema targets remain separate. Fresh direct Go checks passed: focused helper/schema (1.462s), race (2.324s), CLI vet, and architecture (0.297s). gofmt and diff whitespace checks passed. Explicitly absent `SIDECAR_BIN` produced expected red for **both** subject tests (0.440s), with the required diagnostic and no skip/fallback. Full commands are in the handoff.

No subject binary is known or supplied: the actual behavioral lifecycle remains unexecuted, not conformance proof. The precreated lock avoids unspecified newly created lock metadata; this batch does not prove locking/contention/identity behavior or any metadata beyond file names, regular types, modes, and bytes. Stage A approval is unchanged; both independent parallel reviewers APPROVED Stage B with no findings or requested changes. Both verified branch/full HEAD, dirty paths, and `git diff --check`; neither reran Go checks or executed a real subject. This approves test/oracle construction, not behavioral conformance. No code fixes were needed. That historical batch stopped there. The subsequent user authorization permits finite behavioral tables, but still prohibits production, Linux/Neovim, normative edits, and old-framework repair.

## Test shape

Use standard Go tests and a small shared process/filesystem helper. It creates temporary source, sidecar, and input files; invokes independently supplied `SIDECAR_BIN`; asserts exact status, stdout, stderr, resulting paths, modes, and bytes; and has explicit timeouts, output bounds, cleanup, and process-tree termination. An absent `SIDECAR_BIN` fails explicitly.

Tests are CLI-first. Explicitly write each lifecycle invocation (such as create → list/get → reply/edit → delete or check → format → check); use tables for finite repetitive boundaries. Expected bytes and revision hashes are independently derived from inputs and the normative rules, never copied from the subject. Validate boundary JSON with `schema/v1.json`; use schema-derived public-protocol DTOs where typed decoding is needed. Share constants and derived expectations without obscuring assertions.

Each owned function, including an ordinary helper, accepts one request struct except mandated Go callback/interface signatures. Use no `any`, unsafe casts/assertions, mock Sidecar implementations, custom DSL, generic adapter registry, public fault hooks, or fake behavioral proof. Small helper-only doubles may test detection of wrong status/streams/timeouts/filesystem results; they do not prove Sidecar behavior. Public arbitrary-byte CLI tests come before any lower-level property tests. Only faults impossible to induce externally may use narrowly justified, approved test-only dependency injection.

## Sequential stages

| Stage | Deliverable | Gate |
|---|---|---|
| A. Clean start | Verified branch/HEAD; minimal helper/schema tests pass scoped formatting, tests, race, vet, and architecture check; missing binary fails explicitly. | Independently APPROVED; not behavioral proof. |
| B. First lifecycle | One genuine public CLI lifecycle implemented; exact oracles APPROVED by two independent reviewers. | Absent binary verified red; actual subject execution unavailable. Bounded batch complete. |
| C. Finite matrices | Broad public-CLI cases are constructed; six agreed review corrections implemented. | BOTH reviewers APPROVED final corrections; real-subject execution unavailable. |
| D. Adversarial cases | Add process/storage adversaries and only justified supplemental faults. | External induction first; test-only injection is narrow and documented. |
| E. Coverage check | Review applicable rows and required gates. | No mandatory case is skipped or silently accepted. |

Use one implementer followed by **two independent reviewers in parallel** for each bounded batch, per the user's clarified preference. There is no parallel framework program.

## Semantic coverage checklist

This table is a human review checklist, not an executable registry or proof ledger. “Reviewed” means the independently constructed public observation and its normative basis have been reviewed; it does not claim production conformance.

| Family | Expected public observations | Review status |
|---|---|---|
| Format and repair | `validate`, `format`, `fix`, and `--check` status/streams; canonical bytes, repair diagnostics, preservation or change of files. | Original lifecycle approved. Empty/formatter/repair/fix/retry/id cases constructed; multi-thread fix coordinates/order/increasing IDs and literal location ordering added as agreed review corrections, approved by BOTH final reviewers. |
| Domain, Unicode, body transport, source/location | Public validation and mutation outcomes for IDs, Unicode, body stdin/files/bytes, source validity, locations, anchors, limits, and unchanged-on-failure state. | Positive and negative stored metadata/token/Unicode cases, file/stdin normalization, source coordinate/encoding/object matrices and concrete size/count boundaries constructed. Negative request mappings remain blocked, not the existing stored-token tests. Final construction/oracle review approved by BOTH reviewers. |
| CLI grammar, protocol, errors, output modes | Argument placement/repetition, stdin arbitration, JSON/text/id/none channels, exact envelopes and errors, status precedence. | JSON/text/none delete, version reconstruction, generated fix/create/add JSON/id, and revision/source precedence constructed. Negative usage/stdin-arbitration pairings await the unavailable amendment. Generated mutation correction approved by BOTH reviewers. |
| Mutations, revisions, sequential apply | Create/list/get/reply/edit/move/delete results, exact stored bytes/revisions, guards, rollback, and explicit sequential working-copy observations. | Explicit/generated create/add, list/get, changed/equal update, move/no-op, deletes/cascade and concrete guards constructed. All six apply variants plus indexed missing-entity and intermediate-limit rollback constructed; corrected cases approved by BOTH reviewers. Ambiguous collision mismatch mapping remains blocked. |
| Secure storage, durability, independent-process races | Public behavior for object/path classes, locks, modes, durability outcomes, identity changes, and independently launched process races with final state and both responses. | Target names/types, invalid locks/source objects, exact/plus-one target/source limits, existing modes, held-lock timeout and independent-writer combined-state cases constructed. Source regular-type preservation now uses Lstat before bytes. Deterministic identity/durability phase faults remain injection-blocked; final construction/oracle review approved by BOTH reviewers. |
| Bounded public-input fuzz/property tests | Arbitrary-byte CLI `validate`/`format` campaigns: no crash/hang and exact public results; narrowly justified supplemental properties only when CLI cannot establish them. | Finite malformed encoding/endings/location campaign implemented with exact errors and preservation, not merely no-crash; general fuzz/property coverage pending. |

## Gates and unresolved decisions

Keep green helper/schema self-tests separate from the intentionally red subject suite. The red suite must execute real public-process cases against `SIDECAR_BIN`; an unavailable subject is a clear failure, never a skip or pass. Completion means applicable rows have runnable, reviewed tests with exact independent oracles, plus narrowly justified supplemental tests for genuinely uninducible properties. It does not mean the implementation conforms or that a release gate is green. Linux and Neovim remain deferred.

The user-approved usage-error mapping amendment is approved intent but unreviewed. Review it independently, including the original specification ambiguity, before asserting exact error cases; do not silently discard it. Temporary-file sync and temporary-name collision-exhaustion mappings remain unresolved only for affected tests. UUID collision exhaustion is explicitly mapped to io:randomness_exhausted; inducing it externally still requires future approved injection.

## Historical intermediate broad batch verification

The current batch adds ten ordinary Go test files, fifteen subject tests (seventeen selected top-level tests including availability and the approved format lifecycle). `Makefile` now selects `^(TestSubject.*|TestFormatLifecycle)$`, with a 120s suite ceiling; each child is bounded to 5s. Existing approved helper/schema code and staged Stage A/B content were preserved. One tiny request-struct binary resolver is shared; no scenario runner, new dependencies, platform repair, or production changes.

Latest direct verification (60s command bound; no make targets):

- `gofmt -w` on all ten new files: passed.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s`: PASS, package 1.108s / wall 2.561s.
- Same focused command with `-race`: PASS, package 2.271s / wall 3.030s.
- `go vet ./tests/cli`: PASS, wall 0.310s.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s`: PASS, package 0.257s / wall 0.630s.
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s`: expected FAIL, package 0.080s / wall 0.434s. All 17 selected tests explicitly fail for absent SIDECAR_BIN; no skips, mocks, or fallback.
- `git diff --check`: passed. Final status preserves staged protected files; new writes are only tests/cli, Makefile, and these two plans.

No real binary was supplied: **no subject conformance proof**. The new assertions compile but have not executed past executable availability. TWO independent parallel reviews must be launched by the parent; none launched here. Work remains uncommitted.

## Final expansion — supersedes preceding intermediate gaps/results

The same batch now has **22 new Go files / 31 new subject tests**, plus availability and the approved lifecycle (33 selected top-level tests). Existing approved staged content and helper/schema adapter are unchanged. Added negative stored metadata/token/domain matrices; positive scalar limits; formatter thread ordering/separators; version nonempty and exact reconstruction; concrete generated fix assignment validation with independently reconstructed disk/revision, retry/check/id output; invalid source/lock objects; missing source before revision/entity semantics; changed update and last-author mismatch; double-suffix source derivation.

Independent external writers start behind a barrier. Both children are joined before reporting failure; complete responses must match one of two independently computed legal serializations and final combined canonical bytes are fixed. No sleep, response-order, or timing-based overlap oracle is used. This tests independent processes, not deterministic scheduling of precommit races.

Limits now include exact 4 MiB target; exact/plus-one 16 MiB source, body and anchor boundaries; maximum threads/messages per thread/messages per document with rejected next mutation; 256 operations in an exact 1 MiB valid request; exact 16 KiB metadata input and a largest valid all-CRLF body transport. Large/special objects use direct OS checks without expanding snapshots.

Final announced checks (60s aggregate command bound):

- gofmt: PASS on all additions/changed tests.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s`: PASS, package 1.062s / wall 1.817s.
- Same with `-race`: PASS, package 2.334s / wall 3.207s.
- `go vet ./tests/cli`: PASS, wall 0.310s.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s`: PASS, package 0.119s / wall 0.496s.
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s`: expected FAIL, all 33 explicitly fail for absent binary; package 0.107s / wall 0.480s.
- `git diff --check`: PASS. Branch/HEAD red-suite/simple-go / a222e59; all work uncommitted. No staging/reset/clean/push.

**Historical freeze:** both independent reviewers subsequently completed review and agreed on six targeted corrections. See the correction results below. No real binary is supplied; all subject assertions remain unexecuted past availability.

Specific remaining blockers, rather than waived coverage:

1. The approved usage amendment text is unavailable. Exact negative argv/body/stdin/metadata/apply usage field/reason pairings (including request over-limit rejection) cannot be inferred from enum membership. Valid maxima are tested; supply and independently reconcile the amendment before asserting these exact errors.
2. Available prose does not distinguish retry_mismatch versus id_collision sufficiently to assert collision mismatches. Exact retries, last-author guards, and revision precedence are covered; obtain that distinction before adding ambiguous collision errors.
3. No approved injection boundary or deterministic external inducer exists for clock regression/failure, CSPRNG failure, UUID exhaustion, precise identity swaps, descriptor/cleanup phase failures, and durability uncertainty. UUID exhaustion is mapped; temp-fsync and temporary-name exhaustion mappings must not be invented. These require future approved production injection, not mock CLI proof.
4. SIDECAR_BIN is absent. Real external execution is needed to validate generated-output handling, fixture semantic validity, and multiwriter behavior; green helper/race checks are not that proof.

Linux/Neovim/framework/production remain outside authorization. These blockers prevent an honest exhaustive finished-v1 claim.

## Six agreed reviewer corrections — implemented and independently approved

Both independent reviewers explicitly agreed on the six findings summarized by the user. Only those corrections were made:

1. `generated_mutation_test.go`: direct omitted-ID create then omitted-ID add in JSON and successful id modes. Extracted values are validated using the existing normative schema, exact envelopes/disk/revisions are reconstructed independently, and complete fixture state is checked after each call. Id-mode steps also validate exact reconstructed revisions through a real validate invocation. No UUID ordering comparison spans processes.
2. `generated_test.go`: fix now assigns two messages across two threads in one process. Exact physical role lines 2/7, byte offsets, assignment order and ordinals are asserted, with distinct strictly increasing generated IDs, canonical document/revisions and retry/check/no-op preservation. Singleton id fix coverage remains.
3. `apply_remaining_test.go`: successful sequential thread_move/message_add/message_delete with exact working-copy DTOs; a separate move/add/delete batch fails at index 1 when add exceeds 256 messages, even though index 2 deletion would restore the count. Exact apply-local message_add/limit_messages_thread error and complete original bytes are asserted.
4. `location_order_test.go`: literal independently declared canonical sequence covers line 2/10, range numeric coordinates, absent/present anchors, decoded anchor byte ordering opposing base64 lexical order, and UUID tie-breaks, against deliberately shuffled input. No comparator oracle.
5. `storage_test.go`: source Lstat plus exact regular mode is checked before reading bytes, closing the symlink substitution hole.
6. This checklist now describes constructed cases and current review status; generated fix/id/version/invalid objects/writers are not falsely marked pending. Completed final review, blocked mappings/injection, and absent subject execution remain separate.

Correction checks (announced, aggregate command bounded to 60s): gofmt PASS; focused helper/schema `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s` PASS (package 1.127s / wall 1.913s); same with `-race` PASS (2.295s / 3.150s); `go vet ./tests/cli` PASS (wall 0.336s); `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s` PASS (0.271s / 0.668s); `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s` expected FAIL for all 37 selected tests (0.083s / 0.492s), no skip/fallback. `git diff --check` PASS. Branch/HEAD red-suite/simple-go / a222e59; staged work preserved, no commit/staging/reset/clean/push. No Make selection change was needed.

Both final reviewers (`reviewer-1-deafd522`, `reviewer-2-deafd522`) independently APPROVED these corrections with no remaining critical findings or warnings. Each verified branch/HEAD and passing `git diff --check`; neither reran Go checks or executed a real subject. Recorded Go/format/race/vet/architecture/absent-binary results remain worker-reported. This closes construction/oracle review, not exhaustive coverage or behavioral conformance.

One nonblocking future extension suggested by Reviewer B is range end-line ordering with identical start coordinates. General fuzz/property coverage remains pending, alongside the specific mapping/injection/binary blockers above. Further finite-case expansion is not declared exhaustively complete beyond the concrete cases. Production remains outside authorization; all work remains uncommitted.
