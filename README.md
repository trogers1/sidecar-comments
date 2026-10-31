# Sidecar Comments 🛵

**Keep the conversation beside the file, not inside it.**

Sidecar is a small CLI for leaving structured review threads next to a document. Your document takes the wheel; comments ride alongside in a plain-text `.comments` file.

```text
proposal.md           ← the document
proposal.md.comments  ← the conversation
```

Review a draft, discuss a design, annotate research notes, or leave feedback on code. Threads have stable IDs, authors, replies, and locations: a whole file, a line, or a byte range. The original file stays untouched.

No review server or account required. Keep the pair together locally, share them, or commit them to Git so the conversation travels with the document.

> **Versions:** the CLI, the sidecar file format, and the machine protocol are versioned independently; `sidecar --version` shows all three. Sidecar reads only its current format and rejects other format versions, including in `format` and `fix`; there is no migration tool.

**Different text formats, same little sidecar:** Markdown, plain text, source code, and other UTF-8 text files. Location checking reads text; it does not interpret PDF, Word, or other binary document formats.

> **Road conditions:** writes are supported on **macOS Apple Silicon** and **Linux x86-64**; other platforms have reduced, read-only support. Publication is gated on both supported platforms passing the CLI checks. Neovim integration is a [separate future feature](plans/neovim-adapter.md), not a requirement for the standalone CLI.

## Install · hop aboard

### Homebrew · recommended

Install the latest stable release on macOS Apple Silicon or Linux x86-64:

```sh
brew install trogers1/tap/sidecar
sidecar --version
```

Already riding along? Update to the newest stable release:

```sh
brew update
brew upgrade sidecar
sidecar --version
```

The public [Homebrew tap](https://github.com/trogers1/homebrew-tap) installs
SHA-256-checked, versioned release binaries; no Go toolchain or separate tap
command is needed. Homebrew follows stable releases, not development snapshots
or the historical rolling `latest` build.

### Download a checked binary · no Go required

Every push to `main` verifies the committed `VERSION` on both supported platforms,
then publishes that stable release and updates Homebrew in the same workflow.
Maintainers install the pre-push guard with `make install-hooks`, choose a version
with `make bump-patch` (or `bump-minor` / `bump-major`), and commit `VERSION` with
their changes before pushing. CI independently enforces the version gate. See the
[release runbook](docs/releases.md) for setup, protection rules, and recovery.

For a pinned installation, choose an immutable **`vX.Y.Z` tag** from the [releases page](https://github.com/trogers1/sidecar-comments/releases). Use that exact tag below (for example `v0.1.1`).

| Your machine | Archive |
| --- | --- |
| macOS, Apple Silicon | `sidecar_darwin_arm64.tar.gz` |
| Linux, x86-64 | `sidecar_linux_amd64.tar.gz` |

For example, on macOS Apple Silicon, download into a fresh scratch directory:

```sh
RELEASE=v0.1.1
ARCHIVE=sidecar_darwin_arm64.tar.gz
BASE="https://github.com/trogers1/sidecar-comments/releases/download/$RELEASE"
curl -fLO "$BASE/$ARCHIVE"
curl -fLO "$BASE/checksums.txt"
grep "  $ARCHIVE\$" checksums.txt > selected-checksum.txt
shasum -a 256 -c selected-checksum.txt
```

On Linux x86-64, set `ARCHIVE=sidecar_linux_amd64.tar.gz` and use `sha256sum -c selected-checksum.txt` for the last command. Stop if verification fails. After verification, install without `sudo`:

```sh
tar -xzf "$ARCHIVE"
mkdir -p "$HOME/.local/bin"
install -m 755 sidecar "$HOME/.local/bin/sidecar"
export PATH="$HOME/.local/bin:$PATH"
sidecar --version
```

Add the `PATH` export to your shell configuration to keep it for future sessions. To update, repeat the download and verification with a newer stable tag.

### With Go

With **Go 1.27.1 or newer**, install the current development snapshot:

```sh
go install github.com/trogers1/sidecar-comments/cmd/sidecar@main
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` (or your `GOBIN` directory, if you set one), then check your passenger:

```sh
sidecar --version
```

`@main` follows development; for a pinned source install, replace it with an existing `@vX.Y.Z` tag. Source installs report `dev` because `go install` does not apply the release workflow's version linker flags. Use a release archive when you want the embedded release identity.

### From a checkout

```sh
git clone https://github.com/trogers1/sidecar-comments.git
cd sidecar-comments
go build -o bin/sidecar ./cmd/sidecar
export PATH="$PWD/bin:$PATH"
sidecar --version
```

The `PATH` change applies to your current shell, so `sidecar` remains available when you change directories.

## Your first conversation

Try this in a scratch directory. Start with a tiny document:

```sh
printf '# Proposal\n' > proposal.md
```

Leave a note on line 1. `--body-file -` reads the message from standard input:

```sh
printf 'Could we give this proposal a more descriptive title?\n' |
  sidecar --output json thread create \
    --file proposal.md.comments \
    --location line:1 \
    --author-kind human \
    --author-name Taylor \
    --body-file -
```

Sidecar creates `proposal.md.comments`, generates IDs for the thread and its first message, and reports them under `result.generated_ids`. No `init` command needed.

Read the conversation, either through the CLI or by opening the sidecar in your editor:

```sh
sidecar --output json thread list --file proposal.md.comments
```

To reply, copy the `thread_id` from the create result into this variable:

```sh
THREAD_ID='paste-the-thread-UUID-here'

printf 'Good call. I will update the title before we share it.\n' |
  sidecar --output json message add \
    --file proposal.md.comments \
    --thread "$THREAD_ID" \
    --author-kind agent \
    --author-name Copilot \
    --body-file -
```

Check that the sidecar and its locations are valid. The CLI calls the original document the **source**:

```sh
sidecar validate --file proposal.md.comments --source proposal.md
```

That's the ride: **leave a thread, pass along its ID, reply, repeat.**

## Everyday trips

Every command except `version` takes `--file` pointing to the **sidecar**, not the source. Thread and message IDs are UUIDv7 values; use the IDs returned by the CLI.

The following examples reuse `$THREAD_ID` above. Set `$MESSAGE_ID` to the ID of the message you want to change.

```sh
# Read one thread.
sidecar --output json thread get --file proposal.md.comments --id "$THREAD_ID"

# Move a thread to another location.
sidecar thread move --file proposal.md.comments --id "$THREAD_ID" --location file

# Replace a message body with the contents of a UTF-8 text file.
sidecar message update --file proposal.md.comments \
  --thread "$THREAD_ID" --id "$MESSAGE_ID" --body-file revised-note.txt

# Delete a message, or an entire thread.
sidecar message delete --file proposal.md.comments --thread "$THREAD_ID" --id "$MESSAGE_ID"
sidecar thread delete --file proposal.md.comments --id "$THREAD_ID"
```

Deleting the last message removes its thread. Deleting the last thread removes the sidecar file. An absent sidecar is an empty conversation, not an error.

### Where does a thread sit?

| `--location` | Meaning |
| --- | --- |
| `file` | The whole document |
| `line:12` | Line 12 |
| `range:12:1-12:8` | Line 12, byte columns 1 through 7 |
| `lost` | A deliberately unlocated thread, excluded from automatic tracking |

Lines and columns start at **1**. Range ends are exclusive; columns count **UTF-8 bytes**, not displayed characters. Ranges can span lines. See [tracking and locations](docs/v2/tracking.md).

Create and move check the source derived by removing one final `.comments` suffix. The source must exist and the requested location must fit, except for `lost`, which skips source access. Line comments track the whole line excluding its terminator. Line/range selected text is limited to 4,096 UTF-8 bytes; oversize attachments are rejected rather than silently truncated.

### Follow document edits

The CLI does not watch or rewrite your document. After saving edits, inspect reconciliation without changing the sidecar, then persist supported mappings:

```sh
sidecar --output json sync --file proposal.md.comments --check
sidecar --output json sync --file proposal.md.comments
```

Sync looks for the exact selected text, using the surrounding text as evidence; it never guesses the nearest occurrence or uses fuzzy scores. Supported interior edits and boundary expansions are reported as inferred modifications. Ambiguous, deleted, unsupported, or search-limited selections become `unresolved`: the conversation and last tracking evidence are preserved, without pretending stale coordinates are an active attachment. Repeated sync is stable; later unique evidence can recover the attachment. To deliberately reattach, use `thread move` with a current line/range.

Ordinary sync exits 0 when its transaction succeeds even with unresolved outcomes. Check mode never writes or takes a write lock; it reports `changed:false` and `would_change`, exiting 3 for proposed changes or unresolved threads. Read per-thread reasons and evidence in JSON. Replies, reads, edits and deletion remain available without locating every thread or reading the source.

An editor can submit exact saved-source coordinates using `editor-update`; it must supply both the source fingerprint and sidecar storage revision. The CLI reads the saved file, not unsaved buffer text. Source inspection is independent of sidecar validity:

```sh
sidecar --output json source inspect --file proposal.md.comments
```

Moving to `file` or `lost` removes automatic tracking excerpts. Deleting a thread removes its conversation too—there is no separate resolved state or reopen command.

### Retained source context and privacy

Each tracked thread keeps its current selected text, up to 256 UTF-8 bytes on either side, edge markers, coordinates and logical-source fingerprint inside the sidecar. Total decoded tracking text is limited to 1 MiB and the serialized sidecar to 4 MiB. No full-source baseline, snapshot directory, hidden cache, or accumulating context history is written. The source and sidecar alone are portable; Git history is never searched implicitly.

Ordinary machine reads and mutation results return tracking summaries, not retained excerpts. Explicit `thread get` or `thread list` with `--include-tracking` exposes them in JSON. Human summaries and diagnostics do not dump context. The sidecar itself contains retained source text: share it accordingly.

Deletion or moving to file/lost removes tracking from the current file, not from previous Git commits, backups, filesystem snapshots or shared copies. User-authored message quotations are conversation content, not disposable tracking evidence. Secure erasure is not promised.

### Who's riding?

Use `--author-kind human` for people or `--author-kind agent` for coding agents. Both accept an optional `--author-name`. An `external` author requires both `--author-name` and `--external-author-id`.

Messages come from `--body-file PATH` or `--body-file -` for stdin, not an inline `--body` flag. Bodies must be nonempty UTF-8 text; ordinary LF and consistent CRLF input are accepted.

### Keep the sidecar tidy

```sh
# Validate structure without checking the source.
sidecar validate --file proposal.md.comments

# Check canonical formatting without writing; exit 3 means changes are needed.
sidecar format --file proposal.md.comments --check

# Apply canonical formatting.
sidecar format --file proposal.md.comments

# Repair the specific case of missing message IDs.
sidecar fix --add-missing-message-ids --file proposal.md.comments
```

The file is readable text, but its format is deliberately strict—not arbitrary Markdown. Invalid or v1 files are rejected rather than guessed at or migrated. `fix` is a narrow repair tool, not a general recovery button. Nonempty v2 files require an explicit format header and dedicated tracking metadata after each thread opening. See the [format reference](docs/v2/format.md) before hand-editing.

## Need directions?

Comments ride alongside your text; the CLI should tell you where to go next.

```sh
sidecar                       # Overview and quick start
sidecar --help                # Commands and common options
sidecar thread --help         # Thread commands
sidecar thread create --help  # Required flags, options, and examples
```

`-h` works too. Check the installed version with `sidecar --version`, or use
`sidecar --output json --version` for structured version information. The original
`sidecar version` command remains supported. Help never reads or changes your files. Normal output is meant
for people: readable summaries, thread/message IDs, authors, and comment text.
Use the explicit machine modes below when another program is in the driver's seat.

## For scripts and coding agents

The output selector goes **before** the command:

```text
sidecar [--output text|json|id|none] COMMAND ...
```

- **`text`** (default): readable summaries and actionable errors. Prose is not a stable scripting interface.
- **`json`**: a structured envelope with `protocol`, `command`, `changed`, and `result`. Errors go to stderr with `protocol` and `error`.
- **`id`**: generated IDs, one per line; supported by `thread create`, `message add`, and `fix`. Creating a thread prints **two** IDs: thread first, message second.
- **`none`**: suppress successful output for mutation commands; errors still go to stderr.

For concurrent writers, use `--if-revision` with a returned `storage_revision`, or `missing` to require that no sidecar exists. Put it and any `--lock-timeout` **immediately after the command words, before other flags**:

```sh
printf 'First note on this file.\n' |
  sidecar --output json thread create \
    --if-revision missing --lock-timeout 5s \
    --file proposal.md.comments --location file \
    --author-kind human --body-file -
```

That example intentionally fails with a conflict if you've already created `proposal.md.comments`.

For coordinates computed before a mutation, machine clients must use both the sidecar guard and normalized source fingerprint (`--if-source-revision` on create/move). A source change after the final precommit check is outside the sidecar transaction guarantee: the CLI does not lock the source. On conflict, reread and retry explicitly; do not blindly replay generated-ID mutations.

For multiple edits in one transaction, use `sidecar apply --file PATH --operations-file PATH` (or `-` for stdin). The request contains `protocol: 2`, an `operations` array, and optional `if_revision`/`if_source_revision` guards. Editor batches require both guards and reject the whole batch on failure; they are never split into separate commits. See the [v2 protocol and CLI contract](docs/v2/protocol.md) and [schema](docs/v2/schema/v2.json).

| Exit | Meaning |
| --- | --- |
| `0` | Success |
| `2` | Invalid command or request input |
| `3` | Validation/repair failure, failed formatting/repair check, or sync-check drift/unresolved state |
| `4` | Conflict: sidecar/source revision, lock timeout, or file identity |
| `5` | Thread or message not found |
| `6` | Operation precondition failed |
| `7` | I/O failure |
| `8` | Unsupported capability or version |

Writes on unsupported platforms fail with exit 7 (`io`, operation `unsupported_secure_filesystem`). Unsupported apply protocol versions use exit 8.

## Under the seat · development & reference

The [v2 specification](docs/v2/index.md) is authoritative for this checkout; it explicitly inherits unchanged v1 provisions without accepting v1 files. Useful stops:

- [CLI commands, guards and protocol](docs/v2/protocol.md)
- [Text format, lifecycle and privacy](docs/v2/format.md)
- [Source capture and bounded reconciliation](docs/v2/tracking.md)
- [JSON Schema](docs/v2/schema/v2.json)
- [Inherited storage and security guarantees](docs/v1/storage-security.md)
- [Independent executable coverage](contracts/COVERAGE.md)
- [Approved implementation and acceptance plan](plans/thread-context-sync.md)

The CLI stores local conversations; it is not a GitLab client and does not import or synchronize remote reviews. GitLab fields, when supplied, are descriptive metadata only.

### Contributor setup · once per clone

CLI users do not need these development prerequisites. Contributors need Git,
Python 3, Make, and the Go toolchain pinned in `mise.toml`.

From the repository root, install the tracked Git hooks:

```sh
make install-hooks
```

This enables the pre-push version guard for pushes to `main`. It checks the
committed `VERSION` against the destination remote's stable tags, so access to
that remote is required when pushing. CI enforces the same version rule even if
the hook is bypassed. Setup refuses to overwrite a different custom hooks path;
see the [release runbook](docs/releases.md#choose-a-version-before-pushing).

### Build and test

Build the CLI and independent contract harness, then run the focused public-process suite:

```sh
make build
SIDECAR_BIN="$PWD/bin/sidecar" SIDECAR_CLI_VERSION=dev go test ./tests/cli \
  -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s
```

Run the separate contract corpus with an independently supplied expected version (`dev` for an ordinary local build):

```sh
SIDECAR_BIN="$PWD/bin/sidecar" SIDECAR_CLI_VERSION=dev make test-contract
```

See the [release runbook](docs/releases.md) for the maintained CLI-only gate, bounded fuzzing, production race checks, artifact verification, and tag publication. The historical combined CLI/Neovim inventories remain reference material; passing the CLI gate does not claim completion of the deferred adapter or every historical conformance obligation.

## License

[MIT](LICENSE) · take the sidecar for a spin. Binary archives also include the applicable [third-party notices](THIRD_PARTY_NOTICES).
