package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectUpdateAndAuthorGuard(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("y\n")}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "add", "--file", "note.txt.comments", "--thread", threadID, "--author-kind", "human", "--body-file", "body", "--id", "018f0000-0000-7000-8000-000000000003", "--if-last-author-kind", "agent"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"precondition","message":"precondition:message-add:if_last_author_kind","details":{"operation":"message-add","reason":"if_last_author_kind"}}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 6, Stderr: []byte(want)}}); err != nil {
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
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "update", "--if-revision", revision, "--file", "note.txt.comments", "--thread", threadID, "--id", messageID, "--body-file", "body"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	updated := []byte(strings.Replace(canonicalNote, "x\n", "y\n", 1))
	updatedRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(updated))
	message := `{"id":"` + messageID + `","author":{"kind":"human"},"body":"y"}`
	thread := `{"id":"` + threadID + `","location":{"kind":"file"},"messages":[` + message + `]}`
	want = fmt.Sprintf(`{"protocol":1,"command":"message-update","changed":true,"result":{"thread":%s,"message":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, message, updatedRevision, updatedRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[3].Bytes = updated
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectDoubleSuffixSource(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.comments.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x\n")}, {Path: "note.comments", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.comments.comments", Mode: 0640}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.comments.comments", "--location", "file", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	thread := `{"id":"` + threadID + `","location":{"kind":"file"},"messages":[{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}]}`
	want := fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[3].Bytes = []byte(canonicalNote)
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
