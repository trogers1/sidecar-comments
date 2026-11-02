# Format 2, attachment and privacy

## Canonical framing

Use the v1 byte-oriented thread/message framing and body-separator algorithm, with these exact changes:

```
canonical-document = "<!-- sidecar-format: 2 -->" LF thread *(LF thread)
thread = thread-start LF tracking-line LF message *(LF message) thread-end LF
tracking-line = "<!-- sidecar-tracking: " metadata-json " -->"
location = "file" / "line:" positive-int / range / "lost" / "unresolved"
range = "range:" positive-int ":" positive-int "-" positive-int ":" positive-int
```

Thread delimiters, roles, message metadata and body grammar are otherwise unchanged. There is no blank line after the document header or between the opening delimiter, tracking line and first role. There is exactly one blank LF between threads. A nonempty document has exactly one final LF. The header occurs once at byte zero. A header-only document is invalid; canonical empty is absence, never a persisted header. Every thread has exactly one dedicated tracking line, even when its metadata object is `{}`. A tracking line is recognized only at that position; source excerpts are JSON strings, never separately parsed framing. Strip the exact leading prefix and terminal ` -->` suffix, then token-parse the entire intervening JSON object; do not terminate at an earlier `-->` inside a string. JSON strings can safely contain delimiter-looking source text. Conversation body reserved prefixes remain the v1 set; a tracking-looking line in a message body is ordinary message content, not metadata.

`metadata-json` is exactly the schema `ThreadMetadata`: `{}` or `{"tracking":TrackingRecord}`. Never put tracking into a message or GitLab object. The opening location is the active attachment; `unresolved` has no coordinates. Persisted thread reconstruction is `StoredThread`, not public `ThreadDTO`.

Formatter/repair input allowances otherwise remain the v1 allowances applied to this grammar. They never permit absent/extra document headers or absent/malformed tracking metadata. Only missing human/agent message IDs are repairable. Message role headers may have trailing ASCII spaces or tabs in all read modes; this includes bare `---user` / `---bot` headers when checking for missing IDs. Canonical serialization removes this header whitespace. Trailing ASCII spaces, tabs and blank lines after the final thread closing delimiter are also accepted; canonical serialization removes this padding. This allowance does not waive the final newline requirement. Message body whitespace is never trimmed by either allowance. Normal readers otherwise accept canonical bytes and the existing zero-byte exception, not formatter variants.

## Closed tracking record

A line/range/unresolved thread MUST have exactly one record:

```
{version:1, source_revision:Revision, last_location:LineLocation|RangeLocation,
 selected_text:string, prefix:string, suffix:string,
 prefix_at_start:boolean, suffix_at_end:boolean}
```

All fields are required; unknown fields and nulls are forbidden. File/lost threads MUST omit `tracking`; their dedicated metadata line is `{}`. No range `anchor`, base64 anchor, encoded location suffix, legacy assertion, permanent original quotation, or extra history field exists.

`source_revision` hashes all normalized logical source bytes, including a final LF. `last_location` is the coordinate of the last accepted capture, not an active attachment when unresolved. An attached location MUST equal `last_location`, including kind. Range selected text is nonempty and respects half-open coordinate order. A line record contains exactly one whole logical line without its terminator; it may be empty. Selected text and contexts are valid logical UTF-8, contain neither CR nor NUL, and are not NFC-normalized. Context windows and edge flags obey [capture](tracking.md). The record is validated structurally without requiring current source access; historical selected bytes cannot be checked against changed source as if they were current assertions.

## Canonical encoding and order

Use v1 JSON token validation and compact raw UTF-8 encoding, without HTML escaping. Reject duplicate members at all depths, wrong token types, invalid UTF-8/escapes, surrogate escapes, unknown fields and non-decimal/fractional/exponent integer spellings. Emit JSON escapes only as specified by v1; retained LF is encoded as JSON `\n`, not a physical metadata newline.

Metadata key order is `tracking`. Record key order is exactly `version,source_revision,last_location,selected_text,prefix,suffix,prefix_at_start,suffix_at_end`. Location key order is `kind,line` or `kind,start_line,start_column,end_line,end_column`. Message metadata ordering is unchanged. Optional fields are omitted, never null. Record booleans are always emitted, including false.

Canonical thread order is `file,line,range,lost,unresolved`; compare line/range coordinates as in v1 but without anchor comparison; UUID bytes break remaining ties. Unresolved sorting uses ID, not stale coordinates. Preserve message insertion order. Storage revisions hash exact disk bytes, canonical revisions hash the full canonical serialization including header/tracking, and canonical empty hashes zero bytes.

## Lifecycle and privacy

Create/move to line/range captures current validated source. Explicit move refreshes tracking even at equal numeric coordinates. Exact explicit-ID create retry is detected by existing create equality before fresh capture and never recaptures or replaces evidence; supplied source guards still pass, and mismatch remains a precondition failure. Reply, message update, reads and deletion require neither source nor successful reconciliation.

Failed matching or an editor deletion report changes location to `{kind:"unresolved"}` and preserves the entire last accepted record. Later sync can recover it. Explicit move to file/lost purges tracking; lost is deliberate and never automatically matched. File remains file-scoped. Delete purges the conversation and its record; final-message deletion deletes its thread, final-thread deletion unlinks the sidecar. No resolved/reopen state or command exists: resolution means `thread delete`.

Tracking excerpts are potentially sensitive source content stored in the same portable sidecar. Default machine DTOs expose only `TrackingSummary` (record without selected_text/prefix/suffix). Only explicit thread get/list `--include-tracking` may expose full records. Mutation results, sync/editor outcomes, normal human output, diagnostics and logs MUST NOT dump retained excerpts; human presentation retains terminal-control escaping. Messages can contain user-authored quotations and are not tracking data; purge does not scrub conversation content.

No source file is ever rewritten. No full-source snapshots, `.comments.snap`, hidden cache, accumulating history or machine-local recovery store exists. Existing lock files are coordination files, not snapshots. Local deletion cannot erase Git history, backups, snapshots or already shared copies; secure erasure is not promised. Portability means source plus sidecar suffice, not automatic semantic Git merge; unresolved merge conflicts fail validation without destructive repair.
