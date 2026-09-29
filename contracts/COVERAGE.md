# v1 contract-harness behavior inventory

`contracts/behavior-inventory-v1.json` is the machine-readable **seed
inventory**, not an exhaustive or authoritative coverage claim. The shadow
`contracts/packs/obligations-v2.json` reconciles all 152 v1 required-red entries
and 72 registered pack scenarios, records 25 additional pending coverage cases,
and marks four impossible semantic-precondition entries `not_applicable` after
review. All remaining 245 entries are pending; these are traceability records,
not executable proofs or an exhaustive requirement-to-test matrix. The runner
generates the fixture-to-behavior coverage mapping from that seed inventory and
the corpus, then rejects orphaned, duplicate, or mismatched mappings. Its
`required_red` entries are mandatory behavior packs without fixtures; they are
explicit unavailable obligations, not deferred labels. `make contracts-check`
validates the seed, documents, corpus, mapping, and goldens. `make
full-red`/`make release` remains nonzero while any required-red entry exists.

The table below is a human-readable view of seeded direct fixtures only. It
makes no claim that adjacent, unlisted, or mandatory required-red behavior is
covered. The repository is pre-implementation, so executing seeded fixtures
still requires a claimed executable via `SIDECAR_BIN`.

## Seeded behavior IDs

Each ID below names one concrete golden fixture. These IDs are stable corpus
vocabulary and direct-fixture seeds, not a complete requirements inventory.

| Behavior ID | Concrete fixture | Direct harness proof |
|---|---|---|
| `BHV-PRO-VERSION-JSON-ENVELOPE` | `version-json` | JSON success stream and schema validation |
| `BHV-CLI-VERSION-TEXT` | `version-text` | text stream byte comparison |
| `BHV-READ-MISSING-VALIDATE` | `validate-missing` | missing target result |
| `BHV-READ-MISSING-LIST` | `list-missing` | empty list DTO |
| `BHV-READ-MISSING-GET` | `get-missing` | missing entity error |
| `BHV-EMPTY-ZERO-VALIDATE` | `zero-validate` | zero-byte warning |
| `BHV-EMPTY-ZERO-FORMAT-CHECK` | `zero-format-check` | check state without lock or write |
| `BHV-EMPTY-ZERO-FORMAT` | `zero-format` | cleanup and persistent lock |
| `BHV-TARGET-NO-SUFFIX` | `target-no-suffix` | target-name rejection |
| `BHV-TARGET-DOT-NAME` | `target-dot` | `.comments` rejection |
| `BHV-TARGET-TEMP-SUFFIX` | `target-temp` | nonterminal suffix rejection |
| `BHV-FORMAT-CANONICAL-VALIDATE` | `canonical-validate` | canonical document acceptance |
| `BHV-FORMAT-WHITESPACE` | `whitespace` | whitespace-only rejection |
| `BHV-FORMAT-BOM` | `bom` | BOM rejection |
| `BHV-FORMAT-RESERVED-PREFIX` | `reserved` | reserved body prefix rejection |
| `BHV-FORMAT-THREAD-END` | `bad-end` | mismatched end rejection |
| `BHV-META-DUPLICATE-KEY` | `meta-duplicate` | duplicate metadata rejection |
| `BHV-META-UNKNOWN-KEY` | `meta-unknown` | closed metadata rejection |
| `BHV-META-SURROGATE-ESCAPE` | `meta-surrogate` | surrogate escape rejection |
| `BHV-FORMAT-CRLF-CANONICALIZE` | `format-crlf` | writer final bytes, mode, and lock |
| `BHV-REPAIR-VALIDATE` | `repair-validate` | repair result DTO |
| `BHV-REPAIR-FIX-CHECK` | `repair-fix-check` | check result without lock or write |
| `BHV-FMT-REPAIR-FIX-WRITE-GENERATED-ID` | `repair-fix-write-generated-id` | generated repair message ID bound to the written canonical metadata |
| `BHV-SOURCE-ANCHOR` | `source-anchor` | source range/anchor acceptance |
| `BHV-SOURCE-BOM` | `source-bom` | source BOM rejection |
| `BHV-SOURCE-BARE-CR` | `source-cr` | source CR rejection |
| `BHV-SOURCE-ANCHOR-MISMATCH` | `source-anchor-bad` | anchor mismatch rejection |
| `BHV-THREAD-CREATE` | `thread-create` | create final state and lock |
| `BHV-CLI-GENERATED-OUTPUT-ID-THREAD-CREATE` | `generated-output-id-thread-create` | generated thread/message `id` stream IDs bound to canonical final sidecar bytes |
| `BHV-CLI-GENERATED-OUTPUT-ID-MESSAGE-ADD` | `generated-output-id-message-add` | generated message result ID bound to final sidecar bytes |
| `BHV-THREAD-MOVE` | `thread-move` | move final state and lock |
| `BHV-MESSAGE-ADD` | `message-add` | add final state and lock |
| `BHV-MESSAGE-UPDATE-NOOP` | `message-update-noop` | locked no-op final state |
| `BHV-MESSAGE-DELETE-LAST` | `message-delete-last` | unlink and persistent lock |
| `BHV-DOMAIN-ID-PAIR` | `id-pair` | separate ID DTO inputs |
| `BHV-DOMAIN-EXTERNAL-AUTHOR` | `external-required` | external-author requirement |
| `BHV-APPLY-CREATE` | `apply-create` | sequential apply final state and lock |
| `BHV-APPLY-PROTOCOL-VERSION` | `apply-protocol` | request protocol rejection |
| `BHV-APPLY-UNKNOWN-OP` | `apply-unknown` | closed operation rejection |
| `BHV-APPLY-EMPTY` | `apply-empty` | nonempty operations requirement |
| `BHV-WRITER-REVISION-CONFLICT` | `revision-conflict` | typed revision-conflict stderr template without write |
| `BHV-STORE-SYMLINK` | `target-symlink` | symlink fixture materialization |
| `BHV-STORE-DIRECTORY` | `target-directory` | directory fixture materialization |
| `BHV-STORE-HARDLINK` | `target-hardlink` | hardlink identity assertion |
| `BHV-CLI-P3-COMMAND-MISSING` | `p3-command-missing` | missing-command usage error |
| `BHV-CLI-P3-COMMAND-UNKNOWN` | `p3-command-unknown` | unknown-command usage error |
| `BHV-CLI-P3-GLOBAL-ORDER` | `p3-output-after-command` | global output ordering usage error |
| `BHV-CLI-P3-FORBID-NONE-READ` | `p3-output-none-read` | forbidden read/none mode error |
| `BHV-CLI-P3-STDIN-ARBITRATION` | `p3-stdin-arbitration` | body/metadata stdin arbitration |
| `BHV-PRO-P3-APPLY-DUPLICATE` | `p3-apply-stdin-duplicate` | duplicate request token rejection |
| `BHV-PRO-P3-APPLY-SURROGATE` | `p3-apply-stdin-surrogate` | surrogate escape rejection |
| `BHV-PRO-P3-APPLY-UNKNOWN-FIELD` | `p3-apply-stdin-unknown-field` | closed apply DTO rejection |

The repair write golden uses the approved `UUIDV7:bind`/`UUIDV7:ref` template pair: the JSON assignment binds the runtime UUIDv7 and the final sidecar metadata references that same binding. This is direct proof of generated-ID persistence, not a fixture label.

P3 adds declarative argv-error fixtures. `invocation_error:"argv"` is required
only when the runner cannot parse the fixture argv; it demands exit 2, empty
stdout, and one LF-terminated text error. It prevents parser-order failures
from being hidden as invalid fixture artifacts. The runner also accepts the
specified empty successful `id` stream for explicit-ID/no-op operations.

## Required red obligations

The central `contracts/packs/required-red-v1.json` ledger additionally registers all P6, P8–P10, and P11–P12 scenarios one-by-one. Each entry points to its native manifest row, retains its exact golden when one exists, and names the typed harness capability required to make it runnable. The artifact gate rejects any pack golden not represented there, and `full-red` emits every entry as a nonzero `REQUIRED_RED` result. Therefore a pack fixture is not silently counted as coverage merely because it exists outside `corpus/v1.json`.

The inventory explicitly records the unavailable P3 branches as individual
`BHV-RED-P3-*` obligations. The P4 parser/output fixtures are direct only for
their exact declared argv/error branch; adjacent successful mutation, generated
ID, filesystem, and result branches remain required-red obligations unless a
fixture supplies all required inputs and asserts the full resulting stream and
filesystem state. `generated-output-id-thread-create` is such a direct fixture
for generated create IDs, `generated-output-id-message-add` does so for message
add, and `repair-fix-write-generated-id` does so for the single-message repair
write path. They do not discharge other branches. Required-red obligations cover the remaining unproven output-mode
matrix, parser/global-option cases, command flags, stdin arbitration, public
error and channel contracts, and each JSON token failure for every public apply
operation. The granular P4/P7 entries separately retain each unproven format
terminal, metadata form, framing and formatter variant, repair restriction and
retry/order rule, and populated/missing/zero/whitespace read matrix; they are
not coverage labels and may be removed only when a direct fixture is added. The existing P3 fixtures remain direct proof only for the branches
they name; they do not discharge adjacent P3 work. An obligation may move to
`behaviors` only with a direct executable golden fixture referenced from the
corpus.

P5 splits the former broad bounded-read/domain label into direct, independently
clearable obligations for UUID syntax/namespaces/generation, each author and
GitLab union branch, frozen Unicode behavior, and every documented limit. In
particular, every `*-PLUS-ONE-BOUNDED-*` entry requires a fixture containing
that exact byte count and asserting the public exit, stream, and unchanged
filesystem; a memory-allocation probe or a capability label does not clear it.
Clock and CSPRNG fault entries stay red until the runner exposes a typed,
controlled fault capability. The P5 inventory also keeps separate exact
monotonic-clock and collision-retry obligations rather than treating generated
UUID output as proof of either property.

The pre-existing required-red packs for error precedence, secure-store
races/durability, parser/domain properties and fuzzing, and Neovim fail closure
remain. `full-red` and `release` therefore stay non-green; this
pre-implementation repository does not claim P5 completion.

## Harness-only checks

`contracts/runner` directly proves duplicate-key detection, schema
meta-validation, invalid-schema rejection, broken-external-reference rejection,
strict compact one-record JSON streams, expected and observed JSON protocol
validation, exact stream comparison, and filesystem assertion. It also builds a
controlled test-only subject for the version fixture and rejects a deliberately
nonconformant subject.

Capabilities are typed runner checks, not labels. If a required capability is
unknown or unavailable, the fixture is emitted as `UNAVAILABLE`, the run is
nonzero, and it is not counted in any proof total. `fault_injection` is
explicitly unavailable until the harness has a controlled fault injector.
