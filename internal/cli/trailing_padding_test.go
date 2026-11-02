package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
)

func TestRunTrailingDocumentPadding(t *testing.T) {
	id := func() string {
		v, err := domain.NewID(domain.NewIDRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	thread, message := id(), id()
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: thread, Location: domain.Location{Kind: domain.LocationFile}, Messages: []domain.Message{{ID: message, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "Body spaces.  \nBody tab.\t\n\n"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	canonical := string(format.Encode(format.EncodeRequest{Document: value}))
	for _, padding := range []string{"\n\n", "\n \t\n\n", " \t\n", " \t\n\n \t", "\n\t "} {
		for _, command := range []string{"validate", "list", "format", "fix", "fix-missing"} {
			t.Run(command+"/"+strings.ReplaceAll(padding, "\n", "LF"), func(t *testing.T) {
				dir := t.TempDir()
				target := filepath.Join(dir, "note.txt.comments")
				input := strings.TrimSuffix(canonical, "\n") + padding
				if command == "fix-missing" {
					input = strings.Replace(input, `---user <!-- sidecar-meta: {"id":"`+message+`"} -->`, "---user \t", 1)
				}
				if err := os.WriteFile(target, []byte(input), 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"--output", "json"}
				switch command {
				case "list":
					args = append(args, "thread", "list")
				case "fix", "fix-missing":
					args = append(args, "fix", "--add-missing-message-ids")
				default:
					args = append(args, command)
				}
				args = append(args, "--file", target)
				var stdout, stderr bytes.Buffer
				status := Run(Request{Args: args, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
				if status != 0 || stderr.Len() != 0 {
					t.Fatalf("status=%d out=%s err=%s", status, stdout.String(), stderr.String())
				}
				after, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				switch command {
				case "validate", "list":
					if string(after) != input {
						t.Fatal("read command wrote target")
					}
					entries, err := os.ReadDir(dir)
					if err != nil || len(entries) != 1 {
						t.Fatalf("read command created entries: %v %v", entries, err)
					}
				case "format", "fix":
					if string(after) != canonical {
						t.Fatalf("normalization changed body or retained padding: got %q want %q", after, canonical)
					}
				case "fix-missing":
					parsed, err := format.Parse(format.ParseRequest{Bytes: after, Mode: "normal"})
					if err != nil {
						t.Fatal(err)
					}
					threads := parsed.Value.Entries(domain.DocumentEntriesRequest{})
					if len(threads) != 1 || threads[0].DTO(domain.ValueRequest{}).Messages[0].Body != value.Entries(domain.DocumentEntriesRequest{})[0].DTO(domain.ValueRequest{}).Messages[0].Body {
						t.Fatal("repair changed body")
					}
					if !bytes.Equal(after, format.Encode(format.EncodeRequest{Document: parsed.Value})) {
						t.Fatal("repair retained padding")
					}
				}
			})
		}
	}
}

func TestRunTrailingPaddingKeepsStrictBoundariesAndRawLocations(t *testing.T) {
	id := func() string {
		v, err := domain.NewID(domain.NewIDRequest{})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	threads := []domain.Thread{}
	for range 2 {
		threads = append(threads, domain.Thread{ID: id(), Location: domain.Location{Kind: domain.LocationFile}, Messages: []domain.Message{{ID: id(), Author: domain.Author{Kind: domain.AuthorHuman}, Body: "Body.  \t"}}})
	}
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	canonical := string(format.Encode(format.EncodeRequest{Document: value}))
	firstClose := domain.ThreadClosePrefix + threads[0].ID + "]"
	for _, tc := range []struct {
		name, input, code, marker string
		eof                       bool
	}{
		{name: "missing LF", input: strings.TrimSuffix(canonical, "\n"), code: "missing_final_newline", eof: true},
		{name: "padding without LF", input: strings.TrimSuffix(canonical, "\n") + " \t", code: "missing_final_newline", eof: true},
		{name: "whitespace only", input: " \t\n\n ", code: "missing_final_newline", eof: true},
		{name: "internal closing whitespace", input: strings.Replace(canonical, firstClose, firstClose+" \t", 1) + "\n \t", code: "unexpected_delimiter", marker: firstClose},
		{name: "earlier malformed metadata", input: strings.Replace(canonical, `{"id":"`+threads[1].Messages[0].ID+`"}`, `{"id":false}`, 1) + "\n \t", code: "message_metadata", marker: `{"id":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "note.txt.comments")
			if err := os.WriteFile(target, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			status := Run(Request{Args: []string{"--output", "json", "validate", "--file", target}, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr})
			if status != domain.ExitValidation || stdout.Len() != 0 {
				t.Fatalf("status=%d out=%s err=%s", status, stdout.String(), stderr.String())
			}
			var response struct {
				Error struct{ Details domain.ValidationDetails }
			}
			if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			diagnostics := response.Error.Details.Diagnostics
			if len(diagnostics) != 1 {
				t.Fatalf("missing diagnostic: %s", stderr.String())
			}
			offset := len(tc.input)
			if !tc.eof {
				offset = strings.Index(tc.input, tc.marker)
				if offset < 0 {
					t.Fatal("marker missing")
				}
			}
			line := strings.Count(tc.input[:offset], "\n") + 1
			column := offset - strings.LastIndex(tc.input[:offset], "\n")
			d := diagnostics[0]
			if d.Code != tc.code || d.ByteOffset != offset+1 || d.Line != line || d.Column != column {
				t.Fatalf("diagnostic=%+v want %s %d:%d offset %d", d, tc.code, line, column, offset+1)
			}
			after, err := os.ReadFile(target)
			if err != nil || string(after) != tc.input {
				t.Fatal("validation wrote target")
			}
		})
	}
}
