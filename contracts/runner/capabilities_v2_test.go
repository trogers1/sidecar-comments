package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2CapabilityDesignCatalogSchemaAndTypedDispatchAgree(t *testing.T) {
	root := repoRootForTest(t)
	schemaPath := filepath.Join(root, capabilityV2Schema)
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := compileSchemaValidator("https://github.com/trogers1/sidecar-comments/contracts/schema/corpus-v2.json", []schemaResource{{Location: "https://github.com/trogers1/sidecar-comments/contracts/schema/corpus-v2.json", Source: schemaBytes}})
	if err != nil {
		t.Fatalf("compile v2 schema: %v", err)
	}
	catalogBytes, err := os.ReadFile(filepath.Join(root, "contracts/packs/capabilities-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = validator.validate(catalogBytes); err != nil {
		t.Fatalf("catalog schema validation: %v", err)
	}
	catalog, err := decodeCapabilityCatalogV2(catalogBytes)
	if err != nil {
		t.Fatalf("typed catalog validation: %v", err)
	}
	if len(catalog.Capabilities) != len(v2InputKinds) {
		t.Fatalf("catalog has %d capabilities, dispatch has %d", len(catalog.Capabilities), len(v2InputKinds))
	}

	for _, source := range v2FixtureExamples() {
		if err := validator.validate([]byte(source)); err != nil {
			t.Fatalf("schema rejected typed fixture %s: %v", source, err)
		}
		fixture, err := decodeControlledFixtureV2([]byte(source))
		if err != nil {
			t.Fatalf("typed decoder rejected fixture %s: %v", source, err)
		}
		if fixture.Input.inputKindV2() == "" {
			t.Fatal("typed fixture has no dispatch kind")
		}
	}
}

func TestV2ControlledInputsAndCapabilityStatesAreClosed(t *testing.T) {
	unknownInput := `{"fixture_version":2,"id":"unknown","capability":"x","input":{"kind":"shell","argv":["x"]}}`
	if _, err := decodeControlledFixtureV2([]byte(unknownInput)); err == nil {
		t.Fatal("unknown input variant accepted")
	}
	unknownField := `{"fixture_version":2,"id":"unknown","capability":"x","input":{"kind":"editor","argv":["ed"],"protocol":"file-argument-v1","exit_status":0,"extra":true}}`
	if _, err := decodeControlledFixtureV2([]byte(unknownField)); err == nil {
		t.Fatal("unknown input field accepted")
	}
	root := repoRootForTest(t)
	catalog, err := os.ReadFile(filepath.Join(root, "contracts/packs/capabilities-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	badStatus := strings.Replace(string(catalog), `"status": "unavailable"`, `"status": "ready"`, 1)
	if _, err := decodeCapabilityCatalogV2([]byte(badStatus)); err == nil {
		t.Fatal("unknown capability state accepted")
	}
	provenWithoutProof := strings.Replace(string(catalog), `"status": "unavailable"`, `"status": "proven"`, 1)
	if _, err := decodeCapabilityCatalogV2([]byte(provenWithoutProof)); err == nil {
		t.Fatal("proven capability without proof accepted")
	}
	declared, err := decodeCapabilityCatalogV2(catalog)
	if err != nil {
		t.Fatal(err)
	}
	declared.Capabilities[0].Status = "proven"
	declared.Capabilities[0].UnavailableReason = ""
	declared.Capabilities[0].Proof = &capabilityProofV2{Method: "claim", Artifact: "fake"}
	if err := validateCapabilityCatalogV2(declared); err == nil {
		t.Fatal("proof-shaped metadata promoted an unavailable declarative capability")
	}
}

func v2FixtureExamples() []string {
	return []string{
		`{"fixture_version":2,"id":"blob","capability":"deterministic_blob_limit_input","input":{"kind":"deterministic_blob","name":"payload","algorithm":"sha256-counter-v1","seed_b64":"c2VlZA==","byte_length":9,"limit":8}}`,
		`{"fixture_version":2,"id":"actors","capability":"process_orchestration","input":{"kind":"actors_barriers","actors":[{"id":"writer","argv":["sidecar","write"],"ready_barrier":"ready","release_barrier":"go"},{"id":"reader","argv":["sidecar","read"],"ready_barrier":"ready","release_barrier":"go"}],"barriers":["ready","go"],"release_order":["go"],"timeout_ms":1000}}`,
		`{"fixture_version":2,"id":"lock","capability":"held_lock_identity_replacement","input":{"kind":"lock_identity_replacement","lock_path":"target.lock","holder_actor":"writer","at_barrier":"held","replacement":{"kind":"replace_target","object_kind":"file"}}}`,
		`{"fixture_version":2,"id":"fault","capability":"fault_injection","input":{"kind":"fault_injection","scope":"filesystem","operation":"fsync","occurrence":1,"outcome":{"kind":"errno","errno":"EIO"}}}`,
		`{"fixture_version":2,"id":"platform","capability":"platform_object_umask_control","input":{"kind":"platform_object_umask","platform":{"goos":"linux","goarch":"amd64"},"object_kind":"file","umask":"0022"}}`,
		`{"fixture_version":2,"id":"editor","capability":"editor_control","input":{"kind":"editor","argv":["editor","--wait"],"protocol":"file-argument-v1","exit_status":0}}`,
	}
}
