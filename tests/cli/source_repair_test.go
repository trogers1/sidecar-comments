package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestSubjectSourceCoordinates(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct {
		name, location, source, selected, prefix, suffix, path, reason string
		status                                                         int
	}{
		{name: "utf8_bytes", location: "range:1:2-1:4", source: "aéz\n", selected: "é", prefix: "a", suffix: "z\n"},
		{name: "crlf_capture", location: "range:1:2-2:2", source: "aéz\r\nqr\r\n", selected: "éz\nq", prefix: "a", suffix: "r\n"},
		{name: "rune_interior", location: "range:1:3-1:4", source: "aéz\n", path: "source_coordinate", reason: "location", status: 3},
		{name: "missing_line", location: "line:9", source: "aéz\n", path: "source_coordinate", reason: "location", status: 3},
		{name: "legacy_anchor", location: "range:1:2-1:4 | w6k=", source: "aéz\n", path: "argv", reason: "malformed", status: 2},
		{name: "empty_line", location: "line:1", source: ""},
		{name: "unterminated", location: "line:2", source: "first\nlast", selected: "last", prefix: "first\n"},
		{name: "source_bom", location: "file", source: "\xef\xbb\xbfsource", path: "source_encoding", reason: "bom", status: 3},
		{name: "source_nul", location: "file", source: "x\x00", path: "source_encoding", reason: "nul", status: 3},
		{name: "source_utf8", location: "file", source: "\xff", path: "source_encoding", reason: "utf8", status: 3},
		{name: "source_mixed", location: "file", source: "x\r\ny\n", path: "source_endings", reason: "endings", status: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x")}, {Path: "note.txt", Mode: 0640, Bytes: []byte(tc.source)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", tc.location, "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: domain.MaxApplyBytes})
			if err != nil {
				t.Fatal(err)
			}
			var want processResult
			if tc.status != 0 {
				code := "validation"
				field := "path"
				if tc.status == 2 {
					code = "usage"
					field = "field"
				}
				want = processResult{Status: tc.status, Stderr: []byte(fmt.Sprintf(`{"protocol":2,"error":{"code":%q,"message":"%s:%s:%s","details":{"%s":%q,"reason":%q}}}`+"\n", code, code, tc.path, tc.reason, field, tc.path, tc.reason))}
			} else {
				// Coordinates are declared independently for these deliberately tiny fixtures.
				location := `{"kind":"line","line":1}`
				switch tc.name {
				case "utf8_bytes":
					location = `{"kind":"range","start_line":1,"start_column":2,"end_line":1,"end_column":4}`
				case "crlf_capture":
					location = `{"kind":"range","start_line":1,"start_column":2,"end_line":2,"end_column":2}`
				case "unterminated":
					location = `{"kind":"line","line":2}`
				}
				sourceRevision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(strings.ReplaceAll(tc.source, "\r\n", "\n"))))
				summary := fmt.Sprintf(`{"version":%d,"source_revision":%q,"last_location":%s,"prefix_at_start":true,"suffix_at_end":true}`, domain.TrackingVersion, sourceRevision, location)
				metadata := fmt.Sprintf(`{"tracking":{"version":%d,"source_revision":%q,"last_location":%s,"selected_text":%q,"prefix":%q,"suffix":%q,"prefix_at_start":true,"suffix_at_end":true}}`, domain.TrackingVersion, sourceRevision, location, tc.selected, tc.prefix, tc.suffix)
				if err := validateSchema(schemaRequest{JSON: []byte(metadata), Path: contextSchemaPath + "#/$defs/ThreadMetadata"}); err != nil {
					t.Fatal(err)
				}
				document := []byte(formatHeader + "[@th_" + threadID + " " + tc.location + "]\n<!-- sidecar-tracking: " + metadata + " -->\n" + role + "x\n" + endThread)
				revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
				want = processResult{Stdout: []byte(fmt.Sprintf(`{"protocol":2,"command":"thread-create","changed":true,"result":{"thread":{"id":%q,"location":%s,"messages":[{"id":%q,"author":{"kind":"human"},"body":"x"}],"tracking":%s},"storage_revision":%q,"canonical_revision":%q}}`+"\n", threadID, location, messageID, summary, revision, revision))}
				initial = append(initial, fileRequest{Path: "note.txt.comments", Mode: 0600, Bytes: document})
			}
			if err := compareProcess(processExpectation{Got: got, Want: want}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: append(append([]byte{}, got.Stdout...), got.Stderr...)}); err != nil {
				t.Fatal(err)
			}
			if tc.status == 0 {
				// Preserve explicit attachment validation coverage after real capture.
				revision := fmt.Sprintf("sha256:%x", sha256.Sum256(initial[len(initial)-1].Bytes))
				validated, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				expected := fmt.Sprintf(`{"protocol":2,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
				if err := compareProcess(processExpectation{Got: validated, Want: processResult{Stdout: []byte(expected)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: validated.Stdout}); err != nil {
					t.Fatal(err)
				}
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxApplyBytes})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Repair checks retain exact physical offsets after the v2 framing lines.
func TestSubjectRepairBoundaries(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	opening := formatHeader + "[@th_" + threadID + " file]\n" + untrackedMetadata
	repair := opening + "---user\nx\n" + endThread
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(repair)))
	diagnostics := fmt.Sprintf(`[{"code":"missing_message_id","byte_offset":%d,"line":4,"thread_id":%q,"message_ordinal":1}]`, len(opening)+1, threadID)
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
		{"validate", []string{"validate", "--file", "note.txt.comments"}, fmt.Sprintf(`{"protocol":2,"command":"validate","changed":false,"result":{"valid":false,"repair":{"diagnostics":%s},"storage_revision":%q,"canonical_revision":null}}`+"\n", diagnostics, revision), ""},
		{"check", []string{"fix", "--add-missing-message-ids", "--file", "note.txt.comments", "--check"}, fmt.Sprintf(`{"protocol":2,"command":"fix","changed":false,"result":{"assignments":[],"storage_revision":%q,"canonical_revision":null}}`+"\n", revision), ""},
		{"list", []string{"thread", "list", "--file", "note.txt.comments"}, "", `{"protocol":2,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
		{"get", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}, "", `{"protocol":2,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
		{"format", []string{"format", "--file", "note.txt.comments"}, "", `{"protocol":2,"error":{"code":"repair","message":"repair:document","details":{"diagnostics":` + diagnostics + `}}}` + "\n"},
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
