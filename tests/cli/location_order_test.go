package cli_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestSubjectLocationCanonicalOrder(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	source := strings.Repeat("abcdefghijk\n", 10)
	sourceRevision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(source)))
	// This sequence is the independently authored ordering, not a subject comparator.
	rows := []struct {
		id             int
		location, json string
		start, end     int
	}{
		{1, "file", `{"kind":"file"}`, 0, 0},
		{2, "line:2", `{"kind":"line","line":2}`, 12, 23},
		{9, "line:2", `{"kind":"line","line":2}`, 12, 23},
		{3, "line:10", `{"kind":"line","line":10}`, 108, 119},
		{4, "range:2:2-2:3", `{"kind":"range","start_line":2,"start_column":2,"end_line":2,"end_column":3}`, 13, 14},
		{5, "range:2:2-2:10", `{"kind":"range","start_line":2,"start_column":2,"end_line":2,"end_column":10}`, 13, 21},
		{6, "range:2:2-2:10", `{"kind":"range","start_line":2,"start_column":2,"end_line":2,"end_column":10}`, 13, 21},
		{7, "range:2:2-2:10", `{"kind":"range","start_line":2,"start_column":2,"end_line":2,"end_column":10}`, 13, 21},
		{8, "range:2:2-2:11", `{"kind":"range","start_line":2,"start_column":2,"end_line":2,"end_column":11}`, 13, 22},
		{10, "range:2:10-2:11", `{"kind":"range","start_line":2,"start_column":10,"end_line":2,"end_column":11}`, 21, 22},
		{11, "range:10:1-10:2", `{"kind":"range","start_line":10,"start_column":1,"end_line":10,"end_column":2}`, 108, 109},
		{12, "lost", `{"kind":"lost"}`, 0, 0},
	}
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x")}, {Path: "note.txt", Mode: 0640, Bytes: []byte(source)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	threads := make([]string, len(rows))
	dtos := make([]string, len(rows))
	for i, row := range rows {
		tid := fmt.Sprintf("018f0000-0000-7000-8000-%012x", row.id)
		mid := fmt.Sprintf("018f0001-0000-7000-8000-%012x", row.id)
		metadata := "{}"
		summary := ""
		if row.location != "file" && row.location != "lost" {
			metadata = fmt.Sprintf(`{"tracking":{"version":%d,"source_revision":%q,"last_location":%s,"selected_text":%q,"prefix":%q,"suffix":%q,"prefix_at_start":true,"suffix_at_end":true}}`, domain.TrackingVersion, sourceRevision, row.json, source[row.start:row.end], source[:row.start], source[row.end:])
			summary = fmt.Sprintf(`,"tracking":{"version":%d,"source_revision":%q,"last_location":%s,"prefix_at_start":true,"suffix_at_end":true}`, domain.TrackingVersion, sourceRevision, row.json)
		}
		if err := validateSchema(schemaRequest{JSON: []byte(metadata), Path: contextSchemaPath + "#/$defs/ThreadMetadata"}); err != nil {
			t.Fatal(err)
		}
		threads[i] = "[@th_" + tid + " " + row.location + "]\n<!-- sidecar-tracking: " + metadata + " -->\n---user <!-- sidecar-meta: {\"id\":\"" + mid + "\"} -->\nx\n[/@th_" + tid + "]\n"
		dtos[i] = fmt.Sprintf(`{"id":%q,"location":%s,"messages":[{"id":%q,"author":{"kind":"human"},"body":"x"}]%s}`, tid, row.json, mid, summary)
	}
	present := make([]bool, len(rows))
	for _, index := range []int{11, 7, 3, 2, 10, 9, 8, 6, 5, 4, 1, 0} {
		row := rows[index]
		tid := fmt.Sprintf("018f0000-0000-7000-8000-%012x", row.id)
		mid := fmt.Sprintf("018f0001-0000-7000-8000-%012x", row.id)
		got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", row.location, "--author-kind", "human", "--body-file", "body", "--id", tid, "--message-id", mid}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: domain.MaxApplyBytes})
		if err != nil {
			t.Fatal(err)
		}
		present[index] = true
		var active []string
		for i, yes := range present {
			if yes {
				active = append(active, threads[i])
			}
		}
		document := []byte(formatHeader + strings.Join(active, "\n"))
		revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
		want := fmt.Sprintf(`{"protocol":2,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", dtos[index], revision, revision)
		if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
			t.Fatal(err)
		}
		if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
			t.Fatal(err)
		}
		files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxApplyBytes})
		if err != nil {
			t.Fatal(err)
		}
		expected := append(append([]fileRequest{}, initial...), fileRequest{Path: "note.txt.comments", Mode: 0600, Bytes: document})
		if err := compareFiles(filesExpectation{Got: files, Want: expected}); err != nil {
			t.Fatal(err)
		}
	}
	// Reorder independently authored bytes, then prove formatter check and write
	// restore all location classes without changing captured records.
	order := []int{11, 7, 3, 2, 10, 9, 8, 6, 5, 4, 1, 0}
	shuffled := make([]string, len(order))
	for i, index := range order {
		shuffled[i] = threads[index]
	}
	original := []byte(formatHeader + strings.Join(shuffled, "\n"))
	if err := os.WriteFile(filepath.Join(dir, "note.txt.comments"), original, 0600); err != nil {
		t.Fatal(err)
	}
	storage := fmt.Sprintf("sha256:%x", sha256.Sum256(original))
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(formatHeader+strings.Join(threads, "\n"))))
	want := fmt.Sprintf(`{"protocol":2,"command":"format","changed":false,"result":{"formatted":false,"storage_revision":%q,"canonical_revision":%q}}`+"\n", storage, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxApplyBytes})
	if err != nil {
		t.Fatal(err)
	}
	expected := append(append([]fileRequest{}, initial...), fileRequest{Path: "note.txt.comments", Mode: 0600, Bytes: original})
	if err := compareFiles(filesExpectation{Got: files, Want: expected}); err != nil {
		t.Fatal(err)
	}
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want = fmt.Sprintf(`{"protocol":2,"command":"format","changed":true,"result":{"formatted":true,"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	expected[len(expected)-1].Bytes = []byte(formatHeader + strings.Join(threads, "\n"))
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxApplyBytes})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: expected}); err != nil {
		t.Fatal(err)
	}
}
