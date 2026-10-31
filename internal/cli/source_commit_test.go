//go:build (darwin && arm64) || (linux && amd64)

package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/store"
)

type boundaryRunRequest struct {
	T              *testing.T
	Args           []string
	Input          string
	Begin          func(store.BeginRequest) (*store.Transaction, error)
	ExpectedStatus int
}

func boundaryRun(r boundaryRunRequest) ([]byte, []byte) {
	r.T.Helper()
	var stdout, stderr bytes.Buffer
	status := Run(Request{Args: append([]string{"--output", "json"}, r.Args...), Stdin: bytes.NewBufferString(r.Input), Stdout: &stdout, Stderr: &stderr, begin: r.Begin})
	if status != r.ExpectedStatus {
		r.T.Fatalf("args=%v status=%d want=%d stdout=%q stderr=%q", r.Args, status, r.ExpectedStatus, stdout.String(), stderr.String())
	}
	return stdout.Bytes(), stderr.Bytes()
}

// A real store fault hook changes only the source after the temporary sidecar
// has been written. This timing cannot be exercised reliably by racing a child
// process. No transaction, filesystem or source validator is mocked.
func TestRunRejectsSourceChangeAtFinalCommit(t *testing.T) {
	for _, command := range []string{"thread-create", "thread-move", "apply", "editor-update", "sync"} {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := filepath.Join(dir, "note.txt")
			target := sourcePath + ".comments"
			const initialSource = "prefix\nselected\nsuffix\n"
			if err := os.WriteFile(sourcePath, []byte(initialSource), 0600); err != nil {
				t.Fatal(err)
			}
			create := []string{"thread", "create", "--file", target, "--location", "range:2:1-2:9", "--author-kind", "human", "--body-file", "-", "--id", threadID, "--message-id", messageID}
			var original []byte
			if command != "thread-create" {
				boundaryRun(boundaryRunRequest{T: t, Args: create, Input: "Keep the conversation."})
				var err error
				original, err = os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
			}
			observedSource := initialSource
			if command == "sync" {
				observedSource = "above\n" + initialSource
				if err := os.WriteFile(sourcePath, []byte(observedSource), 0600); err != nil {
					t.Fatal(err)
				}
			}
			expectedFingerprint := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(observedSource)))
			changedSource := "source changed but coordinates still fit\nselected\nsuffix\n"
			currentFingerprint := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(changedSource)))
			args := create
			input := "Keep the conversation."
			switch command {
			case "thread-create":
			case "thread-move":
				args = []string{"thread", "move", "--file", target, "--id", threadID, "--location", "line:1"}
			case "apply":
				args = []string{"apply", "--file", target, "--operations-file", "-"}
				input = fmt.Sprintf(`{"protocol":%d,"operations":[{"op":"thread_move","thread_id":%q,"location":{"kind":"line","line":1}}]}`, domain.ProtocolVersion, threadID)
			case "editor-update":
				args = []string{"editor-update", "--file", target, "--operations-file", "-"}
				storage := fmt.Sprintf("sha256:%x", sha256.Sum256(original))
				input = fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[{"thread_id":%q,"location":{"kind":"line","line":1}}]}`, domain.ProtocolVersion, storage, expectedFingerprint, threadID)
			case "sync":
				args = []string{"sync", "--file", target}
			}
			changes := 0
			stdout, stderr := boundaryRun(boundaryRunRequest{T: t, Args: args, Input: input, ExpectedStatus: domain.ExitConflict, Begin: func(r store.BeginRequest) (*store.Transaction, error) {
				r.Fault = func(event store.StorageOperation) error {
					if event.Operation == "sync" && event.Path == "temp" {
						changes++
						if err := os.WriteFile(sourcePath, []byte(changedSource), 0600); err != nil {
							t.Fatal(err)
						}
					}
					return nil
				}
				return store.Begin(r)
			}})
			want := fmt.Sprintf(`{"protocol":%d,"error":{"code":"conflict","message":"conflict:source_revision","details":{"kind":"source_revision","expected":%q,"current_source_revision":%q}}}`+"\n", domain.ProtocolVersion, expectedFingerprint, currentFingerprint)
			if changes != 1 || len(stdout) != 0 || string(stderr) != want {
				t.Fatalf("changes=%d stdout=%q stderr=%q want=%q", changes, stdout, stderr, want)
			}
			got, err := os.ReadFile(target)
			if original == nil {
				if !os.IsNotExist(err) {
					t.Fatalf("failed create left target=%q error=%v", got, err)
				}
			} else if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("target changed: bytes=%q error=%v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != "note.txt" && entry.Name() != "note.txt.comments" && entry.Name() != ".note.txt.comments.sidecar.lock" {
					t.Fatalf("unexpected snapshot/temp/cache: %s", entry.Name())
				}
			}
		})
	}
}
