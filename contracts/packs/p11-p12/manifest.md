# P11/P12 mutation and apply red pack

**Pack status:** granular REQUIRED_RED suite. Every row is registered by `contracts/packs/required-red-v1.json` and checked by the runner's full-red gate; it is not a direct `corpus/v1` fixture unless that fixture has a complete runnable contract. It supplies no production code and makes no claim that the current corpus runner discovers it.

**Normative sources:** `docs/v1/cli.md` (single mutations and apply), `docs/v1/protocol.md` (DTOs, envelopes, indexed errors, request bounds), `docs/v1/source-and-location.md` (implicit source and revalidation), and `docs/v1/storage-security.md` (transaction, retry, collision, identity, and durability rules).

## Fixture convention

`goldens/` contains runner-shaped golden files with the reserved `p11-p12-` prefix.  Their IDs have the same prefix.  The matrix below is the self-contained manifest for the entire red pack: a row is a required black-box case, including rows that cannot honestly be expressed by the unmodified corpus runner.  `golden` identifies an executable seed fixture; `controlled` identifies a fixture requiring a capability-aware harness extension, rather than silently weakening the expectation.

All successful mutation cases require `writer_platform`. Normal success leaves the persistent sibling lock in the final filesystem; failure-before-lock cases do not. Expectations below use protocol names exactly: apply-local errors have a zero-based `index`, and a failed batch leaves target bytes unchanged and has no partial operation result.

## P11 — single-operation mutation matrix

| ID | operation / setup | required black-box expectation | mode |
|---|---|---|---|
| P11-01 | thread create, explicit IDs, missing target, `lost` | creates thread and initial message once | controlled |
| P11-02 | thread create, retry exact same explicit DTO | success no-op (`changed:false`) | controlled |
| P11-03 | thread create, same thread ID but differing DTO | exit 6 `precondition:thread-create:id_collision` | controlled |
| P11-04 | thread create, only one of `--id` / `--message-id` | exit 2 usage before target work | controlled |
| P11-05 | thread create, both IDs omitted | generates two UUIDv7 values only after source/locked checks; `--output id` prints both in order | golden |
| P11-06 | generated thread create, CSPRNG collision 16 times | exit 7 `io:randomness_exhausted`, no target write | controlled:fault_injection |
| P11-07 | thread move to a different resolved location | updates exactly that thread | controlled |
| P11-08 | thread move to equal location | semantic no-op; zero-byte cleanup remains changed | controlled |
| P11-09 | thread move missing thread | exit 5 `not_found:thread` | controlled |
| P11-10 | thread move, stale revision plus missing ID | exit 4 `conflict:revision` wins | controlled |
| P11-11 | thread delete existing final thread | deletes/unlinks target and reports previous storage revision | controlled |
| P11-12 | thread delete absent thread / retry after delete | exit 5 `not_found:thread`; delete is not retry-idempotent | controlled |
| P11-13 | message add explicit ID | appends message to existing thread | controlled |
| P11-14 | message add generated ID | generates exactly one UUIDv7 after checks; `--output id` prints it | golden |
| P11-15 | message add with equal `if-last-author-kind` | succeeds; mismatch or no last message is exit 6 `precondition:message-add:if_last_author_kind` | controlled |
| P11-16 | message add missing thread | exit 5 `not_found:thread` | controlled |
| P11-17 | message add colliding explicit message ID | exit 6 `precondition:message-add:id_collision` | controlled |
| P11-18 | message update changed body / equal body | changed success / semantic no-op respectively | controlled |
| P11-19 | message update missing thread or message | exit 5 `not_found:thread` or `not_found:message` | controlled |
| P11-20 | message delete a nonfinal/final message | deletes message; final deletion atomically deletes thread and unlinks final target | controlled |
| P11-21 | message delete missing thread or message / retry | exit 5 typed not-found; retry is not a no-op | controlled |

### Outcome applicability

The requested outcome families are covered wherever the protocol defines them. `thread_create` has no entity-not-found case; its retry and ID collision are its preconditions. `thread_delete` and `message_delete` have no successful semantic no-op: absence (including a retry) is `not_found`. `thread_move` and `message_update` have no independent semantic precondition beyond limits/source/revision guards. These are deliberate protocol distinctions, not omitted cases.

## P12 — apply transaction matrix

| ID | request / perturbation | required black-box expectation | mode |
|---|---|---|---|
| P12-01 | one `thread_create` DTO | accepts the DTO and commits one atomic result | golden |
| P12-02 | all six DTOs: create, add, update, move, message delete, thread delete | operations/results remain input ordered; later operations see preceding state; one commit only | golden |
| P12-03 | one each of `thread_move`, `thread_delete`, `message_add`, `message_update`, `message_delete` | each DTO is accepted independently and yields its exact result variant | controlled |
| P12-04 | valid operation at index 0 references missing thread/message | exit 5 typed `not_found` with mandatory `index:0`; no write | golden |
| P12-05 | collision, retry mismatch, and `if_last_author_kind` at nonzero indexes | exit 6 typed precondition with operation, reason, mandatory index; no write | controlled |
| P12-06 | create then update/delete; delete then explicit recreate | sequential working-copy semantics, including legal recreate | controlled |
| P12-07 | `if_revision` in CLI and body differs | usage before lock; equal pair is accepted | controlled |
| P12-08 | stale top-level/request revision plus invalid operation | revision conflict wins over operation semantics | controlled |
| P12-09 | 1 operation | request is accepted (semantic result is tested independently) | golden |
| P12-10 | 256 valid DTO tokens | request is accepted; every resulting-document limit is checked after each operation | controlled:large_input |
| P12-11 | 257 otherwise-valid DTO tokens | exit 2 usage before lock/target work; no write | controlled:large_input |
| P12-12 | batch creates the 1,025th thread, 257th message in a thread, or 16,385th document message | exit 6 indexed `limit_threads`, `limit_messages_thread`, or `limit_messages_document`; rollback all prior working-copy changes | controlled:large_input |
| P12-13 | operation N fails after N prior valid operations | target bytes/revision are exactly pre-batch; no partial results or commit | controlled |
| P12-14 | derived source changed between precheck and locked revalidation | source failure before UUID allocation/commit; target unchanged | controlled:process |
| P12-15 | target, parent, or lock identity changes immediately precommit | exit 4 `conflict:identity_changed`; target not replaced by stale commit | controlled:process |
| P12-16 | lock held beyond requested timeout | exit 4 `conflict:lock_timeout`, no write | controlled:process |
| P12-17 | temp-name collisions through 16 attempts, clock/CSPRNG failures, rename/fsync uncertainty | exact `io` mapping; fsync-after-rename has `commit_may_have_succeeded:true` and no success output | controlled:fault_injection |

## Controlled capability declaration

The existing fixture schema advertises `writer_platform`, `large_input`, `process`, and `fault_injection`, but the checked-in runner has no portable mechanism to schedule the required locked source replacement, identity swap, held advisory lock, deterministic UUID collision, or post-rename fsync fault. Therefore P11-06 and P12-10 through P12-17 are **not marked executable merely because a requirement string exists**. A future capability adapter must report these rows as unavailable with the stated reason and a nonzero result; it must never skip, report pass, synthesize an implementation-specific error, or downgrade the required result. `writer_platform` unavailable similarly means mutation cases are unavailable, not passing read-only cases.

## Seed goldens

- `goldens/p11-p12-apply-thread-create-success.json`
- `goldens/p11-p12-apply-six-dto-transaction.json`
- `goldens/p11-p12-apply-indexed-not-found.json`
- `goldens/p11-p12-thread-create-generated-id.json`
- `goldens/p11-p12-message-add-generated-id.json`

The two generated-ID fixtures intentionally use the approved UUID bind/ref templates; they test generated IDs without asserting a clock value. The `apply-six-dto-transaction` request is retained as a declarative red seed because its operation payload is inspected by a capability-aware pack loader, not central runner discovery.
