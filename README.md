# Sidecar Comments

> **Status: v1 CLI implementation in progress.** The normative specification remains authoritative. CLI verification results must be assessed separately from the independent contract harness; the Neovim adapter is not implemented.

Sidecar Comments stores structured review threads in a colocated Markdown-like sidecar. A source file `a.go` maps to `a.go.comments`; the format is deliberately strict, deterministic, and independently implementable.

## Build and CLI verification

Go **1.27.1** is required (see `mise.toml`). Build the production CLI with:

```sh
make build
./bin/sidecar --output json version
```

Run the public-process CLI suite against that executable:

```sh
SIDECAR_BIN="$PWD/bin/sidecar" go test ./tests/cli -run '^(TestSubject.*|TestFormatLifecycle)$' -count=1 -timeout=120s
```

Run the separate contract corpus with an independently supplied expected build version (`1.0.0` for the current CLI):

```sh
SIDECAR_BIN="$PWD/bin/sidecar" SIDECAR_CLI_VERSION=1.0.0 make test-contract
```

The runner's `{{CLI_VERSION}}` templates use `SIDECAR_CLI_VERSION` (or its `-cli-version` option); without either, the runner defaults the expected version to `dev`. Do not obtain the expected value from the subject's output or make production read this harness-only setting.

This focused suite is not a claim that all v1 release gates, platforms, or fault scenarios are covered. The broader release targets remain fail-closed for capabilities that have not been implemented.

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

The future implementation uses module `github.com/trogers1/sidecar-comments`, Go **1.27.1** provisioned through `mise`, and Neovim **0.12**. Mutation support is limited to `darwin/arm64` and `linux/amd64`; other platforms have documented read-only reduced support. `golang.org/x/text` **v0.14.0** is an exact-pinned direct dependency, used by production exclusively through `unicode/norm` for NFC verification. Its module and checksum entries remain in `go.mod` and `go.sum`; promotion from indirect did not change the pinned version or checksums. This version's normalization tables report Unicode **15.0.0**. That table version is distinct from the specification's Unicode 16.0.0 control/whitespace property requirement; the dependency promotion does not amend that requirement.

The contracts-only third-party dependency is `github.com/santhosh-tekuri/jsonschema/v6` at the version and checksums in `go.mod`/`go.sum`. It is user-authorized for contracts only: `contracts/runner` uses it to validate Draft 2020-12 schemas, fixtures, and observed child JSON. Its transitive use of `golang.org/x/text` is separate from production's narrowly authorized NFC-verifier import. `jsonschema` may not be imported by production `cmd/sidecar`, `internal/`, or Lua code; `x/text/unicode/norm` has only the NFC-verifier exception described above.

## Implementation posture

These contracts are implementation-ready, not implementation instructions to weaken. Future code must prefer explicit immutable domain constructors, concrete DTOs at trust boundaries, standard library facilities, bounded I/O, and black-box executable contracts. It must not use `unsafe`, untyped JSON (`any` or `map[string]any`), reflection-based domain dispatch, unchecked conversion of untrusted values, or legacy compatibility paths.

See [conformance](docs/v1/conformance.md) for the required staged implementation, fixture model, checks, and release gate. A checked-in **shadow v2** obligation/capability design reconciles current red work and records omissions; it does not execute those scenarios or make v1 release-ready. The remaining gates are sequenced in [the red-suite roadmap](plans/red-suite-roadmap.md). Cutover inventory, runbook, and replacement skill are caller-owned outside this repository, per [compatibility](docs/v1/compatibility.md).
