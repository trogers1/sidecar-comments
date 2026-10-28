package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectApplyMoveAddDelete(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	const replyID = "018f0000-0000-7000-8000-000000000003"
	request := `{"protocol":1,"operations":[{"op":"thread_move","thread_id":"` + threadID + `","location":{"kind":"lost"}},{"op":"message_add","thread_id":"` + threadID + `","message_id":"` + replyID + `","author":{"kind":"agent"},"body":"reply","if_last_author_kind":"human"},{"op":"message_delete","thread_id":"` + threadID + `","message_id":"` + messageID + `"}]}`
	if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
		t.Fatal(err)
	}
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}, {Path: "operations", Mode: 0600, Bytes: []byte(request)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	human := `{"id":"` + messageID + `","author":{"kind":"human"},"body":"x"}`
	reply := `{"id":"` + replyID + `","author":{"kind":"agent"},"body":"reply"}`
	moved := `{"id":"` + threadID + `","location":{"kind":"lost"},"messages":[` + human + `]}`
	added := `{"id":"` + threadID + `","location":{"kind":"lost"},"messages":[` + human + `,` + reply + `]}`
	document := []byte("[@th_" + threadID + " lost]\n---bot <!-- sidecar-meta: {\"id\":\"" + replyID + "\"} -->\nreply\n" + endThread)
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "apply", "--file", "note.txt.comments", "--operations-file", "operations"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"apply","changed":true,"result":{"operations":[{"index":0,"op":"thread_move","changed":true,"result":{"thread":%s}},{"index":1,"op":"message_add","changed":true,"result":{"thread":%s,"message":%s}},{"index":2,"op":"message_delete","changed":true,"result":{"deleted_message_id":%q}}],"storage_revision":%q,"canonical_revision":%q}}`+"\n", moved, added, reply, messageID, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[2].Bytes = document
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectApplyIntermediateLimitRollback(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	var document strings.Builder
	document.WriteString("[@th_" + threadID + " file]\n")
	for i := 1; i <= 256; i++ {
		if i > 1 {
			document.WriteByte('\n')
		}
		document.WriteString(fmt.Sprintf("---user <!-- sidecar-meta: {\"id\":\"018f0001-0000-7000-8000-%012x\"} -->\nx\n", i))
	}
	document.WriteString(endThread)
	// The deletion at index 2 would restore the count, but validation must fail
	// immediately at index 1 and roll back the preceding move as well.
	request := `{"protocol":1,"operations":[{"op":"thread_move","thread_id":"` + threadID + `","location":{"kind":"lost"}},{"op":"message_add","thread_id":"` + threadID + `","message_id":"` + messageID + `","author":{"kind":"agent"},"body":"reply"},{"op":"message_delete","thread_id":"` + threadID + `","message_id":"018f0001-0000-7000-8000-000000000001"}]}`
	if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
		t.Fatal(err)
	}
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(document.String())}, {Path: "operations", Mode: 0600, Bytes: []byte(request)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "apply", "--file", "note.txt.comments", "--operations-file", "operations"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"precondition","message":"precondition:message_add:limit_messages_thread","details":{"operation":"message_add","reason":"limit_messages_thread","index":1}}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 6, Stderr: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
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
