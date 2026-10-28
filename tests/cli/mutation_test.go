package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSubjectMessageCascadeLifecycle(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("reply\n")}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	const replyID = "018f0000-0000-7000-8000-000000000003"
	const firstMessage = `{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}`
	const reply = `{"id":"` + replyID + `","author":{"kind":"agent"},"body":"reply"}`
	bothThread := `{"id":"` + threadID + `","location":{"kind":"file"},"messages":[` + firstMessage + `,` + reply + `]}`
	bothBytes := []byte("[@th_" + threadID + " file]\n" + role + "x\n\n---bot <!-- sidecar-meta: {\"id\":\"" + replyID + "\"} -->\nreply\n" + endThread)
	bothRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(bothBytes))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "add", "--file", "note.txt.comments", "--thread", threadID, "--author-kind", "agent", "--body-file", "body", "--id", replyID, "--if-last-author-kind", "human"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"message-add","changed":true,"result":{"thread":%s,"message":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", bothThread, reply, bothRevision, bothRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[3].Bytes = bothBytes
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	// Updating an equal normalized body is a semantic no-op.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "update", "--if-revision", bothRevision, "--file", "note.txt.comments", "--thread", threadID, "--id", replyID, "--body-file", "body"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"message-update","changed":false,"result":{"thread":%s,"message":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", bothThread, reply, bothRevision, bothRevision)
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
	// Delete the reply without deleting the thread or reporting a previous revision.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "delete", "--file", "note.txt.comments", "--thread", threadID, "--id", replyID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	originalRevision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	want = fmt.Sprintf(`{"protocol":1,"command":"message-delete","changed":true,"result":{"deleted_message_id":%q,"storage_revision":%q,"canonical_revision":%q}}`+"\n", replyID, originalRevision, originalRevision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[3].Bytes = []byte(canonicalNote)
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	// Deleting the final message cascades through the final thread to unlink.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "message", "delete", "--file", "note.txt.comments", "--thread", threadID, "--id", messageID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	emptyRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	want = fmt.Sprintf(`{"protocol":1,"command":"message-delete","changed":true,"result":{"deleted_thread_id":%q,"deleted_message_id":%q,"previous_storage_revision":%q,"storage_revision":null,"canonical_revision":%q}}`+"\n", threadID, messageID, originalRevision, emptyRevision)
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
	if err := compareFiles(filesExpectation{Got: files, Want: initial[:3]}); err != nil {
		t.Fatal(err)
	}
}
