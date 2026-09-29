# Sidecar Comments

> **Status: v1 normative specification only.** This checkout intentionally contains no production CLI, parser, storage layer, or Neovim implementation.

Sidecar Comments stores structured review threads in a colocated Markdown-like sidecar. A source file `a.go` maps to `a.go.comments`; the format is deliberately strict, deterministic, and independently implementable.

## v1 documents

The normative v1 specification is split by concern:

- [Specification index](docs/v1/index.md)
- [Text format](docs/v1/format.md)
- [Domain and Unicode](docs/v1/domain-and-unicode.md)
- [Source and locations](docs/v1/source-and-location.md)
- [Protocol and schemas](docs/v1/protocol.md)
- [CLI contract](docs/v1/cli.md)
- [Storage and security](docs/v1/storage-security.md)
- [Neovim adapter](docs/v1/neovim.md)
- [Architecture and engineering rules](docs/v1/architecture.md)
- [Compatibility and operations](docs/v1/compatibility.md)
- [Conformance and release gates](docs/v1/conformance.md)

If documents conflict, the precedence order is: `format`, `domain-and-unicode`, `source-and-location`, `protocol` (including `schema/v1.json`), `cli`, `storage-security`, then the remaining documents. The lower-precedence document must be corrected; implementations must not infer a compatibility behavior. `schema/v1.json` is the normative machine-readable JSON contract, supplemented only by the protocol rules it explicitly says JSON Schema cannot express.

## Scope and hard cut

v1 is a hard replacement. A sidecar is only a non-directory target whose case-sensitive basename has a terminal `.comments` suffix and is not exactly `.comments`. Repeated suffixes are valid (`a.comments.comments`); derivation removes one terminal suffix, not every occurrence. `comments.md` is free-form and outside this specification. Missing sidecars mean an empty document. Existing pre-v1 or malformed files are invalid and are never migrated, guessed, converted, or overwritten.

GitLab fields are immutable descriptive metadata; v1 provides no GitLab client, import, mapping, reconciliation, or resolution behavior. No runtime component may read, write, invoke, or depend on `../config`.

## Planned toolchain and support

**Toolchain amendment:** this v1 contract supersedes the Go 1.26 reference in `plans/initial.md`; all v1 implementation and conformance work uses the committed Go **1.27.1** toolchain below.

The future implementation uses module `github.com/trogers1/sidecar-comments`, Go **1.27.1** provisioned through `mise`, and Neovim **0.12**. Mutation support is limited to `darwin/arm64` and `linux/amd64`; other platforms have documented read-only reduced support. `golang.org/x/text` is currently a tidy-resolved **indirect** dependency of the contracts harness through `github.com/santhosh-tekuri/jsonschema/v6`; its version and checksums are therefore tracked in `go.mod`/`go.sum`. It is not a production dependency and must not be imported by production code merely because it is present transitively. A future implementation may promote `golang.org/x/text/unicode/norm` to an exact-pinned **direct** dependency only when implementing the normative NFC verifier; that change must document the version, checksums, and Unicode data version.

The sole current direct third-party dependency is `github.com/santhosh-tekuri/jsonschema/v6` at the version and checksums in `go.mod`/`go.sum`. It is user-authorized for contracts only: `contracts/runner` uses it to validate Draft 2020-12 schemas, fixtures, and observed child JSON. Its `golang.org/x/text` dependency is likewise harness-only. Neither library may be imported by production `cmd/sidecar`, `internal/`, or Lua code.

## Implementation posture

These contracts are implementation-ready, not implementation instructions to weaken. Future code must prefer explicit immutable domain constructors, concrete DTOs at trust boundaries, standard library facilities, bounded I/O, and black-box executable contracts. It must not use `unsafe`, untyped JSON (`any` or `map[string]any`), reflection-based domain dispatch, unchecked conversion of untrusted values, or legacy compatibility paths.

See [conformance](docs/v1/conformance.md) for the required staged implementation, fixture model, checks, and release gate. A checked-in **shadow v2** obligation/capability design reconciles current red work and records omissions; it does not execute those scenarios or make v1 release-ready. The remaining gates are sequenced in [the red-suite roadmap](plans/red-suite-roadmap.md). Cutover inventory, runbook, and replacement skill are caller-owned outside this repository, per [compatibility](docs/v1/compatibility.md).
