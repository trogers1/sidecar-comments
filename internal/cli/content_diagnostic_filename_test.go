package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestRunLocatedDiagnosticFilenameSafety(t *testing.T) {
	for _, tc := range []struct {
		name, filename string
		quoted         bool
	}{
		{name: "ordinary", filename: "ordinary name.txt.comments"},
		{name: "escape", filename: "escape\x1b[31m.txt.comments", quoted: true},
		{name: "c1", filename: "c1\u0085.txt.comments", quoted: true},
		{name: "tab", filename: "tab\tname.txt.comments", quoted: true},
		{name: "newline", filename: "line\nname.txt.comments", quoted: true},
		{name: "delete", filename: "delete\x7f.txt.comments", quoted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, tc.filename)
			const input = "malformed sidecar\n"
			if err := os.WriteFile(target, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			status := Run(Request{Args: []string{"--output", "text", "validate", "--file", target}, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
			if status != domain.ExitValidation || stdout.Len() != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
			filename := target
			if tc.quoted {
				filename = fmt.Sprintf("%q", target)
			}
			prefix := filename + ":1:1:"
			if !strings.Contains(stderr.String(), prefix) {
				t.Fatalf("missing safe location prefix %q: %q", prefix, stderr.String())
			}
			if strings.ContainsFunc(filename, unicode.IsControl) {
				t.Fatalf("raw control in filename prefix: %q", filename)
			}
			// Structural newlines are expected, but no other control may escape into text.
			for _, r := range stderr.String() {
				if unicode.IsControl(r) && r != '\n' {
					t.Fatalf("raw control %U in output: %q", r, stderr.String())
				}
			}
			if tc.quoted && strings.Contains(stderr.String(), target+":1:1:") {
				t.Fatalf("raw filename leaked into location: %q", stderr.String())
			}
			if !tc.quoted && strings.Contains(stderr.String(), fmt.Sprintf("%q:1:1:", target)) {
				t.Fatalf("ordinary filename unexpectedly quoted: %q", stderr.String())
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != input {
				t.Fatalf("target changed: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("validation created entries: %v %v", entries, err)
			}
		})
	}
}
