# Capture and bounded reconciliation

## Source and capture

Coordinates retain v1 1-based UTF-8 byte/rune-boundary semantics and half-open ranges. Empty source has one logical line; line counts (including final-LF handling) come from the existing source validator. Accept consistent LF or CRLF and normalize CRLF to logical LF only in memory. Reject BOM, NUL, invalid UTF-8, bare/mixed CR, oversize and insecure source objects using existing typed failures. `file` create/move still validates source as in v1; `lost` never does. Discussion/deletion/lost-only operations never read source. Supplied source guards are explicit requests for source validation even on a retry, not permission to recapture.

Capture full range selection or full line excluding terminator, at most `MaxAnchorBytes` (4,096 decoded UTF-8 bytes). Retain up to 256 logical bytes immediately before and after, taking the longest window within that cap without splitting a rune. Each edge flag is true exactly when that window reaches its document edge. Empty context is not arbitrary evidence. A fingerprint covers all normalized bytes, not just excerpts, and cannot reconstruct old content.

Aggregate decoded selected_text/prefix/suffix bytes across records MUST fit `MaxApplyBytes` (1 MiB); serialized document MUST fit `MaxTargetBytes` (4 MiB). The existing `MaxMetadataBytes` limit remains for message metadata. Dedicated tracking metadata uses a separately derived encoded-byte bound covering worst-case JSON escaping of every selection/context within the decoded budgets plus bounded structural overhead. It must not reject an otherwise valid record merely because escaping exceeds the message-metadata limit; never truncate. Existing source/thread/message limits still apply. Limits are owned by `internal/domain/types.go`; implementation/tests derive them there, not from independent copies in this prose. The new context cap is 256 bytes, the changed-match context minimum is 32 bytes and the sync work multiplier is 16.

Oversize create/move/editor capture fails atomically; never truncate selections, evict records or silently drop tracking. Offline oversized replacement becomes unresolved/selection_too_large with evidence preserved. Aggregate/document overflow aborts the whole transaction, including otherwise accepted matches.

## Offline algorithm

Process threads in deterministic UUID byte order, preserving IDs/messages. File/lost are skipped without source access. If there are any attached/unresolved threads, validate current source and records before searching. An unchanged record fingerprint and valid active attachment is unchanged/source_unchanged without refreshing bytes. Unresolved is not automatically accepted merely because its fingerprint equals current source: it must acquire sufficient matching evidence.

Find exact selected-text occurrences. Require exactly one supported candidate with at least one full retained nonempty context window or a matching retained document-edge boundary. Never use coordinate proximity. Empty line candidates require evidence beyond empty selection. Line mappings remain exactly one whole line; never convert a line to a range.

Bounded changed-selection matching requires both full retained context windows, each at least 32 bytes and containing some non-whitespace, immediately outside a unique nonempty replacement. Edge-only evidence is insufficient. Replacement must fit the selection limit; a line replacement must be one whole logical line. Ordinarily this applies only if no exact selected-text occurrence remains.

The deliberate boundary-expansion exception prefers a uniquely supported full two-sided envelope `prefix + NEW + selection + suffix` (or insertion at the end) over the narrower exact occurrence. It includes the supported insertion and reports modified/boundary_expansion/inferred. This is inference, intentionally different from exact editor gravity.

Repeated context, competing mappings, deletion/empty replacement, insufficient evidence, unsupported transformations and exhausted search remain unresolved with the old record unchanged. No fuzzy scores, nearest-match selection, arbitrary text between common phrases, full-baseline diff or reconstruction of arbitrary edit history is promised. Refresh selected text, context, coordinates and fingerprint together only for accepted mappings. Repeated sync of unchanged state, including unresolved state, is a no-op; later sufficient unique evidence may recover an unresolved thread.

## Deterministic work limits

At most `MaxApplyOperations` candidate occurrences per search (256), `MaxSourceBytes` inspected/compared bytes per thread (16 MiB), and `16 * MaxSourceBytes` per sync (256 MiB). Count matching passes and evidence/context comparisons, including unchanged-attachment verification, search preprocessing, and search-owned UTF-8 boundary/whitespace inspections—not wall time or conservative reserved allowances. Independent source parsing/indexing, structural/aggregate validation, and capture performed only after a mapping is accepted are separately bounded operations, not search work. They must not establish evidence or uniqueness or provide an unmetered search fallback. Never accept before checking remaining search space for ambiguity. Exhaustion is unresolved/search_limit, including later eligible threads without remaining global budget. Shared scans cannot weaken uniqueness or these caps.

## Outcomes, reasons and evidence

Every outcome has exactly `{thread_id,outcome,reason,evidence,location}`; no excerpt, tracking record or candidate listing is returned. Location is the accepted/prospective location, or unresolved on failure. The following are the complete permitted combinations:

| outcome | reason | evidence |
|---|---|---|
| skipped | file, lost | none |
| unchanged | source_unchanged | unchanged |
| unchanged, moved | exact_match | exact |
| modified | context_replacement, boundary_expansion | inferred |
| unchanged, moved, modified | editor_location | editor |
| unresolved | editor_deleted | editor |
| unresolved | no_match, ambiguous, insufficient_context, unsupported_transformation, selection_too_large, search_limit | none |

`modified` means accepted selected bytes changed, even if coordinates also moved; `moved` means coordinates changed with selected bytes unchanged; otherwise an accepted mapping is `unchanged` (even if context/fingerprint refresh makes the transaction changed). For exact-match recovery from unresolved, use moved because there was no active attachment. Editor mapping from file/lost/unresolved likewise uses moved unless a retained selection changed, then modified. Skipped reason corresponds to its location kind. Editor reasons never occur in offline sync.

Reason precedence when multiple failure descriptions apply: search_limit if uniqueness could not be established within budget; ambiguous if multiple supported envelopes remain; selection_too_large for the uniquely supported oversized replacement; unsupported_transformation for a uniquely supported mapping that cannot preserve location kind; insufficient_context when potential occurrences/envelopes lack required evidence; otherwise no_match (including deletion/empty replacement). No reason may be chosen to bypass the search required for uniqueness.

## Sync transaction and check

Write sync persists accepted mappings and unresolved states in one locked atomic transaction. Unmatched threads do not abort successfully evaluated matches, but any malformed input, source failure, storage overflow or transaction failure aborts the entire write. This is partial matching, never partial commit. Normal successful sync exits 0 even with unresolved threads.

`sync --check` does not take a write lock, mutate bytes, allocate IDs or persist refreshed evidence. It returns prospective outcomes with `changed:false`; `would_change` is whether the write variant would alter/remove persisted state. Exit 3 if would_change or any thread is unresolved, otherwise 0. A stable unresolved record can therefore yield would_change false and exit 3. Revisions in check results describe observed storage/canonical bytes, not hypothetical output. On write, `would_change` equals actual `changed`, including zero-byte cleanup. Source revision is present exactly when matching required source validation, omitted for only file/lost/no threads. Ordinary sync with no tracked threads still preserves existing zero-byte cleanup behavior. Successful check does not imply a race-free snapshot of source and sidecar.
