# CLI and operation contract

Syntax is `sidecar [--output text|json|id|none] COMMAND ...`; global output precedes command. Default `text` is human-readable UTF-8: command-specific summaries, IDs, locations, authors, and comment bodies where relevant, with actionable diagnostics on stderr. Human text is presentation, not a machine protocol; scripts must select `json`, `id`, or `none`. Unsafe terminal-control characters in comment bodies are displayed as visible escapes; readable Unicode and intentional line breaks remain readable. This escaping is presentation-only and never changes stored bodies or machine-mode results. `json` follows [protocol](protocol.md) without changes to envelopes, details, or exit codes. `id` is valid only for create, message add, and fix; it prints generated UUID(s), one LF per value, no output for no-op. `none` is valid only for mutations and suppresses successful output. Errors in `id` and `none` retain `error CODE MESSAGE` followed by LF. Other combinations are exit 2 `usage`.

`sidecar --version` is an alias for `sidecar version`, including with a preceding
`--output json`; the JSON command name remains `version`.

## Help

`sidecar` with no arguments displays the overview and exits 0. Use `sidecar --help` (or `-h`), `sidecar thread --help`, `sidecar message --help`, or append `--help` / `-h` to a command for its syntax, flags, and examples. Explicit help writes plain text to stdout, exits 0, and never reads input files/stdin, locks, or mutates a sidecar. It is an informational mode, not a JSON result, even when combined with `--output json`. A help-looking token consumed as a flag value is not a help request. With `--output json` and no command, the normal missing-command usage error remains unchanged.

Mutation commands require optional `--if-revision REV|missing` and `--lock-timeout DURATION` immediately after their command words and before command flags. Default timeout is 5s; duration is Go duration syntax in inclusive 1ms–60s. `missing` matches only absent targets. Any Revision matches exact storage bytes under lock. Reads have no lock options.

```
version
validate --file PATH [--source PATH]
format [--if-revision REV|missing] [--lock-timeout DURATION] --file PATH [--check]
fix [--if-revision REV|missing] [--lock-timeout DURATION] --add-missing-message-ids --file PATH [--check]
thread list --file PATH
thread get --file PATH --id UUIDv7
thread create [revision options] --file PATH --location LOCATION --author-kind human|agent|external [--author-name NAME] --body-file PATH|- [--external-author-id ID] [--id UUIDv7 --message-id UUIDv7] [--gitlab-json PATH|-]
thread move [revision options] --file PATH --id UUIDv7 --location LOCATION
thread delete [revision options] --file PATH --id UUIDv7
message add [revision options] --file PATH --thread UUIDv7 --author-kind human|agent|external [--author-name NAME] --body-file PATH|- [--external-author-id ID] [--id UUIDv7] [--gitlab-json PATH|-] [--if-last-author-kind KIND]
message update [revision options] --file PATH --thread UUIDv7 --id UUIDv7 --body-file PATH|-
message delete [revision options] --file PATH --thread UUIDv7 --id UUIDv7
apply [revision options] --file PATH --operations-file PATH|-
```

Flags not displayed, repeated flags, positional operands, malformed flags, and incompatible author fields are usage errors. `--author-name` is allowed only human/agent; external requires both `--external-author-id` and `--author-name`. GitLab JSON is exactly `GitLabDTO`, token-validated like metadata. Across body, GitLab, and operations path inputs, at most one path may be `-`.

The exact mappings for repeated global output selectors, missing required flags, and incomplete apply requests are specified in [protocol](protocol.md#specific-input-diagnostics). Request validation precedes unsupported-version dispatch, including when a request has both an unsupported protocol and an invalid operation array.

## Input paths and body transport

Target naming errors and target file errors are validation exit 3. Body, metadata, operation request, flags, and stdin failures (including absent, unreadable, malformed, or oversized request-input paths) are usage exit 2; target/source filesystem security and durability failures retain their specified validation/io mapping. Body transport is exact. Read at most 131,074 raw bytes (the largest valid all-CRLF representation of 65,537 normalized bytes); reject a 131,075th byte before unbounded allocation. Reject a leading UTF-8 BOM, NUL, invalid UTF-8, bare CR, and mixed LF/CRLF endings. Replace every CRLF with LF, require the normalized transport to be at most 65,537 bytes, then remove **exactly one** final LF if present and perform body-domain validation (including its 65,536-byte limit). Thus `x\n\n` and `x\r\n\r\n` both produce logical `x\n`; `x` remains `x`; and an empty or whitespace-only logical result is invalid. This rule applies identically to `--body-file PATH` and `--body-file -`; `-` is raw transport bytes, not a line-oriented reader. JSON apply `body` is already logical: its decoded string is not BOM-transport checked or line-ending-normalized, must contain neither CR nor NUL, and never loses a trailing LF. Metadata input is limited to 16 KiB raw bytes. Apply input is limited to 1 MiB.

## Read commands

`version`, `validate`, list, and get use results in [protocol](protocol.md). `validate --source` applies the source rules in [source-and-location](source-and-location.md). Without `--source`, it validates sidecar structure/domain only. Missing target validates as empty. A zero-byte target succeeds with `noncanonical_empty`. Repairable invalid input returns valid false and exit 3; other invalid target states error exit 3. List returns no threads for missing/zero-byte; get on either has exit 5.

## Formatting and repair

`format` accepts exactly the formatter input set in [format](format.md), canonicalizes atomically, and returns `formatted:true` iff bytes were written or zero-byte was removed. `--check` never locks for write or writes; it returns `formatted:false` plus exit 3 if formatting would alter/remove bytes, otherwise exit 0. Invalid input is error exit 3.

`fix --add-missing-message-ids` repair-parses and only fixes one-or-more `missing_message_id` diagnostics with no other error. It locks, rereads, repeats parsing, allocates UUIDv7 values in physical document message order after validation, writes once, and returns ordered assignments. A retry seeing normal canonical content returns `changed:false,assignments:[]`. `--check` writes nothing: when repair is needed it exits 3 with `assignments:[]`, the existing non-null `storage_revision`, and `canonical_revision:null`; prospective assignments and IDs are omitted. When already normal it exits 0 with empty assignments and its normal canonical revision; any other invalidity is an exit-3 error. Fix does not invent any other value.

## Single mutations

Create either supplies both `--id` and `--message-id`, or neither; supplying exactly one is usage error. IDs are generated only after all request/source checks and locked reread checks. Message add similarly generates an omitted ID at that point. Explicit create is a no-op only if IDs, location, author, normalized body, and GitLab are all exact; otherwise collision/precondition exit 6. Move to equal location and update to equal body are no-op. Missing target entities are exit 5. Delete is not retry-idempotent after absence. Deleting a final message atomically deletes its thread; deleting the final thread unlinks the file. Any successful mutation on a zero-byte file removes it, even a semantic no-op.

Create and move validate the implicit derived source as specified in [source-and-location](source-and-location.md). `lost` is valid without a source. `--if-last-author-kind` on add checks the current working thread's last message before appending; absence/mismatch is precondition exit 6.

## Apply

Apply prevalidates its closed request before lock. Top-level request `if_revision`, if present, must equal CLI revision. It evaluates 1–256 operations sequentially against one working copy; later operations see earlier changes. Create then update/delete is valid; delete then explicit create with that ID is a new create. Each operation has only its typed semantic preconditions. Any operation-local failure reports its normal typed error details with mandatory zero-based `index`: missing entity is exit 5 `not_found {entity,id,index}`, while semantic failure is exit 6 `precondition {operation,reason,index}`. The index is never encoded into `id`. The batch changes no bytes and writes no partial result. Every resulting-document limit is checked after every operation. A batch succeeds with one atomic commit and ordered per-operation results.

## Error precedence

The closed code/detail/message symbols and full precedence are in [protocol](protocol.md). In order: usage request parsing; secure target/source classification and reads; lock setup; locked revision comparison; locked structural/repair state; source revalidation; indexed apply/single-operation semantics; precommit identity conflict; then commit/durability outcome. A symlink/nonregular target or source is validation; permission/read/open failure is io; oversized target/source is validation and oversized request input is usage. Revision mismatch wins over semantics. A detected target/parent/lock identity change before commit is `conflict:identity_changed`; lock timeout is `conflict:lock_timeout`; a revision mismatch is `conflict:revision`. A commit or parent-fsync uncertainty is io even if the mutation otherwise succeeded. This matrix is expanded into executable fixtures in [conformance](conformance.md).
