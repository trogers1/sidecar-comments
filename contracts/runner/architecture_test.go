package runner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modulePath                = "github.com/trogers1/sidecar-comments/"
	architectureTestFile      = "contracts/runner/architecture_test.go"
	architectureRulesTestFile = "contracts/runner/architecture_rules_test.go"
	// docs/v1/index.md permits this exact package for the production NFC verifier.
	// That contract also requires it to become a pinned direct dependency when used.
	authorizedNFCVerifierImport = "golang.org/x/text/unicode/norm"
)

// TestArchitectureBoundaries keeps the contracts-only generic JSON adapter
// narrow and makes the planned production package graph enforceable before the
// packages exist.
func TestArchitectureBoundaries(t *testing.T) {
	root := repoRootForTest(t)
	adapter := "contracts/runner/schema.go"
	var violations []string
	// Deliberately visit only source roots owned by this module. In particular,
	// do not descend through ignored caches (such as .tmp/module-cache), Go
	// toolchain testdata, or generated output that may happen to contain Go.
	for _, sourceRoot := range []string{"contracts", "cmd", "internal", "tests/cli"} {
		directory := filepath.Join(root, sourceRoot)
		if _, statErr := os.Stat(directory); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			t.Fatal(statErr)
		}
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", ".tmp", "bin", "generated", "node_modules", "testdata", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".go" {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			// The architecture tests contain intentionally forbidden examples.
			if rel == architectureTestFile || rel == architectureRulesTestFile {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileViolations, err := architectureViolations(rel, data, adapter)
			if err != nil {
				return err
			}
			violations = append(violations, fileViolations...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) != 0 {
		t.Fatalf("architecture boundary violation(s): %s", strings.Join(violations, "; "))
	}
}

// architectureViolations is separate from the filesystem walk so the rules can
// be exercised against small synthetic source files. It intentionally checks
// source text for ../config: a runtime path assembled from that literal is just
// as prohibited as an import path.
func architectureViolations(rel string, data []byte, adapter string) ([]string, error) {
	var violations []string
	if strings.Contains(string(data), "../config") {
		violations = append(violations, rel+": runtime ../config reference is forbidden")
	}

	file, err := parser.ParseFile(token.NewFileSet(), rel, data, 0)
	if err != nil {
		return nil, err
	}
	isAdapter := rel == adapter
	isContract := strings.HasPrefix(rel, "contracts/")
	isProduction := !strings.HasSuffix(rel, "_test.go") && (strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "internal/"))
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, "\"")
		if importPath == "github.com/santhosh-tekuri/jsonschema/v6" && !isAdapter && rel != "tests/cli/schema_test.go" {
			violations = append(violations, rel+": jsonschema import is reserved for "+adapter+" and tests/cli/schema_test.go")
		}
		if isContract && importsInternal(importPath) {
			violations = append(violations, rel+": contracts must not import internal package "+importPath)
		}
		if !isProduction {
			continue
		}
		switch importPath {
		case "unsafe":
			violations = append(violations, rel+": production unsafe import is forbidden")
		case "reflect":
			// A complete static analysis of reflection would be fragile; production
			// imports are banned so reflection cannot become a dispatch mechanism.
			violations = append(violations, rel+": production reflect import is forbidden")
		}
		if strings.Contains(importPath, ".") && !strings.HasPrefix(importPath, modulePath) && importPath != authorizedNFCVerifierImport {
			violations = append(violations, rel+": external production import "+importPath)
		}
		if strings.HasPrefix(importPath, modulePath) && !allowedProductionImport(rel, strings.TrimPrefix(importPath, modulePath)) {
			violations = append(violations, rel+": forbidden dependency direction to "+importPath)
		}
	}
	if isAdapter {
		return violations, nil
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.Ident:
			if value.Name == "any" {
				violations = append(violations, rel+": plain any is reserved for "+adapter)
			}
		case *ast.InterfaceType:
			if value.Methods != nil && len(value.Methods.List) == 0 {
				violations = append(violations, rel+": generic interface{} is reserved for "+adapter)
			}
		case *ast.MapType:
			if identifier, ok := value.Value.(*ast.Ident); ok && identifier.Name == "any" {
				violations = append(violations, rel+": map[string]any is reserved for "+adapter)
			}
		}
		return true
	})
	return violations, nil
}

func importsInternal(importPath string) bool {
	return importPath == "internal" || strings.HasPrefix(importPath, "internal/") ||
		strings.Contains(importPath, "/internal/") || strings.HasSuffix(importPath, "/internal")
}

func allowedProductionImport(from, to string) bool {
	from = strings.TrimSuffix(from, ".go")
	switch {
	case strings.HasPrefix(from, "cmd/sidecar/"):
		return to == "internal/cli"
	case strings.HasPrefix(from, "internal/cli/"):
		return to == "internal/domain" || to == "internal/format" || to == "internal/operations" || to == "internal/store"
	case strings.HasPrefix(from, "internal/operations/"):
		return to == "internal/domain"
	case strings.HasPrefix(from, "internal/store/"):
		return to == "internal/domain" || to == "internal/format"
	case strings.HasPrefix(from, "internal/format/"):
		return to == "internal/domain"
	case strings.HasPrefix(from, "internal/domain/"):
		return false
	}
	return false
}
