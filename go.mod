module github.com/trogers1/sidecar-comments

go 1.27.1

// jsonschema is authorized exclusively for contracts/runner: it validates
// checked-in Draft 2020-12 contract artifacts and observed child protocol
// output. Production Sidecar packages must not import it.
require github.com/santhosh-tekuri/jsonschema/v6 v6.0.2

require golang.org/x/text v0.14.0 // indirect
