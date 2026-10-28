package cli_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type schemaRequest struct{ JSON []byte }

// This test-only bridge exposes errors, never generic JSON data. Schema alone
// does not establish the supplemental lexical/domain rules in protocol.md;
// future CLI callers must independently check those before using this bridge.
func validateSchema(r schemaRequest) error {
	if len(r.JSON) > 1<<20 {
		return fmt.Errorf("boundary JSON exceeds 1 MiB limit")
	}
	path, err := filepath.Abs("../../docs/v1/schema/v1.json")
	if err != nil {
		return fmt.Errorf("locate normative schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(path)
	if err != nil {
		return fmt.Errorf("compile normative schema: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(r.JSON))
	if err != nil {
		return fmt.Errorf("decode boundary JSON: %w", err)
	}
	return schema.Validate(value)
}

func TestSchemaBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, json string
		valid      bool
	}{
		{"version", `{"protocol":1,"command":"version","changed":false,"result":{"cli_version":"test","format_version":1,"protocol":1}}`, true},
		{"error", `{"protocol":1,"error":{"code":"usage","message":"usage:argv:missing","details":{"field":"argv","reason":"missing"}}}`, true},
		{name: "utf8_error", json: `{"protocol":1,"error":{"code":"validation","message":"validation:target_encoding:utf8","details":{"path":"target_encoding","reason":"utf8"}}}`, valid: true},
		{"unknown_member", `{"protocol":1,"command":"version","changed":false,"extra":true,"result":{"cli_version":"test","format_version":1,"protocol":1}}`, false},
		{"wrong_protocol", `{"protocol":2,"command":"version","changed":false,"result":{"cli_version":"test","format_version":1,"protocol":1}}`, false},
		{"wrong_result", `{"protocol":1,"command":"version","changed":false,"result":{}}`, false},
		{"apply", `{"protocol":1,"operations":[{"op":"thread_delete","thread_id":"018f0000-0000-7000-8000-000000000001"}]}`, true},
		{"empty_apply", `{"protocol":1,"operations":[]}`, false},
		{"nested_unknown", `{"protocol":1,"command":"version","changed":false,"result":{"cli_version":"test","format_version":1,"protocol":1,"extra":true}}`, false},
		{"malformed", `{`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSchema(schemaRequest{JSON: []byte(tc.json)})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t, schema error=%v", tc.valid, err)
			}
		})
	}
}
