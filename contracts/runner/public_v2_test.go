package runner

import (
	"path/filepath"
	"testing"
)

func TestPublicV2Artifacts(t *testing.T) {
	if err := ValidateArtifacts(filepath.Join(repoRootForTest(t), "contracts/corpus/v2.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPublicV2StoredAndRequestShapes(t *testing.T) {
	compiler, err := compileV2Schemas(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		definition, instance string
		valid                bool
	}{
		{"StoredDocument", `{"format":2,"threads":[]}`, true},
		{"StoredDocument", `{"format":1,"threads":[]}`, false},
		{"ThreadMetadata", `{}`, true},
		{"ThreadMetadata", `{"tracking":null}`, false},
		{"ThreadMetadata", `{"history":[]}`, false},
		{"EditorUpdateRequest", `{"protocol":2,"updates":[]}`, false},
		{"InputLocation", `{"kind":"unresolved"}`, false},
		{"InputLocation", `{"kind":"range","start_line":1,"start_column":1,"end_line":1,"end_column":2,"anchor":"eA=="}`, false},
	} {
		schema, err := compiler.Compile(v2DocumentSchemaURL + "#/$defs/" + test.definition)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateSchemaInstance(schema, []byte(test.instance)); (err == nil) != test.valid {
			t.Fatalf("%s: %v", test.definition, err)
		}
	}
}

func TestPublicV2LexicalAndCommandBoundary(t *testing.T) {
	for _, source := range [][]byte{[]byte(`{"excerpt":"\ud800"}`), {'"', 0xff, '"'}, []byte(`{"version":2e0}`)} {
		if uniqueJSON(source) == nil {
			t.Fatalf("accepted invalid lexical JSON %q", source)
		}
	}
	for _, argv := range [][]string{{"source", "inspect", "--file", "a.comments"}, {"editor-update", "--file", "a.comments", "--operations-file", "-"}, {"sync", "--check", "--file", "a.comments"}} {
		if _, err := parseInvocation(argv); err != nil {
			t.Fatal(err)
		}
	}
	if !checkSuccessCommand("sync", []string{"sync", "--check"}) || checkSuccessCommand("sync", []string{"sync"}) {
		t.Fatal("incorrect sync exit-3 legality")
	}
}

func TestPublicV2ProtocolRejectsLegacyEnvelope(t *testing.T) {
	validator, err := compileV2ProtocolValidator(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{\"cli_version\":\"dev\",\"format_version\":1,\"protocol\":1}}\n",
		"{\"protocol\":2,\"command\":\"version\",\"changed\":false,\"result\":{\"cli_version\":\"dev\",\"format_version\":1,\"protocol\":2}}\n",
	} {
		if err := validator.validateActual([]byte(source), nil); err == nil {
			t.Fatal("accepted legacy version")
		}
	}
}
