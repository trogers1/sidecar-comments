# P8/P9/P10 storage transaction red pack

This is an **unregistered contribution pack**.  It deliberately does not alter
`contracts/corpus/v1.json`, `contracts/behavior-inventory-v1.json`, or the
runner.  `manifest.json` is self-contained: it defines the case schedule,
required host controls, expected protocol outcome, and the precise
`UNAVAILABLE` result required until the current runner gains that control.

The files in `goldens/` are uniquely prefixed (`p8p9p10-`) declarative golden
inputs for the manifest, not members of the central v1 corpus.  They are
intentionally red: no implementation, Python/Node helper, or production code
is supplied by this contribution.

A future pack-aware runner must not silently skip a case whose capabilities are
unavailable.  It must emit the corresponding `unavailable` object from the
manifest and return nonzero.  Cases that need a controlled injected syscall
failure use the already-known runner capability `fault_injection`; its current
reason string is pinned verbatim.  Cases requiring deterministic independent
process interleavings use the proposed `process_orchestration` capability; the
unmodified central runner currently reports it as unknown, also pinned here.
