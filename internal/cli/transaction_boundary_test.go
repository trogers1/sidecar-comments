//go:build (darwin && arm64) || (linux && amd64)

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/store"
)

const threadID = "018f0000-0000-7000-8000-000000000001"
const messageID = "018f0000-0000-7000-8000-000000000002"
const transactionDocument = "[@th_" + threadID + " file]\n---user <!-- sidecar-meta: {\"id\":\"" + messageID + "\"} -->\nx\n[/@th_" + threadID + "]\n"

// Run uses real parsing, rendering and transactions. The per-request constructor
// changes disk after initial validation and immediately before real acquisition;
// this is an ordering proof, not a claim of kernel lock contention.
func TestRunRevalidatesAtTransactionBoundary(t *testing.T) {
	document := []byte(transactionDocument)
	for _, changed := range []string{"revision", "missing-created", "source-coordinate", "source-encoding"} {
		t.Run(changed, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "note.txt")
			target := source + ".comments"
			if changed != "missing-created" {
				if err := os.WriteFile(target, document, 0640); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(source, []byte("one\ntwo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			oldRevision := domain.HashRevision(domain.HashRevisionRequest{Bytes: document})
			if changed == "missing-created" {
				oldRevision = domain.RevisionMissing
			}
			args := []string{"--output", "json", "thread", "move", "--if-revision", oldRevision, "--file", target, "--id", threadID, "--location", "line:2"}
			expectedTarget := document
			expectedStatus := domain.ExitValidation
			expectedStderr := `{"protocol":1,"error":{"code":"validation","message":"validation:source_coordinate:location","details":{"path":"source_coordinate","reason":"location"}}}` + "\n"
			if changed == "revision" || changed == "missing-created" {
				if changed == "revision" {
					expectedTarget = bytes.Replace(document, []byte("\nx\n"), []byte("\nconcurrent edit\n"), 1)
				}
				current := domain.HashRevision(domain.HashRevisionRequest{Bytes: expectedTarget})
				expectedStatus = domain.ExitConflict
				expectedStderr = fmt.Sprintf(`{"protocol":1,"error":{"code":"conflict","message":"conflict:revision","details":{"kind":"revision","expected":%q,"current_storage_revision":%q}}}`+"\n", oldRevision, current)
			} else if changed == "source-encoding" {
				expectedStderr = `{"protocol":1,"error":{"code":"validation","message":"validation:source_encoding:nul","details":{"path":"source_encoding","reason":"nul"}}}` + "\n"
			}
			calls := 0
			var stdout, stderr bytes.Buffer
			status := Run(Request{Args: args, Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr, begin: func(r store.BeginRequest) (*store.Transaction, error) {
				calls++
				if r.Path != target {
					t.Fatalf("transaction path=%q", r.Path)
				}
				if changed == "revision" || changed == "missing-created" {
					if err := os.WriteFile(target, expectedTarget, 0640); err != nil {
						t.Fatal(err)
					}
				} else {
					content := []byte("one\n")
					if changed == "source-encoding" {
						content = []byte("one\x00\ntwo\n")
					}
					if err := os.WriteFile(source, content, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return store.Begin(r)
			}})
			if calls != 1 {
				t.Fatalf("begin calls=%d; initial validation did not reach transaction boundary", calls)
			}
			if status != expectedStatus || stdout.Len() != 0 || stderr.String() != expectedStderr {
				t.Fatalf("status=%d stdout=%q stderr=%q; want status=%d stderr=%q", status, stdout.String(), stderr.String(), expectedStatus, expectedStderr)
			}
			if !json.Valid(stderr.Bytes()) {
				t.Fatal("invalid error JSON")
			}
			got, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(got, expectedTarget) {
				t.Fatalf("target=%q error=%v; want unchanged %q", got, err, expectedTarget)
			}
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0640 {
				t.Fatalf("target mode=%v", info.Mode())
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 3 {
				t.Fatalf("unexpected files (including leaked temp): %v", entries)
			}
		})
	}
}
