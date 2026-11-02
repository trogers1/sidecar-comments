package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
)

func TestRunRoleHeaderHorizontalWhitespace(t *testing.T) {
	thread, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	message, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: thread, Location: domain.Location{Kind: domain.LocationFile}, Messages: []domain.Message{{ID: message, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "Body preserves spaces.  \nAnd tabs.\t"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	canonical := string(format.Encode(format.EncodeRequest{Document: value}))
	header := "---" + domain.RoleUser + ` <!-- sidecar-meta: {"id":"` + message + `"} -->`
	for _, bare := range []bool{false, true} {
		for _, suffix := range []string{" ", "\t", " \t  "} {
			t.Run(map[bool]string{false: "metadata", true: "bare"}[bare]+suffix, func(t *testing.T) {
				dir := t.TempDir()
				target := filepath.Join(dir, "note.txt.comments")
				replacement := header + suffix
				if bare {
					replacement = "---" + domain.RoleUser + suffix
				}
				input := strings.Replace(canonical, header, replacement, 1)
				if err := os.WriteFile(target, []byte(input), 0600); err != nil {
					t.Fatal(err)
				}
				run := func(args ...string) (int, string) {
					var out, stderr bytes.Buffer
					status := Run(Request{Args: append([]string{"--output", "json"}, args...), Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr})
					if stderr.Len() == 0 {
						return status, out.String()
					}
					return status, stderr.String()
				}
				status, stderr := run("validate", "--file", target)
				if bare {
					var response struct {
						Result struct {
							Valid  bool
							Repair domain.RepairDetails
						}
					}
					if err := json.Unmarshal([]byte(stderr), &response); err != nil {
						t.Fatal(err)
					}
					if status != domain.ExitValidation || response.Result.Valid || len(response.Result.Repair.Diagnostics) != 1 || response.Result.Repair.Diagnostics[0].Code != "missing_message_id" {
						t.Fatalf("repair response: %d %s", status, stderr)
					}
				} else if status != 0 {
					t.Fatalf("metadata validate: %d %s", status, stderr)
				}
				before, err := os.ReadFile(target)
				if err != nil || string(before) != input {
					t.Fatal("validation wrote target")
				}
				if bare {
					// Formatting recognizes the whitespace-bearing bare role as a
					// repair need, rather than a fatal syntax error.
					status, stderr = run("format", "--file", target)
					var response struct {
						Error struct {
							Code    string
							Details domain.RepairDetails
						}
					}
					if err := json.Unmarshal([]byte(stderr), &response); err != nil {
						t.Fatal(err)
					}
					if status != domain.ExitValidation || response.Error.Code != domain.CodeRepair || len(response.Error.Details.Diagnostics) != 1 {
						t.Fatalf("format repair response: %d %s", status, stderr)
					}
					unchanged, err := os.ReadFile(target)
					if err != nil || string(unchanged) != input {
						t.Fatal("format wrote repair-needed target")
					}
					status, stderr = run("fix", "--file", target, "--add-missing-message-ids")
				} else {
					status, stderr = run("format", "--file", target)
				}
				if status != 0 {
					t.Fatalf("normalize: %d %s", status, stderr)
				}
				after, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(after, []byte("Body preserves spaces.  \nAnd tabs.\t\n")) {
					t.Fatalf("body whitespace changed: %s", after)
				}
				for _, line := range strings.Split(string(after), "\n") {
					if strings.HasPrefix(line, "---") && strings.TrimRight(line, " \t") != line {
						t.Fatalf("role suffix not normalized: %q", line)
					}
				}
				if status, stderr = run("validate", "--file", target); status != 0 {
					t.Fatalf("normalized validate: %d %s", status, stderr)
				}
			})
		}
	}
}

func TestRunMalformedMutationDoesNotWrite(t *testing.T) {
	for _, command := range []string{"format", "fix"} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "note.txt.comments")
			const input = "<!-- sidecar-format: 2 -->\nnot a thread\n"
			if err := os.WriteFile(target, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			args := []string{"--output", "json", command, "--file", target}
			if command == "fix" {
				args = append(args, "--add-missing-message-ids")
			}
			status := Run(Request{Args: args, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr})
			if status != domain.ExitValidation || out.Len() != 0 || !strings.Contains(stderr.String(), `"diagnostics"`) {
				t.Fatalf("status=%d out=%s err=%s", status, out.String(), stderr.String())
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != input {
				t.Fatal("target changed")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != filepath.Base(target) && entry.Name() != "."+filepath.Base(target)+".sidecar.lock" {
					t.Fatalf("unexpected file: %s", entry.Name())
				}
			}
		})
	}
}
