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

// Concrete fields mirror VersionResult and FixAssignmentDTO in the normative
// schema. Schema validation precedes extraction; exact reconstruction then
// rejects extra/duplicate fields, wrong ordering/escaping, and stray bytes.
func TestSubjectVersion(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "version"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result struct {
			CLIVersion string `json:"cli_version"`
		} `json:"result"`
	}
	if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
		t.Fatal(err)
	}
	var version strings.Builder
	version.WriteByte('"')
	for _, r := range envelope.Result.CLIVersion {
		switch r {
		case '"':
			version.WriteString(`\"`)
		case '\\':
			version.WriteString(`\\`)
		case '\b':
			version.WriteString(`\b`)
		case '\f':
			version.WriteString(`\f`)
		case '\n':
			version.WriteString(`\n`)
		case '\r':
			version.WriteString(`\r`)
		case '\t':
			version.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&version, `\u%04x`, r)
			} else {
				version.WriteRune(r)
			}
		}
	}
	version.WriteByte('"')
	want := `{"protocol":1,"command":"version","changed":false,"result":{"cli_version":` + version.String() + `,"format_version":1,"protocol":1}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectFixRetry(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	opening := "[@th_" + threadID + " lost]\n"
	firstRepair := opening + "---user\nx\n" + endThread
	const secondThreadID = "018f0000-0000-7000-8000-000000000004"
	secondOpening := "[@th_" + secondThreadID + " lost]\n"
	secondEnd := "[/@th_" + secondThreadID + "]\n"
	repair := firstRepair + "\n" + secondOpening + "---bot\ny\n" + secondEnd
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(repair)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "fix", "--add-missing-message-ids", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Result struct {
			Assignments []struct {
				MessageID string `json:"message_id"`
			} `json:"assignments"`
		} `json:"result"`
	}
	if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.Assignments) != 2 {
		t.Fatalf("want two assignments, got %d", len(envelope.Result.Assignments))
	}
	id := envelope.Result.Assignments[0].MessageID
	secondID := envelope.Result.Assignments[1].MessageID
	if id >= secondID {
		t.Fatal("fix IDs must be distinct and strictly increasing within one process")
	}
	// Only the unpredictable UUID comes from the validated response. Every disk
	// byte and revision is independently reconstructed, never read as an oracle.
	document := []byte(opening + "---user <!-- sidecar-meta: {\"id\":\"" + id + "\"} -->\nx\n" + endThread + "\n" + secondOpening + "---bot <!-- sidecar-meta: {\"id\":\"" + secondID + "\"} -->\ny\n" + secondEnd)
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
	want := fmt.Sprintf(`{"protocol":1,"command":"fix","changed":true,"result":{"assignments":[{"thread_id":%q,"message_ordinal":1,"line":2,"byte_offset":%d,"message_id":%q},{"thread_id":%q,"message_ordinal":1,"line":7,"byte_offset":%d,"message_id":%q}],"storage_revision":%q,"canonical_revision":%q}}`+"\n", threadID, len(opening)+1, id, secondThreadID, len(firstRepair)+1+len(secondOpening)+1, secondID, revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	initial[1].Bytes = document
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "fix", "--add-missing-message-ids", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":1,"command":"fix","changed":false,"result":{"assignments":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
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
	// Deterministic id-mode no-op and normal fix-check allocate nothing.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "id", "fix", "--add-missing-message-ids", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{}}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "fix", "--add-missing-message-ids", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
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
