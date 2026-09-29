# Compatibility, provisioning, and cutover

## Compatibility policy

v1 accepts only the v1 canonical format, the explicitly enumerated formatter-only variants, and zero-byte noncanonical empty. Every legacy/pre-v1 shape is invalid exit 3 and is never migrated, guessed, rewritten, or treated as a synthetic ID source. There are no shims, old plugin modes, or retained readers/writers. `comments.md` remains unrelated free-form text. GitLab DTOs are stored descriptive metadata only.

Any future protocol/format change requires a new explicit version, published schemas and goldens, an interoperability statement, and a compatibility decision. A v1 implementation encountering another protocol exits 8 `unsupported`; it does not attempt best effort. v1 claims compatibility only after passing the public v1 corpus against its executable.

## Provisioning and supported environments

A fresh clone provisions Go **1.27.1** with `mise` using the checked-in `mise.toml`; the final implementation must pin precisely `go = "1.27.1"`, not a floating minor. It uses module `github.com/trogers1/sidecar-comments`. Neovim integration requires 0.12. Supported mutation test/release environments are current macOS on Apple Silicon (`darwin/arm64`) and Ubuntu LTS on x86-64 (`linux/amd64`) with local POSIX filesystem semantics named in [storage-security](storage-security.md). Other OS/architectures may run read-only CLI operations but mutations hard-fail.

Builds must be reproducible from tracked files: documented `mise install`, `make build`, and checked-in dependency checksums only. No untracked tool, external configuration repository, or `../config` inventory is allowed.

## No inventory or external configuration

v1 has no inventory command, project discovery, configuration discovery, migration, or cutover mechanism. It MUST NOT read `../config`, an external inventory, editor configuration, or any sibling repository at build time, runtime, or during tests. A caller that wants to process a set of paths supplies each target explicitly and owns any backup, discovery, reporting, and rollback procedure outside this protocol. Such operational tooling must not be represented as a v1 compatibility feature or used to infer sidecar state.

## Durability recovery

On `io` with `commit_may_have_succeeded:true`, stop automation; reread the target, validate it, and compare revision/entity state. Record the outcome. Use a new revision guard for any intentional retry. Generated IDs must be retained from a known successful response or not replayed blindly. Lock files are durable siblings and need no deletion; a valid unlocked lock is reused, while invalid lock objects require human repair.
