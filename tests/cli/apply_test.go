package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Sequential operations observe the working copy; a failure at index 1 rolls
// back the successful index 0 create. A later successful create/update/delete
// batch leaves no sidecar, while reporting each intermediate result exactly.
func TestSubjectApplyLifecycle(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	create := `{"op":"thread_create","thread_id":"` + threadID + `","message_id":"` + messageID + `","location":{"kind":"lost"},"author":{"kind":"human"},"body":"x"}`
	deletion := `{"op":"thread_delete","thread_id":"` + threadID + `"}`
	rollback := `{"protocol":1,"operations":[` + create + `,{"op":"message_update","thread_id":"` + threadID + `","message_id":"018f0000-0000-7000-8000-000000000003","body":"y"}]}`
	success := `{"protocol":1,"operations":[` + create + `,{"op":"message_update","thread_id":"` + threadID + `","message_id":"` + messageID + `","body":"y\n"},` + deletion + `]}`
	if err := validateSchema(schemaRequest{JSON: []byte(rollback)}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: []byte(success)}); err != nil {
		t.Fatal(err)
	}
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "rollback.json", Mode: 0600, Bytes: []byte(rollback)}, {Path: "success.json", Mode: 0600, Bytes: []byte(success)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "apply", "--if-revision", "missing", "--file", "note.txt.comments", "--operations-file", "rollback.json"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"not_found","message":"not_found:message","details":{"entity":"message","id":"018f0000-0000-7000-8000-000000000003","index":1}}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 5, Stderr: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "apply", "--if-revision", "missing", "--file", "note.txt.comments", "--operations-file", "success.json"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	firstMessage := `{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}`
	lastMessage := `{"id":"` + messageID + `","author":{"kind":"human"},"body":"y\n"}`
	firstThread := `{"id":"` + threadID + `","location":{"kind":"lost"},"messages":[` + firstMessage + `]}`
	lastThread := `{"id":"` + threadID + `","location":{"kind":"lost"},"messages":[` + lastMessage + `]}`
	emptyRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	want = fmt.Sprintf(`{"protocol":1,"command":"apply","changed":true,"result":{"operations":[{"index":0,"op":"thread_create","changed":true,"result":{"thread":%s}},{"index":1,"op":"message_update","changed":true,"result":{"thread":%s,"message":%s}},{"index":2,"op":"thread_delete","changed":true,"result":{"deleted_thread_id":%q}}],"storage_revision":null,"canonical_revision":%q}}`+"\n", firstThread, lastThread, lastMessage, threadID, emptyRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
