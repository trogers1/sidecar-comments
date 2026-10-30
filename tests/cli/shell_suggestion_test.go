package cli_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Execute the displayed remedy through a real POSIX shell. Shell syntax in a
// filename must remain an argument, never a substitution or a second command.
func TestSubjectShellSuggestionsPreservePaths(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, command := range []string{"format", "fix"} {
		for _, name := range []string{"note$(touch marker-dollar).txt.comments", "note`touch marker-backtick`.txt.comments", "note 'single' and \"double\" space.txt.comments"} {
			t.Run(command+"/"+name, func(t *testing.T) {
				dir := t.TempDir()
				input := strings.Replace(canonicalNote, " file]", " lost]", 1)
				if command == "format" {
					input = strings.Replace(input, `{"id":`, `{ "id" : `, 1)
				} else {
					input = strings.Replace(input, role, "---user\n", 1)
				}
				if err := writeFile(fileRequest{Path: filepath.Join(dir, name), Mode: 0640, Bytes: []byte(input)}); err != nil {
					t.Fatal(err)
				}
				args := []string{command, "--file", name, "--check"}
				if command == "fix" {
					args = append(args, "--add-missing-message-ids")
				}
				got, err := runProcess(runRequest{Binary: binary, Dir: dir, Args: args, Timeout: 5 * time.Second, OutputLimit: 16384})
				if err != nil {
					t.Fatal(err)
				}
				if got.Status == 0 || len(got.Stderr) != 0 {
					t.Fatalf("expected check needing repair: %+v", got)
				}
				// Retain only the copyable command in the remedy line, excluding prose.
				suggestion := ""
				for _, line := range strings.Split(string(got.Stdout), "\n") {
					if _, after, ok := strings.Cut(line, "Run sidecar "); ok {
						suggestion = "sidecar " + strings.TrimSuffix(after, " to write them.")
						break
					}
				}
				if suggestion == "" {
					t.Fatalf("missing actionable shell suggestion: %s", got.Stdout)
				}
				// Supply the actual tested executable without relying on the caller's PATH.
				script := `binary=$1; sidecar() { "$binary" "$@"; }; ` + suggestion
				executed, err := runProcess(runRequest{Binary: "/bin/sh", Dir: dir, Args: []string{"-c", script, "sidecar-suggestion", binary}, Timeout: 5 * time.Second, OutputLimit: 16384})
				if err != nil {
					t.Fatal(err)
				}
				if executed.Status != 0 || len(executed.Stderr) != 0 {
					t.Fatalf("suggestion failed for literal path %q: command=%q stdout=%q stderr=%q", name, suggestion, executed.Stdout, executed.Stderr)
				}
				for _, marker := range []string{"marker-dollar", "marker-backtick"} {
					if _, err := os.Stat(filepath.Join(dir, marker)); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("shell substitution executed or unexpected marker error: %s: %v", marker, err)
					}
				}
				after, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if string(after) == input {
					t.Fatalf("suggestion did not change intended sidecar %q", name)
				}
				check, err := runProcess(runRequest{Binary: binary, Dir: dir, Args: args, Timeout: 5 * time.Second, OutputLimit: 16384})
				if err != nil {
					t.Fatal(err)
				}
				if check.Status != 0 || len(check.Stderr) != 0 {
					t.Fatalf("suggestion did not resolve check for %q: %+v", name, check)
				}
				machine, err := runProcess(runRequest{Binary: binary, Dir: dir, Args: []string{"--output", "json", "thread", "get", "--file", name, "--id", threadID}, Timeout: 5 * time.Second, OutputLimit: 16384})
				if err != nil {
					t.Fatal(err)
				}
				if machine.Status != 0 || len(machine.Stderr) != 0 {
					t.Fatalf("repaired sidecar must remain readable: %+v", machine)
				}
				if err := validateSchema(schemaRequest{JSON: machine.Stdout}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSubjectControlPathsHaveNoRunnableSuggestion(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, command := range []string{"format", "fix"} {
		for _, control := range []string{"\n", "\x1b", "\u009b"} {
			t.Run(fmt.Sprintf("%s/%q", command, control), func(t *testing.T) {
				dir := t.TempDir()
				name := "note" + control + ".txt.comments"
				input := strings.Replace(canonicalNote, " file]", " lost]", 1)
				if command == "format" {
					input = strings.Replace(input, `{"id":`, `{ "id" : `, 1)
				} else {
					input = strings.Replace(input, role, "---user\n", 1)
				}
				if err := writeFile(fileRequest{Path: filepath.Join(dir, name), Mode: 0640, Bytes: []byte(input)}); err != nil {
					t.Fatal(err)
				}
				args := []string{command, "--file", name, "--check"}
				if command == "fix" {
					args = append(args, "--add-missing-message-ids")
				}
				got, err := runProcess(runRequest{Binary: binary, Dir: dir, Args: args, Timeout: 5 * time.Second, OutputLimit: 16384})
				if err != nil {
					t.Fatal(err)
				}
				if got.Status == 0 || len(got.Stderr) != 0 {
					t.Fatalf("expected pending check: %+v", got)
				}
				text := string(got.Stdout)
				if strings.Contains(text, "sidecar "+command+" --file") {
					t.Errorf("control path must not produce concrete runnable command: %q", text)
				}
				for _, ch := range text {
					if ch != '\n' && (ch < 0x20 || ch == 0x7f || ch >= 0x80 && ch <= 0x9f) {
						t.Errorf("raw control U+%04X in path display: %q", ch, text)
					}
				}
				for _, term := range []string{fmt.Sprintf("%q", name), command, "control"} {
					if !strings.Contains(text, term) {
						t.Errorf("safe instruction missing %q: %q", term, text)
					}
				}
				after, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != input {
					t.Fatal("check mutated control-named file")
				}
			})
		}
	}
}
