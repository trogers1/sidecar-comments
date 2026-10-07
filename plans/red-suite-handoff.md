# Handoff: simple Go behavioral CLI red suite

## Decision and scope

Do **not** continue, repair, or use as a dependency the experimental v3 runner, proof ledger, fixture-generation system, controlled-subject flows, custom DSL, generic adapter registry, aliases migration, or proof framework. In particular, the next session must **not repair the experimental v3 framework**.

This is a documentation-authorized plan for a small, ordinary Go behavioral suite for the public Sidecar CLI. Production implementation remains paused; no production conformance or green release is claimed. Linux and Neovim coverage are deferred. The suite may be intentionally red while no Sidecar binary exists.

The normative source is [`docs/v1/index.md`](../docs/v1/index.md) and its contract map and precedence: in particular `protocol.md`, `cli.md`, `storage-security.md`, `schema/v1.json`, and `conformance.md`. This plan does not amend those documents. [`initial.md`](initial.md) is historical and does not control this work.

## Safe fresh start and retention

Preserve this experimental checkout, its original tree, and its branch for reference or recovery. Do not automatically reset, clean, delete, move, or commit it. If the user wants to remove the distraction, the safe recommendation is:

1. Obtain the user's consent to create a new clean branch/worktree from a **verified pre-experiment baseline**.
2. In the new session, verify the selected commit, refs, Git status, and untracked files; do not assume current `HEAD` is that baseline and do not invent a baseline hash.
3. Carry only these revised plans and independently reviewed normative decisions into the clean worktree.
4. Do not use unreviewed experimental code as an implementation template once that start point is selected.

The last mechanical snapshot reported 14 modified tracked files and 281 nonignored untracked files relative to its `HEAD`. Those are snapshot facts, not a fresh count. Its narrow and full runner tests failed; `make preimplementation-check` and `make full-red` also failed before a meaningful red-suite gate, including a `mise` sandbox restriction and separate corpus/clause-scaffold failures. There is no production Sidecar binary. Historical v1 seeds, packs, and specifications may inform cases, but neither their execution compatibility nor the v3 machinery is required.

## First task in the new session

After the fresh-start choice is verified, agree and implement only the minimal test boundary and its self-tests:

- ordinary Go tests plus a small shared process/filesystem helper;
- temporary source, sidecar, and input files;
- invocation of an independently supplied `SIDECAR_BIN`;
- exact exit status, stdout, stderr, paths, modes, and resulting bytes assertions;
- explicit failure when `SIDECAR_BIN` is absent; and
- separate helper/schema self-tests from the subject red suite.

Do not build a scenario language. Write successive public invocations directly in lifecycle tests (for example, create → list/get → reply/edit → delete, or check → format → check). Use table cases only for repetitive finite boundaries. Independently construct expected bytes and revision hashes from declared inputs and normative rules; never ask the subject for an oracle or copy its output into expected results.

Validate JSON at external trust boundaries against `docs/v1/schema/v1.json`. When typed decoding is useful, use schema-derived public-protocol DTOs; do not force schemas into every helper. Share constants and derived expectations, but do not hide lifecycle assertions. Every owned function, including a normal test helper, takes one request struct, except signatures mandated by Go callbacks or interfaces. Use standard Go, explicit errors and cleanup, per-process timeouts, output bounds, and process-tree termination. Do not use `any`, unsafe casts/assertions, or mock Sidecar implementations.

A tiny test double may prove that the process helper detects an incorrect status, stream, timeout, or filesystem result. Its agreement proves helper behavior only, never Sidecar behavior. Public CLI arbitrary-byte campaigns precede any justified lower-level property test. A genuinely externally uninducible fault may use already-approved, narrow **test-only** dependency injection; it must never become a public CLI hook.

## Sequential deliverables and definition of done

Use one implementer and then one reviewer for each batch, sequentially; do not run a parallel framework-building program.

1. Establish the verified clean baseline, minimal helper, and green helper/schema self-tests.
2. Add one real public CLI lifecycle with independently reviewed exact oracles.
3. Add finite semantic case matrices in reviewed tranches.
4. Add adversarial process/storage cases and only narrowly justified supplemental fault tests.
5. Check the coverage table and gates.

Construction is done when applicable table behaviors have runnable, reviewed public-process tests with exact independent oracles, and genuinely uninducible properties have narrowly justified supplemental tests. Mandatory cases are neither skipped nor silently accepted when the subject is absent. A green helper/schema harness is distinct from the deliberately red subject suite. This milestone is not production conformance or a release gate.

## Decisions needing review, not invention

The user-approved usage-error mapping amendment expresses approved intent, but remains unreviewed. Treat it as a candidate for independent review before using exact errors as an oracle; do not silently disregard that intent. The original specification has ambiguity around the mapping. Likewise, flag temporary-file sync and collision-exhaustion I/O mappings only in affected cases; do not block unrelated tests or quietly invent outcomes.
