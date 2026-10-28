package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSubjectInvalidSourceAndLock(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, object := range []string{"source", "lock"} {
		for _, kind := range []string{"symlink", "directory", "fifo"} {
			t.Run(object+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				name := "note.txt"
				path := "source_type"
				args := []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}
				if object == "lock" {
					name = ".note.txt.comments.sidecar.lock"
					path = "lock_type"
					args = []string{"--output", "json", "thread", "delete", "--file", "note.txt.comments", "--id", threadID}
				}
				regular := []fileRequest{{Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}, {Path: "referent", Mode: 0640, Bytes: []byte("source\n")}}
				for _, f := range regular {
					if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
						t.Fatal(err)
					}
				}
				special := filepath.Join(dir, name)
				reason := ""
				switch kind {
				case "symlink":
					if err := os.Symlink("referent", special); err != nil {
						t.Fatal(err)
					}
					reason = "symlink"
				case "directory":
					if err := os.Mkdir(special, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(special, 0700); err != nil {
						t.Fatal(err)
					}
					reason = "directory"
				case "fifo":
					if err := syscall.Mkfifo(special, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(special, 0600); err != nil {
						t.Fatal(err)
					}
					reason = "nonregular"
				default:
					t.Fatal("unknown object")
				}
				before, err := os.Lstat(special)
				if err != nil {
					t.Fatal(err)
				}
				got, err := runProcess(runRequest{Binary: binary, Args: args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf(`{"protocol":1,"error":{"code":"validation","message":"validation:%s:%s","details":{"path":%q,"reason":%q}}}`+"\n", path, reason, path, reason)
				if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stderr: []byte(want)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != 3 {
					t.Fatalf("unexpected entries: %v", entries)
				}
				for _, entry := range entries {
					if entry.Name() != name && entry.Name() != regular[0].Path && entry.Name() != regular[1].Path {
						t.Fatalf("unexpected entry: %s", entry.Name())
					}
				}
				for _, f := range regular {
					info, err := os.Lstat(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					if info.Mode() != f.Mode || !bytes.Equal(data, f.Bytes) {
						t.Fatalf("regular fixture changed: %s", f.Path)
					}
				}
				after, err := os.Lstat(special)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(before, after) || after.Mode() != before.Mode() {
					t.Fatal("special object changed")
				}
				if kind == "symlink" {
					link, err := os.Readlink(special)
					if err != nil {
						t.Fatal(err)
					}
					if link != "referent" {
						t.Fatal("symlink changed")
					}
				}
				if kind == "directory" {
					entries, err := os.ReadDir(special)
					if err != nil {
						t.Fatal(err)
					}
					if len(entries) != 0 {
						t.Fatal("directory changed")
					}
				}
			})
		}
	}
}

func TestSubjectMissingSourcePrecedence(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	// Source open precedes a deliberately mismatched revision and entity lookup.
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "move", "--if-revision", "missing", "--file", "note.txt.comments", "--id", "018f0000-0000-7000-8000-000000000009", "--location", "file"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"io","message":"io:open","details":{"operation":"open","path":"source"}}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 7, Stderr: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
