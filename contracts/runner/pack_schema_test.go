package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerShapedPackGoldenRejectsInvalidExpectedValidateResultDTO(t *testing.T) {
	root := repoRootForTest(t)
	sourcePath := filepath.Join(root, "contracts/packs/p6/goldens/p6-location-file.json")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	const validResult = `\"result\":{\"valid\":true,\"warnings\":[],\"storage_revision\":\"{{sha256:initial:a.txt.comments}}\",\"canonical_revision\":\"{{sha256:initial:a.txt.comments}}\"}`
	mutated := strings.Replace(string(source), validResult, `\"result\":{}`, 1)
	if mutated == string(source) {
		t.Fatal("P6 fixture no longer contains expected validate result DTO")
	}
	path := filepath.Join(t.TempDir(), "invalid-p6-golden.json")
	if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	validators, err := compilePackGoldenValidators(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePackGolden(path, "P6-TEST", "p6", validators); err == nil {
		t.Fatal("runner-shaped P6 golden accepted expected validate result:{}")
	}
}

func TestRunnerShapedPackGoldenRejectsInvalidExpectedErrorDTO(t *testing.T) {
	root := repoRootForTest(t)
	sourcePath := filepath.Join(root, "contracts/packs/p6/goldens/p6-source-bom.json")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(source), `\"path\":\"source_encoding\"`, `\"path\":\"not_a_validation_path\"`, 1)
	if mutated == string(source) {
		t.Fatal("P6 fixture no longer contains expected validation error DTO")
	}
	path := filepath.Join(t.TempDir(), "invalid-p6-error-golden.json")
	if err := os.WriteFile(path, []byte(mutated), 0o600); err != nil {
		t.Fatal(err)
	}
	validators, err := compilePackGoldenValidators(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePackGolden(path, "P6-TEST", "p6", validators); err == nil {
		t.Fatal("runner-shaped P6 golden accepted invalid expected error DTO")
	}
}
