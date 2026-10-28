# Handoff: simple Go behavioral CLI red suite

## Resume here

Work in the **same directory**, on `red-suite/simple-go`. No new worktree, branch, reset, clean, or push is needed. The fresh start has already happened; do not repeat it.

The user’s priority is **SIMPLE, high-level behavioral tests of the actual CLI and its functionality**. Do not build a testing platform or expand helper machinery. Use one implementation worker followed by **two independent reviewers in parallel** per bounded batch, per the user's clarified review preference.

**Current milestone: Stage A and the first Stage B lifecycle remain independently APPROVED. BOTH independent reviewers completed broad-suite review and explicitly agreed on six targeted corrections. Those fixes are implemented and BOTH reviewers independently APPROVED the final code with no remaining blocking findings. The 37-test subject selection is reviewed implementation guidance, not exhaustive v1 coverage or real-subject conformance. Historical pending descriptions below are superseded by the correction accounting at the end and the roadmap checklist.** Do not resume old asynchronous agents.

## Verified checkout state

Latest direct verification before this handoff update:

- Branch: `red-suite/simple-go`.
- HEAD: `a222e59` (revised simple-test plans).
- Baseline `9e45668` is an ancestor of HEAD.
- Archive branch: `archive/red-suite-experiment`, tip `fe67620`.
- Working tree is **not clean**: the Stage A work and plan updates below are uncommitted.
- Modified tracked files: `Makefile`, `contracts/runner/architecture_test.go`, `docs/v1/architecture.md`, `docs/v1/index.md`, `go.mod`, `plans/red-suite-handoff.md`, `plans/red-suite-roadmap.md`.
- Those original CLI test files are now staged additions, along with the approved `format_test.go`; preserve the staged contents. The current expansion adds unstaged files under `tests/cli`.
- Stage B adds untracked `tests/cli/format_test.go`; `tests/cli/boundary_test.go` has only an updated availability-test comment. Reverified branch `red-suite/simple-go`, HEAD `a222e59`, and preserved dirty Stage A paths before implementation.

Preserve this work. No implementation commits were made in this session. Ignored scratch/build artifacts are not inputs. Do not inspect the archive as an implementation template, restore its code, repair its v3 runner, or depend on its DSL/proof/fixture-generation machinery. Existing baseline framework files remain, but are not the new suite’s dependencies.

## Stage A: implemented and reviewed

New ordinary Go test files in `tests/cli` provide small process/filesystem helpers, helper-only self-tests, a normative JSON Schema boundary validator, and an explicit `SIDECAR_BIN` availability failure. They do **not** yet exercise Sidecar functionality.

- `helpers_test.go`: 209 lines of bounded process execution and exact filesystem/process observations.
- `boundary_test.go`: 142 lines of helper self-tests and subject availability checking.
- `schema_test.go`: 59 lines of schema adapter/self-tests.
- Total: 410 lines. Reviewer accepted this boundary; do not grow it into a platform.
- Additive Make targets: `test-cli-helpers`, `test-cli-helpers-race`, `vet-cli`, `test-cli-subject`. At Stage A the subject target covered availability only; Stage B now runs the real lifecycle too.

User explicitly approved extending the **existing pinned** JSON Schema dependency’s authorization to the new test-only boundary validator. Consistent narrow changes were made to the `go.mod` comment, normative index/architecture authorization wording, and the existing architecture check’s exact-path exception for `tests/cli/schema_test.go`. Dependency versions are unchanged. No production imports are authorized. The adapter uses locally inferred generic schema input solely for validation, returns errors only, and exposes no generic data/type assertions/domain dispatch.

### Verification already performed

These are implementation-worker results, not results freshly rerun during handoff preparation:

- `gofmt -w tests/cli/helpers_test.go tests/cli/boundary_test.go tests/cli/schema_test.go contracts/runner/architecture_test.go` — succeeded; subsequent `gofmt -l` on these paths returned no files.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s` — PASS (1.154s).
- `go test -race ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s` — PASS (2.174s).
- `go vet ./tests/cli` — PASS.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s` — PASS (0.267s).
- `SIDECAR_BIN= go test ./tests/cli -run '^TestSubjectAvailable$' -count=1 -timeout=30s` — expected FAIL, exit 1: `SIDECAR_BIN is required; helper/schema success is not Sidecar behavioral proof`.
- `git diff --check` — PASS.

### Independent review outcome

Reviewer read all three new files completely, changed tracked files, plans, and relevant normative documents. Verified branch/full HEAD and exact changed paths; independently ran `git diff --check`. Verdict: **APPROVE**, no critical findings or blocking warnings. Reviewer did not rerun Go checks because its profile was read-only; test results above remain worker-reported.

Carry these review notes into Stage B:

1. Resolve supplied `SIDECAR_BIN` to an absolute path **before** switching child working directory to a temporary fixture directory. Do this directly, not through a binary-discovery framework.
2. The delayed-marker descendant cleanup self-test is supplemental timing-dependent evidence, not exhaustive proof against escaping descendants.
3. Schema validation is not complete lexical/domain protocol validation. Use exact independent expected envelope bytes for deterministic outputs, including compact spelling, displayed field order, and one terminating LF. Do not turn the schema adapter into a generic DTO/domain decoder.

## Stage B: first lifecycle implemented and independently approved

`tests/cli/format_test.go` now directly implements **one** public CLI lifecycle. Its fixture/oracles were checked against full cli, format, protocol, schema, storage, source/location, domain/Unicode, and index documents; both independent reviewers have now approved them:

1. Hand-construct a valid one-thread, file-location document with noncanonical metadata whitespace/key ordering, plus its valid source file.
2. `format --check`: expected exit 3 and the specified success envelope, original storage revision, independently computed canonical revision, unchanged files.
3. `format`: exact canonical bytes and response.
4. `validate --source`: exact response and unchanged files.
5. `format --check`: expected exit 0 and unchanged canonical bytes.

Verify exact supported flags, syntax, envelope field order, and all semantics from the normative documents before coding. Derive SHA-256 revisions directly from the declared original/canonical bytes, never by asking the subject or copying its output. Assert exit status, stdout, stderr, schema-valid external JSON, directory entries, modes, bytes, and source preservation through explicit lifecycle steps. Reuse the existing helpers. Do not use a mock Sidecar to validate the lifecycle.

**User-approved fixture decision:** precreate a valid empty `.note.txt.comments.sidecar.lock` with mode `0600` and assert preservation. This avoids inventing unspecified contents/mode for a newly created lock. It is a fixture choice, not a normative amendment or new-lock creation test.

`test-cli-subject` now selects both `TestSubjectAvailable` and `TestFormatLifecycle`; helper/schema targets remain separate. The lifecycle resolves the supplied executable absolutely before child temp cwd, uses independently declared original/canonical bytes and SHA-256 hashes, and directly checks each process and complete immediate regular-file state through existing Stage A helpers. No helpers or platform machinery were added.

Fresh bounded-batch verification:

- `gofmt -w tests/cli/format_test.go` — succeeded; later `gofmt -l tests/cli/format_test.go tests/cli/boundary_test.go` returned no files.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s` — PASS (1.462s).
- Same focused command with `-race` — PASS (2.324s).
- `go vet ./tests/cli` — PASS.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s` — PASS (0.297s).
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubjectAvailable|TestFormatLifecycle)$' -count=1 -timeout=30s` — expected FAIL (0.440s); both tests explicitly report `SIDECAR_BIN is required; helper/schema success is not Sidecar behavioral proof`. No skip, fake subject, or fallback.
- `git diff --check` — PASS.

No actual Sidecar binary was supplied or discovered; lifecycle process/output/filesystem assertions have **not** run against a real subject. Compilation and green plumbing checks are not behavioral proof. The lock fixture does not test new-lock metadata, lock identity preservation, or lock contention; immediate file names/modes/bytes are checked, not ownership/ACLs/xattrs/timestamps. No Linux, Neovim, production, matrix, framework repair, or normative edits were made in this batch. Both original Stage B independent reviews are complete. The later broad expansion described below requires fresh independent review.

### Stage B independent review outcome

Two reviewers ran in parallel without consulting each other's findings: `reviewer-1-e21ee039` and `reviewer-1-1385b4fe`. Both returned **APPROVE**, with no critical findings, warnings, or suggested changes for this batch. Each fully read the four CLI test files, Makefile, both plans, and relevant full normative sources; independently verified branch/full HEAD `a222e59a68f8405c0255ec583c88443d0e34d1d7`, dirty paths, and passing `git diff --check`; and made no edits. They confirmed fixture validity, independent canonical bytes/revisions, argv, exact statuses/envelopes/schema field order/LF, filesystem assertions, approved lock fixture, absolute executable resolution, absent-binary failure, and bounded scope. Neither reran Go checks or had a real subject: approval establishes reviewed test/oracle quality only, not executed behavioral conformance. No code fixes were needed.

## Requirements and scope

Normative source: [`docs/v1/index.md`](../docs/v1/index.md) and its precedence/contract map, especially `protocol.md`, `cli.md`, `format.md`, `storage-security.md`, `schema/v1.json`, and `conformance.md`. [`initial.md`](initial.md) is historical, not controlling. Read related documents as needed.

- Ordinary Go tests, direct CLI invocations, table cases only for repetitive finite boundaries.
- Independently constructed expected bytes/revisions; no subject oracle.
- Schema validation at external JSON boundaries; schema-derived public DTOs only where useful, not schemas in every helper.
- Each owned function takes one request struct except mandated Go callbacks/interfaces.
- No `any`, unsafe casts/assertions, hidden recovery, or silent acceptance. Derive/share existing constants/types rather than redefine them.
- Explicit errors, bounded output, per-process timeouts, cleanup and process-tree termination.
- Helper doubles test helpers only, never intended CLI behavior.
- No scenario DSL, generic registry, proof ledger, platform, public fault hooks, or archived framework repair.
- Linux, Neovim, and production implementation remain deferred.
- Green helpers are not production conformance or a release gate. Applicable cases ultimately need runnable reviewed real public-process tests; missing subject is not success.

Unresolved specification decisions: the user-approved usage-error mapping amendment remains unreviewed; independently review the original ambiguity before exact usage-error oracles. Temporary-file sync and temporary-name collision-exhaustion mappings remain unresolved only for affected cases; UUID collision exhaustion is explicitly mapped to io:randomness_exhausted but needs future approved injection to induce. Do not invent outcomes or block unrelated formatting tests.

## Operational lessons for the next orchestrator

Interrupted/replacement workers repeatedly encountered harness permission issues; this was not evidence of CLI/test failure. The final Stage A worker and reviewer completed successfully.

- Give child scope metadata the explicit paths it must inspect or edit. Even a reviewer with `writes: []` was blocked from **reading** the new tests; supply permissible paths but instruct the reviewer to remain read-only.
- Added scopes did not take effect when resuming an existing worker session. Start a fresh worker when scope changes.
- The guard interpreted a `make test-cli-helpers` target as a filesystem path. Direct focused `go test`/`go vet` commands worked.
- Read documentation through explicit file paths. Do not read protected toolchain/module-cache source or work around a denial.
- Old Stage B agents failed or were interrupted without producing a lifecycle. The last replacement was instructed to stop all writes when the user requested this handoff. Start a fresh agent, not an old question sidecar.

## Suggested new-session prompt

> Continue in this same dirty checkout on `red-suite/simple-go` at `a222e59`, preserving all work. Read both plans. Stage A and the first Stage B lifecycle are independently approved; Stage B had two parallel reviewers with no findings. No real subject is available, so reviewed oracles are not behavioral conformance. The user has authorized broad simple public-CLI expansion; finish remaining families without production changes, then request two independent parallel reviews. Keep tests simple and public-CLI-first, reuse approved helpers, and use two independent parallel reviewers after each implementation batch. No scratch/archive inputs, platform expansion, production/Linux/Neovim, normative edits, or framework repair without explicit authorization.

## Current authorized broad expansion (not yet independently reviewed)

The user superseded the historical first-lifecycle stop. Branch/HEAD were verified as `red-suite/simple-go` / `a222e59`; existing staged changes were preserved. New unstaged test files:

- `subject_test.go`: shared absolute binary resolution; missing/zero-byte validate/list/get/check/cleanup; CRLF/final-LF/role-separator/canonical-first formatter variants; finite malformed-byte campaign with exact encoding/endings/location errors.
- `source_repair_test.go`: UTF-8 byte columns, multiline CRLF anchors, coordinate/anchor failures, source encoding/endings, empty/unterminated sources; exact repair diagnostics, fix-check, read/format repair boundaries.
- `apply_test.go`: schema-validated requests, indexed missing-message rollback after create, then sequential create/update/delete success with exact intermediate operation DTOs and no final target.
- `body_test.go`: five normalized body variants via file and stdin; explicit-ID create/retry; revision mismatch before missing-entity semantics.
- `metadata_test.go`: composed/non-BMP names, agent/external authors, decomposed external identity, GitLab variants/max integer, cross-namespace equal UUID text, non-whitespace U+200B body.
- `mutation_test.go`: add with last-author guard, equal-body update, nonfinal deletion, final-message/thread/file cascade.
- `read_move_test.go`: list/get, revision-guarded move to lost, equal-location no-op.
- `output_test.go`: exact json/text/none delete success and repeated-delete not_found failure.
- `storage_test.go`: target-name rejection, symlink/directory/FIFO/hardlink target rejection, 4 MiB+1 target rejection. Special/large objects use direct OS assertions, not expanded snapshot helpers.
- `lock_test.go`: deterministic real held advisory-lock timeout, no sleep-based overlap. This is not multiwriter no-lost-update evidence.

All deterministic calls assert exact status/stdout/stderr and complete immediate state, with schema validation for JSON boundaries. Expected document bytes/SHA-256 are declared independently. Regular fixtures reuse approved write/observe/compare helpers; all writer fixtures reuse precreated empty 0600 locks. `Makefile` selects all 17 top-level subject tests via `^(TestSubject.*|TestFormatLifecycle)$` with 120s suite timeout, retaining green helper/schema targets. No approved helper/schema code was changed.

Latest bounded direct checks (60s aggregate command bound):

- gofmt on all ten new files: PASS.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s`: PASS, package 1.108s, wall 2.561s.
- Same with `-race`: PASS, package 2.271s, wall 3.030s.
- `go vet ./tests/cli`: PASS, wall 0.310s.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s`: PASS, package 0.257s, wall 0.630s.
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s`: expected FAIL, package 0.080s, wall 0.434s; all 17 fail explicitly for missing binary, with no skip/fallback.
- `git diff --check`: PASS. No staging/commits/reset/clean/push.

**No actual Sidecar was supplied; these are construction/plumbing checks, not real-subject conformance proof. Parent must launch TWO independent parallel reviewers over the frozen changes. None were launched by this worker.**

## Final continuation and freeze — supersedes preceding intermediate gaps/results

The user required finishing the straightforward remaining families before review. There are now **22 new test files / 31 new subject tests**, plus the approved lifecycle and availability gate (33 selected top-level tests). Existing staged work and helper/schema adapter remain untouched. Additional files:

- `domain_negative_test.go`: negative stored metadata/token/domain cases including duplicate/nested/unknown fields, UUID spelling/version/variant/nil, surrogate/NFC/control/name limits, GitLab number grammar, incompatible author metadata, body limit/reserved prefixes/Unicode White_Space. `metadata_test.go` adds positive name/identity scalar limits.
- `generated_test.go`, `id_output_test.go`: nonempty version via schema and exact canonical reconstruction; concrete generated fix assignment extraction, independently reconstructed bytes/revisions, retry/check/no-op id and successful id-output validation. No generic JSON or type assertions.
- `concurrency_test.go`: two real independent writers behind a start barrier, both children joined, exact response pair must equal one of two legal independently computed serializations, exact deterministic combined final document. No timing oracle or assumed response order; no proof of forced precommit overlap claimed.
- `object_boundaries_test.go`: invalid lock/source symlink/directory/FIFO, direct complete state checks, and missing source before revision/entity semantics.
- `guards_test.go`: changed update, last-author mismatch, valid double-suffix derivation.
- `limits_guard_test.go`: exact thread/per-thread-message/document-message maxima, rejected next mutation, complete preservation. Large file checks are direct, not expanded snapshots.
- `size_boundaries_test.go`, `anchor_boundary_test.go`, `target_boundary_test.go`: exact/plus-one source, body, anchor, and target boundaries.
- `apply_boundary_test.go`: 256 operations in an exact 1 MiB valid request, exact no-op results/state.
- `input_boundary_test.go`: exact 16 KiB metadata and largest valid all-CRLF body input, independent normalized bytes/revision.
- `subject_test.go` also adds formatter thread ordering and missing thread separators.

Final bounded direct checks (60s aggregate command ceiling):

- gofmt: PASS on all new/changed test files.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s`: PASS, package 1.062s / wall 1.817s.
- Same with `-race`: PASS, package 2.334s / wall 3.207s.
- `go vet ./tests/cli`: PASS, wall 0.310s.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s`: PASS, package 0.119s / wall 0.496s.
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s`: expected FAIL, all 33 explicitly fail for absent binary, package 0.107s / wall 0.480s. No skips/fallback/mock subject.
- `git diff --check`: PASS. Branch/HEAD remain red-suite/simple-go / a222e59. All work uncommitted; no staging/reset/clean/push.

**Historical freeze:** both independent reviewers subsequently completed review and agreed on six targeted corrections. See the latest results below. No supplied real binary means no subject conformance proof, including generated-output and multiwriter assertions. The green race command exercises helper/schema code only.

Exact remaining blockers:

1. Missing approved usage-amendment text prevents selecting negative argv/body/stdin/metadata/apply field/reason pairings, including request limit-plus-one errors. Enum membership alone is not the amendment. Valid request maxima are covered; obtain/reconcile the text before asserting ambiguous usage errors.
2. Missing distinction between retry_mismatch and id_collision prevents exact collision-mismatch oracles. Exact retries, revision guards, and last-author mismatch are covered.
3. Clock/CSPRNG/UUID exhaustion, precise identity swaps, descriptor/cleanup phase faults, and durability uncertainty lack an approved injection boundary or deterministic external inducer. UUID exhaustion has a public mapping; temporary-fsync and temporary-name exhaustion symbols must not be invented. Future approved production injection is required, not fake CLI behavior.
4. No SIDECAR_BIN supplied: execution past availability and real-subject conformance remain unavailable.

These limitations prevent an exhaustive finished-v1 claim. Linux/Neovim/production/framework work remains prohibited, not counted as skipped passes.

## Latest: six consensus corrections fixed and approved by BOTH reviewers

Both independent reviewers explicitly agreed on the six findings summarized by the user. This correction pass changed only authorized CLI tests and these plans:

- `generated_mutation_test.go` (new): omitted-ID create/add direct lifecycle in both JSON and successful id modes, schema-validated extracted unpredictable values, independently reconstructed exact DTOs/envelopes/disk/SHA-256, and complete fixture preservation. Id modes include real validate calls to assert reconstructed revisions. No UUID ordering comparison crosses processes.
- `generated_test.go`: expanded fix to two missing messages spanning two threads. Exact physical assignment lines 2/7, byte offsets/ordinals/order, and distinct strictly increasing IDs from ONE process; independent resulting canonical bytes/revisions; retry/check/no-op state assertions retained. Singleton successful id fix remains in `id_output_test.go`.
- `apply_remaining_test.go` (new): successful thread_move/message_add/message_delete working-copy sequence; indexed intermediate message limit rollback at index 1 despite a later deletion that would restore the limit. Exact message_add/limit_messages_thread apply-local error and complete original state.
- `location_order_test.go` (new): finite shuffled fixture with a literal canonical ordering for line 2/10, range numeric comparisons, anchor absence/presence/decoded-byte order, and UUID ties. No comparator oracle.
- `storage_test.go`: source preservation now Lstats and requires exact regular mode before reading bytes, preventing symlink-following false positives.
- Roadmap semantic checklist now marks generated fix/id/version, negative stored token cases, invalid objects and writers as constructed—not pending—and distinguishes pending review from unresolved mapping/injection blockers and absent execution. No exhaustive simple-coverage claim is made.

The suite now selects 37 top-level tests. Make selection already includes all four new tests; no Make change was needed. Approved staged files and helpers/schema adapter were preserved.

Latest announced bounded checks (60s aggregate ceiling):

- gofmt on the five changed/new test paths: PASS.
- `go test ./tests/cli -run '^(TestHelpers|TestHelper.*|TestSchemaBoundary)$' -count=1 -timeout=30s`: PASS, package 1.127s / wall 1.913s.
- Same with `-race`: PASS, package 2.295s / wall 3.150s.
- `go vet ./tests/cli`: PASS, wall 0.336s.
- `go test ./contracts/runner -run '^TestArchitectureBoundaries$' -count=1 -timeout=30s`: PASS, package 0.271s / wall 0.668s.
- `SIDECAR_BIN= go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s`: expected FAIL, all 37 explicitly fail absent binary, package 0.083s / wall 0.492s. No skip/fallback/mock.
- `git diff --check`: PASS. Branch/HEAD red-suite/simple-go / a222e59, staged work preserved, all work uncommitted. No staging/commit/reset/clean/push.

**Final review complete:** `reviewer-1-deafd522` and `reviewer-2-deafd522` independently reread the corrected final files and their relevant shared wiring/contracts. Both returned APPROVE with no critical findings or warnings, confirming all six agreed corrections. Both independently verified branch/HEAD and passing `git diff --check`; neither reran Go checks or had a real subject. Execution timings above remain worker-reported; approval concerns test construction/oracles, not behavioral conformance.

Reviewer B noted one nonblocking future extension: isolate range end-line comparison using equal start coordinates and differing end lines. General fuzz/property coverage also remains pending as shown in the roadmap. The usage-amendment/collision-mapping/injection/binary blockers above remain unchanged; do not call the suite exhaustive or interpret the remaining list as proof that every other finite case is covered. No production implementation is authorized by this review. All work remains uncommitted.
