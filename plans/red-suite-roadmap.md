# Simple Go behavioral CLI red-suite roadmap

Status: **pre-implementation; fresh start required.** This replaces the experimental v3 completion roadmap. It is a readable test-construction plan, not a proof ledger, executable scenario DSL, migration-cardinality requirement, or release plan. Do **not repair the experimental v3 framework**.

Normative behavior remains defined by [`docs/v1/index.md`](../docs/v1/index.md), especially `protocol.md`, `cli.md`, `storage-security.md`, `schema/v1.json`, and `conformance.md`. This roadmap changes no normative specification. [`initial.md`](initial.md) is historical only.

## Starting policy

Keep the current experimental tree and branch intact. With user consent, start a clean branch/worktree from a verified pre-experiment baseline, verify the chosen commit/refs/status including untracked files in that new session, and carry only these plans plus independently reviewed normative decisions. Do not assume a baseline hash or current `HEAD`; do not reset, clean, delete, move, or commit the original checkout. Current artifacts are unverified candidates outside the clean next worktree, not prerequisites or implementation templates.

The prior mechanical snapshot's 14 modified tracked files and 281 nonignored untracked files are historical counts, not a new status check. It reported runner and make-target failures before a meaningful red-suite gate, including a `mise` sandbox restriction and distinct corpus/clause-scaffold failures. No production Sidecar binary exists. Historical v1 seeds, packs, and specifications can suggest cases but do not need to execute and do not make v3 machinery a dependency.

## Test shape

Use standard Go tests and a small shared process/filesystem helper. It creates temporary source, sidecar, and input files; invokes independently supplied `SIDECAR_BIN`; asserts exact status, stdout, stderr, resulting paths, modes, and bytes; and has explicit timeouts, output bounds, cleanup, and process-tree termination. An absent `SIDECAR_BIN` fails explicitly.

Tests are CLI-first. Explicitly write each lifecycle invocation (such as create → list/get → reply/edit → delete or check → format → check); use tables for finite repetitive boundaries. Expected bytes and revision hashes are independently derived from inputs and the normative rules, never copied from the subject. Validate boundary JSON with `schema/v1.json`; use schema-derived public-protocol DTOs where typed decoding is needed. Share constants and derived expectations without obscuring assertions.

Each owned function, including an ordinary helper, accepts one request struct except mandated Go callback/interface signatures. Use no `any`, unsafe casts/assertions, mock Sidecar implementations, custom DSL, generic adapter registry, public fault hooks, or fake behavioral proof. Small helper-only doubles may test detection of wrong status/streams/timeouts/filesystem results; they do not prove Sidecar behavior. Public arbitrary-byte CLI tests come before any lower-level property tests. Only faults impossible to induce externally may use narrowly justified, approved test-only dependency injection.

## Sequential stages

| Stage | Deliverable | Gate |
|---|---|---|
| A. Clean start | Verify baseline and agree the minimal helper boundary; add green helper/schema self-tests. | One implementer, then one reviewer. |
| B. First lifecycle | Add one genuine public CLI lifecycle with independently reviewed exact oracles. | The subject test is runnable and red if the binary is absent/nonconforming. |
| C. Finite matrices | Expand the semantic families below in small reviewed tranches. | Review each tranche before the next. |
| D. Adversarial cases | Add process/storage adversaries and only justified supplemental faults. | External induction first; test-only injection is narrow and documented. |
| E. Coverage check | Review applicable rows and required gates. | No mandatory case is skipped or silently accepted. |

One implementer followed by one reviewer is the only staffing model for each batch. There is no parallel framework program.

## Semantic coverage checklist

This table is a human review checklist, not an executable registry or proof ledger. “Reviewed” means the independently constructed public observation and its normative basis have been reviewed; it does not claim production conformance.

| Family | Expected public observations | Review status |
|---|---|---|
| Format and repair | `validate`, `format`, `fix`, and `--check` status/streams; canonical bytes, repair diagnostics, preservation or change of files. | Pending |
| Domain, Unicode, body transport, source/location | Public validation and mutation outcomes for IDs, Unicode, body stdin/files/bytes, source validity, locations, anchors, limits, and unchanged-on-failure state. | Pending |
| CLI grammar, protocol, errors, output modes | Argument placement/repetition, stdin arbitration, JSON/text/id/none channels, exact envelopes and errors, status precedence. | Pending; usage mapping needs review |
| Mutations, revisions, sequential apply | Create/list/get/reply/edit/move/delete results, exact stored bytes/revisions, guards, rollback, and explicit sequential working-copy observations. | Pending |
| Secure storage, durability, independent-process races | Public behavior for object/path classes, locks, modes, durability outcomes, identity changes, and independently launched process races with final state and both responses. | Pending; only affected I/O mappings blocked |
| Bounded public-input fuzz/property tests | Arbitrary-byte CLI `validate`/`format` campaigns: no crash/hang and exact public results; narrowly justified supplemental properties only when CLI cannot establish them. | Pending |

## Gates and unresolved decisions

Keep green helper/schema self-tests separate from the intentionally red subject suite. The red suite must execute real public-process cases against `SIDECAR_BIN`; an unavailable subject is a clear failure, never a skip or pass. Completion means applicable rows have runnable, reviewed tests with exact independent oracles, plus narrowly justified supplemental tests for genuinely uninducible properties. It does not mean the implementation conforms or that a release gate is green. Linux and Neovim remain deferred.

The user-approved usage-error mapping amendment is approved intent but unreviewed. Review it independently, including the original specification ambiguity, before asserting exact error cases; do not silently discard it. Temporary-file sync and collision-exhaustion I/O mappings are unresolved only for affected tests and must not delay unrelated cases or be invented quietly.
