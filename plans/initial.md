# 🛵 Sidecar Comments v1 — implementation-ready blueprint

## 1. Scope, hard cut, and engineering rules

This repository owns the Go **1.26** `sidecar` CLI/specification, reusable Neovim **0.12** adapter and headless tests, replacement skill template, and a future config-cutover runbook. It is a hard replacement: only v1 below is accepted; no legacy parser, migration, shim, synthetic legacy ID, or old plugin behavior exists. `../config` is inventory/runbook reference only: no source, test, fixture, subprocess, or CI action may read, write, or depend on it at runtime.

Only a non-directory target whose basename case-sensitively ends in `.comments`, but is not exactly `.comments`, is a sidecar. Reject trailing separators, `foo.comments.tmp`, directories, and every other basename. `comments.md` is free-form and out of scope. Missing sidecar is canonical empty; GitLab data is typed descriptive metadata only—never a client, ingestion, mapping, reconciliation, or resolution policy. Existing `*.comments` files are treated exactly as target input: v1 files are accepted, zero-byte files have the specified empty behavior, and every legacy/pre-v1 structure is invalid (exit 3) and is never migrated, guessed, or rewritten. Cutover inventories and reports them but does not convert or delete them.

Trust boundaries (file text, flags, stdin, JSON, Lua process output) decode concrete DTO structs, validate tokens/duplicate keys/schema/version first, then call constructors for immutable domain types. Standard-library `encoding/json` into concrete DTOs and ordinary checked byte/string conversion are allowed. Prohibit `map[string]any`, `any`, unsafe package use, unchecked named-type conversion of untrusted values, reflection-based domain dispatch, and trust-boundary type-assertion dispatch. Tagged-union switches are exhaustive. Use the Go standard library by default. A narrowly scoped, established dependency is acceptable only where the standard library cannot satisfy the specified descriptor-relative filesystem, advisory-locking, or Unicode requirements; document the gap and pin the dependency. Do not add a general CLI framework unless standard `flag` parsing is demonstrably inadequate.

## 2. Domain, limits, locations, and source validation

`ThreadID` and `MessageID` are distinct named types/namespaces. Each textual ID is a lowercase, hyphenated RFC 9562 UUIDv7 (version nibble `7`, variant `8|9|a|b`, nonnil); IDs are constructed only by validation/generation. Thread and message IDs are each document-unique. Thread header/end text additionally uses `th_` before its UUID; message metadata never does.

Authors are a closed union: `human`, `agent`, `external`. A `Name` is NFC UTF-8, 1–128 Unicode scalar values, and has no control, CR, LF, or NUL. User/bot names are optional; external identity is mandatory and is `{id: ExternalIdentity, name: Name}`, where `ExternalIdentity` is nonempty UTF-8 <=256 scalars with no control/CR/LF/NUL. Optional GitLab metadata is a closed union: `note {project_id: positive integer, discussion_id: nonempty UTF-8 <=256 scalars, note_id: positive integer}` or `system_note {project_id: positive integer, note_id: positive integer}`. It is immutable after create and has no behavior beyond storage.

A message body is a logical UTF-8 string, with LF line separators, excluding the single framing LF immediately before the next structural line. It is <=65,536 normalized UTF-8 bytes and contains >=1 non-Unicode-whitespace scalar. It may contain blank lines and may itself end in LF. Every logical line is scanned; any line beginning byte-for-byte with `[@th_`, `[/@th_`, `---user`, `---bot`, or `---external` is rejected—there is no escape syntax. Equality is exact normalized UTF-8 bytes.

Locations are the closed union `file`, `line:N`, `range:SL:SC-EL:EC [| BASE64]`, or `lost`. Lines and columns are **1-based UTF-8 byte coordinates**. Range end is half-open; every coordinate is a positive integer and `(SL,SC)<(EL,EC)` lexicographically. Columns count bytes excluding terminators: column 1 is the first byte. An anchor is range-only, standard padded RFC 4648 base64, decoded nonempty UTF-8 <=4096 bytes, and is exact selected source bytes. Structural validation checks only location shape/order/anchor encoding.

`validate --source PATH` requires an existing regular, non-symlink source (missing exit 7; symlink/directory/nonregular exit 3), <=16 MiB. Reject BOM, NUL, invalid UTF-8, bare CR, and mixed LF/CRLF. Accept consistent LF or CRLF, normalize CRLF to logical LF, then check lines/byte columns on UTF-8 rune boundaries. Empty source has one empty line; a final unterminated line exists. Valid columns are 1 through line-byte-length+1. Multiline anchors include logical LF between lines and must exactly match selected bytes. `file` requires readable valid source; `lost` always passes. Neovim converts line/column to its 0-based byte API by subtracting one.

For create, move, and apply operations that create or move a thread, source validation is mandatory and implicit: derive the source by removing exactly one terminal `.comments` from `--file` (for example `file.txt.comments → file.txt`); the derived source must pass the same validation and the requested location must resolve in it. This is checked before ID generation and again after acquiring the sidecar lock immediately before commit. No separate mutation `--source` flag exists. `lost` is exempt as above. The sidecar lock is not a source lock: if the source changes between the second validation and commit, concurrent external source writers are outside the atomicity guarantee; anchor mismatch at either validation fails closed.

Limits: stored sidecar 4 MiB; 1,024 threads; 256 messages/thread; 16,384 messages/document; decoded anchor 4 KiB; body 64 KiB; metadata 16 KiB; apply 1 MiB and 1–256 operations. File/source failures are exit 3; request-input failures exit 2.

## 3. Exact text format and repair AST

Nonempty canonical files are UTF-8 without BOM/CR/NUL and have exactly one final LF. Terminals: `LF=%x0A`, `SP=%x20`, `DIGIT=%x30-39`, `positive-int=%x31-39 *DIGIT`; `UUIDv7` is defined above; `text` has no CR/LF/NUL.

```text
canonical-document = thread *(LF thread)
thread             = thread-start LF message *(LF message) thread-end LF
thread-start       = "[@th_" UUIDv7 SP location "]"
thread-end         = "[/@th_" UUIDv7 "]"
message            = role-line LF logical-body LF
role-line          = "---user" [SP meta-comment] / "---bot" [SP meta-comment] / "---external" SP meta-comment
meta-comment       = "<!-- sidecar-meta: " meta-json " -->"
logical-body       = body-line *(LF body-line)
body-line          = *text
location           = "file" / "line:" positive-int / range / "lost"
range              = "range:" positive-int ":" positive-int "-" positive-int ":" positive-int [SP "|" SP base64-anchor]
```

Each thread end must carry exactly its opening UUID. A thread has at least one message. A role line starts a message; the prior message ends implicitly immediately before the next role line or its ID-matched thread end. **Only threads are paired.** The final LF after a logical body is framing and not domain body. Between messages the preceding framing LF plus exactly one separator LF precedes the next role line; before thread end there is no separator beyond framing LF; between threads there is exactly one blank LF. Thus logical `x` before a next role is stored `x\n\n---bot`; logical `x\n` is stored `x\n\n\n---bot`. Parser removes exactly those structural framing/separator LFs.

Parsing is byte-oriented and deterministic. After a valid role line, scan physical lines until the first line that is either a valid role line or the matching valid thread-end. All preceding lines are candidate body lines; reserved-prefix lines that are not the applicable valid delimiter are malformed. At a next role delimiter, remove its immediately preceding separator LF and the mandatory body framing LF (the final two LFs of the candidate run); at a thread-end remove only the mandatory body framing LF (the final LF). Every earlier LF belongs to the body. Therefore, before another role, `x\n\n---bot`, `x\n\n\n---bot`, and `x\n\n\n\n---bot` decode respectively to `x`, `x\n`, and `x\n\n`; before a thread end, `x\n[/@th_…]`, `x\n\n[/@th_…]`, and `x\n\n\n[/@th_…]` decode to `x`, `x\n`, and `x\n\n`. Canonical goldens cover each case, blank internal lines, and adjacent threads. Any line beginning a reserved structural prefix but not exactly valid in its grammar position is malformed, including `---userish`, malformed metadata, or thread syntax inside a body.

A role maps to author kind: `user→human`, `bot→agent`, `external→external`. Canonical normal-domain messages always have metadata with `id`. A hand-written `---user` or `---bot` with no metadata is accepted only by the **repair parser**, which produces `RepairDocument`, not `Document`, and records a `missing_message_id` at its role-line byte offset, 1-based physical line, and message ordinal within the thread. Such a document is repairably invalid: validate reports it; list/get/format and every normal mutation fail closed. `---external` without metadata, or without required external identity, is not repairable and is invalid. Missing metadata is the only repairable condition; malformed/unknown/duplicate metadata and all other invalidity are never guessed or repaired.

Canonical example:

```md
[@th_018f0000-0000-7000-8000-000000000001 line:4]
---user <!-- sidecar-meta: {"id":"018f0000-0000-7000-8000-000000000101","name":"Taylor"} -->
First

---external <!-- sidecar-meta: {"id":"018f0000-0000-7000-8000-000000000102","external_author":{"id":"gitlab:42","name":"Ada"},"gitlab":{"kind":"note","project_id":1,"discussion_id":"d1","note_id":42}} -->
Reply
[/@th_018f0000-0000-7000-8000-000000000001]
```

A convenient repairable handwritten message is:

```md
---user
Please explain this.
```

## 4. Metadata JSON and canonical encoder

Before decoding metadata, token-scan recursively to reject duplicate keys, non-object values, unknown fields, wrong JSON token kinds, invalid UTF-8, invalid escapes, and all `\uD800`–`\uDFFF` surrogate escapes. Accept insignificant whitespace, key order changes, and permitted ordinary JSON escapes only as noncanonical formatter/fix input.

Metadata DTO schemas are closed and role-specific:

- user/bot: `{id: MessageID, name?: Name, gitlab?: GitLabDTO}`;
- external: `{id: MessageID, external_author: {id: ExternalIdentity, name: Name}, gitlab?: GitLabDTO}`.

Canonical metadata keys are ordered `id,name,external_author,gitlab`, omitting inapplicable/absent fields; external-author keys are `id,name`; GitLab note keys are `kind,project_id,discussion_id,note_id`; system note keys are `kind,project_id,note_id`. Encoder writes compact JSON, raw UTF-8 and no HTML escaping (`<`, `>`, `&` literal), and only escapes `"`, `\\`, `\b`, `\f`, `\n`, `\r`, `\t`, plus other U+0000–U+001F as lowercase `\u00xx`.

Canonical thread ordering: kind rank `file,line,range,lost`; line by N; range by SL,SC,EL,EC, absent-anchor before present-anchor, then decoded anchor bytes; UUID bytes break ties. Messages preserve insertion order and are never sorted.

## 5. Empty/noncanonical behavior and revisions

Missing target is canonical empty: `storage_revision:null`; `canonical_revision:sha256:` + SHA-256(empty bytes). Existing zero-byte regular target is valid noncanonical empty: `validate` and `thread list` succeed with warning `noncanonical_empty`; get/update/delete are not-found; it is not a normal domain document. Whitespace-only is invalid. Plain `format` removes zero-byte target; any successful mutation or successful semantic no-op against it removes it. `format --check` returns 3 without writing. A last-thread deletion unlinks target.

For existing canonical files, storage and canonical revision are `sha256:` plus lowercase SHA-256 of exact bytes and are equal. `--if-revision missing` matches missing only; other values exactly match storage revision under lock. A deletion result includes `previous_storage_revision`; afterward storage is null and canonical is empty revision.

Read/mutate commands accept canonical v1 plus zero-byte empty. `format` additionally accepts only otherwise-valid v1: consistent CRLF replacing every LF; zero-byte empty; noncanonical thread order; metadata JSON differing only by insignificant whitespace/key order/allowed escapes; omitted final LF or one extra terminal LF; and one omitted separator LF between adjacent role lines or threads. It rejects all pre-v1 forms, whitespace-only files, malformed/reserved-prefix lines, and every other difference. Plain format canonicalizes; check reports noncanonical.

## 6. CLI, output, and errors

Syntax: `sidecar [--output text|json|id|none] COMMAND ...`. Human-readable `text` output is the default when `--output` is omitted. JSONL and other streaming output modes are not part of v1 and must not be added until a concrete streaming use case is specified and versioned. Global output precedes command. `id` is create/add/fix only; `none` is mutation only. Mutations take `--if-revision REV|missing` and `--lock-timeout DURATION` immediately after command (default 5s; 1ms–60s) before command flags. JSON success is exactly one LF-terminated `{protocol:1,command:string,changed:bool,result:ResultDTO}` on stdout. JSON error is exactly one LF-terminated `{protocol:1,error:{code:string,message:string,details:ErrorDetailsDTO}}` on stderr, no stdout. Text is deterministic one-line UTF-8; none suppresses success. Exit codes: 0 success; 2 flags/request/body/DTO invalid; 3 target/source invalid, repairable-invalid, or format/fix check needed; 4 lock timeout/revision conflict; 5 not found; 6 semantic precondition/conflicting explicit create; 7 I/O/security/durability; 8 unsupported protocol.

Closed error details: usage `{field,reason}`, validation `{path,reason}`, repair `{diagnostics:[{code:"missing_message_id",byte_offset,line,thread_id,message_ordinal}]}`, conflict `{expected,current_storage_revision}`, not_found `{entity,id}`, precondition `{operation,reason,index?}`, io `{operation,path,commit_may_have_succeeded?}`, unsupported `{received}`. Error `code` is exactly one of `usage`, `validation`, `repair`, `conflict`, `not_found`, `precondition`, `io`, or `unsupported`; each code permits only its corresponding details object. `path` and `operation` are stable symbolic names, not OS error strings.

Exit 3 check states are results, not errors: JSON writes the ordinary success envelope to stdout and exits 3 for repairable-invalid `validate`, and for `format --check`/`fix --check` when work is needed. All other exit-3 invalid target/source states use the error envelope on stderr. `warnings` is an array of `{code}` and its v1 enum is only `noncanonical_empty`. Revisions are either lowercase `sha256:` strings or JSON `null`; optional DTO fields are omitted, never `null`. All public JSON DTOs are closed: unknown fields, wrong token kinds, out-of-range integers, and absent required fields are request/validation errors. The protocol document supplies a machine-readable JSON Schema and matching golden fixtures for `LocationDTO`, role-specific `AuthorDTO`, `GitLabDTO`, every result DTO, every apply per-operation result, and every error detail; the CLI contract harness rejects schema/fixture drift.

Commands/results:

```text
version
validate --file PATH [--source PATH]
format [--if-revision REV|missing] [--lock-timeout DURATION] --file PATH [--check]
fix [--if-revision REV|missing] [--lock-timeout DURATION] --add-missing-message-ids --file PATH [--check]
thread list --file PATH
thread get --file PATH --id UUIDv7
thread create --file PATH --location LOCATION --author-kind human|agent|external [--author-name NAME] --body-file PATH|- [--external-author-id ID] [--id UUIDv7 --message-id UUIDv7] [--gitlab-json PATH|-]
thread move --file PATH --id UUIDv7 --location LOCATION
thread delete --file PATH --id UUIDv7
message add --file PATH --thread UUIDv7 --author-kind human|agent|external [--author-name NAME] --body-file PATH|- [--external-author-id ID] [--id UUIDv7] [--gitlab-json PATH|-] [--if-last-author-kind KIND]
message update --file PATH --thread UUIDv7 --id UUIDv7 --body-file PATH|-
message delete --file PATH --thread UUIDv7 --id UUIDv7
apply --file PATH --operations-file PATH|-
```

`version` result is `{cli_version,format_version:1,protocol:1}`; validate `{valid:true,warnings,storage_revision,canonical_revision}` or the same result envelope with `{valid:false,repair:{diagnostics}}` and exit 3; format `{formatted:bool,storage_revision,canonical_revision}`; list `{threads:[ThreadDTO],warnings,storage_revision,canonical_revision}`; get `{thread,storage_revision,canonical_revision}`. `ThreadDTO={id,location,messages:[MessageDTO]}`; `MessageDTO={id,author:{kind,name?,external_author?},body,gitlab?}`. Create/move return `{thread,storage_revision,canonical_revision,generated_ids?}`; add/update return `{thread,message,storage_revision,canonical_revision,generated_id?}`; thread delete returns `{deleted_thread_id,previous_storage_revision,storage_revision,canonical_revision}`; message delete returns `{deleted_thread_id?,deleted_message_id,previous_storage_revision?,storage_revision,canonical_revision}`. `fix` returns its specified assignments and revisions. `--output id` prints the generated ID for create/add, and for fix prints one assigned UUID per LF-terminated line in assignment order; a successful no-op prints no IDs. `--output id` is otherwise invalid.

Body-file handling: read bounded bytes, reject BOM/NUL/invalid UTF-8/bare CR/mixed endings, normalize consistent CRLF, strip exactly one final LF transport framing if present, then validate logical body. Apply JSON `body` is already logical: reject CR and do not strip LF. At most one of body/metadata/operations paths may be `-`; otherwise exit 2.

## 7. Fix and mutation semantics

Single thread create supplies both thread/message IDs or neither; exactly one is exit 2. Missing pair is generated cryptographically only after full request/source validation and target lock. Message add similarly generates omitted ID after those checks. External creates/adds require external-author ID and name. Human and agent names are optional everywhere; when omitted they are absent (never an empty substitute). Apply’s `author` DTO is role-specific: `{kind:"human"|"agent",name?:Name}` or `{kind:"external",external_author:{id:ExternalIdentity,name:Name}}`; it has no other fields. Explicit create retry is no-op only if every creation field exactly matches: IDs, location, role/author details, normalized body, and GitLab metadata. Otherwise exit 6. Update/move/delete absent is exit 5. `if-last-author-kind` checks current working thread state.

`fix --add-missing-message-ids` first repair-parses. It accepts only a document with one or more `missing_message_id` diagnostics and no other error, locks/rereads, repeats that validation, allocates UUIDv7 IDs in physical document message order after full validation, constructs normal domain data, validates/canonicalizes, and atomically writes once. It returns `{assignments:[{thread_id,message_ordinal,line,byte_offset,message_id}],storage_revision,canonical_revision}` in that same physical order. IDs are random but assignment ordering/locations are deterministic. A concurrent retry sees repaired canonical content and returns success `changed:false,assignments:[]`; `--check` writes nothing and returns 3 if assignments would be needed, 0 with empty assignments if already valid, and 3 for any other invalidity. Fix never invents names, external identity, metadata, or repairs malformed/duplicate IDs.

Deleting the final message of a thread deletes that thread atomically; the result identifies both the message and implicitly deleted thread. A message delete against a missing thread/message is exit 5, including in apply (with its operation index). Moving to the current location and updating to the exact current body are successful `changed:false` no-ops. A single mutation’s delete is never retry-idempotent once its entity is absent. In apply, a delete followed by create using the deleted ID is a new create; all resulting document-wide thread/message ID uniqueness and limits are checked against the sequential working copy. A duplicate explicit create is a no-op only under the exact-retry rule; every other collision is exit 6. Any resulting-document limit excess is exit 6 `precondition` with the violated limit, leaves bytes unchanged, and applies equally to a batch.

Apply token-validates closed request `{protocol:1,if_revision?,operations}` and all concrete operation DTOs before lock; request revisions are top-level only and equal CLI revision if both. Variants (all create IDs explicit) are `thread_create {op,thread_id,message_id,location,author,body,gitlab?}`, `thread_move {op,thread_id,location}`, `thread_delete {op,thread_id}`, `message_add {op,thread_id,message_id,author,body,gitlab?,if_last_author_kind?}`, `message_update {op,thread_id,message_id,body}`, and `message_delete {op,thread_id,message_id}`. Operations have only typed semantic preconditions. After lock/read/revision, evaluate sequentially against working copy: later sees earlier; create→update/delete works; delete→create is new; missing update/delete is 5; duplicate create is exact retry only or 6. Any failure includes index and leaves bytes unchanged. Success result `{operations:[{index,op,changed,result}],storage_revision,canonical_revision}`. Mutation no-ops do not write except zero-byte cleanup, which is changed true.

## 8. Secure storage and Neovim

Hardened mutation support is Darwin/Linux only. Other OS mutation fails exit 7 `unsupported_secure_filesystem`; read-only is documented reduced support. The lock and filesystem race protection are cooperative: a hostile process with authority to mutate the parent directory is outside the threat model. Use no-follow descriptor-relative parent/target/lock/temp operations; never path-string TOCTOU fallback. Securely open parent; target must be absent or regular nonsymlink link-count one. Reject target/lock symlinks, multi-link target, directory/nonregular targets. Sibling lock `.<basename>.sidecar.lock` is no-follow regular, identity-verified after advisory exclusive lock. Every writer—format, fix, single mutation, and apply—uses this transaction: validate request bytes; open/lock; securely reread and parse; repeat all validation authorizing the write (including implicit-source validation); compare revision; evaluate the operation(s); return no-op without a write unless zero-byte cleanup is required; revalidate parent/target identities; commit or identity-checked unlink; fsync parent; release/clean up. Changes detected before commit are conflict 4. Correctness must not depend on an in-process mutex or on all writers sharing one process. Contract and adversarial subprocess tests must demonstrate that concurrent external processes cannot silently lose successful updates. Create 0600 random exclusive same-directory temp, retry collision <=16, verify it, write/fsync, chmod original mode or `0600 & ~umask`, fsync, descriptor-relative atomic rename, parent fsync. Identity-checked unlink plus parent fsync deletes. Precommit failure preserves old bytes. Post rename/unlink parent-fsync failure is exit 7 with `commit_may_have_succeeded:true`.

Adapter source mapping appends/removes exactly one `.comments` (`a.go→a.go.comments`; `a.comments→a.comments.comments`). Only named regular non-symlink file buffers work; unnamed, directory, terminal, URI/non-file, invalid path, or symlink source diagnose and disable UI. Lua owns extmarks/navigation/cursor/visual capture/dirty buffers/popups/diagnostics, uses argv APIs and typed JSON only, and has no parser/serializer. Post-save/external sidecar changes run validate; repair diagnostics offer explicit `fix --add-missing-message-ids` action, other errors fail-close all mutations until repaired. Never overwrite dirty sidecar buffer; health reports missing/incompatible CLI.

## 9. Green sequence, deliverables, and gates

Deliver `cmd/sidecar`, `internal/{cli,domain,format,operations,store}`, Lua/plugin/help, docs (`format`, `protocol`, `cli`, `architecture`, `neovim`, `cutover`), `templates/SKILL.md`, contracts/goldens, and CI. Skill requires CLI JSON reads and CLI/apply writes. The cutover runbook inventories every existing `*.comments` target and validates it with the new binary before installing the adapter or deleting old integration; invalid legacy files are reported as invalid and left untouched. It takes a backup, supports dry-run, aborts before changes on inventory/validation failure, documents rollback from that backup, installs the replacement adapter/skill, then deletes old plugin/docs/keymaps/skills/readers/writers and updates the address-comments workflow—without retained compatibility code. Post-cutover it validates all discovered targets and confirms no dirty sidecar buffers before declaring success.

### Required development and contract workflow

For every externally visible behavior: (1) specify it in the relevant `docs/` contract; (2) add a failing black-box contract fixture against the compiled executable; (3) implement the smallest change that makes it pass; (4) add package-level tests only where behavior cannot be exercised adequately through the executable; and (5) keep all previously published contract fixtures green during refactoring and protocol evolution. Contract fixtures describe only initial filesystem state, command arguments, stdin bytes, expected exit status, expected stdout and stderr bytes, and expected final filesystem state. The contract runner invokes `SIDECAR_BIN`, with a documented default built binary, and must not import `internal/` packages or depend on private implementation details. The same corpus must be runnable against any executable claiming v1 compatibility.

Every phase adds fixtures with implementation and ends green: (1) bootstrap/docs/binary contract harness; (2) constructors/token JSON tests; (3) parser/read/format/fuzz; (4) secure store and adversarial race tests before any writer; (5) mutations/apply/fix contracts; (6) real-binary Neovim headless tests; (7) skill/runbook/release docs. Phase gates name their deliverables, prerequisite phase, executable commands, contract subset, and completion criteria; phase 4 includes failure injection and adversarial subprocess tests, and phase 6 enumerates dirty buffers, stale async responses, selection ranges, extmark lifecycle, external changes, protocol negotiation, and fail closure.

Test every terminal, role/meta schema, duplicate keys/escapes/surrogates/HTML bytes, repair AST/fix/check/retry, canonical/noncanonical variants, empty/missing, bodies/framing/reserved prefixes, locations/source endings/anchors, IDs/retries/apply sequencing, stdin/output/exits, target names, locks/symlinks/races/modes/fsync failure, and adapter popup/extmark/fail-closure flows. Define and test an error-precedence table for missing, permission-denied, symlink, nonregular, oversized, malformed, concurrent-change, and durability failures for sidecar, source, body, metadata, and operations paths. Reads are bounded/streaming before allocation; metadata’s 16 KiB limit is raw metadata JSON bytes and resulting-document limits apply after every mutation/batch operation. Pin the Unicode version/properties for NFC, control, and whitespace in the protocol, reject rather than normalize non-NFC names, and add cross-language Unicode conformance fixtures. Specify UUIDv7 CSPRNG/clock failure behavior, collision retry bound, and monotonicity, with injectable clock/randomness tests. Document supported macOS/Ubuntu versions and architectures, filesystem assumptions, unsupported-OS read-only behavior, pinned Go/Neovim provisioning, fuzz bounds, installation/reproducible build, compatibility policy, lock recovery, and `commit_may_have_succeeded` recovery.

Required property tests: arbitrary bytes supplied to every parser must never panic; every successfully parsed normal `Document` must canonically serialize and reparse to a semantically equivalent document; canonical serialization must be idempotent; revisions must be deterministic for identical bytes; and message bodies, including trailing logical LFs and internal blank lines, must round-trip exactly according to the framing rules. Seed fuzz tests with every canonical, noncanonical-format, repair, and malformed format fixture.

Add automated architecture checks for forbidden production uses (`unsafe`, reflection dispatch, `any`, `map[string]any`, and runtime `../config` access), package dependency direction, JSON-schema/golden freshness, and limit-plus-one bounded-read cases. Provide checked-in targets for `make build`, `make test-contract`, `make test`, `make test-race`, `make test-fuzz`, `make test-neovim`, and `make check`. `make check` runs the complete release gate, including schema/golden freshness and formatting/module-drift checks. The documented provisioning path plus these targets must work from a fresh clone without relying on untracked files or `../config`. Release gate: build, contract executable tests, `go test ./...`, race, vet, gofmt, `go mod tidy` drift, bounded fuzz, and Neovim 0.12 headless all pass; no runtime access to `../config`.
