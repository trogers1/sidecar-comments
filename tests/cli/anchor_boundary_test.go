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

// The approved selection budget applies to CLI capture, not legacy anchors.
func TestSubjectSelectionCaptureBoundary(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, size := range []int{domain.MaxAnchorBytes, domain.MaxAnchorBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dir := t.TempDir()
			source := strings.Repeat("x", size)
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x")}, {Path: "note.txt", Mode: 0640, Bytes: []byte(source)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", fmt.Sprintf("range:1:1-1:%d", size+1), "--author-kind", "human", "--body-file", "body", "--id", threadID, "--message-id", messageID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: domain.MaxApplyBytes})
			if err != nil {
				t.Fatal(err)
			}
			want := processResult{Status: 6, Stderr: []byte(`{"protocol":2,"error":{"code":"precondition","message":"precondition:thread-create:limit_selection","details":{"operation":"thread-create","reason":"limit_selection"}}}` + "\n")}
			if size == domain.MaxAnchorBytes {
				location := fmt.Sprintf(`{"kind":"range","start_line":1,"start_column":1,"end_line":1,"end_column":%d}`, size+1)
				sourceRevision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(source)))
				summary := fmt.Sprintf(`{"version":%d,"source_revision":%q,"last_location":%s,"prefix_at_start":true,"suffix_at_end":true}`, domain.TrackingVersion, sourceRevision, location)
				record := fmt.Sprintf(`{"version":%d,"source_revision":%q,"last_location":%s,"selected_text":%q,"prefix":"","suffix":"","prefix_at_start":true,"suffix_at_end":true}`, domain.TrackingVersion, sourceRevision, location, source)
				metadata := `{"tracking":` + record + `}`
				if err := validateSchema(schemaRequest{JSON: []byte(metadata), Path: contextSchemaPath + "#/$defs/ThreadMetadata"}); err != nil {
					t.Fatal(err)
				}
				document := []byte(formatHeader + "[@th_" + threadID + fmt.Sprintf(" range:1:1-1:%d]\n", size+1) + "<!-- sidecar-tracking: " + metadata + " -->\n" + role + "x\n" + endThread)
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
