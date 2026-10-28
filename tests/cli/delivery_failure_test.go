package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const deliveryFailureStatus = 7

// Broken pipes exercise real process delivery, not a mocked CLI writer.
func TestSubjectDeliveryFailure(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct {
		name   string
		args   []string
		stream failureStream
	}{
		{name: "result_stdout", args: []string{"--output", "json", "version"}, stream: stdoutFailure},
		{name: "error_stderr", args: []string{"--output", "json", "thread", "get", "--file", "absent.comments", "--id", threadID}, stream: stderrFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096, FailOutput: tc.stream})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: deliveryFailureStatus}}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// A delivery failure is not rollback: the durable result must be reread.
func TestSubjectCommittedDeliveryFailure(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{
		{Path: ".note.txt.comments.sidecar.lock", Mode: 0600},
		{Path: "body", Mode: 0600, Bytes: []byte("x\n")},
	}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096, FailOutput: stdoutFailure})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: deliveryFailureStatus}}); err != nil {
		t.Fatal(err)
	}
	canonical := strings.Replace(canonicalNote, " file]", " lost]", 1)
	want := append(initial, fileRequest{Path: "note.txt.comments", Mode: 0600, Bytes: []byte(canonical)})
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: want}); err != nil {
		t.Fatal(err)
	}
	// Read the committed thread through the public entry point, rather than replaying create.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "get", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonical)))
	body := strings.TrimSuffix(string(initial[1].Bytes), "\n")
	expected := fmt.Sprintf(`{"protocol":1,"command":"thread-get","changed":false,"result":{"thread":{"id":%q,"location":{"kind":"lost"},"messages":[{"id":%q,"author":{"kind":"human"},"body":%q}]},"storage_revision":%q,"canonical_revision":%q}}`+"\n", threadID, messageID, body, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(expected)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: want}); err != nil {
		t.Fatal(err)
	}
}
