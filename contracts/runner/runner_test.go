package runner

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func repoRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCorpusIsLoadableAndInternallyConsistent(t *testing.T) {
	root := repoRootForTest(t)
	if err := ValidateArtifacts(filepath.Join(root, "contracts/corpus/v1.json")); err != nil {
		t.Fatalf("contract artifacts: %v", err)
	}
}
func TestDraft202012SchemaRejectsInvalidSchemasBrokenReferencesAndInstances(t *testing.T) {
	root := repoRootForTest(t)
	corpusSchema, err := os.ReadFile(filepath.Join(root, "contracts/schema/corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureSchema, err := compileSchemaValidator(corpusSchemaURL+"#/$defs/golden", []schemaResource{{Location: corpusSchemaURL, Source: corpusSchema}})
	if err != nil {
		t.Fatal(err)
	}
	if err = fixtureSchema.validate([]byte(`{"fixture_version":1}`)); err == nil {
		t.Fatal("Draft 2020-12 golden schema accepted missing required fields")
	}
	protocolSchema, err := os.ReadFile(filepath.Join(root, "docs/v1/schema/v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	const normativeProtocolURL = "https://github.com/trogers1/sidecar-comments/schema/v1.json"
	protocol, err := compileSchemaValidator(normativeProtocolURL, []schemaResource{{Location: normativeProtocolURL, Source: protocolSchema}})
	if err != nil {
		t.Fatal(err)
	}
	if err = protocol.validate([]byte(`{"protocol":1,"command":"version","changed":false,"result":[]}`)); err == nil {
		t.Fatal("Draft 2020-12 protocol schema accepted invalid result DTO")
	}
	if _, err = compileSchemaValidator("https://example.invalid/invalid.json", []schemaResource{{Location: "https://example.invalid/invalid.json", Source: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":12}`)}}); err == nil {
		t.Fatal("Draft 2020-12 accepted an invalid schema")
	}
	if _, err = compileSchemaValidator("https://example.invalid/broken.json", []schemaResource{{Location: "https://example.invalid/broken.json", Source: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"https://example.invalid/missing.json"}`)}}); err == nil {
		t.Fatal("Draft 2020-12 accepted a broken external reference")
	}
}

func TestMalformedProtocolStreamsAreRejected(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "missing LF", data: `{"protocol":1,"command":"version","changed":false,"result":{}}`},
		{name: "two records", data: "{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{}}\n{}\n"},
		{name: "duplicate key", data: "{\"protocol\":1,\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{}}\n"},
		{name: "wrong discriminator", data: "{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":[],\"error\":{}}\n"},
		{name: "CRLF", data: "{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{}}\r\n"},
		{name: "leading whitespace", data: " {\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{}}\n"},
		{name: "trailing whitespace", data: "{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{}} \n"},
		{name: "error envelope on success stream", data: "{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:x:y\",\"details\":{}}}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := decodeEnvelope([]byte(test.data)); err == nil {
				t.Fatal("malformed success stream accepted")
			}
		})
	}
}

func TestMalformedErrorProtocolStreamsAreRejected(t *testing.T) {
	tests := []string{
		`{"protocol":1,"error":{"code":"usage","message":"usage:x:y","details":{}}}`,
		"{\"protocol\":1,\"error\":{\"code\":\"usage\",\"code\":\"usage\",\"message\":\"usage:x:y\",\"details\":{}}}\n",
		"{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:x:y\",\"details\":{}}}\r\n",
		"{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:x:y\",\"details\":{}}} \n",
	}
	for _, data := range tests {
		if err := decodeErrorEnvelope([]byte(data)); err == nil {
			t.Fatal("malformed error stream accepted")
		}
	}
}

func TestStrictJSONRejectsDuplicateKeysAtEveryLevel(t *testing.T) {
	if uniqueJSON([]byte(`{"fixture":{"id":"one","id":"two"}}`)) == nil {
		t.Fatal("nested duplicate key accepted")
	}
}
func TestUnknownMandatoryCapabilityIsNotSkipped(t *testing.T) {
	if err := requireCapabilities([]string{"not-a-capability"}); err == nil {
		t.Fatal("unknown capability was accepted")
	}
}

func TestUnavailableMandatoryCapabilityIsNotSkipped(t *testing.T) {
	if err := requireCapabilities([]string{"fault_injection"}); err == nil {
		t.Fatal("unavailable mandatory capability was accepted")
	}
}

func TestTextErrorRequiresSeparateCodeAndMessage(t *testing.T) {
	if !textError([]byte("error usage usage:argv:malformed\n")) {
		t.Fatal("normative text error rejected")
	}
	if textError([]byte("error usage:argv:malformed\n")) {
		t.Fatal("legacy two-token text error accepted")
	}
}

func TestArtifactAllowsExecutableArgvErrorFixtures(t *testing.T) {
	status := 2
	fixture := golden{
		Argv:   []string{"validate", "--output", "json"},
		Expect: expected{Status: &status, InvocationError: "argv"},
	}
	if err := validateEnvelopes(fixture, status, nil, []byte("error usage usage:argv:malformed\n")); err != nil {
		t.Fatalf("declared argv error rejected: %v", err)
	}
	fixture.Expect.InvocationError = ""
	if err := validateEnvelopes(fixture, status, nil, []byte("error usage usage:argv:malformed\n")); err == nil {
		t.Fatal("undeclared argv error fixture accepted")
	}
	fixture = golden{Argv: []string{"--output", "id", "fix"}, Expect: expected{Status: &status}}
	if err := validateEnvelopes(fixture, 0, nil, nil); err != nil {
		t.Fatalf("empty id-mode success rejected: %v", err)
	}
}

func TestArtifactRejectsIllegalCommandOutputStatusAndStreams(t *testing.T) {
	success := []byte("{\"protocol\":1,\"command\":\"validate\",\"changed\":false,\"result\":{\"valid\":true,\"warnings\":[],\"storage_revision\":null,\"canonical_revision\":\"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\"}}\n")
	errorStream := []byte("{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:x:y\",\"details\":{\"field\":\"x\",\"reason\":\"y\"}}}\n")
	status := 0
	fixture := golden{Argv: []string{"--output", "json", "validate"}, Expect: expected{Status: &status}}
	if err := validateEnvelopes(fixture, 0, success, nil); err != nil {
		t.Fatalf("legal JSON success rejected: %v", err)
	}
	if err := validateEnvelopes(fixture, 2, success, nil); err == nil {
		t.Fatal("success envelope with usage exit accepted")
	}
	if err := validateEnvelopes(fixture, 3, nil, errorStream); err == nil {
		t.Fatal("usage envelope with validation exit accepted")
	}
	fixture.Argv = []string{"--output", "none", "validate"}
	if err := validateEnvelopes(fixture, 0, nil, nil); err == nil {
		t.Fatal("validate --output none accepted")
	}
	fixture.Argv = []string{"validate", "--output", "json"}
	if err := validateEnvelopes(fixture, 0, success, nil); err == nil {
		t.Fatal("post-command --output accepted")
	}
}

func TestArtifactRejectsTemplatesAndMaliciousFilesystemDeclarations(t *testing.T) {
	if _, err := expand([]byte("{{unapproved}}"), expandRequest{}); err == nil {
		t.Fatal("unapproved output template accepted")
	}
	if err := validateFS(fsSpec{Entries: []entry{{Path: "{{escape}}", Kind: "file", Bytes: "", Mode: "0600"}}}); err == nil {
		t.Fatal("template filesystem path accepted")
	}
	if err := validateFS(fsSpec{Entries: []entry{{Path: "safe", Kind: "symlink", Target: "../escape"}}}); err == nil {
		t.Fatal("unsafe filesystem link target accepted")
	}
}

func TestUUIDV7TemplatesAreStrictAndFixtureScoped(t *testing.T) {
	valid := "018f0000-0000-7000-8000-000000000001"
	for _, invalid := range []string{
		"018f0000-0000-6000-8000-000000000001", // version
		"018f0000-0000-7000-c000-000000000001", // variant
		"018F0000-0000-7000-8000-000000000001", // case
	} {
		t.Run(invalid, func(t *testing.T) {
			if err := matchUUIDTemplate([]byte("{{UUIDV7:bind:id}}"), []byte(invalid), newUUIDBindings()); err == nil {
				t.Fatal("noncanonical UUIDv7 binding accepted")
			}
		})
	}
	bindings := newUUIDBindings()
	if err := matchUUIDTemplate([]byte("{{UUIDV7:bind:id}}"), []byte(valid), bindings); err != nil {
		t.Fatal(err)
	}
	if err := matchUUIDTemplate([]byte("{{UUIDV7:ref:id}}"), []byte(valid), bindings); err != nil {
		t.Fatal(err)
	}
	if err := declareUUIDTemplate([]byte("{{UUIDV7:ref:missing}}"), newUUIDBindings()); err == nil {
		t.Fatal("undefined UUIDV7 reference accepted")
	}
	if err := declareUUIDTemplate([]byte("{{UUIDV7:bind:id}}{{UUIDV7:bind:id}}"), newUUIDBindings()); err == nil {
		t.Fatal("rebound UUIDV7 name accepted")
	}
}

func TestUUIDV7BindingRendersBeforeFinalRevisionHash(t *testing.T) {
	id := "018f0000-0000-7000-8000-000000000001"
	final := fsSpec{Entries: []entry{{Path: "sidecar.comments", Kind: "file", BytesTemplate: "id={{UUIDV7:ref:id}}"}}}
	contents := []byte("id=" + id)
	sum := sha256.Sum256(contents)
	actual := []byte(id + " sha256:" + fmt.Sprintf("%x", sum))
	expected := []byte("{{UUIDV7:bind:id}} {{sha256:final:sidecar.comments}}")
	if err := matchRuntimeTemplate(expected, actual, newUUIDBindings(), fsSpec{}, final); err != nil {
		t.Fatalf("UUID-bound final revision must hash rendered bytes: %v", err)
	}
}

func TestUUIDV7TemplatesRejectUnsafePlacement(t *testing.T) {
	status := 0
	fixture := golden{Argv: []string{"{{UUIDV7:bind:id}}"}, Env: []envEntry{}, Stdin: "", Initial: fsSpec{Entries: []entry{}}, Expect: expected{Status: &status, Stdout: stringPointer(""), Stderr: stringPointer(""), Final: fsSpec{Entries: []entry{}}}}
	if err := validateUUIDTemplates(fixture); err == nil {
		t.Fatal("UUIDV7 argv template accepted")
	}
	fixture.Argv = []string{"version"}
	fixture.Initial = fsSpec{Entries: []entry{{Path: "input", Kind: "file", Bytes: "e3tVVUlEVjc6YmluZDppZH19", Mode: "0600"}}}
	if err := validateUUIDTemplates(fixture); err != nil {
		t.Fatalf("literal UUIDV7-looking filesystem bytes rejected: %v", err)
	}
	fixture.Initial = fsSpec{Entries: []entry{{Path: "input", Kind: "file", BytesTemplate: "{{UUIDV7:bind:id}}", Mode: "0600"}}}
	if err := validateUUIDTemplates(fixture); err == nil {
		t.Fatal("UUIDV7 initial filesystem template accepted")
	}
}
func stringPointer(value string) *string { return &value }

func TestRequiredRedInventoryIsExplicitlyUnavailable(t *testing.T) {
	root := repoRootForTest(t)
	obligations, err := RequiredRedObligations(filepath.Join(root, "contracts/corpus/v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obligations) == 0 {
		t.Fatal("required-red obligations were silently omitted")
	}
	registry, err := validateRequiredRedRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Scenarios) != 72 {
		t.Fatalf("pack scenarios omitted: got %d want 72", len(registry.Scenarios))
	}
	if err = requireCapabilities([]string{"process_orchestration"}); err == nil {
		t.Fatal("unavailable orchestration capability was accepted")
	}
}

func TestSeedInventoryRejectsOrphanedCoverageMapping(t *testing.T) {
	root := repoRootForTest(t)
	manifest, err := decodeCorpus(filepath.Join(root, "contracts/corpus/v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := decodeInventory(filepath.Join(root, "contracts/behavior-inventory-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Fixtures[0].Coverage[0] = "BHV-NOT-IN-SEED"
	if err := validateCoverageMapping(manifest, inventory); err == nil {
		t.Fatal("orphaned corpus coverage mapping accepted")
	}
}
func TestFixtureMaterializationIsDeterministic(t *testing.T) {
	root := t.TempDir()
	fixture := fsSpec{Entries: []entry{
		{Path: "dir", Kind: "directory", Mode: "0755"},
		{Path: "dir/original", Kind: "file", Bytes: "c2hhcmVk", Mode: "0600"},
		{Path: "dir/linked", Kind: "hardlink", Target: "dir/original"},
	}}
	if err := materialize(materializeRequest{Root: root, FS: fixture}); err != nil {
		t.Fatal(err)
	}
	if err := assertFS(assertFSRequest{Root: root, Want: fixture}); err != nil {
		t.Fatal(err)
	}
}

func TestControlledConformingProcessIsAcceptedForItsFixture(t *testing.T) {
	root := repoRootForTest(t)
	binary := filepath.Join(t.TempDir(), "conforming")
	build := exec.Command("go", "build", "-o", binary, "./contracts/testbin/conforming")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build conforming test subject: %v: %s", err, output)
	}
	fixture, err := decodeGolden(filepath.Join(root, "contracts/goldens/v1/version-json.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = execute(Config{Binary: binary, CLIVersion: "test", Timeout: time.Second}, fixture); err != nil {
		t.Fatalf("controlled conforming process rejected: %v", err)
	}
}

func TestNonconformantExecutableIsRejected(t *testing.T) {
	root := repoRootForTest(t)
	binary := filepath.Join(t.TempDir(), "nonconformant")
	build := exec.Command("go", "build", "-o", binary, "./contracts/testbin/nonconformant")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nonconformant binary: %v: %s", err, output)
	}
	var stdout, stderr bytes.Buffer
	report, err := Run(Config{CorpusPath: filepath.Join(root, "contracts/corpus/v1.json"), Binary: binary, CLIVersion: "test", Timeout: time.Second, Stdout: &stdout, Stderr: &stderr})
	if err == nil || report.Failed == 0 {
		t.Fatalf("nonconformant executable passed: report=%+v stdout=%s stderr=%s", report, stdout.String(), stderr.String())
	}
	if report.Passed != 0 {
		t.Fatalf("nonconformant executable unexpectedly passed %d fixture(s)", report.Passed)
	}
	launchedAndRejected := false
	for _, failure := range report.Failures {
		switch failure.Class {
		case FailureConformance:
			launchedAndRejected = true
		case FailureArtifact, FailureProcess, FailureTimeout:
			t.Fatalf("nonconformant executable was not proved nonconformant: infrastructure failure=%+v stdout=%s stderr=%s", failure, stdout.String(), stderr.String())
		}
	}
	if !launchedAndRejected {
		t.Fatalf("nonconformant executable was not launched and rejected for a conformance mismatch: report=%+v stdout=%s stderr=%s", report, stdout.String(), stderr.String())
	}
}
