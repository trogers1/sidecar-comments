//go:build (darwin && arm64) || (linux && amd64)

package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type transactionFixtureRequest struct{ T *testing.T }

func transactionFixture(r transactionFixtureRequest) (*Transaction, string) {
	r.T.Helper()
	path := filepath.Join(r.T.TempDir(), "note.comments")
	if err := os.WriteFile(path, []byte("old bytes"), 0640); err != nil {
		r.T.Fatal(err)
	}
	tx, err := Begin(BeginRequest{Path: path, Timeout: time.Second})
	if err != nil {
		r.T.Fatal(err)
	}
	r.T.Cleanup(func() {
		if err := tx.Close(TransactionCloseRequest{}); err != nil {
			r.T.Error(err)
		}
	})
	if _, err := tx.Read(TransactionReadRequest{}); err != nil {
		r.T.Fatal(err)
	}
	return tx, path
}

// These failures cannot be induced portably at the CLI boundary (especially
// fsync and close). Exercise the real on-disk transaction, injecting only one
// syscall result; all other IO, locking, identity checks and cleanup stay real.
func TestTransactionFaults(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		for _, stage := range []storageOperation{{"write", "temp"}, {"sync", "temp"}, {"close", "temp"}, {"rename", "target"}, {"unlink", "target"}, {"sync", "parent"}} {
			if deleting && stage.Path == "temp" || deleting && stage.Operation == "rename" || !deleting && stage.Operation == "unlink" {
				continue
			}
			name := "replace/"
			if deleting {
				name = "delete/"
			}
			t.Run(name+stage.Operation+"/"+stage.Path, func(t *testing.T) {
				tx, path := transactionFixture(transactionFixtureRequest{T: t})
				fired := false
				tx.fault = func(r storageOperation) error {
					if r == stage {
						fired = true
						return syscall.EIO
					}
					return nil
				}
				next := []byte("new bytes")
				if deleting {
					next = nil
				}
				if stage.Operation == "write" {
					tx.fault = func(r storageOperation) error {
						if r != stage {
							return nil
						}
						fired = true
						// Model a partial write before EIO in the actual temp.
						temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
						if err != nil || len(temps) != 1 {
							t.Fatalf("temps=%v error=%v", temps, err)
						}
						if err := os.WriteFile(temps[0], []byte("partial"), 0600); err != nil {
							t.Fatal(err)
						}
						return syscall.EIO
					}
				}
				err := tx.Commit(CommitRequest{Bytes: next})
				operation := stage.Operation
				if operation == "sync" && stage.Path == "temp" {
					operation = "write"
				}
				request := domain.IORequest{Operation: operation, Path: stage.Path}
				if stage.Path == "parent" {
					yes := true
					request.Operation = "fsync_parent"
					request.CommitMayHaveSucceeded = &yes
				}
				if !fired || !reflect.DeepEqual(err, domain.IO(request)) {
					t.Fatalf("fired=%v error=%#v want=%#v", fired, err, domain.IO(request))
				}
				got, readErr := os.ReadFile(path)
				if stage.Path != "parent" {
					if readErr != nil || !bytes.Equal(got, []byte("old bytes")) {
						t.Fatalf("precommit bytes=%q error=%v", got, readErr)
					}
				} else if deleting {
					if !errors.Is(readErr, os.ErrNotExist) {
						t.Fatalf("delete error=%v", readErr)
					}
				} else if readErr != nil || !bytes.Equal(got, next) {
					t.Fatalf("committed bytes=%q error=%v", got, readErr)
				}
				matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
				if err != nil || len(matches) != 0 {
					t.Fatalf("temps=%v error=%v", matches, err)
				}
				tx.fault = nil
			})
		}
	}
}

func TestTransactionDurableCleanup(t *testing.T) {
	for _, deleting := range []bool{false, true} {
		for _, stage := range []storageOperation{{"close", "parent"}, {"unlock", "lock"}, {"close", "lock"}} {
			name := "replace/"
			if deleting {
				name = "delete/"
			}
			t.Run(name+stage.Operation+"/"+stage.Path, func(t *testing.T) {
				tx, path := transactionFixture(transactionFixtureRequest{T: t})
				fired := false
				tx.fault = func(r storageOperation) error {
					if r == stage {
						fired = true
						return syscall.EIO
					}
					return nil
				}
				next := []byte("new bytes")
				if deleting {
					next = nil
				}
				if err := tx.Commit(CommitRequest{Bytes: next}); err != nil {
					t.Fatal(err)
				}
				if err := tx.Close(TransactionCloseRequest{}); err != nil {
					t.Fatal(err)
				}
				if !fired {
					t.Fatal("fault not exercised")
				}
				got, err := os.ReadFile(path)
				if deleting {
					if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("delete: %v", err)
					}
				} else if err != nil || !bytes.Equal(got, next) {
					t.Fatalf("bytes=%q error=%v", got, err)
				}
			})
		}
	}
}

// The barrier runs only after a failed real flock, not after a guessed delay.
// This tests the store boundary; command-level revision/source ordering remains
// the responsibility of the CLI/operations behavioral suite.
func TestTransactionLockWaitSnapshot(t *testing.T) {
	for _, changed := range []string{"revision", "source"} {
		t.Run(changed, func(t *testing.T) {
			tx, path := transactionFixture(transactionFixtureRequest{T: t})
			source := filepath.Join(filepath.Dir(path), "note")
			if err := os.WriteFile(source, []byte("one\ntwo\n"), 0600); err != nil {
				t.Fatal(err)
			}
			observed := false
			next, err := begin(beginRequest{BeginRequest: BeginRequest{Path: path, Timeout: time.Second}, waiting: func() {
				observed = true
				changedPath, content := path, "new revision"
				if changed == "source" {
					changedPath, content = source, "one\n"
				}
				if err := os.WriteFile(changedPath, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				if err := tx.Close(TransactionCloseRequest{}); err != nil {
					t.Fatal(err)
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := next.Close(TransactionCloseRequest{}); err != nil {
					t.Error(err)
				}
			}()
			if !observed {
				t.Fatal("never observed lock contention")
			}
			snapshot, err := next.Read(TransactionReadRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if changed == "revision" {
				want := domain.HashRevision(domain.HashRevisionRequest{Bytes: []byte("new revision")})
				if snapshot.Revision == nil || *snapshot.Revision != want || string(snapshot.Bytes) != "new revision" {
					t.Fatalf("stale snapshot: %#v", snapshot)
				}
			} else {
				err := ValidateSource(ValidateSourceRequest{Path: source, Locations: []domain.Location{{Kind: domain.LocationLine, Line: 2}}})
				want := domain.Validation(domain.ValidationRequest{Path: "source_coordinate", Reason: "location"})
				if !reflect.DeepEqual(err, want) {
					t.Fatalf("source error=%v want=%v", err, want)
				}
			}
		})
	}
}

func TestTransactionIdentityChanges(t *testing.T) {
	for _, kind := range []string{"target", "lock", "parent"} {
		for _, deleting := range []bool{false, true} {
			name := kind + "/replace"
			if deleting {
				name = kind + "/delete"
			}
			t.Run(name, func(t *testing.T) {
				tx, path := transactionFixture(transactionFixtureRequest{T: t})
				changed := path
				switch kind {
				case "lock":
					changed = filepath.Join(filepath.Dir(path), tx.lockName)
				case "parent":
					changed = filepath.Dir(path)
				}
				if err := os.Rename(changed, changed+".original"); err != nil {
					t.Fatal(err)
				}
				if kind == "parent" {
					if err := os.Mkdir(changed, 0700); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(changed, []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
				next := []byte("new bytes")
				if deleting {
					next = nil
				}
				err := tx.Commit(CommitRequest{Bytes: next})
				want := domain.Conflict(domain.ConflictRequest{Kind: "identity_changed"})
				if !reflect.DeepEqual(err, want) {
					t.Fatalf("error=%v want=%v", err, want)
				}
				original := path
				if kind == "parent" {
					original = filepath.Join(changed+".original", filepath.Base(path))
				} else if kind == "target" {
					original = changed + ".original"
				}
				got, err := os.ReadFile(original)
				if err != nil || string(got) != "old bytes" {
					t.Fatalf("original=%q error=%v", got, err)
				}
			})
		}
	}
}
