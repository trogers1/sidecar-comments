# Source and locations

## Location value grammar

```
location = "file" / "line:" positive-int / range / "lost"
range = "range:" positive-int ":" positive-int "-" positive-int ":" positive-int [" | " base64]
positive-int = %x31-39 *DIGIT
```

Coordinates are 1-based UTF-8 **byte** coordinates. A line location is `line:N`, `N>=1`. A range is half-open, has positive coordinates, and requires `(SL,SC) < (EL,EC)` lexicographically. Columns exclude terminators; byte column 1 is the first byte. `base64` is standard RFC 4648 padded base64 (`A–Z a–z 0–9 + /` with required correct `=` padding), decodes to nonempty valid UTF-8 no longer than 4,096 bytes, and is permitted only on a range. The public schema's `Base64Anchor` has `minLength:1` and enforces the padded-base64 lexical form; decoded nonemptiness, UTF-8 validity, and the 4,096-byte bound remain document validation requirements. Structural format parsing checks only grammar, coordinate ordering, and anchor encoding; resolution is a source operation.

Canonical location ordering is `file`, `line`, `range`, `lost`; lines compare N; ranges compare SL, SC, EL, EC, absence/presence of anchor (absent first), decoded anchor bytes; UUID bytes finally break ties.

## Source validator

`validate --source PATH` requires an existing readable regular non-symlink file at most 16 MiB. Missing is exit 7; symlink, directory, and any nonregular source are exit 3. Read failures (including permission denial) are exit 7. Reject UTF-8 BOM, NUL, invalid UTF-8, bare CR, and a mixture of LF and CRLF endings. Consistent LF, consistent CRLF, and a file with no terminators are accepted. CRLF is normalized to logical LF before all location work.

An empty source has one empty line. A final unterminated line exists. Each logical line has valid columns `1..byte_length+1`, and a coordinate must fall on a UTF-8 rune boundary (the end column is allowed). For a multiline range, selected logical bytes join selected line fragments using logical LF. A range anchor, if present, must exactly equal these selected bytes. `file` requires a valid readable source but has no coordinate check. `lost` always resolves successfully and never reads source.

For `line:N`, N must name an existing logical line. For a single-line range, both columns address that line and `SC<EC`. For a multiline range, both lines exist, start/end columns are valid, and the lexical order rule applies. A missing or unreadable required source is `io {operation:"open",path:"source"}` (exit 7), not a validation path.

### Public location failure symbols

This table is normative and removes any parser/location ambiguity before a fixture may assert a public error. A malformed stored location grammar (unknown kind, zero coordinate, reversed range, malformed/empty/non-UTF-8/over-limit anchor, or an anchor on a non-range) is `validation {path:"target_content",reason:"location"}`. A syntactically valid location that cannot resolve against an otherwise valid source (missing line, out-of-line/out-of-column coordinate, or non-rune-boundary column) is `validation {path:"source_coordinate",reason:"location"}`. An anchor whose decoded bytes do not equal the selected logical source bytes is `validation {path:"source_anchor",reason:"location"}`. Source object, size, encoding, and endings failures retain their existing `source_type`, `source_size`, `source_encoding`, and `source_endings` paths with the corresponding reason. These symbols apply identically to pre-lock and locked revalidation.

## When validation occurs

`validate --source` validates the supplied source and every non-`lost` stored location against it; it does not derive a source. For create, move, and apply operations that create or move a thread, derive the source by stripping one suffix from `--file`, validate it and resolve the requested location before ID generation, then repeat after taking the sidecar lock immediately before commit. There is no mutation `--source`. `lost` bypasses source access. A source change after that second validation is outside the sidecar transaction guarantee; an anchor mismatch at either check fails closed.

Neovim uses its 0-based byte APIs by subtracting one from validated line and column values; it never converts character columns.
