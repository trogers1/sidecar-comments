# Neovim 0.12 adapter public contract

The reusable adapter supports Neovim 0.12 only. It is a UI and source-mapping client of the v1 executable, never a format parser, serializer, store, or GitLab client. Lua uses argv-form process APIs (never shell concatenation), typed JSON decode/encode into closed DTO tables, and validates protocol `1` before consuming a response.

## Public Lua API, commands, and state

The public module is `require("sidecar-comments")`. `setup(opts)` is idempotent and is the only configuration entry point; it returns no value and raises a Lua error for an invalid option. Its closed options are `binary` (an absolute executable path only) and `enabled` (boolean, default true). Unknown keys are errors. `setup` creates exactly one augroup `SidecarComments` and the buffer-local commands below for every eligible buffer. Repeated setup replaces its own autocmds and never duplicates commands, extmarks, signs, or watchers.

Public commands are buffer-local, take no arguments, and are available only while the buffer is eligible: `:SidecarCommentsRefresh`, `:SidecarCommentsList`, `:SidecarCommentsCreate`, `:SidecarCommentsReply`, `:SidecarCommentsEdit`, `:SidecarCommentsDelete`, and `:SidecarCommentsFix`. They are the complete v1 command surface; the adapter creates no default mappings. Calling a command while disabled is a no-op with one warning notification. `Refresh` invokes `--output json validate` then `--output json thread list`; `List` refreshes if state is stale and opens the list UI; mutations use JSON commands or `apply` only. `Fix` is enabled only for the latest repair-required validation state and asks for explicit confirmation before `fix --add-missing-message-ids --output json`.

For each eligible source buffer, the adapter maintains private-but-observable buffer state: `disabled`, `validating`, `ready`, `repair_required`, `blocked`, or `stale`. `vim.b.sidecar_comments_state` contains exactly that string, and `vim.b.sidecar_comments_reason` is absent except in `disabled`, `repair_required`, or `blocked`, where it is a stable protocol/health symbol. A state transition fires `User SidecarCommentsState` with `vim.v.event = {buf:BUF,state:STATE,reason?:SYMBOL}`. No thread data is exposed in buffer variables. `:checkhealth sidecar-comments` reports binary, negotiated version, writer support, and current-buffer eligibility.

## Binary resolution and eligibility

Default binary is exactly `sidecar` resolved through `PATH`. The sole override accepts an **absolute** executable path; empty, relative, `~`-expanded-by-plugin, list, command string, or environment-derived override is rejected with a health diagnostic. The adapter does not search project configuration. It maps a named source path by appending exactly `.comments`: `a.go→a.go.comments`, `a.comments→a.comments.comments`.

Only a named, regular, non-symlink local file buffer is eligible. Unnamed, directory, terminal, URI/non-file, invalid path, and symlink source buffers set `disabled`, publish the stable reason, and expose no commands/UI. The adapter must not follow a source symlink merely because the editor resolved it. It must not invoke mutations on unsupported writer platforms.

## Observable UI and async rules

In `ready`, each resolved non-lost thread has one extmark in a dedicated namespace at its start line, with stable `user_data.thread_id`; its sign/virtual-text text is `SC` and its highlight group is `SidecarCommentsThread`. A lost thread has no source extmark and appears only in the list. A refresh replaces the namespace atomically only after a complete successful validated list; it clears all marks on every non-ready state. The list is a scratch, nofile, nomodifiable buffer named `sidecar-comments://{source-path}`. Its lines are in CLI canonical thread order and include thread UUID, location, and message count; selecting a line jumps to that thread or displays `lost` without moving the cursor. Create captures cursor as `line:N`; visual create captures the selected half-open byte range. Reply/Edit/Delete always require a selected list item or an extmark under cursor, otherwise notify `selection_required`.

Lua owns extmarks, signs/virtual text, navigation, cursor/visual capture, dirty buffers, popups, diagnostics, and asynchronous request correlation. It converts v1 1-based byte coordinates to Neovim’s 0-based byte API by subtracting one; selections are captured as half-open byte ranges. It preserves exact selected normalized source bytes when making a range anchor. It asks CLI JSON read commands for thread data and uses JSON mutation/apply writes; it never inspects sidecar text.

Every process request carries buffer number, buffer changedtick, canonical source path, last observed sidecar storage revision, and a monotonically increasing token. A response is discarded unless all still match; discarded responses have no state/UI/notification effect. `BufWritePost` and detected external sidecar changes make state `stale` and schedule validation. During `validating`/`stale`, existing marks may remain only until validation completes, but every mutation command is disabled.

## Fail closure and dirty buffers

After source save and after detected external sidecar change, run `validate --output json`. A repair response enters `repair_required` and offers the explicit `Fix` action; it never fixes automatically. Any other CLI, protocol, validation, source, or parse failure enters `blocked`, clears marks, disables all mutations, and shows one diagnostic until a successful fresh validation. A missing sidecar is a valid empty `ready` state.

Never overwrite a dirty sidecar buffer. If the derived sidecar buffer is open and modified, state is `blocked` with `sidecar_dirty`; no automatic format/fix/write/reload occurs until it is no longer modified and refresh succeeds. Before every mutation, reject a dirty source buffer: v1 disables mutation, reports `source_dirty`, and asks the user to save. A source/sidecar revision conflict refreshes to `stale` and requires an explicit user retry; it never retries a mutation automatically.

Health checks report missing executable, nonexecutable/invalid override, incompatible `version` protocol or format version, unsupported mutation platform, and unusable buffer paths. No fallback parser or old plugin behavior is permitted.
