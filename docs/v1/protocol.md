# Protocol, DTOs, JSON Schema, and errors

[`schema/v1.json`](schema/v1.json) is the normative Draft 2020-12 public JSON Schema. It validates exactly one top-level `SuccessEnvelope`, `ErrorEnvelope`, or `ApplyRequest`; all public DTO definitions are in `$defs` under their displayed names. It is intentionally supplemented by this document and the domain contracts where JSON Schema cannot express duplicate-member rejection, integer token spelling, byte limits, UTF-8/NFC, non-nil UUIDs, decoded base64, or cross-field/source semantics. An implementation MUST token-scan before decoding: duplicate members at any depth, unknown members, missing required members, wrong token kinds, invalid UTF-8/escapes, surrogate escapes, exponent/fractional numbers, and integers outside `1..9223372036854775807` are rejected.

## Envelopes and output mode

For `--output json`, a success is exactly one compact UTF-8 stdout object followed by one LF (`U+000A`):

```json
{"protocol":1,"command":"COMMAND","changed":false,"result":{}}
```

A JSON-mode failure is exactly one compact UTF-8 stderr object followed by one LF (`U+000A`), and writes no stdout:

```json
{"protocol":1,"error":{"code":"CODE","message":"MESSAGE","details":{}}}
```

The reference encoder emits fields in schema property order; JSON key order is not semantic. Optional properties are omitted, never `null`. The only permitted result nulls are `storage_revision` (a missing target after a read or deletion) and `canonical_revision` in `ValidateRepairResult` or the repair-needed `fix --check` result.

`--output text` (the default) emits `COMMAND changed=BOOL result=JSON` followed by one LF (`U+000A`) on success and `error CODE MESSAGE` followed by one LF (`U+000A`) on failure. `--output id` is valid only for `thread create`, `message add`, and `fix`: it emits generated UUIDs only (one LF each), emits none for a no-op, and emits the text error form on failure. `--output none` is valid only for mutation commands, suppresses successful output, and emits the text error form on failure. Thus JSON envelopes are selected only by `--output json`; `id` and `none` never produce a success envelope. Invalid command/output combinations are `usage` exit 2.

## DTOs and result variants

`LocationDTO`, `AuthorDTO`, `GitLabDTO`, `MessageDTO`, `ThreadDTO`, `RepairDiagnostic`, every result DTO, each apply operation/result, and each error-details DTO are closed (`additionalProperties:false`). `LocationDTO` is exactly one of `{kind:"file"}`, `{kind:"line",line}`, `{kind:"range",start_line,start_column,end_line,end_column,anchor?}`, or `{kind:"lost"}`. `AuthorDTO` is exactly human/agent with optional `name`, or external with required `external_author`. `GitLabDTO` is exactly `note` or `system_note`.

A normal or empty document has a canonical revision. A repair document has no canonical serialization and reports `canonical_revision:null`; its storage revision is non-null because it exists. Consequently, when `fix --check` finds repairable missing message IDs, it returns the repair-needed check result `{assignments:[],storage_revision:Revision,canonical_revision:null}` with exit 3. It does not manufacture prospective IDs or a canonical revision. A normal `fix --check` result has a canonical revision. Missing has `storage_revision:null` and the SHA-256 empty canonical revision. Existing zero-byte targets have both empty hashes and warning `{"code":"noncanonical_empty"}`.

The command/result/`changed` relationship is fixed:

| command | result | changed |
|---|---|---|
| `version` | `VersionResult` | false |
| `validate` | `ValidateNormalResult` or `ValidateRepairResult` | false |
| `format` | `FormatResult` | iff bytes changed or zero-byte target was removed |
| `fix` | `FixResult` | iff IDs were written |
| `thread-list`, `thread-get` | `ListResult`, `GetResult` | false |
| `thread-create`, `message-add` | `CreateResult`, `AddResult` | iff created (including zero-byte cleanup) |
| `thread-move`, `message-update` | `MoveResult`, `UpdateResult` | iff semantic state changed (including zero-byte cleanup) |
| `thread-delete`, `message-delete` | delete result | true |
| `apply` | `ApplyResult` | iff an operation changed state or zero-byte cleanup occurred |

The complete result members are: `VersionResult={cli_version,format_version:1,protocol:1}`; `ValidateNormalResult={valid:true,warnings,storage_revision,canonical_revision}`; `ValidateRepairResult={valid:false,repair:{diagnostics},storage_revision,canonical_revision:null}`; `FormatResult={formatted,storage_revision,canonical_revision}`; `ListResult={threads,warnings,storage_revision,canonical_revision}`; `GetResult={thread,storage_revision,canonical_revision}`; `CreateResult={thread,storage_revision,canonical_revision,generated_ids?}`; `MoveResult={thread,storage_revision,canonical_revision}`; `AddResult={thread,message,storage_revision,canonical_revision,generated_id?}`; `UpdateResult={thread,message,storage_revision,canonical_revision}`; `ThreadDeleteResult={deleted_thread_id,previous_storage_revision,storage_revision,canonical_revision}`; `MessageDeleteResult={deleted_thread_id?,deleted_message_id,previous_storage_revision?,storage_revision,canonical_revision}`; `FixResult={assignments,storage_revision,canonical_revision}`; and `ApplyResult={operations,storage_revision,canonical_revision}`.

`generated_ids` appears only when thread create generated both IDs; `generated_id` appears only when message add generated it. `ThreadID` and `MessageID` are separate schema definitions and namespaces; every field denoting one uses its respective definition (including apply operations, results, and diagnostics). A thread-delete always has `previous_storage_revision`. A message-delete has `deleted_thread_id` exactly when its final message deleted that thread, and has `previous_storage_revision` exactly when that also unlinked the target. Apply operation results deliberately omit document revisions and generated-ID fields: create/move return `{thread}`, add/update return `{thread,message}`, thread delete returns `{deleted_thread_id}`, and message delete returns `{deleted_message_id,deleted_thread_id?}`. They are ordered input order and paired by their `index` and `op`.

## Errors and exits

Error codes are closed and each selects exactly one non-overlapping details DTO in the schema:

| code / exit | details | exact message |
|---|---|---|
| `usage` / 2 | `{field,reason}` | `usage:{field}:{reason}` |
| `validation` / 3 | `{path,reason}` | `validation:{path}:{reason}` |
| `repair` / 3 | `{diagnostics}` | `repair:document` |
| `conflict` / 4 | `{kind:"revision",expected,current_storage_revision}` | `conflict:revision` |
| `conflict` / 4 | `{kind:"lock_timeout"}` | `conflict:lock_timeout` |
| `conflict` / 4 | `{kind:"identity_changed"}` | `conflict:identity_changed` |
| `not_found` / 5 | `{entity,id,index?}` | `not_found:{entity}` |
| `precondition` / 6 | `{operation,reason,index?}` | `precondition:{operation}:{reason}` |
| `io` / 7 | `{operation,path,commit_may_have_succeeded?}` | `io:{operation}` |

`lock_timeout` is used only when advisory-lock acquisition exhausts the requested timeout. `identity_changed` is used only for a detected precommit parent, target, or lock identity change. Neither detail carries a revision guard, so either is representable even when the caller supplied no `--if-revision`. Invalid lock objects use `validation` details with `path:"lock_type"`.

Clock acquisition failure is exit 7 with `code:"io"`, message `io:clock`, and details `{operation:"clock",path:"generator"}`. CSPRNG read failure is exit 7 with `code:"io"`, message `io:csprng`, and details `{operation:"csprng",path:"generator"}`. UUID collision exhaustion remains `io:randomness_exhausted` with `{operation:"randomness_exhausted",path:"generator"}`. These are the complete public mappings for UUID generation failures.| `unsupported` / 8 | `{received}` | `unsupported:{received}` |

The string enums for every field in that table are normative in the schema. `index` is zero-based; it is required for an apply-local `not_found` or `precondition` failure and forbidden for the corresponding single-command failure. `repair` is used for repair-required commands except `validate`, whose repair-needed state is a success envelope with exit 3. `format --check` and `fix --check` work-needed states are also success envelopes with exit 3. Every other invalid target/source state uses an error envelope. OS text, errno, and filesystem paths MUST NOT occur in public messages or details.

Error precedence is: (1) argv and bounded request/body/metadata input syntax and DTO tokens (`usage`); (2) target/source secure classification and bounded reads (`validation` for target name, symlink/type/size/content; `io {operation:"open",path:"source"}` for a missing or unreadable required source and `io` for other open/read failure); (3) lock setup (timeout `conflict:lock_timeout`, invalid lock object `validation:lock_type`, other failure `io`); (4) locked reread and revision (`conflict:revision` before content semantics); (5) locked structural/repair state; (6) required source revalidation; (7) single or indexed sequential operation semantics; (8) precommit identity conflict (`conflict:identity_changed`); (9) commit/durability outcome.

## Apply request

`ApplyRequest` is exactly `{protocol:1,if_revision?:Revision|"missing",operations:[Operation,...]}`. It is token-validated before locking, bounded to 1 MiB and 1–256 operations, and a top-level request revision must equal a CLI revision if both are supplied. Apply `body` is already logical UTF-8: it has no CR or NUL, is not line-ending-normalized, and never loses a final LF. Operations are the six closed schema variants and all explicit IDs are required. They execute sequentially against one working copy; failure returns its typed indexed error and commits nothing.
