package cli_test

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const contextSchemaPath = "../../docs/v2/schema/v2.json"

type schemaRequest struct {
	JSON []byte
	Path string
}

// This test-only bridge exposes errors, never generic JSON data. Schema alone
// does not establish the supplemental lexical/domain rules in protocol.md;
// future CLI callers must independently check those before using this bridge.
func validateSchema(r schemaRequest) error {
	if len(r.JSON) > 1<<20 {
		return fmt.Errorf("boundary JSON exceeds 1 MiB limit")
	}
	schemaPath := r.Path
	if schemaPath == "" {
		schemaPath = contextSchemaPath
	}
	schemaPath, fragment, _ := strings.Cut(schemaPath, "#")
	path, err := filepath.Abs(schemaPath)
	if err != nil {
		return fmt.Errorf("locate normative schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	// The v2 contract reuses frozen v1 leaves. Register the documented URI
	// alias locally so schema validation never depends on a network fetch.
	inherited, err := os.ReadFile("../../docs/v1/schema/v1.json")
	if err != nil {
		return fmt.Errorf("read inherited normative schema: %w", err)
	}
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(inherited))
	if err != nil {
		return fmt.Errorf("decode inherited normative schema: %w", err)
	}
	if err := compiler.AddResource("https://github.com/trogers1/sidecar-comments/docs/v1/schema/v1.json", resource); err != nil {
		return fmt.Errorf("register inherited normative schema: %w", err)
	}
	location := (&url.URL{Scheme: "file", Path: path, Fragment: fragment}).String()
	schema, err := compiler.Compile(location)
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
		{"version", `{"protocol":2,"command":"version","changed":false,"result":{"cli_version":"test","format_version":2,"protocol":2}}`, true},
		{"error", `{"protocol":2,"error":{"code":"usage","message":"usage:argv:missing","details":{"field":"argv","reason":"missing"}}}`, true},
		{name: "utf8_error", json: `{"protocol":2,"error":{"code":"validation","message":"validation:target_encoding:utf8","details":{"path":"target_encoding","reason":"utf8"}}}`, valid: true},
		{"unknown_member", `{"protocol":2,"command":"version","changed":false,"extra":true,"result":{"cli_version":"test","format_version":2,"protocol":2}}`, false},
		{"wrong_protocol", `{"protocol":1,"command":"version","changed":false,"result":{"cli_version":"test","format_version":2,"protocol":2}}`, false},
		{"wrong_result", `{"protocol":2,"command":"version","changed":false,"result":{}}`, false},
		{"apply", `{"protocol":2,"operations":[{"op":"thread_delete","thread_id":"018f0000-0000-7000-8000-000000000001"}]}`, true},
		{"empty_apply", `{"protocol":2,"operations":[]}`, false},
		{"nested_unknown", `{"protocol":2,"command":"version","changed":false,"result":{"cli_version":"test","format_version":2,"protocol":2,"extra":true}}`, false},
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
