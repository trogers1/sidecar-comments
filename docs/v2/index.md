# Sidecar Comments v2 — normative contract

This directory specifies CLI 0.2.0, sidecar format 2 and protocol 2. It is a contract, **not a claim that runtime v2 support has shipped**. MUST/MUST NOT are normative.

- [Format and domain](format.md): closed persistence metadata, canonical framing, attachment lifecycle and privacy.
- [Protocol and CLI](protocol.md): requests, results, errors and guards.
- [Tracking and sync](tracking.md): capture, bounded inference, evidence and check semantics.
- [Normative Draft 2020-12 schema](schema/v2.json).

## Inherited semantics and precedence

V2 changes only the behavior explicitly specified here. The following frozen v1 provisions remain normative by reference: [shared definitions](../v1/index.md#shared-definitions), [domain and Unicode](../v1/domain-and-unicode.md) (IDs, authors, messages, bodies, limits and Unicode), [source validator](../v1/source-and-location.md#source-validator) (excluding anchors), [body transport and unrelated commands](../v1/cli.md), [framing/repair rules](../v1/format.md), and [secure storage, durability, delivery and revisions](../v1/storage-security.md). V2 overrides their version numbers, location/anchor/tracking rules, create retry capture, equal-coordinate moves, and newly defined commands. V1 conformance/release assertions do not establish v2 implementation conformance. No adapter implementation, new resolution state, source writeback, integration, migration, or Git operation is introduced.

Existing format/validate/fix behavior, messages, immutable author/GitLab metadata, deletion, ID namespaces/generation, error delivery and transaction semantics remain unchanged except where expressly overridden. A tracking record's `version:1` is its own record-layout version, not protocol/format v1 support.

## Schema use and validation

The schema's root accepts a protocol response, apply request, or editor request. Validate persistence metadata against `#/$defs/ThreadMetadata`; validate a reconstructed document against `#/$defs/StoredDocument`. `StoredDocument` describes an in-memory reconstruction, **not an alternative JSON persistence format**. All concrete objects are closed. Unchanged leaf DTOs and unchanged result/error shapes reference the frozen v1 schema rather than redefining them.

Use a Draft 2020-12 validator with local resolution of `../../v1/schema/v1.json` relative to `docs/v2/schema/v2.json`. When using schema `$id` resolution, map `https://github.com/trogers1/sidecar-comments/docs/v1/schema/v1.json` to the local v1 file, and register that file's declared `$id` as well. No network fetch is necessary. JSON Schema is only the structural gate: also perform the normative lexical, byte-limit, cross-field, source, lifecycle and privacy checks in these documents. In particular maxLength counts scalars, not UTF-8 bytes.

`scripts/contract_definitions.go` derives the public `contracts/schema/runtime-v2.json` artifact from authoritative domain versions, limits and enum definitions. `make contract-definitions-check` regenerates it in memory and checks exact bytes without writing; intentional updates use `go run scripts/contract_definitions.go --write`. The independent runner consumes the public artifact, never private production packages. This is generated-definition freshness, not generation of the entire normative schema or proof of production behavior. Schema/prose coherence, byte limits, lifecycle checks and behavioral tests remain separate obligations. A complete validation must meta-validate both schemas, resolve every reference locally, validate representative positive/negative request/result and reconstructed persistence fixtures, and check invariants not expressible in JSON Schema. Passing one of these checks does not imply the rest ran.

## Version rejection

A missing or zero-byte target retains existing empty handling. Every nonempty v2 target MUST carry the format-2 header. Headerless v1 input and explicitly unsupported format versions fail with `unsupported:format` (exit 8), including through format/fix; no coordinates are converted into tracking. Malformed framing remains validation failure, not repairable migration. Protocol requests require the v2 shape; missing/wrongly typed protocol is usage, and an otherwise structurally valid request with another positive integer protocol is `unsupported:protocol` (exit 8). Old writers must reject the new header instead of discarding tracking. No v1 or legacy Neovim migration, compatibility shim, best-effort conversion, or legacy anchor input is supported.
