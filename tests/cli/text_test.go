package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
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
			// A thread result already contains its messages; none is rendered twice.
			for _, id := range []string{messageID, replyID} {
				if n := strings.Count(string(got.Stdout), "Message "+id); n > 1 {
					t.Errorf("message %s rendered %d times: %s", id, n, got.Stdout)
				}
			}
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
	operations := `{"protocol":2,"operations":[{"op":"thread_delete","thread_id":"` + threadID + `"}]}`
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

type guidanceRunRequest struct {
	T           *testing.T
	Binary, Dir string
	Args        []string
}

// guidanceRevisions holds the revisions a setup command reports.
type guidanceRevisions struct {
	StorageRevision string `json:"storage_revision"`
	SourceRevision  string `json:"source_revision"`
}

// guidanceJSON runs a successful JSON command during fixture setup.
func guidanceJSON(r guidanceRunRequest) guidanceRevisions {
	r.T.Helper()
	got, err := runProcess(runRequest{Binary: r.Binary, Args: append([]string{"--output", "json"}, r.Args...), Dir: r.Dir, Timeout: 5 * time.Second, OutputLimit: 16384})
	if err != nil || got.Status != 0 {
		r.T.Fatalf("setup %v: status=%d err=%v stderr=%s", r.Args, got.Status, err, got.Stderr)
	}
	var envelope struct {
		Result guidanceRevisions `json:"result"`
	}
	if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
		r.T.Fatal(err)
	}
	return envelope.Result
}

type guidanceFixtureRequest struct {
	T           *testing.T
	Binary, Dir string
}

// guidanceTracked creates a line thread on note.txt and returns the editor guards.
func guidanceTracked(r guidanceFixtureRequest) (string, string) {
	r.T.Helper()
	run := guidanceRunRequest{T: r.T, Binary: r.Binary, Dir: r.Dir}
	run.Args = []string{"thread", "create", "--file", "note.txt.comments", "--location", "line:1", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}
	guidanceJSON(run)
	run.Args = []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}
	revision := guidanceJSON(run).StorageRevision
	run.Args = []string{"source", "inspect", "--file", "note.txt.comments"}
	return revision, guidanceJSON(run).SourceRevision
}

type guidanceWriteRequest struct {
	T          *testing.T
	Path, Text string
}

func guidanceWrite(r guidanceWriteRequest) {
	r.T.Helper()
	if err := os.WriteFile(r.Path, []byte(r.Text), 0640); err != nil {
		r.T.Fatal(err)
	}
}

// Human errors explain the failure in plain words and name a concrete next
// step; the JSON form of each error is unchanged.
func TestSubjectHumanErrorGuidance(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	v1 := "[@th_" + threadID + " file]\n" + role + "x\n" + endThread
	zero := "sha256:" + strings.Repeat("0", 64)
	lockName := ".note.txt.comments.sidecar.lock"
	create := []string{"thread", "create", "--file", "note.txt.comments", "--location", "line:1", "--author-kind", "human", "--body-file", "body"}
	editor := []string{"editor-update", "--file", "note.txt.comments", "--operations-file", "ops.json"}
	for _, tc := range []struct {
		name, symbol  string
		status        int
		args          []string
		terms, absent []string
		setup         func(t *testing.T, dir string)
	}{
		{name: "coordinate_outside_source", symbol: "validation:source_coordinate:location", status: 3,
			args:  []string{"thread", "create", "--file", "note.txt.comments", "--location", "line:99", "--author-kind", "human", "--body-file", "body"},
			terms: []string{"not inside the source file", "sidecar source inspect"}},
		{name: "source_named_as_sidecar", symbol: "validation:target_name:invalid", status: 3,
			args:  []string{"thread", "create", "--file", "note.txt", "--location", "file", "--author-kind", "human", "--body-file", "body"},
			terms: []string{"named after their source", `--file "note.txt.comments"`}},
		{name: "directory_named_as_sidecar", symbol: "validation:target_name:invalid", status: 3,
			args:   []string{"thread", "list", "--file", "sub/"},
			terms:  []string{"named after their source", "--file chapter.md.comments for chapter.md"},
			absent: []string{"sub/.comments"}},
		{name: "stale_revision", symbol: "conflict:revision", status: 4,
			args:  []string{"thread", "delete", "--if-revision", zero, "--file", "note.txt.comments", "--id", threadID},
			terms: []string{"changed since its revision was read", "Read the sidecar again"},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt.comments"), Text: canonicalNote})
			}},
		{name: "lock_timeout", symbol: "conflict:lock_timeout", status: 4,
			args:  []string{"thread", "delete", "--lock-timeout", "1ms", "--file", "note.txt.comments", "--id", threadID},
			terms: []string{"holding the writer lock", "Wait for it to finish"},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt.comments"), Text: canonicalNote})
				lock, err := os.OpenFile(filepath.Join(dir, lockName), os.O_RDWR|os.O_CREATE, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := errors.Join(syscall.Flock(int(lock.Fd()), syscall.LOCK_UN), lock.Close()); err != nil {
						t.Error(err)
					}
				})
			}},
		{name: "v1_sidecar", symbol: "unsupported:format", status: 8,
			args:  []string{"thread", "list", "--file", "note.txt.comments"},
			terms: []string{"Other format versions are not migrated"},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt.comments"), Text: v1})
			}},
		{name: "crlf_sidecar", symbol: "validation:target_endings:endings", status: 3,
			args:  []string{"thread", "list", "--file", "note.txt.comments"},
			terms: []string{"must use LF line endings", "sidecar format --file PATH"},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt.comments"), Text: strings.ReplaceAll(canonicalNote, "\n", "\r\n")})
			}},
		{name: "source_symlink", symbol: "validation:source_type:symlink", status: 3, args: create,
			terms: []string{"does not follow links", "Replace the source link"},
			setup: func(t *testing.T, dir string) {
				if err := os.Rename(filepath.Join(dir, "note.txt"), filepath.Join(dir, "referent")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("referent", filepath.Join(dir, "note.txt")); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "sidecar_directory", symbol: "validation:target_type:directory", status: 3,
			args:  []string{"thread", "list", "--file", "note.txt.comments"},
			terms: []string{"sidecar is a directory", "Remove or rename the directory"},
			setup: func(t *testing.T, dir string) {
				if err := os.Mkdir(filepath.Join(dir, "note.txt.comments"), 0700); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "lock_directory", symbol: "validation:lock_type:directory", status: 3,
			args:  []string{"thread", "delete", "--file", "note.txt.comments", "--id", threadID},
			terms: []string{"writer lock file is a directory", lockName},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt.comments"), Text: canonicalNote})
				if err := os.Mkdir(filepath.Join(dir, lockName), 0700); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "source_bom", symbol: "validation:source_encoding:bom", status: 3, args: create,
			terms: []string{"byte-order mark", "Save the source file as UTF-8"},
			setup: func(t *testing.T, dir string) {
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "note.txt"), Text: "\ufeffsource\n"})
			}},
		{name: "missing_source", symbol: "io:open", status: 7, args: create,
			terms: []string{"expected beside the sidecar", "source file exists and is readable"},
			setup: func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "note.txt")); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "corrupt_tracking", symbol: "validation:tracking:invalid", status: 3,
			args:   []string{"thread", "move", "--file", "note.txt.comments", "--id", threadID, "--location", "lost"},
			terms:  []string{"malformed or inconsistent", "Restore the sidecar from version control"},
			absent: []string{"move the affected thread"},
			setup: func(t *testing.T, dir string) {
				guidanceTracked(guidanceFixtureRequest{T: t, Binary: binary, Dir: dir})
				path := filepath.Join(dir, "note.txt.comments")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				corrupt := strings.Replace(string(data), `"tracking":{"version":1,`, `"tracking":{"version":9,`, 1)
				if corrupt == string(data) {
					t.Fatal("fixture: tracking version not found")
				}
				guidanceWrite(guidanceWriteRequest{T: t, Path: path, Text: corrupt})
			}},
		{name: "editor_duplicate", symbol: "precondition:editor-update:duplicate_thread", status: 6, args: editor,
			terms: []string{"more than once in the batch (item 2, index 1)", "Send each thread once"},
			setup: func(t *testing.T, dir string) {
				revision, source := guidanceTracked(guidanceFixtureRequest{T: t, Binary: binary, Dir: dir})
				update := fmt.Sprintf(`{"thread_id":%q,"location":{"kind":"line","line":1}}`, threadID)
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "ops.json"), Text: fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[%s,%s]}`, domain.ProtocolVersion, revision, source, update, update)})
			}},
		{name: "editor_protocol", symbol: "unsupported:protocol", status: 8, args: editor,
			terms: []string{fmt.Sprintf("speaks protocol %d", domain.ProtocolVersion), fmt.Sprintf("protocol field to %d", domain.ProtocolVersion)},
			setup: func(t *testing.T, dir string) {
				revision, source := guidanceTracked(guidanceFixtureRequest{T: t, Binary: binary, Dir: dir})
				guidanceWrite(guidanceWriteRequest{T: t, Path: filepath.Join(dir, "ops.json"), Text: fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[{"thread_id":%q,"location":{"kind":"line","line":1}}]}`, domain.ProtocolVersion+1, revision, source, threadID)})
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "body", Mode: 0600, Bytes: []byte("A note.\n")}} {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 16384})
			if err != nil {
				t.Fatal(err)
			}
			assertHumanOutput(humanOutputExpectation{T: t, Got: got, Status: tc.status, Terms: tc.terms, Error: true})
			// Explanations after the "error CODE:" header are prose, not
			// re-spelled machine symbols.
			_, explanation, _ := bytes.Cut(got.Stderr, []byte(": "))
			for _, fragment := range append([]string{"validation: ", "conflict: ", "precondition: ", "_"}, tc.absent...) {
				if bytes.Contains(explanation, []byte(fragment)) {
					t.Errorf("stderr contains %q: %s", fragment, got.Stderr)
				}
			}
			machine, err := runProcess(runRequest{Binary: binary, Args: append([]string{"--output", "json"}, tc.args...), Dir: dir, Timeout: 5 * time.Second, OutputLimit: 16384})
			if err != nil {
				t.Fatal(err)
			}
			if machine.Status != tc.status || !bytes.Contains(machine.Stderr, []byte(`"message":"`+tc.symbol+`"`)) {
				t.Fatalf("JSON error changed: status=%d stderr=%s", machine.Status, machine.Stderr)
			}
		})
	}
}
