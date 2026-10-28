package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectApplyMaximumRequest(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	operation := `{"op":"message_update","thread_id":"` + threadID + `","message_id":"` + messageID + `","body":"x"}`
	operations := make([]string, 256)
	results := make([]string, 256)
	message := `{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}`
	thread := `{"id":"` + threadID + `","location":{"kind":"file"},"messages":[` + message + `]}`
	for i := range operations {
		operations[i] = operation
		results[i] = fmt.Sprintf(`{"index":%d,"op":"message_update","changed":false,"result":{"thread":%s,"message":%s}}`, i, thread, message)
	}
	request := `{"protocol":1,"operations":[` + strings.Join(operations, ",") + `]}`
	request += strings.Repeat(" ", (1<<20)-len(request))
	if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
		t.Fatal(err)
	}
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}, {Path: "operations", Mode: 0600, Bytes: []byte(request)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "apply", "--file", "note.txt.comments", "--operations-file", "operations"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	want := fmt.Sprintf(`{"protocol":1,"command":"apply","changed":false,"result":{"operations":[%s],"storage_revision":%q,"canonical_revision":%q}}`+"\n", strings.Join(results, ","), revision, revision)
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
