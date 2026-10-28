package cli_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectFixIDOutput(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	opening := "[@th_" + threadID + " lost]\n"
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(opening + "---user\nx\n" + endThread)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "id", "fix", "--add-missing-message-ids", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(string(got.Stdout), "\n")
	quoted, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	// Validate the extracted value against the existing MessageID definition via
	// a real closed ApplyRequest, rather than redefining the UUID grammar.
	request := `{"protocol":1,"operations":[{"op":"message_delete","thread_id":"` + threadID + `","message_id":` + string(quoted) + `}]}`
	if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(id + "\n")}}); err != nil {
		t.Fatal(err)
	}
	document := []byte(opening + "---user <!-- sidecar-meta: {\"id\":\"" + id + "\"} -->\nx\n" + endThread)
	initial[1].Bytes = document
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
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
