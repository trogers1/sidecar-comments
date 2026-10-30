# Sidecar Comments 🛵

**Keep the conversation beside the file, not inside it.**

Sidecar is a small CLI for leaving structured review threads next to a document. Your document takes the wheel; comments ride alongside in a plain-text `.comments` file.

```text
proposal.md           ← the document
proposal.md.comments  ← the conversation
```

Review a draft, discuss a design, annotate research notes, or leave feedback on code. Threads have stable IDs, authors, replies, and locations: a whole file, a line, or a byte range. The original file stays untouched.

No review server or account required. Keep the pair together locally, share them, or commit them to Git so the conversation travels with the document.

**Different text formats, same little sidecar:** Markdown, plain text, source code, and other UTF-8 text files. Location checking reads text; it does not interpret PDF, Word, or other binary document formats.

> **Road conditions:** writes are supported on **macOS Apple Silicon** and **Linux x86-64**; other platforms have reduced, read-only support. Publication is gated on both supported platforms passing the CLI checks. Neovim integration is a [separate future feature](plans/neovim-adapter.md), not a requirement for the standalone CLI.

## Install · hop aboard

### Homebrew · stable releases

On macOS Apple Silicon or Linux x86-64:

```sh
brew install trogers1/tap/sidecar
sidecar --output json version
```

Update with `brew update && brew upgrade sidecar`. The public
[Homebrew tap](https://github.com/trogers1/homebrew-tap) installs checked,
versioned release binaries; no Go toolchain is needed. It does not follow the
rolling `latest` build. The tap must be pushed before this installation is available.

### Download a checked binary · no Go required

The release workflow publishes a rolling **[Latest build](https://github.com/trogers1/sidecar-comments/releases/tag/latest)** after passing checks on `main`. Its download URLs stay the same, but its contents move forward. The binary reports a commit-specific version so you can identify the build.

Maintainers can publish a stable version from **Actions → CLI verification and releases → Run workflow** on **main**, choosing a **major**, **minor**, or **patch** bump; both native gates must pass before a new tag is created. See the [release runbook](docs/releases.md) for version selection and repository protection setup.

For a pinned installation, choose an immutable **`vX.Y.Z` tag** from the [releases page](https://github.com/trogers1/sidecar-comments/releases). Replace `latest` below with that exact tag. A release link is available only after the workflow has successfully published it; this checkout does not itself publish a release.

| Your machine | Archive |
| --- | --- |
| macOS, Apple Silicon | `sidecar_darwin_arm64.tar.gz` |
| Linux, x86-64 | `sidecar_linux_amd64.tar.gz` |

For example, on macOS Apple Silicon, download into a fresh scratch directory:

```sh
RELEASE=latest
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
sidecar --output json version
```

Add the `PATH` export to your shell configuration to keep it for future sessions. To update, repeat the download and verification with `latest` or a newer pinned tag.

### With Go

With **Go 1.27.1 or newer**, install the current development snapshot:

```sh
go install github.com/trogers1/sidecar-comments/cmd/sidecar@main
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` (or your `GOBIN` directory, if you set one), then check your passenger:

```sh
sidecar --output json version
```

`@main` follows development; for a pinned source install, replace it with an existing `@vX.Y.Z` tag. Source installs report `dev` because `go install` does not apply the release workflow's version linker flags. Use a release archive when you want the embedded release identity.

### From a checkout

```sh
git clone https://github.com/trogers1/sidecar-comments.git
cd sidecar-comments
go build -o bin/sidecar ./cmd/sidecar
export PATH="$PWD/bin:$PATH"
sidecar --output json version
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
| `lost` | A thread without a resolved source location |

Lines and columns start at **1**. Range ends are exclusive; columns count **UTF-8 bytes**, not displayed characters. Ranges can span lines and optionally carry an anchor; see [locations](docs/v1/source-and-location.md).

Create and move check the source derived by removing one final `.comments` suffix. The source must exist and the requested location must fit, except for `lost`, which skips source access. Threads do **not** automatically follow document edits.

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

The file is readable text, but its format is deliberately strict—not arbitrary Markdown. Invalid or pre-v1 files are rejected rather than guessed at or migrated. `fix` is a narrow repair tool, not a general recovery button. See the [format reference](docs/v1/format.md) before hand-editing.

## For scripts and coding agents

The output selector goes **before** the command:

```text
sidecar [--output text|json|id|none] COMMAND ...
```

- **`text`** (default): one line with the command, change status, and JSON result.
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

For multiple edits in one transaction, use `sidecar apply --file PATH --operations-file PATH` (or `-` for stdin). The request contains `protocol: 1` and an `operations` array. See the [CLI contract](docs/v1/cli.md#apply) and [protocol/schema](docs/v1/protocol.md) for request shapes and preconditions.

| Exit | Meaning |
| --- | --- |
| `0` | Success |
| `2` | Invalid command or request input |
| `3` | Validation failure, repair needed, or a failed formatting/repair check |
| `4` | Conflict: revision, lock timeout, or file identity |
| `5` | Thread or message not found |
| `6` | Operation precondition failed |
| `7` | I/O failure |
| `8` | Unsupported capability or version |

Writes on unsupported platforms fail with exit 7 (`io`, operation `unsupported_secure_filesystem`). Unsupported apply protocol versions use exit 8.

## Under the seat · development & reference

The [v1 specification](docs/v1/index.md) is authoritative. Useful stops:

- [CLI commands and flags](docs/v1/cli.md)
- [Text format](docs/v1/format.md)
- [Source locations](docs/v1/source-and-location.md)
- [JSON protocol](docs/v1/protocol.md) and [schema](docs/v1/schema/v1.json)
- [Storage and security](docs/v1/storage-security.md)
- [Conformance and release gates](docs/v1/conformance.md)
- [Remaining release work](plans/red-suite-roadmap.md)

The CLI stores local conversations; it is not a GitLab client and does not import or synchronize remote reviews. GitLab fields, when supplied, are descriptive metadata only.

For contributors, the toolchain is pinned in `mise.toml`. Build the CLI and independent contract harness, then run the focused public-process suite:

```sh
make build
SIDECAR_BIN="$PWD/bin/sidecar" go test ./tests/cli \
  -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s
```

Run the separate contract corpus with an independently supplied expected version (`dev` for an ordinary local build):

```sh
SIDECAR_BIN="$PWD/bin/sidecar" SIDECAR_CLI_VERSION=dev make test-contract
```

See the [release runbook](docs/releases.md) for the maintained CLI-only gate, bounded fuzzing, production race checks, artifact verification, and tag publication. The historical combined CLI/Neovim inventories remain reference material; passing the CLI gate does not claim completion of the deferred adapter or every historical conformance obligation.

## License

[MIT](LICENSE) · take the sidecar for a spin. Binary archives also include the applicable [third-party notices](THIRD_PARTY_NOTICES).
