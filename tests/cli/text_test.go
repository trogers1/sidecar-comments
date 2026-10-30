package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type humanOutputExpectation struct {
	T      *testing.T
	Got    processResult
	Status int
	Terms  []string
	Error  bool
}

func assertHumanOutput(r humanOutputExpectation) {
	r.T.Helper()
	if r.Got.Status != r.Status {
		r.T.Fatalf("status=%d want %d; stdout=%s stderr=%s", r.Got.Status, r.Status, r.Got.Stdout, r.Got.Stderr)
	}
	text, other := r.Got.Stdout, r.Got.Stderr
	if r.Error {
		text, other = r.Got.Stderr, r.Got.Stdout
	}
	if len(other) != 0 {
		r.T.Fatalf("unexpected other stream: %q", other)
	}
	if len(bytes.TrimSpace(text)) == 0 || json.Valid(text) || bytes.Contains(text, []byte("result=")) || bytes.Contains(text, []byte(`"protocol":`)) {
		r.T.Fatalf("expected readable human output, got %q", text)
	}
	for _, term := range r.Terms {
		if !strings.Contains(strings.ToLower(string(text)), strings.ToLower(term)) {
			r.T.Errorf("output missing %q: %s", term, text)
		}
	}
}

// A complete default-output workflow exercises the executable, not renderer internals.
func TestSubjectHumanMutationLifecycle(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	for _, f := range []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "body", Mode: 0600, Bytes: []byte("Please clarify this note.\n")}} {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	const replyID = "018f0000-0000-7000-8000-000000000003"
	for _, tc := range []struct {
		name        string
		args, terms []string
	}{
		{"create", []string{"thread", "create", "--file", "note.txt.comments", "--location", "file", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}, []string{threadID, messageID, "Please clarify this note.", "human", "file"}},
		{"list", []string{"thread", "list", "--file", "note.txt.comments"}, []string{threadID, "Please clarify this note.", "file"}},
		{"get", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}, []string{threadID, messageID, "Please clarify this note."}},
		{"add", []string{"message", "add", "--file", "note.txt.comments", "--thread", threadID, "--author-kind", "agent", "--body-file", "body", "--id", replyID}, []string{threadID, replyID, "agent", "Please clarify this note."}},
		{"move", []string{"thread", "move", "--file", "note.txt.comments", "--id", threadID, "--location", "lost"}, []string{threadID, "lost"}},
		{"update", []string{"message", "update", "--file", "note.txt.comments", "--thread", threadID, "--id", replyID, "--body-file", "body"}, []string{replyID, "Please clarify this note."}},
		{"message_delete", []string{"message", "delete", "--file", "note.txt.comments", "--thread", threadID, "--id", replyID}, []string{"deleted", replyID}},
		{"thread_delete", []string{"thread", "delete", "--file", "note.txt.comments", "--id", threadID}, []string{"deleted", threadID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 16384})
			if err != nil {
				t.Fatal(err)
			}
			assertHumanOutput(humanOutputExpectation{T: t, Got: got, Terms: tc.terms})
		})
	}
}

func TestSubjectHumanMaintenance(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	noncanonical := strings.Replace(canonicalNote, `{"id":`, `{ "id" : `, 1)
	for _, f := range []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(noncanonical)}} {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name        string
		args, terms []string
		status      int
	}{
		{"version", []string{"version"}, []string{"Sidecar", "format", "protocol"}, 0},
		{"format_check_needed", []string{"format", "--file", "note.txt.comments", "--check"}, []string{"note.txt.comments", "format", "sidecar"}, 3},
		{"format", []string{"format", "--file", "note.txt.comments"}, []string{"formatted", "note.txt.comments"}, 0},
		{"format_check_clean", []string{"format", "--file", "note.txt.comments", "--check"}, []string{"passed", "note.txt.comments"}, 0},
		{"validate", []string{"validate", "--file", "note.txt.comments"}, []string{"validation", "passed", "note.txt.comments"}, 0},
		{"fix_check", []string{"fix", "--file", "note.txt.comments", "--add-missing-message-ids", "--check"}, []string{"passed", "note.txt.comments"}, 0},
		{"fix", []string{"fix", "--file", "note.txt.comments", "--add-missing-message-ids"}, []string{"message", "ID", "note.txt.comments"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			assertHumanOutput(humanOutputExpectation{T: t, Got: got, Status: tc.status, Terms: tc.terms})
		})
	}
	operations := `{"protocol":1,"operations":[{"op":"thread_delete","thread_id":"` + threadID + `"}]}`
	if err := validateSchema(schemaRequest{JSON: []byte(operations)}); err != nil {
		t.Fatal(err)
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"apply", "--file", "note.txt.comments", "--operations-file", "-"}, Stdin: []byte(operations), Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	assertHumanOutput(humanOutputExpectation{T: t, Got: got, Terms: []string{"operation", "note.txt.comments", threadID, "deleted"}})
}

func TestSubjectHumanErrors(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct {
		name        string
		args, terms []string
		status      int
	}{
		{"missing_flag", []string{"thread", "get", "--file", "note.txt.comments"}, []string{"--id", "--help"}, 2},
		{"unknown_flag", []string{"version", "--unknown"}, []string{"--unknown", "--help"}, 2},
		{"missing_thread", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}, []string{"thread", threadID}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: t.TempDir(), Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			assertHumanOutput(humanOutputExpectation{T: t, Got: got, Status: tc.status, Terms: tc.terms, Error: true})
		})
	}
}
