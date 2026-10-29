# Neovim adapter — deferred feature

## Decision and scope

Neovim integration is a future feature, not a prerequisite for shipping the standalone Sidecar CLI. This separation was requested during the CLI release-readiness audit. The CLI supports review of UTF-8 documents, including but not limited to source code; the adapter should preserve that scope.

No adapter is implemented today. `make release` and `make cli-gate` now verify the standalone CLI independently of Neovim; see [the release runbook](../docs/releases.md). The historical combined v1 inventories remain separate obligations under `make full-red`, and `make test-neovim` still fails explicitly as deferred. This split does not mark missing adapter coverage as passed or weaken the applicable CLI safety checks.

## Existing design

Use [the adapter contract](../docs/v1/neovim.md) as the design baseline, together with the [CLI contract](../docs/v1/cli.md), [protocol](../docs/v1/protocol.md), and [source/location rules](../docs/v1/source-and-location.md). Resolve design inconsistencies before implementation rather than copying an invalid command invocation from prose. In particular, the CLI's global `--output json` flag precedes the command words.

The current design targets Neovim 0.12 and exposes `require("sidecar-comments").setup(opts)`, buffer-local commands, thread navigation, and health checks. Reconfirm the supported Neovim version when this feature is scheduled.

## Implementation sequence

1. **Client and health checks.** Resolve the CLI from PATH or an explicitly configured absolute executable path. Use argv-form process APIs, validate closed JSON responses, negotiate protocol/format versions, and detect writer support. Never parse or serialize `.comments` text in Lua.
2. **Eligibility and lifecycle.** Support named regular non-symlink local text documents. Make setup idempotent; expose buffer state and manage autocmds, process requests, and teardown without duplicate resources.
3. **Read-only review UI.** Validate and list threads through the real CLI. Show ordered threads, replies, and lost locations; add extmarks and navigation. Convert 1-based UTF-8 byte coordinates to Neovim's 0-based byte APIs.
4. **Explicit mutations.** Add create, reply, edit, delete, and narrowly scoped fix actions. Capture cursor/visual locations correctly, use revisions, and ask for repair confirmation. Do not add default key mappings.
5. **Failure and change handling.** Block writes for dirty original-document or sidecar buffers, invalid data, unsupported writers, and incompatible responses. Discard stale async responses, refresh after external changes, and require explicit retries after conflicts.
6. **Documentation and independent feature gate.** Document setup and commands; add a separately named adapter verification target that runs real headless Neovim against the production CLI. Integrating that target must not make standalone CLI releases depend on an optional editor feature.

## Acceptance evidence

Use headless Neovim and a freshly built real CLI, with isolated temporary documents. Cover:

- repeated setup, buffer eligibility, disable/enable, and cleanup;
- missing/invalid binary and protocol/format negotiation;
- absent sidecar, populated threads, lost locations, canonical ordering;
- multibyte text, cursor and visual half-open ranges, exact anchors;
- create/reply/edit/delete/fix and user confirmation;
- dirty document and sidecar buffers, malformed/repair-required data;
- stale responses after edits, buffer switches, and path changes;
- external sidecar changes and revision conflicts without automatic replay;
- extmark replacement/clearing and observable state transitions;
- operation on supported platforms and fail-closed unsupported writers.

Feature completion requires its own passing gate and documentation. It is not evidence for, or a substitute for, CLI storage, parser, concurrency, or release verification.
