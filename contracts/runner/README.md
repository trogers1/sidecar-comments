# Sidecar v1 black-box contract runner

`sidecar-contract` executes the checked-in corpus against a process named by
`SIDECAR_BIN`. It does not import or link a Sidecar implementation. Each case
gets a new temporary working directory, one stable `BHV-` behavior ID, exact
argv and stdin bytes, and a byte-for-byte comparison of exit status, stdout, stderr, final filesystem
entries, file modes, links, and persistent lock files when a fixture declares
them.

Artifacts are a closed typed format. The runner rejects duplicate JSON object
members (including nested members), unknown DTO fields, unsafe paths, invalid
base64, unexpanded templates, illegal command/output/status/stream combinations,
duplicate behavior IDs, unreferenced goldens, uncovered normative requirements,
and fixture/golden requirement drift. Fixtures whose argv is intentionally
invalid declare `expect.invocation_error:"argv"`; those are executable
exit-2/text-error cases rather than malformed harness artifacts. The seed inventory generates and validates
the fixture-to-behavior mapping; it is not an exhaustive coverage claim. Before a subject starts it performs real
Draft 2020-12 meta-schema, corpus/golden instance, `$ref`, and JSON-mode
protocol-stream validation with the pinned harness-only JSON Schema library.
The library's generic-instance API is confined to `schema.go` as a harness-only
adapter after concrete corpus/golden DTO decoding; it is not a production DTO
boundary and is forbidden elsewhere. Only the manifest-declared templates are expanded: `{{CLI_VERSION}}`, SHA-256
values of explicitly listed initial/final regular files, and the narrowly typed
UUIDv7 binding pair. `{{UUIDV7:bind:name}}` may appear once only in an expected
stdout/stderr template; it captures exactly one canonical lowercase UUIDv7.
`{{UUIDV7:ref:name}}` may appear later in an expected stream template or an
expected final regular-file `bytes_template`, and must equal that capture.
UUID templates are forbidden in argv, environment, paths, link targets, and
initial filesystem `bytes_template` fields. Base64-encoded stdin and initial
filesystem bytes are literal payloads, so template-looking body text is allowed
there and in base64-encoded expected streams. This preserves byte-exact comparison:
every non-template byte remains exact, while a bind validates all UUID version,
variant, case, and punctuation bytes. All other `{{...}}` text is an artifact
error.

`contracts/packs/required-red-v1.json` is the central machine-checked ledger for every P6, P8–P10, and P11–P12 pack scenario. It retains each native manifest pointer, every available pack golden, and typed required capabilities. Artifact validation rejects an orphan pack golden, stale reference, duplicate scenario, or unknown capability; `full-red` prints every scenario as `REQUIRED_RED` and exits nonzero until a direct corpus fixture replaces it.

`contracts/packs/obligations-v2.json` and `capabilities-v2.json` are **shadow design artifacts**, not executable proof. Artifact checks now validate their schemas and reconcile the v2 obligation list with all v1 red IDs and registered pack scenarios; four impossible semantic-precondition requests are recorded as reviewed `not_applicable`. Twenty-five newly identified coverage cases remain pending, including architecture, compatibility, Neovim, transport, output, storage, and source behavior; they are individually tracked but do not yet form exhaustive test matrices. V2 capability inputs are closed typed declarations and every adapter remains unavailable. Neither ledger currently has a promotion/execution path; `full-red` remains the active v1 unresolved-obligations report. Schema validity and `pending` do not imply conformance.

There is no capability-success skip path. An unknown or unavailable capability
emits an explicit `UNAVAILABLE` fixture result and makes the run nonzero, so it
cannot be counted as coverage. Capabilities are concrete typed runner checks;
a writer fixture on a platform other than `darwin/arm64` or `linux/amd64` is
unavailable. `fault_injection` remains explicitly unavailable until this
pre-implementation harness has a controlled injector. The deliberately nonconformant executable
under `contracts/testbin/nonconformant` is used only to prove this rejection
path:

```sh
make test-contract-red
make test-contract SIDECAR_BIN=/absolute/path/to/sidecar
```

This repository is pre-implementation. `make build` builds only the harness;
`make preimplementation-check` is the green harness-only gate. `make full-red`,
`make release`, and the legacy `make check` release alias intentionally remain
non-green while the inventory's explicit `required_red` fixture packs are
unavailable; none claims that the partial seed corpus is full coverage.
