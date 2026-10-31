package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Source inspection is independent of sidecar parsing and attachment validity.
// Each journey observes the real CLI and exact directory bytes before/after.
func TestSubjectContextSourceInspect(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct {
		name, raw, logical string
		lines              int
	}{
		{name: "lf", raw: "aé\nz\n", logical: "aé\nz\n", lines: 2},
		{name: "crlf", raw: "aé\r\nz\r\n", logical: "aé\nz\n", lines: 2},
		{name: "empty", lines: 1},
		{name: "unterminated", raw: "last", logical: "last", lines: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{
				{Path: "note.txt", Mode: 0640, Bytes: []byte(tc.raw)},
				{Path: "note.txt.comments", Mode: 0640, Bytes: []byte("deliberately invalid sidecar\n")},
			}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "source", "inspect", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != 0 || len(got.Stderr) != 0 {
				t.Fatalf("source inspection failed: status=%d stdout=%q stderr=%q", got.Status, got.Stdout, got.Stderr)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout, Path: contextSchemaPath}); err != nil {
				t.Fatal(err)
			}
			fingerprint := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(tc.logical)))
			want := fmt.Sprintf(`{"protocol":2,"command":"source-inspect","changed":false,"result":{"source_revision":%q,"line_count":%d,"byte_count":%d}}`+"\n", fingerprint, tc.lines, len(tc.logical))
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
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
