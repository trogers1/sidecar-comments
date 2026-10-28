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

func TestSubjectTargetNames(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, name := range []string{".comments", "note.txt", "note.comments.tmp", "note.COMMENTS", "note.comments/"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: "source", Mode: 0640, Bytes: []byte("preserve\n")}}
			if err := writeFile(fileRequest{Path: filepath.Join(dir, "source"), Mode: initial[0].Mode, Bytes: initial[0].Bytes}); err != nil {
				t.Fatal(err)
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", name}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want := `{"protocol":1,"error":{"code":"validation","message":"validation:target_name:invalid","details":{"path":"target_name","reason":"invalid"}}}` + "\n"
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stderr: []byte(want)}}); err != nil {
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
		})
	}
}

// Nonregular objects are deliberately checked directly: observeFiles is not
// extended into a recursive or special-object snapshot framework.
func TestSubjectUnsafeTargetObjects(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, kind := range []string{"symlink", "directory", "fifo", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "note.txt.comments")
			source := filepath.Join(dir, "note.txt")
			original := []byte("preserve\n")
			if err := writeFile(fileRequest{Path: source, Mode: 0640, Bytes: original}); err != nil {
				t.Fatal(err)
			}
			reason := ""
			switch kind {
			case "symlink":
				if err := os.Symlink("note.txt", target); err != nil {
					t.Fatal(err)
				}
				reason = "symlink"
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				reason = "directory"
			case "fifo":
				if err := syscall.Mkfifo(target, 0600); err != nil {
					t.Fatal(err)
				}
				reason = "nonregular"
			case "hardlink":
				if err := os.Link(source, target); err != nil {
					t.Fatal(err)
				}
				reason = "multilink"
			default:
				t.Fatal("unknown fixture object")
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf(`{"protocol":1,"error":{"code":"validation","message":"validation:target_type:%s","details":{"path":"target_type","reason":%q}}}`+"\n", reason, reason)
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
			if len(entries) != 2 || entries[0].Name() != "note.txt" || entries[1].Name() != "note.txt.comments" {
				t.Fatalf("unexpected directory entries: %v", entries)
			}
			after, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("target object replaced or mode changed")
			}
			if kind == "symlink" {
				link, err := os.Readlink(target)
				if err != nil {
					t.Fatal(err)
				}
				if link != "note.txt" {
					t.Fatalf("link changed: %q", link)
				}
			}
			if kind == "directory" {
				children, err := os.ReadDir(target)
				if err != nil {
					t.Fatal(err)
				}
				if len(children) != 0 {
					t.Fatal("directory target changed")
				}
			}
			info, err := os.Lstat(source)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode() != 0640 {
				t.Fatal("source is not the expected regular file")
			}
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, original) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestSubjectOversizedTarget(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	target := filepath.Join(dir, "note.txt.comments")
	original := bytes.Repeat([]byte("x"), (4<<20)+1)
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"validation","message":"validation:target_size:too_large","details":{"path":"target_size","reason":"too_large"}}}` + "\n"
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
	if len(entries) != 1 || entries[0].Name() != "note.txt.comments" {
		t.Fatal("unexpected directory entries")
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0640 || !bytes.Equal(data, original) {
		t.Fatal("oversized target changed")
	}
}
