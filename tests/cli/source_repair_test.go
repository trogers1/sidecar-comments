package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectSourceCoordinates(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct{ name, location, source, path, reason string }{
		{"utf8_bytes", "range:1:2-1:4 | w6k=", "aéz\n", "", ""},
		{"crlf_anchor", "range:1:2-2:2 | w6l6CnE=", "aéz\r\nqr\r\n", "", ""},
		{"rune_interior", "range:1:3-1:4", "aéz\n", "source_coordinate", "location"},
		{"missing_line", "line:9", "aéz\n", "source_coordinate", "location"},
		{"anchor_mismatch", "range:1:2-1:4 | eA==", "aéz\n", "source_anchor", "location"},
		{"empty_line", "line:1", "", "", ""},
		{"unterminated", "line:2", "first\nlast", "", ""},
		{"source_bom", "file", "\xef\xbb\xbfsource", "source_encoding", "bom"},
		{"source_nul", "file", "x\x00", "source_encoding", "nul"},
		{"source_utf8", "file", "\xff", "source_encoding", "utf8"},
		{"source_mixed", "file", "x\r\ny\n", "source_endings", "endings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			document := strings.Replace(canonicalNote, " file]", " "+tc.location+"]", 1)
			initial := []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: []byte(tc.source)}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(document)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(document)))
			want := processResult{Stdout: []byte(fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision))}
			if tc.path != "" {
				want = processResult{Status: 3, Stderr: []byte(fmt.Sprintf(`{"protocol":1,"error":{"code":"validation","message":"validation:%s:%s","details":{"path":%q,"reason":%q}}}`+"\n", tc.path, tc.reason, tc.path, tc.reason))}
			}
			if err := compareProcess(processExpectation{Got: got, Want: want}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: append(append([]byte{}, got.Stdout...), got.Stderr...)}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Repair diagnostics use physical role-line coordinates, not logical body
// positions. Check must not allocate prospective IDs or modify the document.
func TestSubjectRepairBoundaries(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	opening := "[@th_" + threadID + " file]\n"
	repair := opening + "---user\nx\n" + endThread
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(repair)))
	diagnostics := fmt.Sprintf(`[{"code":"missing_message_id","byte_offset":%d,"line":2,"thread_id":%q,"message_ordinal":1}]`, len(opening)+1, threadID)
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(repair)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name         string
		args         []string
		out, failure string
	}{
		{"validate", []string{"validate", "--file", "note.txt.comments"}, fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":false,"repair":{"diagnostics":%s},"storage_revision":%q,"canonical_revision":null}}`+"\n", diagnostics, revision), ""},
		{"check", []string{"fix", "--add-missing-message-ids", "--file", "note.txt.comments", "--check"}, fmt.Sprintf(`{"protocol":1,"command":"fix","changed":false,"result":{"assignments":[],"storage_revision":%q,"canonical_revision":null}}`+"\n", revision), ""},
		{"list", []string{"thread", "list", "--file", "note.txt.comments"}, "", `{"protocol":1,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
		{"get", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}, "", `{"protocol":1,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
		{"format", []string{"format", "--file", "note.txt.comments"}, "", `{"protocol":1,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Args: append([]string{"--output", "json"}, tc.args...), Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stdout: []byte(tc.out), Stderr: []byte(tc.failure)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: []byte(tc.out + tc.failure)}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
