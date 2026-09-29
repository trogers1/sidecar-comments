# Storage, locking, and durability

## Supported writer platforms

Hardened writes are supported only on `darwin/arm64` and `linux/amd64`. Every mutation (`format`, `fix`, single mutation, `apply`) on another OS/architecture fails before opening the target with exit 7 `io`, operation `unsupported_secure_filesystem`, path `target`. Reads are reduced support only and still reject malformed/oversized data; they make no hardened race-safety claim.

The v1 writer requires descriptor-relative no-follow filesystem operations and advisory exclusive locking using only Go's standard-library/syscall surface on both writer platforms. If that capability cannot be compiled or proved available at startup, it MUST hard-fail as `unsupported_secure_filesystem`; it MUST NOT fall back to path-string TOCTOU, an in-process mutex, or an unprotected write. No dependency may be added for locking. Cooperative locks cannot protect against a hostile principal authorized to alter the parent directory; that principal is outside the threat model.

## Secure object rules

Open the parent directory securely. All target, sibling lock, and temp work is descriptor-relative and no-follow. The target may be absent or a regular nonsymlink with link count exactly one. Reject symlinks, directories, device/FIFO/socket, and multi-link target as validation exit 3. The sibling lock name is `.<basename>.sidecar.lock`; it too is no-follow regular and its device/inode identity is checked before and after acquisition. Lock it with an advisory exclusive lock, retrying until the timeout; timeout is exit 4 `conflict:lock_timeout` with details `{kind:"lock_timeout"}`. Lock files persist after clean release and are reusable; a stale unlocked valid lock is not an error. Invalid lock objects are exit 3 `validation:lock_type` with details `{path:"lock_type",reason:REASON}`.

## Required writer transaction

1. Validate all request bytes and paths that do not depend on target state.
2. Securely open parent and target, establish/verify the sibling lock, and acquire its exclusive advisory lock.
3. Securely reread bounded target bytes; classify absent/zero/canonical/repair/invalid and calculate storage revision.
4. Repeat every validation that authorizes a write: revision match, formatter/fix parser state, and implicit-source validation/location resolution for creates/moves.
5. Evaluate operation(s) on an immutable working copy; check uniqueness and limits after each operation. Return semantic no-op without writing, except zero-byte cleanup is a changed write/delete.
6. Recheck parent, target, and lock identities immediately before commit. Any observed change is exit 4 `conflict:identity_changed` with details `{kind:"identity_changed"}`.
7. Write or unlink through descriptors, fsync parent, release lock, and report revisions.

The sidecar lock is not a source lock. Source changes after final source validation and before commit are outside atomicity; a checked anchor mismatch always fails closed.

## Atomic replace and deletion

For replacement, create a random same-directory temporary with exclusive no-follow create and mode `0600`; retry name collisions at most 16 times. Verify regular identity, write all bytes, fsync it, and set its final mode to original target mode or `0600 & ~umask` for a new target. Re-verify identities, atomically rename descriptor-relatively over target, then fsync parent. Never preserve ownership, ACLs, extended attributes, or timestamps as a v1 promise. For deletion, identity-check target then unlink descriptor-relatively and fsync parent.

Failure before rename/unlink preserves old bytes. The durability decision point is successful parent-directory fsync after rename/unlink. If rename/unlink happened but that fsync fails or its outcome is unknown, return exit 7 with `io` details `commit_may_have_succeeded:true`; no success output may be emitted. All earlier IO failures omit that field. Before that decision point, a close, unlock, identity-verification, or cleanup failure is an `io` error with respectively stable operation symbols `close`, `unlock`, `identity`, or `cleanup` (and carries `commit_may_have_succeeded:true` only if rename/unlink has already occurred). The required `path` remains the affected stable symbol (`target`, `parent`, `lock`, or `temp`), never an OS path. After successful parent fsync, the externally observable outcome is a durable success: subsequent best-effort lock release/descriptor-close cleanup failure MUST NOT replace it with an error, add output, or change `changed`/revisions. Implementations may record such post-durability cleanup failures only in a private diagnostic sink; it is not protocol output and cannot affect the returned outcome.

A caller receiving `commit_may_have_succeeded` MUST reread with `validate`/list, compare the expected revision/entity, and then choose a fresh revision-guarded retry only if the intended state is absent. It must never blindly replay a generated-ID mutation.

## Revisions, concurrency, and permissions

Storage revision hashes exact reread bytes under lock. Canonical revision hashes canonical serialization; missing uses canonical empty; zero-byte existing has both empty hashes but is still noncanonical. Every writer must perform the transaction even if another process of the same executable exists; correctness cannot depend on memory-local coordination. The contract suite must use independent adversarial subprocesses to prove successful concurrent updates are not silently lost.

Filesystem assumptions: a local POSIX filesystem supporting directory fsync, exclusive advisory locks, atomic same-directory rename, stable inode/device identity, and no-follow descriptor operations. Network, overlay, or filesystems violating these assumptions are unsupported for mutation and MUST hard-fail when capability checks reveal it; unknown behavior is not silently accepted.
