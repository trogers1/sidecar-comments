//go:build (darwin && arm64) || (linux && amd64)

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/store"
)

type diskExpectation struct {
	T       *testing.T
	Path    string
	Bytes   []byte
	Deleted bool
}

func checkTransactionDisk(r diskExpectation) {
	r.T.Helper()
	got, err := os.ReadFile(r.Path)
	if r.Deleted {
		if !errors.Is(err, os.ErrNotExist) {
			r.T.Fatalf("deleted target bytes=%q error=%v", got, err)
		}
	} else {
		if err != nil || !bytes.Equal(got, r.Bytes) {
			r.T.Fatalf("target bytes=%q error=%v want=%q", got, err, r.Bytes)
		}
		info, err := os.Stat(r.Path)
		if err != nil {
			r.T.Fatal(err)
		}
		if info.Mode().Perm() != 0640 {
			r.T.Fatalf("mode=%v", info.Mode())
		}
	}
	lock := filepath.Join(filepath.Dir(r.Path), "."+filepath.Base(r.Path)+".sidecar.lock")
	lockBytes, err := os.ReadFile(lock)
	if err != nil || len(lockBytes) != 0 {
		r.T.Fatalf("lock bytes=%q error=%v", lockBytes, err)
	}
	lockInfo, err := os.Stat(lock)
	if err != nil {
		r.T.Fatal(err)
	}
	if lockInfo.Mode().Perm() != 0600 {
		r.T.Fatalf("lock mode=%v", lockInfo.Mode())
	}
	entries, err := os.ReadDir(filepath.Dir(r.Path))
	if err != nil {
		r.T.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(r.Path) && entry.Name() != filepath.Base(strings.TrimSuffix(r.Path, ".comments")) && entry.Name() != "."+filepath.Base(r.Path)+".sidecar.lock" {
			r.T.Fatalf("unexpected transaction artifact %q", entry.Name())
		}
	}
}

func TestRunRealStoreFaults(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		for _, stage := range []store.StorageOperation{{Operation: "write", Path: "temp"}, {Operation: "sync", Path: "parent"}, {Operation: "close", Path: "lock"}} {
			if deleting && stage.Path == "temp" {
				continue
			}
			name := "replace/"
			if deleting {
				name = "delete/"
			}
			t.Run(name+stage.Operation+"/"+stage.Path, func(t *testing.T) {
				target := filepath.Join(t.TempDir(), "note.txt.comments")
				old := []byte(transactionDocument + "\n")
				next := []byte(transactionDocument)
				args := []string{"--output", "json", "format", "--file", target}
				revision := domain.HashRevision(domain.HashRevisionRequest{Bytes: next})
				success := fmt.Sprintf(`{"protocol":1,"command":"format","changed":true,"result":{"formatted":true,"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
				if deleting {
					old = []byte(transactionDocument)
					next = nil
					args = []string{"--output", "json", "thread", "delete", "--file", target, "--id", threadID}
					previous := domain.HashRevision(domain.HashRevisionRequest{Bytes: old})
					empty := domain.HashRevision(domain.HashRevisionRequest{})
					success = fmt.Sprintf(`{"protocol":1,"command":"thread-delete","changed":true,"result":{"deleted_thread_id":%q,"previous_storage_revision":%q,"storage_revision":null,"canonical_revision":%q}}`+"\n", threadID, previous, empty)
				}
				if err := os.WriteFile(target, old, 0640); err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				fired := 0
				status := Run(Request{Args: args, Stdin: bytes.NewReader(nil), Stdout: &stdout, Stderr: &stderr, begin: func(r store.BeginRequest) (*store.Transaction, error) {
					r.Fault = func(event store.StorageOperation) error {
						if event == stage {
							fired++
							return syscall.EIO
						}
						return nil
					}
					return store.Begin(r)
				}})
				if fired != 1 {
					t.Fatalf("fault count=%d", fired)
				}
				wantStatus := domain.ExitIO
				wantOut := ""
				wantErr := `{"protocol":1,"error":{"code":"io","message":"io:write","details":{"operation":"write","path":"temp"}}}` + "\n"
				expected := next
				deleted := deleting
				if stage.Path == "temp" {
					expected = old
					deleted = false
				} else if stage.Path == "parent" {
					wantErr = `{"protocol":1,"error":{"code":"io","message":"io:fsync_parent","details":{"operation":"fsync_parent","path":"parent","commit_may_have_succeeded":true}}}` + "\n"
				} else {
					wantStatus = 0
					wantOut = success
					wantErr = ""
				}
				if status != wantStatus || stdout.String() != wantOut || stderr.String() != wantErr {
					t.Fatalf("status=%d stdout=%q stderr=%q; want status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String(), wantStatus, wantOut, wantErr)
				}
				checkTransactionDisk(diskExpectation{T: t, Path: target, Bytes: expected, Deleted: deleted})
			})
		}
	}
}

func TestRunApplyRevalidatesAtTransactionBoundary(t *testing.T) {
	for _, changed := range []string{"revision", "missing-created", "source"} {
		t.Run(changed, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "note.txt")
			target := source + ".comments"
			document := []byte(transactionDocument)
			oldRevision := domain.HashRevision(domain.HashRevisionRequest{Bytes: document})
			if changed == "missing-created" {
				oldRevision = domain.RevisionMissing
			} else if err := os.WriteFile(target, document, 0640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("one\ntwo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// A body update precedes the location-bearing operation; neither may leak
			// to disk when revalidation rejects the batch.
			body := fmt.Sprintf(`{"protocol":1,"if_revision":%q,"operations":[{"op":"message_update","thread_id":%q,"message_id":%q,"body":"must not persist"},{"op":"thread_move","thread_id":%q,"location":{"kind":"line","line":2}}]}`, oldRevision, threadID, messageID, threadID)
			expected := document
			if changed == "revision" {
				expected = bytes.Replace(document, []byte("\nx\n"), []byte("\nconcurrent edit\n"), 1)
			}
			statusWant := domain.ExitConflict
			current := domain.HashRevision(domain.HashRevisionRequest{Bytes: expected})
			errWant := fmt.Sprintf(`{"protocol":1,"error":{"code":"conflict","message":"conflict:revision","details":{"kind":"revision","expected":%q,"current_storage_revision":%q}}}`+"\n", oldRevision, current)
			if changed == "source" {
				statusWant = domain.ExitValidation
				errWant = `{"protocol":1,"error":{"code":"validation","message":"validation:source_coordinate:location","details":{"path":"source_coordinate","reason":"location"}}}` + "\n"
			}
			calls := 0
			var stdout, stderr bytes.Buffer
			status := Run(Request{Args: []string{"--output", "json", "apply", "--file", target, "--operations-file", "-"}, Stdin: bytes.NewBufferString(body), Stdout: &stdout, Stderr: &stderr, begin: func(r store.BeginRequest) (*store.Transaction, error) {
				calls++
				if changed == "source" {
					if err := os.WriteFile(source, []byte("one\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(target, expected, 0640); err != nil {
					t.Fatal(err)
				}
				return store.Begin(r)
			}})
			if calls != 1 || status != statusWant || stdout.Len() != 0 || stderr.String() != errWant {
				t.Fatalf("calls=%d status=%d stdout=%q stderr=%q; want status=%d stderr=%q", calls, status, stdout.String(), stderr.String(), statusWant, errWant)
			}
			checkTransactionDisk(diskExpectation{T: t, Path: target, Bytes: expected})
			expectedSource := "one\ntwo\n"
			if changed == "source" {
				expectedSource = "one\n"
			}
			gotSource, err := os.ReadFile(source)
			if err != nil || string(gotSource) != expectedSource {
				t.Fatalf("source=%q error=%v", gotSource, err)
			}
		})
	}
}
