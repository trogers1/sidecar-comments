package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectReadMoveLifecycle(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	thread := `{"id":"` + threadID + `","location":{"kind":"file"},"messages":[{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}]}`
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "list", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"thread-list","changed":false,"result":{"threads":[%s],"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "get", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"thread-get","changed":false,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	lostBytes := []byte(strings.Replace(canonicalNote, " file]", " lost]", 1))
	lostRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(lostBytes))
	lostThread := strings.Replace(thread, `"kind":"file"`, `"kind":"lost"`, 1)
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "move", "--if-revision", revision, "--file", "note.txt.comments", "--id", threadID, "--location", "lost"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"thread-move","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", lostThread, lostRevision, lostRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[2].Bytes = lostBytes
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "move", "--file", "note.txt.comments", "--id", threadID, "--location", "lost"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"thread-move","changed":false,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", lostThread, lostRevision, lostRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
