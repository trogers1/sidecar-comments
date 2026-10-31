//go:build (!darwin || !arm64) && (!linux || !amd64)

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

// Uses the real CLI entry point and real platform capability, not a capability
// override. A cross-compiled test binary can execute on an unsupported runtime.
func TestTrackingUnsupportedMutationsRejectBeforeIO(t *testing.T) {
	for _, command := range []string{"sync", "editor-update"} {
		for _, directory := range []bool{false, true} {
			for _, missing := range []bool{false, true} {
				t.Run(command+map[bool]string{false: "/regular", true: "/directory"}[directory]+map[bool]string{false: "/unreadable-source", true: "/missing-source"}[missing], func(t *testing.T) {
					dir := t.TempDir()
					source := filepath.Join(dir, "note.txt")
					target := source + ".comments"
					const contents = "must remain untouched"
					if directory {
						if err := os.Mkdir(target, 0700); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(target, []byte(contents), 0600); err != nil {
						t.Fatal(err)
					}
					if !missing {
						if err := os.WriteFile(source, []byte(contents), 0000); err != nil {
							t.Fatal(err)
						}
					}
					before, err := os.ReadDir(dir)
					if err != nil {
						t.Fatal(err)
					}
					baseline := make([]os.FileInfo, len(before))
					for i, entry := range before {
						baseline[i], err = entry.Info()
						if err != nil {
							t.Fatal(err)
						}
					}
					var stdout, stderr bytes.Buffer
					args := []string{"--output", "json", command, "--file", target}
					var input []byte
					if command == "editor-update" {
						deleted := true
						request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: domain.RevisionMissing, SourceRevision: domain.HashRevision(domain.HashRevisionRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Deleted: &deleted}}}
						input, err = json.Marshal(request)
						if err != nil {
							t.Fatal(err)
						}
						if err := domain.ValidateDTO(domain.ValidateDTORequest{Bytes: input, Kind: "editor"}); err != nil {
							t.Fatal(err)
						}
						args = append(args, "--operations-file", "-")
					}
					status := Run(Request{Args: args, Stdin: bytes.NewReader(input), Stdout: &stdout, Stderr: &stderr})
					if status != domain.ExitIO || stdout.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(`"operation":"unsupported_secure_filesystem"`)) {
						t.Fatalf("capability did not precede target/source I/O: status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
					}
					after, err := os.ReadDir(dir)
					if err != nil {
						t.Fatal(err)
					}
					if len(before) != len(after) {
						t.Fatal("mutation created filesystem entries")
					}
					for i := range before {
						first := baseline[i]
						last, err := after[i].Info()
						if err != nil {
							t.Fatal(err)
						}
						if first.Name() != last.Name() || first.Mode() != last.Mode() || first.Size() != last.Size() {
							t.Fatal("mutation changed filesystem entries")
						}
					}
					if !directory {
						data, err := os.ReadFile(target)
						if err != nil || string(data) != contents {
							t.Fatalf("target changed: %q %v", data, err)
						}
					}
					if !missing {
						if err := os.Chmod(source, 0600); err != nil {
							t.Fatal(err)
						}
						data, err := os.ReadFile(source)
						if err != nil || string(data) != contents {
							t.Fatalf("source changed: %q %v", data, err)
						}
					}
				})
			}
		}
	}
}

func TestTrackingUnsupportedCheckRemainsReadOnly(t *testing.T) {
	dir := t.TempDir()
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	status := Run(Request{Args: []string{"--output", "json", "sync", "--file", filepath.Join(dir, "note.txt.comments"), "--check"}, Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr})
	if status != 0 || stderr.Len() != 0 || !bytes.Contains(stdout.Bytes(), []byte(`"changed":false`)) {
		t.Fatalf("check rejected: status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read-only check created filesystem entries")
	}
}

func TestTrackingUnsupportedMalformedRequestPrecedesCapability(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	status := Run(Request{Args: []string{"--output", "json", "editor-update", "--file", filepath.Join(dir, "note.txt.comments"), "--operations-file", "-"}, Stdin: bytes.NewBufferString("{"), Stdout: &stdout, Stderr: &stderr})
	if status != domain.ExitUsage || stdout.Len() != 0 || !bytes.Contains(stderr.Bytes(), []byte(`usage:operations:malformed`)) {
		t.Fatalf("syntax precedence changed: status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("invalid request changed filesystem")
	}
}
