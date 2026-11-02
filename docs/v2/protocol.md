# Protocol 2 and CLI

## Syntax and envelopes

Keep v1 CLI syntax, output selectors before command words, flag validation, help, body transport, revision options, output delivery and terse error modes. Replace protocol/format version constants with 2. CLI release target is 0.2.0. New syntax:

```
source inspect --file PATH
thread list --file PATH [--include-tracking]
thread get --file PATH --id UUIDv7 [--include-tracking]
thread create [revision options] [--if-source-revision SHA256] ...
thread move [revision options] [--if-source-revision SHA256] ...
sync [revision options] --file PATH [--check]
editor-update [revision options] --file PATH --operations-file PATH|-
```

`SHA256` is `sha256:` plus 64 lowercase hexadecimal characters, not a bare digest. Existing create/move flags replace `...`; no additional flags or location anchor suffix are introduced. User locations are only file/line/range/lost; unresolved is output/internal state, never user input. Explicit `--include-tracking` is accepted only for thread get/list. It exposes excerpts in JSON; human presentation still does not dump retained context. Global `id` remains limited to create/message add/fix; `none` is permitted for new mutation commands sync/editor-update, not source inspect. Check output is a successful result, not an error merely because exit is 3.

JSON success stdout is one compact object plus LF:

```
{protocol:2,command:COMMAND,changed:boolean,result:RESULT}
```

Failure is one compact stderr object plus LF and no stdout:

```
{protocol:2,error:{code,message,details}}
```

The new command symbols are `source-inspect`, `editor-update`, `sync`. Existing hyphenated command symbols remain unchanged. JSON key order is nonsemantic; reference encoding follows schema property order. Optional properties are omitted; only existing revision result nulls remain permitted. Token scanning rejects duplicate members at all depths, invalid UTF-8/escapes/surrogate escapes, unknown fields, wrong types and noncanonical integer token spelling before decode. Zero is allowed for counts/indexes where their schema permits it; positive coordinates/IDs retain existing bounds.

Schema acceptance is necessary but not sufficient: response shape also depends on invocation. Ordinary list/get MUST use redacted `ListResult`/`GetResult`, not full tracking variants; explicit opt-in uses `TrackingListResult`/`TrackingGetResult`. List mode must be consistent across all threads. The `anyOf` is intentional because an empty/file/lost-only result has the same representation in either mode. Every mutation/apply result is redacted. No caller supplies trusted tracking records.

## Source inspect and guards

Source inspect validates target suffix naming, derives source by stripping exactly one `.comments`, validates only that source, and returns `{source_revision,line_count,byte_count}` with changed false. Counts refer to normalized logical UTF-8, not raw CRLF bytes. It returns no excerpts, does not parse/require a sidecar, does not inspect stale attachments, lock, allocate IDs or write. Existing `validate --source PATH` remains attachment validation, not source inspection; unresolved threads have no active coordinate to validate, and their stored record is structurally validated, not asserted against current text.

Ordinary create/move accept optional `--if-source-revision`. Apply accepts optional top-level `if_source_revision`. Editor updates require `source_revision` and `if_revision`. Sidecar guards refer to exact storage bytes (or `missing`); source guards refer to all normalized logical source bytes. Machine clients using previously computed coordinates MUST supply both. Interactive create/move may omit source guards and deliberately attach to validated current saved disk text.

Check supplied source guards before locking and again under the sidecar lock during final source revalidation immediately before commit. A matching explicit-ID create retry is recognized before fresh capture and remains a no-op without recapture, but does not bypass supplied guards. Mismatched create inputs retain retry_mismatch/id_collision semantics. Source guards do not cause implicit sync or silently relocate unrelated threads. Requests without source-dependent work must not induce source reads except an explicitly supplied source guard. Source is never locked or written: changes after final validation are outside transaction guarantees.

Apply is exactly `{protocol:2,if_revision?,if_source_revision?,operations}`. Existing six operations, explicit IDs, ordered working-copy evaluation, atomic commit and index rules remain. Only create/move use `InputLocation`; no anchor or unresolved input is permitted. Requests are at most `MaxApplyBytes`, with 1–`MaxApplyOperations` operations. Request and CLI sidecar guards, when both supplied, must agree; disagreement is usage:revision:incompatible. No ordinary CLI source-guard flag is added to apply; its source guard is the request member.

## Editor batch

Request is exactly `{protocol:2,if_revision,source_revision,updates}`. Both guards are mandatory; CLI if-revision, if also supplied, must equal the request guard. Input is bounded to `MaxApplyBytes` and 1–`MaxApplyOperations` updates. Each item is exactly `{thread_id,location:LineLocation|RangeLocation}` or `{thread_id,deleted:true}`. Reject duplicate thread IDs, nonexistent threads and invalid coordinates. A deleted report requires existing retained tracking to preserve; report not_tracked rather than manufacturing evidence for file/lost. A location mapping explicitly captures current saved source for the named existing thread, including reattachment of unresolved/file/lost. No unsaved source text or trusted context payload is accepted.

A deleted report preserves IDs/messages and record but makes the active location unresolved, never lost. Explicit coordinates recapture CLI-owned tracking. Batch validation, both guards and final locked source revalidation precede one atomic commit. Any failure rejects the entire batch; no split commits. Results are `{source_revision,storage_revision,canonical_revision,outcomes}`, with outcomes in request order and editor evidence. Even a deleted-only batch validates its mandatory source guard. The sidecar cannot be absent on success because each update names an existing thread.

Exact editor coordinates exclude insertions at either selection boundary (`right_gravity=true`, `end_right_gravity=false`). This is a client mapping responsibility, not an offline inference rule. Simulated supplied-coordinate tests cannot establish actual Neovim behavior.

## Results and revisions

New result definitions are normative in the schema. Source inspect is always changed false. Editor changed means persisted state changed, including refreshed record; an identical accepted capture is a no-op. Sync returns `{check,would_change,source_revision?,previous_storage_revision,storage_revision,canonical_revision,outcomes}`. Outcomes are in deterministic thread-ID order, exactly one per thread. Previous storage revision is the observed pretransaction revision; final revisions describe actual persisted state, or observed unchanged state for check. Source revision is present iff there were tracked/unresolved threads. Sync outcomes never expose excerpts or historical coordinates; see [tracking](tracking.md) for exact reason/evidence combinations and exit rules.

All unrelated results retain v1 fields and semantics, replacing every embedded Thread with the v2 redacted DTO. Version is `{cli_version,format_version:2,protocol:2}`. Thread move uses GetResult shape; message update uses MessageResult. Generated IDs, previous revision fields on deletions, repair/check nullability, warnings, canonical empty hashes and operation-result revision omission are unchanged. Canonical hashes include v2 header and metadata; missing storage is null. Full tracking reads do not alter revision meaning.

## Errors and precedence

The schema closes all error detail enums. Error messages exactly interpolate their detail symbols, never OS messages, paths, source bytes or excerpts:

| code | exit | message / details |
|---|---:|---|
| usage | 2 | `usage:{field}:{reason}`, `{field,reason}` |
| validation | 3 | `validation:{path}:{reason}`, `{path,reason,diagnostics?}` |
| repair | 3 | `repair:document`, existing `{diagnostics}` |
| conflict | 4 | `conflict:revision`, existing revision details |
| conflict | 4 | `conflict:source_revision`, `{kind:"source_revision",expected:Revision,current_source_revision:Revision}` |
| conflict | 4 | `conflict:lock_timeout`, `{kind:"lock_timeout"}` |
| conflict | 4 | `conflict:identity_changed`, `{kind:"identity_changed"}` |
| not_found | 5 | `not_found:{entity}`, existing `{entity,id,index?}` |
| precondition | 6 | `precondition:{operation}:{reason}`, `{operation,reason,index?}` |
| io | 7 | `io:{operation}`, existing IO details |
| unsupported | 8 | `unsupported:{received}`, existing unsupported details |

### Located sidecar diagnostics

Parser validation errors may include `details.diagnostics`, a nonempty array of
`{code,message,hint,line,column,byte_offset}`. `code` identifies the specific
problem; `message` explains it and `hint` describes how to fix it. These fields
contain CLI-authored guidance, not excerpts of the file. The existing envelope
code, symbolic message, path, reason and exit status are unchanged.

Coordinates refer to the **sidecar passed with `--file`**, not its source.
`line`, `column` and `byte_offset` are all one-based; columns and offsets count
UTF-8 bytes. An EOF diagnostic can point just after the last byte. Editors should
convert to zero-based coordinates and clamp EOF positions for display. Do not
attach unlocated filesystem, source or argument errors to a sidecar line.

The parser reports the first fatal error rather than guessing how to recover
past damaged framing. Fix it and validate again to discover subsequent errors.
The existing missing-message-ID repair diagnostics remain a separate shape and
can contain multiple entries. Human text shows located errors and their fix
hints directly; JSON clients consume the structured array on stderr. Successful
`validate` responses with `valid:false` continue to report repair diagnostics in
`result.repair.diagnostics` on stdout.

Unresolved matching is success data, never an error envelope. Source guard mismatch is conflict, not source-coordinate validation. Missing/unreadable required source stays io:open with path source, not a revision conflict with null current. `source_anchor` is removed; invalid legacy anchor input is rejected structurally. Corrupt persisted records use validation:tracking:invalid (or too_large for decoded bounds); inconsistent location/record lifecycle uses validation:tracking:invalid. Malformed stored location remains validation:target_content:location; source coordinate failures remain validation:source_coordinate:location.

Oversized explicit selected-text capture is precondition limit_selection; aggregate decoded tracking overflow is precondition limit_tracking. A dedicated tracking metadata object exceeding its separately derived encoded-byte bound is validation:tracking:too_large; the message-metadata bound does not apply to tracking records. Serialized target overflow remains validation:target_size:too_large. Editor duplicate IDs use precondition:editor-update:duplicate_thread, deleted-without-record uses precondition:editor-update:not_tracked, and nonexistent IDs use not_found:thread. Editor local precondition/not_found failures require zero-based index just like apply; single commands forbid index. For sync, limits are whole-transaction failures without an index. Other existing preconditions retain their original mapping.

Malformed editor/apply requests, missing/wrongly typed guards and illegal input locations are usage:operations:malformed. A malformed ordinary source-guard flag uses usage:source_revision:malformed. Input byte overflow is usage:operations:too_large. Required CLI flags remain usage:argv:required. Unsupported protocol dispatch happens only after validating the remaining request shape and bounds.

Preserve v1 precedence: input syntax; secure classification/bounded reads; lock setup; locked revision comparison before target semantics; locked structural/repair validation; required source revalidation; operation semantics; identity checks; durability/delivery. Source guards are checked with source validation prelock and again at the locked source step; if both guarded revisions mismatch at locked revalidation, sidecar revision wins. A prelock source failure can precede acquisition of any lock. No source conflict implies a source lock or source/sidecar atomic snapshot. Delivery failure and uncertain commit remain the existing exit-7/no-second-envelope policy; callers reread before retry, never blindly replay generated-ID mutations.
