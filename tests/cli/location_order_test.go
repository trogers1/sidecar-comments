package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectLocationCanonicalOrder(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	// This literal sequence IS the expected ordering, not a comparator oracle.
	// 0IA= decodes to UTF-8 Ѐ. Its encoded spelling sorts before YQ==,
	// while its decoded bytes sort after ASCII a (YQ==).
	ordered := []struct {
		id       int
		location string
	}{
		{1, "file"}, {2, "line:2"}, {9, "line:2"}, {3, "line:10"},
		{4, "range:2:2-2:3"}, {5, "range:2:2-2:10"},
		{6, "range:2:2-2:10 | YQ=="}, {7, "range:2:2-2:10 | 0IA="},
		{8, "range:2:2-2:11"}, {10, "range:2:10-2:11"}, {11, "range:10:1-10:2"}, {12, "lost"},
	}
	threads := make([]string, len(ordered))
	for i, row := range ordered {
		tid := fmt.Sprintf("018f0000-0000-7000-8000-%012x", row.id)
		mid := fmt.Sprintf("018f0001-0000-7000-8000-%012x", row.id)
		threads[i] = "[@th_" + tid + " " + row.location + "]\n---user <!-- sidecar-meta: {\"id\":\"" + mid + "\"} -->\nx\n[/@th_" + tid + "]\n"
	}
	canonical := []byte(strings.Join(threads, "\n"))
	shuffled := []string{threads[11], threads[7], threads[3], threads[2], threads[10], threads[9], threads[8], threads[6], threads[5], threads[4], threads[1], threads[0]}
	original := []byte(strings.Join(shuffled, "\n"))
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("preserve\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: original}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	storage := fmt.Sprintf("sha256:%x", sha256.Sum256(original))
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"format","changed":false,"result":{"formatted":false,"storage_revision":%q,"canonical_revision":%q}}`+"\n", storage, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"format","changed":true,"result":{"formatted":true,"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[2].Bytes = canonical
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
