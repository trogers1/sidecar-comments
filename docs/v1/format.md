# On-disk format and repair parser

## Canonical bytes and grammar

A nonempty canonical file is UTF-8 without BOM, CR, or NUL and has exactly one final LF. In this grammar, `LF = %x0A`, `SP = %x20`, and `text` excludes CR (`%x0D`), LF (`%x0A`), and NUL (`%x00`). The shared [LF notation](index.md#shared-definitions) applies to every inline `\n` example below.

```
canonical-document = thread *(LF thread)
thread = thread-start LF message *(LF message) thread-end LF
thread-start = "[@th_" UUIDv7 SP location "]"
thread-end = "[/@th_" UUIDv7 "]"
message = role-line LF logical-body LF
role-line = "---user" [SP meta-comment] / "---bot" [SP meta-comment] / "---external" SP meta-comment
meta-comment = "<!-- sidecar-meta: " meta-json " -->"
logical-body = body-line *(LF body-line)
body-line = *text
```

Every thread end exactly repeats the opening UUID. Threads have at least one message. Roles map `user→human`, `bot→agent`, and `external→external`. Canonical normal messages always have metadata. Metadata obeys [protocol](protocol.md); user/bot may syntactically omit it only for repair parsing. External metadata is mandatory and must contain its required external author.

## Framing algorithm

Parsing is byte-oriented. After a valid role line, scan physical lines to the first **valid applicable** role line or matching valid thread end. Prior lines are candidate body lines. A candidate line beginning a reserved prefix is malformed unless it is that delimiter: prefixes are `[@th_`, `[/@th_`, `---user`, `---bot`, `---external`. Thus `---userish`, malformed metadata, an unmatched end, and thread syntax in a body are invalid.

At a next role delimiter, remove the immediately preceding separator LF and mandatory body-framing LF: the last two LFs of the candidate run. At a thread end, remove only the mandatory framing LF. Earlier LFs belong to the body. Consequently before a next role `x\n\n---bot`, `x\n\n\n---bot`, `x\n\n\n\n---bot` decode to `x`, `x\n`, `x\n\n`; before an end, one, two, three LFs before the end decode to `x`, `x\n`, `x\n\n` respectively. Serialization is the inverse: body plus framing LF; add one separator LF before a following role and one blank LF between threads.

A body must be nonempty under the domain whitespace rule. The parser rejects missing framing/separator bytes rather than trying to infer a body. Canonical emission sorts threads by canonical location order and preserves each thread’s message insertion order.

## Metadata and canonical encoding

Metadata is token-scanned recursively before DTO decode. Reject duplicate keys at every object depth, non-object metadata, unknown fields, wrong token kinds, invalid UTF-8/escapes, and every `\uD800`–`\uDFFF` escape. Accept only insignificant JSON whitespace, key reorder, and ordinary permitted escapes as formatter input. Canonical JSON is compact raw UTF-8 with no HTML escaping. It escapes only `"`, `\\`, `\b`, `\f`, `\n`, `\r`, `\t`, and other U+0000–U+001F as lowercase `\u00xx`.

Key order: message `id,name,external_author,gitlab`; external author `id,name`; GitLab note `kind,project_id,discussion_id,note_id`; system note `kind,project_id,note_id`. Omit absent/inapplicable fields.

## Empty and formatter-only variants

Missing target is canonical empty. Existing zero bytes is valid **noncanonical empty**: read/validate/list succeed with warning `noncanonical_empty`; get/update/delete are not found. Whitespace-only is invalid. Plain `format` removes zero-byte targets. Successful mutation, including semantic no-op, removes it. Deleting the final thread unlinks the target.

`format` accepts only otherwise-valid v1 with one or more of: all LF replaced consistently by CRLF; zero-byte file; noncanonical thread order; permitted metadata JSON noncanonicality; omitted final LF; exactly one extra final LF; exactly one omitted separator LF between adjacent role lines or adjacent threads. The formatter parser is **canonical-first**: it first performs normal canonical framing; it may try the omitted-separator interpretation only if that parse fails solely because the required separator is absent, and only once at that boundary. An omitted role separator means the single LF immediately before the next role is the body framing LF (no separator is removed); an omitted thread separator means the prior thread’s terminal LF directly precedes the next thread start. It never chooses that interpretation when canonical framing succeeds, so a body’s trailing LFs are not silently reassigned. These decode to the same domain values and canonicalize by inserting the required separator. It rejects all other deviations and all pre-v1 structures. `format --check` does not write and returns exit 3 when canonicalization/removal is needed. Normal read/mutation accepts canonical v1 and zero-byte empty only.

The repair parser accepts only missing user/bot metadata as repairable. It emits `missing_message_id` with role-line byte offset (one-based), physical line (one-based), thread ID, and one-based message ordinal. Any other error makes parsing invalid; repair never guesses fields.
