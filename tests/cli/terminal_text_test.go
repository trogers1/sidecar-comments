package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These are valid body bytes, not malformed protocol input. Human presentation
// must be safe without sanitizing the persisted message or machine response.
const terminalBody = "日本語 café 👋\nsecond line: \x1b[31mred\x1b[0m \x1b]52;c;Y2xpcGJvYXJk\a back\bspace \u009b31mC1\u009d52;c;data\u009c \u0085end"

type terminalBodyExpectation struct {
	T                 *testing.T
	Binary, Dir, Body string
}

func assertStoredTerminalBody(r terminalBodyExpectation) {
	r.T.Helper()
	got, err := runProcess(runRequest{Binary: r.Binary, Dir: r.Dir, Args: []string{"--output", "json", "thread", "get", "--file", "note.txt.comments", "--id", threadID}, Timeout: 5 * time.Second, OutputLimit: 16384})
	if err != nil {
		r.T.Fatal(err)
	}
	if got.Status != 0 || len(got.Stderr) != 0 {
		r.T.Fatalf("machine get failed: %+v", got)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		r.T.Fatal(err)
	}
	var envelope struct {
		Result struct {
			Thread struct {
				Messages []struct {
					Body string `json:"body"`
				} `json:"messages"`
			} `json:"thread"`
		} `json:"result"`
	}
	if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
		r.T.Fatal(err)
	}
	if len(envelope.Result.Thread.Messages) != 1 || envelope.Result.Thread.Messages[0].Body != r.Body {
		r.T.Fatalf("machine body changed: %q", got.Stdout)
	}
	disk, err := os.ReadFile(filepath.Join(r.Dir, "note.txt.comments"))
	if err != nil {
		r.T.Fatal(err)
	}
	expected := "[@th_" + threadID + " lost]\n" + role + r.Body + "\n" + endThread
	if !bytes.Equal(disk, []byte(expected)) {
		r.T.Fatalf("stored body changed: got %q want %q", disk, expected)
	}
}

type terminalOutputExpectation struct {
	T   *testing.T
	Got processResult
}

func assertTerminalSafeOutput(r terminalOutputExpectation) {
	r.T.Helper()
	assertHumanOutput(humanOutputExpectation{T: r.T, Got: r.Got, Terms: []string{"日本語 café 👋", "second line:", "Y2xpcGJvYXJk", "back", "space", "C1"}})
	for _, ch := range string(r.Got.Stdout) {
		if ch != '\n' && (ch < 0x20 || ch == 0x7f || ch >= 0x80 && ch <= 0x9f) {
			r.T.Errorf("raw terminal control U+%04X in %q", ch, r.Got.Stdout)
		}
	}
	// Check visible notation for ESC, backspace and C1, not an exact prose snapshot.
	for _, alternatives := range [][]string{{`\x1b`, `\u001b`, `\e`}, {`\x08`, `\u0008`, `\b`}, {`\u009b`, `\x9b`}, {`\u009d`, `\x9d`}} {
		found := false
		for _, escape := range alternatives {
			if strings.Contains(string(r.Got.Stdout), escape) {
				found = true
			}
		}
		if !found {
			r.T.Errorf("missing visible escape from %q in %q", alternatives, r.Got.Stdout)
		}
	}
	if !strings.Contains(string(r.Got.Stdout), "日本語 café 👋\n") {
		r.T.Errorf("ordinary Unicode/newline not preserved: %q", r.Got.Stdout)
	}
}

func TestSubjectTerminalSafeHumanBodies(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	if err := writeFile(fileRequest{Path: filepath.Join(dir, "body"), Mode: 0600, Bytes: []byte(terminalBody)}); err != nil {
		t.Fatal(err)
	}
	request := runRequest{Binary: binary, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 16384}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"create", []string{"thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}},
		{"get", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}},
		{"list", []string{"thread", "list", "--file", "note.txt.comments"}},
		{"update", []string{"message", "update", "--file", "note.txt.comments", "--thread", threadID, "--id", messageID, "--body-file", "body"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request.Args = tc.args
			got, err := runProcess(request)
			if err != nil {
				t.Fatal(err)
			}
			assertTerminalSafeOutput(terminalOutputExpectation{T: t, Got: got})
			assertStoredTerminalBody(terminalBodyExpectation{T: t, Binary: binary, Dir: dir, Body: terminalBody})
		})
	}
	// Marshal a real protocol request so control characters remain valid JSON.
	bodyJSON, err := json.Marshal(terminalBody + " applied")
	if err != nil {
		t.Fatal(err)
	}
	operations := []byte(`{"protocol":1,"operations":[{"op":"message_update","thread_id":"` + threadID + `","message_id":"` + messageID + `","body":` + string(bodyJSON) + `}]}`)
	if err := validateSchema(schemaRequest{JSON: operations}); err != nil {
		t.Fatal(err)
	}
	request.Args = []string{"apply", "--file", "note.txt.comments", "--operations-file", "-"}
	request.Stdin = operations
	got, err := runProcess(request)
	if err != nil {
		t.Fatal(err)
	}
	assertTerminalSafeOutput(terminalOutputExpectation{T: t, Got: got})
	assertStoredTerminalBody(terminalBodyExpectation{T: t, Binary: binary, Dir: dir, Body: terminalBody + " applied"})
}
