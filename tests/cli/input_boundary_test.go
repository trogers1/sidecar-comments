package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectMaximumBodyAndMetadataInput(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	gitlab := `{"kind":"system_note","project_id":1,"note_id":1}`
	metadata := gitlab + strings.Repeat(" ", 16384-len(gitlab))
	raw := "x" + strings.Repeat("\r\n", 65536)
	logical := "x" + strings.Repeat("\n", 65535)
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte(raw)}, {Path: "metadata", Mode: 0600, Bytes: []byte(metadata)}, {Path: "note.txt.comments", Mode: 0640}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	document := []byte("[@th_" + threadID + " lost]\n---user <!-- sidecar-meta: {\"id\":\"" + messageID + "\",\"gitlab\":" + gitlab + "} -->\n" + logical + "\n" + endThread)
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID, "--gitlab-json", "metadata"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	thread := fmt.Sprintf(`{"id":%q,"location":{"kind":"lost"},"messages":[{"id":%q,"author":{"kind":"human"},"body":%q,"gitlab":%s}]}`, threadID, messageID, logical, gitlab)
	want := fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	initial[3].Bytes = document
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
