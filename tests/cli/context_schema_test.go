package cli_test

import "testing"

func TestContextSchemaFragments(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{name: "untracked_metadata", input: `{}`, valid: true},
		{name: "unknown_field", input: `{"extra":true}`},
		{name: "null_record", input: `{"tracking":null}`},
		{name: "incomplete_record", input: `{"tracking":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSchema(schemaRequest{JSON: []byte(tc.input), Path: contextSchemaPath + "#/$defs/ThreadMetadata"})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t error=%v", tc.valid, err)
			}
		})
	}
}
