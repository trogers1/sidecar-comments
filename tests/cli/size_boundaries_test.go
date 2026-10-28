package cli_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectSourceSizeBoundary(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, size := range []int{16 << 20, (16 << 20) + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dir := t.TempDir()
			source := bytes.Repeat([]byte("x"), size)
			f, err := os.OpenFile(filepath.Join(dir, "note.txt"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write(source); err != nil {
				t.Fatal(err)
			}
			if err := f.Chmod(0640); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if err := writeFile(fileRequest{Path: filepath.Join(dir, "note.txt.comments"), Mode: 0640, Bytes: []byte(canonicalNote)}); err != nil {
				t.Fatal(err)
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
			want := processResult{Stdout: []byte(fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision))}
			if size > 16<<20 {
				want = processResult{Status: 3, Stderr: []byte(`{"protocol":1,"error":{"code":"validation","message":"validation:source_size:too_large","details":{"path":"source_size","reason":"too_large"}}}` + "\n")}
			}
			if err := compareProcess(processExpectation{Got: got, Want: want}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: append(append([]byte{}, got.Stdout...), got.Stderr...)}); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 2 || entries[0].Name() != "note.txt" || entries[1].Name() != "note.txt.comments" {
				t.Fatal("unexpected directory entries")
			}
			for _, f := range []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: source}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}} {
				info, err := os.Lstat(filepath.Join(dir, f.Path))
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, f.Path))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode() != f.Mode || !bytes.Equal(data, f.Bytes) {
					t.Fatalf("fixture changed: %s", f.Path)
				}
			}
		})
	}
}

func TestSubjectBodyMaximum(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	body := strings.Repeat("x", 65536)
	document := []byte(strings.Replace(canonicalNote, "x\n", body+"\n", 1))
	initial := []fileRequest{{Path: "note.txt.comments", Mode: 0640, Bytes: document}}
	if err := writeFile(fileRequest{Path: filepath.Join(dir, initial[0].Path), Mode: initial[0].Mode, Bytes: document}); err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "get", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"thread-get","changed":false,"result":{"thread":{"id":%q,"location":{"kind":"file"},"messages":[{"id":%q,"author":{"kind":"human"},"body":%q}]},"storage_revision":%q,"canonical_revision":%q}}`+"\n", threadID, messageID, body, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
