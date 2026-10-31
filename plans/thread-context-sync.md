# Portable thread tracking and context-based sync

Status: implementation scope and behavior confirmed by the user. This document specifies intended behavior; it does not establish implemented runtime guarantees. Update the plan first, before runtime implementation.

## Goal and scope

Keep comments attached to UTF-8 prose, code, and other text as it changes. Prefer exact editor knowledge when available; provide bounded, conservative offline reconciliation otherwise. Preserve conversations and prior attachment evidence when reconciliation is uncertain.

Implement the standalone CLI feature, public contracts, schemas, generated artifacts, executable corpus, tests, help, documentation, and independent reviews. Prepare CLI **0.2.0**, format **2**, and protocol **2**. Do not publish as part of this effort.

Explicit exclusions:

- No resolved conversation state, `thread resolve` alias, or reopen command. **Resolution means deletion**, exposed only through `thread delete`.
- No v1 compatibility or migration tooling. Normal commands reject v1 files; do not infer tracking from their coordinates.
- No legacy Neovim format migration, reusable adapter implementation, or config-repository modification.
- No full-source snapshots, `.comments.snap` directory, hidden cache, or accumulating context history.
- No implicit Git searches, staging, commits, pushes, tags, releases, or Homebrew writes. Those require separate authorization.

## Verified baseline

Relevant existing contracts: [CLI](../docs/v1/cli.md), [protocol](../docs/v1/protocol.md), [format](../docs/v1/format.md), [source/location](../docs/v1/source-and-location.md), [storage](../docs/v1/storage-security.md), and [releases](../docs/releases.md). These describe v1, not the new feature. Publish explicit v2 contracts rather than silently extending closed v1 shapes.

- `internal/domain/types.go` owns existing DTOs and limits. Import or derive definitions in implementation and tests; do not repeat them.
- Existing limits include 4,096-byte anchors, 4 MiB targets, 16 MiB sources, 1,024 threads, and 1 MiB/256-operation apply requests.
- Existing anchors assert exact current selected bytes; they are not historical quotations or a relocation algorithm.
- Existing move/apply support explicit relocation, not context capture or discovery.
- Current lost locations cannot carry an anchor; current threads have neither tracking metadata nor resolution state.
- Sidecar writes already use a locked atomic transaction. Keep tracking in that same file.
- v1 has no document version header and rejects unknown shapes. A distinct v2 header must make old writers reject new data rather than discard tracking.
- The existing PR workflow verifies native macOS ARM64 and Linux AMD64; main pushes can publish automatically. Do not push main.

## Ownership and portable storage

The CLI owns parsing, serialization, source validation, capture, matching, lifecycle validation, and atomic writes. Editors use the public protocol, not parallel parsers or writers.

Retain readable conversation bodies and thread delimiters. A nonempty v2 sidecar has the explicit header `<!-- sidecar-format: 2 -->` and dedicated closed per-thread JSON metadata for attachment/tracking. Encode retained excerpts as JSON strings so source content cannot break framing. Do not hide tracking in message or GitLab metadata.

Store one current versioned tracking record per tracked thread: selected text (whole-line text for line comments), prefix/suffix context, document-edge markers, prior coordinates, and SHA-256 fingerprint of normalized logical source. A fingerprint detects change; it cannot reconstruct a previous document. Keep active attachment separate from stale last-known coordinates.

Preserve existing missing/zero-byte handling and final-thread removal semantics. No header-only persistent empty document is needed. v1 input must fail closed, with no automatic conversion or best-effort rewrite. Preserve unrelated v1 behavior in v2 unless explicitly changed here; update the public schemas and corpus to the new version rather than claiming v1 conformance.

### Capture and budgets

Use existing logical-source validation and CRLF-to-LF normalization; never normalize, rewrite, or save the source file itself.

- Retain the complete selection or whole-line text, at most `MaxAnchorBytes` (currently 4,096 bytes).
- Capture up to **256 logical UTF-8 bytes before and 256 after**, cutting only at rune boundaries.
- Record whether each window reaches the document edge. Empty context is not arbitrary matching evidence.
- Cap aggregate decoded selected/prefix/suffix bytes at `MaxApplyBytes` (currently 1 MiB), and retain the existing `MaxTargetBytes` serialized-document cap (currently 4 MiB).
- Reject oversized create/move/editor capture atomically; never silently truncate selected text, evict another record, or downgrade to manual-only tracking.
- If offline matching would produce an oversized selection, leave that thread unresolved and retain its evidence. Aggregate storage overflow aborts the entire write.
- Derive limits centrally; do not copy constants into tests or independent implementations. Dedicated tracking metadata has its own encoded-byte bound, derived from worst-case JSON escaping of the approved decoded selection/context budgets plus bounded structural overhead. Do not reuse the 16 KiB message-metadata cap to reject otherwise valid tracking records; message-metadata limits remain unchanged.

### Location and attachment lifecycle

- File comments remain file-scoped and have no tracking excerpts.
- Line comments track the exact whole logical line with surrounding context. Empty lines require contextual evidence; unsupported splits/joins remain unresolved. Never silently convert a line comment to a range.
- Range comments track their full nonempty half-open UTF-8 byte selection. V2 removes the legacy range-anchor field and encoded anchor syntax; the tracking record is the sole automatically retained selected text. Retain the existing centrally defined selection-size bound rather than duplicating it.
- Whole-line selected text excludes line terminators. Fingerprints cover every normalized logical source byte, including a final LF; logical line/count semantics remain those of the existing source validator (including one line for empty source).
- Deliberately lost comments are never automatic matching candidates.
- Failed reconciliation removes the active attachment and records unresolved state, retaining the previous tracking evidence separately. Later sync may recover it when evidence becomes sufficient and unique.
- Explicit move to a current line/range validates and refreshes tracking, even when numeric coordinates equal the previous ones.
- An exact explicit-ID create retry remains a no-op and never recaptures tracking. Detect a matching existing create before fresh capture; preserve its evidence. Supplied source guards must still pass, and mismatched create inputs still fail. Refresh requires explicit move/editor update.
- Explicit move to file/lost purges tracking and stops automatic recovery.
- Reply, message edit, reads, and deletion do not require source access or successful reconciliation.
- Delete removes the conversation and its tracking. Deleting a final message deletes its thread; deleting the final thread removes the sidecar.
- No permanent original quotation is retained separately. User-authored message quotations remain conversation content and are not purged as tracking data.

Local removal cannot erase Git history, backups, filesystem snapshots, or previously shared copies. Document this without claiming secure erasure.

## Public CLI and protocol

Global output selectors continue to precede command words. Define closed protocol-v2 request/result DTOs and precise error symbols before feature tests depend on them.

```sh
sidecar thread create --file chapter.md.comments --location range:3:1-3:24 \
  --author-kind human --body-file note.txt
sidecar --output json source inspect --file chapter.md.comments
sidecar --output json sync --file chapter.md.comments --check
sidecar sync --file chapter.md.comments
sidecar thread move --file chapter.md.comments --id THREAD_ID --location range:8:1-8:20
sidecar --output json editor-update --file chapter.md.comments --operations-file updates.json
sidecar --output json thread get --file chapter.md.comments --id THREAD_ID --include-tracking
```

### Source inspection and preconditions

`source inspect --file PATH` derives the source using existing suffix rules, validates only the source, and returns its logical-source SHA-256 fingerprint and logical line/byte counts. It returns no excerpts and works without a populated sidecar or valid current attachments. Keep the existing `validate --source PATH` attachment-validation behavior; source inspection is a separate entry point.

Add optional `--if-source-revision SHA256` to ordinary create/move and an equivalent top-level apply guard for source-dependent operations. Guards refer to normalized logical source, not raw disk bytes. When supplied, check them both before locking and during locked source revalidation; mismatch is a typed conflict. Machine clients supplying previously computed coordinates must use both source and sidecar guards. Interactive ordinary commands may omit them and explicitly attach to validated current disk text. Discussion/delete and lost-only operations do not read source.

Editor updates always require both source and sidecar revision guards and operate only against the current **saved source**. Do not accept unsaved text payloads or write the source. Revalidate source under the sidecar lock immediately before committing. The source is not locked by that transaction: a change after final validation remains outside the guarantee.

### Editor batch

`editor-update --file PATH --operations-file PATH|-` takes a closed request with `protocol:2`, mandatory `if_revision`, mandatory `source_revision`, and an updates collection. Each item is exactly one of:

- `{thread_id,location}` for a valid current line/range;
- `{thread_id,deleted:true}` when the editor knows the tracked selection was deleted.

Bound requests by `MaxApplyBytes` and 1–`MaxApplyOperations` updates. Thread IDs must be unique and exist. Invalid mappings, duplicate IDs, missing threads, stale guards, and capture/storage limits fail the entire batch. Never split the batch into independent commits. Supplied locations refresh CLI-captured context; clients never supply trusted context records.

Deleted-selection reports preserve messages/IDs and prior tracking as unresolved, not deliberately lost. Exact editor coordinates exclude insertions at either boundary, following the existing extmark gravity policy (`right_gravity=true`, `end_right_gravity=false`). Test the CLI protocol with realistic supplied coordinates; do not claim that simulated coordinates prove Neovim behavior.

### Reads, privacy, and sync output

Only explicit `--include-tracking` on thread get/list exposes full retained tracking excerpts. Ordinary JSON thread DTOs expose state/fingerprints without excerpts. Normal human output, sync outcomes, diagnostics, and logs must not dump retained context. Preserve terminal-control escaping for human presentation.

Sync reports per-thread outcome, reason, and evidence, plus revisions and overall `changed`. Distinguish unchanged, moved, modified, unresolved, and skipped; distinguish editor-tracked evidence from inference.

- Normal sync exits **0** after a successful transaction even if some threads remain unresolved.
- `sync --check` computes prospective reconciliation without replacing persisted tracking and performs no sidecar mutation, persisted context refresh, ID allocation, or write locking. It reports `changed:false` and separate `would_change`; exit **3** when writing would change state or any thread is unresolved, otherwise **0**.
- Unresolved results use structured success results, not malformed-input/error envelopes.
- Malformed data, IO failures, and revision conflicts retain typed failures and the existing exit taxonomy where applicable.
- Write sync persists accepted mappings and explicit unresolved state in one atomic transaction. This partial matching policy is not a partially committed transaction.
- Repeated sync against unchanged source/state is a no-op, including stable unresolved evidence.

## Offline reconciliation

1. Validate source and retained record before matching. With an unchanged fingerprint, preserve a valid attachment without unnecessary rewriting.
2. Find exact selected-text candidates. Require one uniquely supported result with at least one full retained nonempty context window or matching document-edge boundary. Empty lines require contextual evidence beyond their empty selection. Never choose by coordinate proximity.
3. Support bounded changed-selection matching with **both full retained context windows**, each containing at least **32 bytes and some non-whitespace**, immediately outside one unique nonempty replacement. Document-edge-only evidence is insufficient. Replacement text must fit `MaxAnchorBytes`; line replacements must be exactly one whole line.
4. Ordinarily changed-selection matching applies when no exact selected-text occurrence remains. The approved boundary-expansion exception is deliberate: if full two-sided context uniquely supports `prefix + NEW + selection + suffix`, select `NEW + selection`; likewise include supported insertions at the end. Prefer that uniquely supported expanded envelope over its narrower exact-text candidate. Report **modified/inferred**, not editor certainty. This differs intentionally from editor boundary gravity.
5. Repeated context, competing supported mappings, deletion, empty replacements, insufficient evidence, unsupported transformations, and search exhaustion remain unresolved. Do not use fuzzy similarity scores or attach arbitrary text between common phrases.
6. Refresh selected text, context, coordinates, and fingerprint together only on accepted mappings. Preserve the previous record on unresolved outcomes.

Context cannot reconstruct arbitrary edit history. Large rewrites, repeated passages, simultaneous context changes, and line transformations can legitimately remain unresolved. No full-baseline diff algorithm is promised.

### Deterministic search bounds

- At most `MaxApplyOperations` candidate occurrences per search (currently 256).
- At most `MaxSourceBytes` inspected/compared bytes per thread (currently 16 MiB).
- At most **16 × `MaxSourceBytes`** across a sync (currently 256 MiB).
- Count matching passes and context comparisons, not elapsed time. Process threads in deterministic thread-ID order.
- Never accept a match without checking the remaining search space for ambiguity. Exhaustion produces unresolved/search_limit, retaining evidence; later threads unable to search receive the same reason.
- Shared scanning/optimization must not weaken the documented caps or uniqueness requirements. Test repetitive text and precise limit boundaries.

## Test-first acceptance matrix

Exercise freshly built public CLI entry points with temporary files, schema-validated requests/results, real storage wiring, and preserved IDs/messages. Use prose and code fixtures. Derive production limits; never derive expected locations by invoking the matcher under test.

| ID | Journey | Required observation |
| --- | --- | --- |
| CTX-01 | Create/read/reply/sync unchanged range | Context captured; conversation intact; sync no-op |
| CTX-02 | Insert/delete lines above selection | Supported attachment moves; selection unchanged |
| CTX-03 | Edit earlier bytes on same line | Correct UTF-8 byte columns |
| CTX-04 | Edit strictly inside selection | Exact editor mapping; supported offline modified/inferred result; unsupported case unresolved |
| CTX-05 | Combine edits above/inside | Correct mapping; IDs/messages preserved |
| CTX-06 | Insert at either boundary | Editor excludes insertion; offline includes unique supported two-sided expansion; insufficient/ambiguous case unresolved |
| CTX-07 | Multiline edits, splits/joins, whole-line/empty-line comments | Valid half-open ranges; explicit whole-line behavior; unsupported line transformations unresolved |
| CTX-08 | Move unchanged passage | Unique supported match; no nearest-match heuristic |
| CTX-09 | Duplicate selection/context | Only sufficient unique evidence disambiguates |
| CTX-10 | Delete selection or replace selection/context | Conversation/evidence retained unresolved; no unrelated attachment |
| CTX-11 | Explicitly reattach unresolved thread | Tracking refreshed, including equal-coordinate move; next sync no-op |
| CTX-12 | Unicode/emoji/combining characters, LF/CRLF, edges | Valid logical capture and byte coordinates; source bytes unchanged |
| CTX-13 | External reply before editor save | Sidecar conflict; explicit reread/retry preserves reply |
| CTX-14 | Source changes after coordinate capture | Source conflict without mutation; test pre-lock/locked checks |
| CTX-15 | Mix attached/unresolved/file/lost threads | Deterministic outcomes; one atomic write; no lost auto-attachment |
| CTX-16 | Move tracked thread to file/lost | Automatic excerpts purged; conversation retained; no automatic recovery |
| CTX-17 | Delete one thread among several | Only its conversation/tracking removed; others semantically preserved |
| CTX-18 | Delete last message/thread | Sidecar removed; no tracking/cache/snapshot remains |
| CTX-19 | Fresh source/sidecar-only copy | No machine-local cache or Git dependency |
| CTX-20 | Missing/malformed/oversized/symlinked/unreadable source; corrupt tracking | Typed failures; no partial writes; source-independent discussion still works |
| CTX-21 | Capture/storage/batch/search limits | Precise bounds, atomic rejection or documented unresolved result; no silent truncation |
| CTX-22 | Storage conflict/commit failure/output failure | Existing revision/durability/delivery semantics preserved |
| CTX-23 | v1 input and old binary against v2 | Fail closed; no migration, inferred capture, or destructive rewrite |
| CTX-24 | Help/text/JSON/quiet/check/explicit tracking reads | Approved exits/results; controlled excerpt exposure; terminal safety |

Also cover source inspection without sidecars/stale attachments, optional ordinary source guards, mandatory editor guards, unique batch IDs, unsupported protocol/format handling, stable repeated unresolved results, and recovery when retained evidence becomes uniquely matchable again.

Privacy tests inspect sidecar bytes and directory contents, not just DTOs. Use uniquely attributable retained text: shared wording and explicit message quotations are not tracking leaks. Add focused unit/property tests only for complex validation/matching edges not economically covered through public lifecycle tests.

## Implementation sequence

1. **Update this plan first.** Record approved decisions and replace the former resolve/reopen and v1 compatibility scenarios.
2. **Publish concrete v2 contracts.** Specify closed schemas, framing, attachment/tracking unions, canonical encoding, request/result shapes, errors, and version negotiation. Keep source/sidecar guarantees truthful.
3. **Inventory tests and add honest red coverage.** Map existing coverage to CTX journeys. Use an explicitly named feature-red target until the feature exists; no skips/fake implementations or claims that old gates prove new behavior.
4. **Implement versioned domain/persistence.** Add validated values, aggregate bounds, canonical serialization, fail-closed version handling, round trips, and privacy-purge coverage.
5. **Capture and exact updates.** Implement source inspection, create/move/apply capture, guards, explicit tracking reads, and atomic editor updates before offline inference.
6. **Implement bounded offline sync.** Add exact/context matching, approved boundary expansion, unresolved/recovery state, deterministic work accounting, check/no-op semantics, and property coverage.
7. **Integrate cleanup/public behavior.** Exercise every deletion and tracking-purge path, help/rendering, corpus, README, compatibility/privacy docs, and CLI 0.2.0 metadata. Do not purge user-authored quotations as tracking.
8. **Integrate gates and review.** Fold implemented feature tests into `make cli-gate`; remove the temporary feature-red target when no longer needed. Read the review-policy skill and run two independent read-only reviewers on a frozen diff; reconcile both and verify fixes.
9. **Verify both native platforms.** Run local macOS and supplemental Docker checks; require successful native macOS ARM64 and Linux AMD64 CI gates before declaring completion. Publication and any Git writes remain separately authorized.

## Verification and completion

Run generated artifact freshness, formatting, vet, isolated module tidy, lifecycle tests, independent executable corpus, race suites, and bounded fuzzing through the maintained CLI gate. Extend fuzz/property coverage to tracking DTOs, capture, ambiguity, Unicode, and resource bounds.

The verified local host is Darwin ARM64. Docker is available through a Linux ARM64 engine; AMD64 containers here are emulated and **do not count as native Linux AMD64 evidence**. Use them for supplemental testing. CI must confirm both supported native platforms; a local passing gate alone is insufficient.

Confirm that source bytes are never written and no unadvertised cache, full-source baseline, or history files exist. Existing documented lock files are not snapshots. Confirm deletion/file/lost purge behavior through raw storage assertions. Unresolved Git merge conflicts fail validation without destructive repair; portability is not automatic semantic merging.

Completion requires the approved acceptance matrix, updated v2 compatibility/privacy/public documentation, both native gates, and two independent reconciled reviews. No published release, Homebrew installation, or actual Neovim integration is claimed by this scope.
