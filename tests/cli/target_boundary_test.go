package cli_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectTargetMaximum(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	opening := "[@th_" + threadID + " lost]\n"
	roles := make([]string, 65)
	overhead := len(opening) + len(endThread)
	for i := range roles {
		roles[i] = fmt.Sprintf("---user <!-- sidecar-meta: {\"id\":\"018f0001-0000-7000-8000-%012x\"} -->\n", i+1)
		overhead += len(roles[i]) + 2
		if i > 0 {
			overhead++
		}
	}
	remaining := (4 << 20) - overhead
	var document strings.Builder
	document.WriteString(opening)
	for i := range roles {
		if i > 0 {
			document.WriteByte('\n')
		}
		document.WriteString(roles[i])
		extra := min(remaining, 65535)
		document.WriteString(strings.Repeat("x", 1+extra))
		remaining -= extra
		document.WriteByte('\n')
	}
	document.WriteString(endThread)
	original := []byte(document.String())
	if remaining != 0 || len(original) != 4<<20 {
		t.Fatal("invalid maximum-size fixture construction")
	}
	target := filepath.Join(dir, "note.txt.comments")
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(original); err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod(0640); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	revision := fmt.Sprintf("sha256:%x", sha256.Sum256(original))
	want := fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "note.txt.comments" {
		t.Fatal("unexpected directory entries")
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0640 || !bytes.Equal(data, original) {
		t.Fatal("maximum target changed")
	}
}
