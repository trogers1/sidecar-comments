package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
)

func TestRunLocatedContentDiagnostics(t *testing.T) {
	id := func() string {
		v, err := domain.NewID(domain.NewIDRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	thread, first, second := id(), id(), id()
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: thread, Location: domain.Location{Kind: domain.LocationFile}, Messages: []domain.Message{
		{ID: first, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "Multibyte café body."},
		{ID: second, Author: domain.Author{Kind: domain.AuthorAgent}, Body: "Reply."},
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	canonical := string(format.Encode(format.EncodeRequest{Document: value}))
	role := "---" + domain.RoleUser
	tracking := "<!-- sidecar-tracking: {} -->"
	cases := []struct {
		name, content, code, marker string
		eof                         bool
	}{
		{name: "header", content: strings.Replace(canonical, "<!-- sidecar-format:", "<!-- broken-format:", 1), code: "invalid_header", marker: "<!-- broken"},
		{name: "bom", content: "\ufeff" + canonical, code: "encoding_bom", marker: "\ufeff"},
		{name: "nul", content: strings.Replace(canonical, "café", "café\x00", 1), code: "encoding_nul", marker: "\x00"},
		{name: "utf8", content: strings.Replace(canonical, "café", "café\xff", 1), code: "encoding_utf8", marker: "\xff"},
		{name: "endings", content: strings.Replace(canonical, "\n", "\r\n", 1), code: "line_endings", marker: "\r"},
		{name: "newline", content: strings.TrimSuffix(canonical, "\n"), code: "missing_final_newline", eof: true},
		{name: "tracking", content: strings.Replace(canonical, tracking, "<!-- sidecar-tracking: [] -->", 1), code: "tracking_metadata", marker: "<!-- sidecar-tracking:"},
		{name: "role", content: strings.Replace(canonical, role, "---unknown", 1), code: "role_delimiter", marker: "---unknown"},
		{name: "metadata", content: strings.Replace(canonical, `{"id":"`+first+`"}`, `{"id":false}`, 1), code: "message_metadata", marker: `{"id":false}`},
		{name: "duplicate", content: strings.Replace(canonical, second, first, 1), code: "duplicate_message_id", marker: "---" + domain.RoleBot},
		{name: "close", content: strings.Replace(canonical, domain.ThreadClosePrefix+thread, domain.ThreadClosePrefix+second, 1), code: "unexpected_delimiter", marker: domain.ThreadClosePrefix + second},
		{name: "eof", content: canonical[:strings.Index(canonical, domain.ThreadClosePrefix)], code: "thread_close", eof: true},
		{name: "canonical", content: strings.Replace(canonical, `{"id":"`+first+`"}`, `{ "id":"`+first+`"}`, 1), code: "message_metadata_noncanonical", marker: `{ "id":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "note.txt.comments")
			if err := os.WriteFile(target, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			offset := len(tc.content)
			if !tc.eof {
				offset = strings.Index(tc.content, tc.marker)
				if offset < 0 {
					t.Fatal("missing marker")
				}
			}
			line := strings.Count(tc.content[:offset], "\n") + 1
			column := offset - strings.LastIndex(tc.content[:offset], "\n")
			for _, output := range []string{"json", "text"} {
				var stdout, stderr bytes.Buffer
				status := Run(Request{Args: []string{"--output", output, "validate", "--file", target}, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
				if status != domain.ExitValidation || stdout.Len() != 0 {
					t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
				}
				if output == "json" {
					var response struct {
						Error struct {
							Code, Message string
							Details       domain.ValidationDetails
						}
					}
					if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					diagnostics := response.Error.Details.Diagnostics
					if response.Error.Code != domain.CodeValidation || len(diagnostics) != 1 {
						t.Fatalf("unexpected response: %s", stderr.String())
					}
					d := diagnostics[0]
					if tc.name == "close" && (!strings.Contains(d.Message, "closing delimiter") || !strings.Contains(d.Hint, domain.ThreadClosePrefix+thread+"]")) {
						t.Fatalf("missing matching closing-ID guidance: %+v", d)
					}
					if d.Code != tc.code || d.ByteOffset != offset+1 || d.Line != line || d.Column != column || d.Message == "" || d.Hint == "" {
						t.Fatalf("diagnostic=%+v want %s at %d:%d offset %d", d, tc.code, line, column, offset+1)
					}
				} else {
					if !strings.Contains(stderr.String(), fmt.Sprintf("%s:%d:%d:", target, line, column)) || strings.Contains(stderr.String(), "Run sidecar validate") {
						t.Fatalf("unhelpful text: %s", stderr.String())
					}
					if tc.name == "close" && !strings.Contains(stderr.String(), domain.ThreadClosePrefix+thread+"]") {
						t.Fatalf("missing matching closing-ID guidance: %s", stderr.String())
					}
				}
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != tc.content {
				t.Fatalf("target changed: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("created filesystem entries: %v %v", entries, err)
			}
		})
	}
}
