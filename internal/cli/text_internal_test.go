package cli

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type ioSchemaEnumRequest struct {
	T        *testing.T
	Property string
}

// ioSchemaEnum reads one IODetails enum from the published schema, the single
// source of truth for which operations and paths an IO error may carry.
func ioSchemaEnum(r ioSchemaEnumRequest) []string {
	r.T.Helper()
	data, err := os.ReadFile("../../docs/v1/schema/v1.json")
	if err != nil {
		r.T.Fatal(err)
	}
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		r.T.Fatal(err)
	}
	values := schema.Defs["IODetails"].Properties[r.Property].Enum
	if len(values) == 0 {
		r.T.Fatalf("schema has no IODetails %s enum", r.Property)
	}
	sort.Strings(values)
	return values
}

func TestIOTextCoversSchemaEnums(t *testing.T) {
	for _, tc := range []struct {
		property string
		table    []string
	}{
		{property: "operation", table: operationKeys(operationKeysRequest{})},
		{property: "path", table: nounKeys(nounKeysRequest{})},
	} {
		want := ioSchemaEnum(ioSchemaEnumRequest{T: t, Property: tc.property})
		if len(want) != len(tc.table) {
			t.Fatalf("%s: schema %v, human text table %v", tc.property, want, tc.table)
		}
		for i := range want {
			if want[i] != tc.table[i] {
				t.Fatalf("%s: schema %v, human text table %v", tc.property, want, tc.table)
			}
		}
	}
}

type operationKeysRequest struct{}

func operationKeys(r operationKeysRequest) []string {
	out := make([]string, 0, len(ioOperations))
	for k := range ioOperations {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type nounKeysRequest struct{}

func nounKeys(r nounKeysRequest) []string {
	out := make([]string, 0, len(ioNouns))
	for k := range ioNouns {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Randomness for temporary file names is not ID generation; explicit IDs
// cannot help, so the text must not suggest them.
func TestIOTextTemporaryNameRandomness(t *testing.T) {
	for _, operation := range []string{"csprng", "randomness_exhausted"} {
		explanation, next := ioText(ioTextRequest{Details: domain.IODetails{Operation: operation, Path: "temp"}})
		if !strings.Contains(explanation, "temporary file") || strings.Contains(explanation+next, "--id") {
			t.Fatalf("%s on temp: %q / %q", operation, explanation, next)
		}
	}
}
