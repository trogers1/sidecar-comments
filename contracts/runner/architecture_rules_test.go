package runner

import (
	"strings"
	"testing"
)

func TestArchitectureRulesAllowAuthorizedImports(t *testing.T) {
	violations := architectureViolationsForTest(t, "internal/domain/nfc.go", `package domain
import "golang.org/x/text/unicode/norm"
var isNFC = norm.NFC.IsNormalString
`)
	if len(violations) != 0 {
		t.Fatalf("authorized NFC verifier import was rejected: %v", violations)
	}
}

func TestArchitectureRulesRejectForbiddenImportsAndPaths(t *testing.T) {
	tests := []struct {
		name     string
		rel      string
		source   string
		contains string
	}{
		{
			name:     "unsafe in production",
			rel:      "internal/store/unsafe.go",
			source:   "package store\nimport \"unsafe\"\n",
			contains: "production unsafe import is forbidden",
		},
		{
			name:     "reflection in production",
			rel:      "cmd/sidecar/main.go",
			source:   "package main\nimport \"reflect\"\n",
			contains: "production reflect import is forbidden",
		},
		{
			name:     "other x text package",
			rel:      "internal/domain/text.go",
			source:   "package domain\nimport \"golang.org/x/text/width\"\n",
			contains: "external production import golang.org/x/text/width",
		},
		{
			name:     "contract importing internal",
			rel:      "contracts/runner/leak.go",
			source:   "package runner\nimport \"github.com/trogers1/sidecar-comments/internal/domain\"\n",
			contains: "contracts must not import internal package",
		},
		{
			name:     "runtime config path",
			rel:      "internal/cli/config.go",
			source:   "package cli\nconst configPath = \"../config\"\n",
			contains: "runtime ../config reference is forbidden",
		},
		{
			name:     "invalid production dependency direction",
			rel:      "cmd/sidecar/main.go",
			source:   "package main\nimport \"github.com/trogers1/sidecar-comments/internal/domain\"\n",
			contains: "forbidden dependency direction",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			violations := architectureViolationsForTest(t, test.rel, test.source)
			if !strings.Contains(strings.Join(violations, "; "), test.contains) {
				t.Fatalf("violations %v do not contain %q", violations, test.contains)
			}
		})
	}
}

func architectureViolationsForTest(t *testing.T, rel, source string) []string {
	t.Helper()
	violations, err := architectureViolations(rel, []byte(source), "contracts/runner/schema.go")
	if err != nil {
		t.Fatalf("architectureViolations(%q): %v", rel, err)
	}
	return violations
}
