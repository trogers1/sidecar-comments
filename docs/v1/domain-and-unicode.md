# Domain, limits, and Unicode

## Immutable domain

Trust-boundary input decodes into concrete DTO structs, is token/schema validated, then enters constructors which return immutable values. `ThreadID` and `MessageID` are distinct named types and namespaces. Constructors reject cross-use. Thread IDs are document-unique within the thread namespace and message IDs are document-unique within the message namespace. The namespaces are distinct: the same UUID text MAY occur once as a thread ID and once as a message ID, and that cross-type equality is not a collision.

An ID is exactly a non-nil lowercase hyphenated RFC 9562 UUIDv7: `^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`. Thread text delimiters add `th_`; metadata IDs never do. UUID parsing additionally rejects the all-zero UUID. Generation uses CSPRNG randomness and a clock. It MUST return an error, never a substitute, on clock or randomness failure: clock acquisition failure is exit 7 `io:clock` with `{operation:"clock",path:"generator"}`, and CSPRNG read failure is exit 7 `io:csprng` with `{operation:"csprng",path:"generator"}`. Within one process generated UUIDv7 values MUST be strictly increasing in UUID byte order even when the clock repeats or regresses; maintain a 74-bit random/counter field and wait for a later millisecond or fail if it would overflow. Retry a document collision at most 16 times, then return exit 7 `io:randomness_exhausted` with `{operation:"randomness_exhausted",path:"generator"}`.

A `Document` has ordered threads. A `Thread` has ID, location, and one or more insertion-ordered messages. A `Message` has ID, author, logical body, and optional immutable GitLab metadata. Equality is all normalized stored fields byte-for-byte, including optional-field presence.

## Authors and metadata

Author is a closed tagged union:

* `human {name?: Name}`;
* `agent {name?: Name}`;
* `external {external_author:{id: ExternalIdentity,name: Name}}`.

Human/agent omit an absent name; empty is not substituted. An external author always has both fields. `Name` is valid NFC UTF-8, 1–128 scalars, no `Cc`, CR, LF, or NUL. `ExternalIdentity` is nonempty valid UTF-8, at most 256 scalars, no `Cc`, CR, LF, or NUL; NFC is not required for identity.

Optional `gitlab` is a closed immutable union with no operational meaning:

* `note {project_id: positive integer, discussion_id: ExternalIdentity, note_id: positive integer}`;
* `system_note {project_id: positive integer, note_id: positive integer}`.

`project_id` and `note_id` are decimal integers in `1..9223372036854775807`. No field may be added, inferred, or changed after creation.

## Bodies and structural safety

A body is a logical valid UTF-8 string using LF only. It is at most 65,536 normalized UTF-8 bytes and contains at least one scalar that is not Unicode `White_Space`. It may have blank lines and end in LF. Its equality is exact bytes. Scan every logical line (including final empty line): no line may begin byte-for-byte with `[@th_`, `[/@th_`, `---user`, `---bot`, or `---external`. There is no escaping mechanism.

## Limits

| Item | Maximum |
|---|---:|
| existing sidecar bytes | 4 MiB (4,194,304) |
| threads/document | 1,024 |
| messages/thread | 256 |
| messages/document | 16,384 |
| decoded anchor bytes | 4,096 |
| body normalized bytes | 65,536 |
| raw metadata JSON bytes | 16,384 |
| source bytes | 16 MiB (16,777,216) |
| apply request bytes | 1 MiB (1,048,576) |
| apply operations | 1–256 |

All bounded reads reject limit-plus-one before allocating an unbounded buffer. Resulting-document limits are checked after each sequential mutation.

## Unicode conformance

Valid UTF-8 and scalar count are evaluated after permitted line-ending normalization. NFC is tested with `golang.org/x/text/unicode/norm` only; names are rejected rather than normalized. Controls and whitespace use Unicode 16.0.0 properties as frozen in fixtures. The conformance corpus MUST include decomposed versus composed names, all White_Space boundary cases, Cc examples, non-BMP scalars, malformed UTF-8, and forbidden surrogate JSON escapes.
