module github.com/trogers1/sidecar-comments

go 1.27.1

require (
	// jsonschema is authorized only for contracts/runner and the tiny test-only
	// tests/cli/schema_test.go validator of the normative public schema.
	// Production Sidecar packages must not import it.
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	golang.org/x/text v0.14.0
)
