# Sidecar Comments v1 — normative index

This directory is the complete v1 contract. Terms **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative. A conforming implementation accepts only behavior specified here; absence of a rule is not permission to add behavior.

## Contract map

| Document | Normative subject |
|---|---|
| [format](format.md) | on-disk grammar, parser, canonical serialization, repair AST |
| [domain-and-unicode](domain-and-unicode.md) | immutable domain values, limits, IDs, Unicode |
| [source-and-location](source-and-location.md) | source validation and location resolution |
| [protocol](protocol.md) and [schema/v1.json](schema/v1.json) | JSON envelopes, DTOs, normative machine-readable schema, errors |
| [cli](cli.md) | command syntax and operation semantics |
| [storage-security](storage-security.md) | secure transactions, locking, durability |
| [neovim](neovim.md) | adapter boundary and UX safety |
| [architecture](architecture.md) | implementation boundaries and engineering rules |
| [compatibility](compatibility.md) | support, provisioning, cutover, recovery |
| [conformance](conformance.md) | fixtures, properties, phases, release gate |

## Shared definitions

* **Sidecar target:** a path passed as `--file` whose basename case-sensitively has a terminal `.comments` suffix and whose basename is not `.comments`. “One terminal suffix” means one occurrence is removed for derivation; it does **not** forbid earlier occurrences, so `a.comments.comments` is a target. A trailing separator is invalid. A directory is never a target. `x.comments.tmp` is not a target.
* **Source derivation:** remove exactly one terminal `.comments` from the target path. Thus `a.comments.comments` derives `a.comments`.
* **Normal document:** a fully valid `Document`; a parser may construct it only after every structural and domain check succeeds.
* **Repair document:** an otherwise valid parsed document having one or more and only `missing_message_id` diagnostics. It is not a `Document` and cannot be listed, fetched, formatted, or mutated except by `fix`.
* **Canonical empty revision:** `sha256:` followed by SHA-256 of zero bytes. Revisions use lowercase hexadecimal only. A repair document has no canonical revision and reports JSON `canonical_revision:null` in the repair-required `validate` result and in a repair-needed `fix --check` result.
* **LF notation:** **LF** means exactly the one byte `0x0A` (Unicode `U+000A`). In inline byte/text examples, `\n` denotes that one LF byte, not the two literal bytes backslash and `n`; `CRLF` means exactly `0x0D 0x0A`. When explicitly describing JSON lexical escapes, `\n` instead names JSON's two-character escape spelling, which decodes to that same LF byte. A phrase such as “followed by one LF” has this same meaning. No other newline convention is implicit.
* **Normalized bytes:** UTF-8 bytes after the input-specific CRLF-to-LF normalization explicitly allowed by this specification. NFC is never silently applied to values: required-NFC values are rejected if not already NFC.

## Deterministic decisions not left to implementations

1. Integer grammar allows ASCII decimal only, no sign, leading zero, exponent, or whitespace. It is parsed without overflow; values outside the declared range are invalid.
2. “Unicode scalar” excludes surrogate code points. Valid UTF-8 is decoded as Unicode scalar values.
3. Control means Unicode General Category `Cc`; NUL, CR, and LF are called out even though they are controls. Unicode whitespace uses the pinned Unicode `White_Space` property (not Go's evolving convenience predicates).
4. A production NFC verifier MUST use only `golang.org/x/text/unicode/norm`, promote it to an exact-pinned direct dependency only when that production code is introduced, and record its version/checksums and Unicode data version in conformance fixtures. This contracts-only checkout already records `golang.org/x/text` only as the tidy-resolved indirect dependency of the contracts-only JSON Schema harness and the tiny test-only `tests/cli/schema_test.go` validator; that transitive presence neither authorizes a production import nor satisfies the direct-dependency requirement for an NFC verifier. A later version/data change is a protocol change, not an implementation choice.
5. SHA-256 preimages are exact on-disk bytes for storage revisions and canonical serialized bytes for canonical revisions. Missing has storage revision `null`; zero-byte existing has storage revision SHA-256(empty) and canonical revision SHA-256(empty).
6. Result object key ordering is not semantically significant JSON, but the reference encoder MUST emit compact UTF-8 JSON with fields in each schema’s displayed order. Exactly one terminating LF is required for every JSON envelope.

## Out of scope

No legacy parser/migration/shim, GitLab integration, streaming JSONL, configuration discovery, source writeback, or Neovim parser/serializer belongs to v1. A future version must explicitly version any addition.
