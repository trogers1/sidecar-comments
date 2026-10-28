package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is an availability gate only, not a behavioral Sidecar case.
// TestFormatLifecycle exercises behavior; no fallback or skip exists.
func TestSubjectAvailable(t *testing.T) {
	binary := os.Getenv("SIDECAR_BIN")
	if binary == "" {
		t.Fatal("SIDECAR_BIN is required; helper/schema success is not Sidecar behavioral proof")
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		t.Fatalf("SIDECAR_BIN must identify an executable: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		t.Fatal("SIDECAR_BIN must be an executable regular file")
	}
}

func TestHelpers(t *testing.T) {
	dir := t.TempDir()
	// Shell doubles exercise helper mechanics only; none implements Sidecar.
	got, err := runProcess(runRequest{Binary: "/bin/sh", Args: []string{"-c", "cat; printf 'err\\n' >&2; exit 7"}, Dir: dir, Stdin: []byte("out\x00\n"), Timeout: 2 * time.Second, OutputLimit: 32})
	if err != nil {
		t.Fatal(err)
	}
	want := processResult{Status: 7, Stdout: []byte("out\x00\n"), Stderr: []byte("err\n")}
	if err := compareProcess(processExpectation{Got: got, Want: want}); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []processResult{
		{Status: 0, Stdout: want.Stdout, Stderr: want.Stderr},
		{Status: 7, Stdout: []byte("wrong"), Stderr: want.Stderr},
		{Status: 7, Stdout: want.Stdout, Stderr: []byte("wrong")},
	} {
		if compareProcess(processExpectation{Got: got, Want: wrong}) == nil {
			t.Fatal("incorrect process oracle accepted")
		}
	}
	for _, file := range []fileRequest{
		{Path: "body", Mode: 0600, Bytes: []byte("body\x00\n")},
		{Path: "source", Mode: 0640, Bytes: []byte("source\n")},
		{Path: "source.comments", Mode: 0600, Bytes: []byte{}},
	} {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, file.Path), Mode: file.Mode, Bytes: file.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 32})
	if err != nil {
		t.Fatal(err)
	}
	expected := []fileRequest{{Path: "body", Mode: 0600, Bytes: []byte("body\x00\n")}, {Path: "source", Mode: 0640, Bytes: []byte("source\n")}, {Path: "source.comments", Mode: 0600, Bytes: []byte{}}}
	if err := compareFiles(filesExpectation{Got: files, Want: expected}); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []fileRequest{
		{Path: "wrong", Mode: 0600, Bytes: expected[0].Bytes},
		{Path: "body", Mode: 0644, Bytes: expected[0].Bytes},
		{Path: "body", Mode: 0600, Bytes: []byte("wrong")},
	} {
		if compareFiles(filesExpectation{Got: files, Want: []fileRequest{wrong, expected[1], expected[2]}}) == nil {
			t.Fatal("incorrect filesystem oracle accepted")
		}
	}
	if compareFiles(filesExpectation{Got: files, Want: expected[:2]}) == nil {
		t.Fatal("extra path accepted")
	}
	if _, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 2}); err == nil {
		t.Fatal("oversized file accepted")
	}
	if err := writeFile(fileRequest{Path: filepath.Join(dir, "body"), Mode: 0600}); err == nil {
		t.Fatal("fixture overwrite accepted")
	}
	if err := os.Symlink("source", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 32}); err == nil {
		t.Fatal("symlink disappeared from oracle")
	}
}

func TestHelperTimeoutAndOutputBound(t *testing.T) {
	for _, tc := range []struct {
		name, script, message string
		timeout               time.Duration
	}{
		{"timeout_tree", "sleep 10 & wait", "process timeout", 100 * time.Millisecond},
		{"inherited_pipes", "sleep 10 & exit 0", "process timeout", 100 * time.Millisecond},
		{"output_bound", "while :; do printf '0123456789'; done", "process output exceeds limit", 2 * time.Second},
		{"stderr_bound", "while :; do printf '0123456789' >&2; done", "process output exceeds limit", 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			result, err := runProcess(runRequest{Binary: "/bin/sh", Args: []string{"-c", tc.script}, Dir: t.TempDir(), Timeout: tc.timeout, OutputLimit: 32})
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %s; got %v", tc.message, err)
			}
			if len(result.Stdout) > 32 || len(result.Stderr) > 32 {
				t.Fatal("unbounded output")
			}
			if time.Since(start) > 3*time.Second {
				t.Fatal("process tree cleanup exceeded bound")
			}
		})
	}
	if _, err := runProcess(runRequest{Binary: "/no-such-cli-helper", Dir: t.TempDir(), Timeout: time.Second, OutputLimit: 32}); err == nil {
		t.Fatal("start failure concealed")
	}
	if _, err := runProcess(runRequest{}); err == nil {
		t.Fatal("invalid helper bounds accepted")
	}
}

func TestHelperDescendantCleanup(t *testing.T) {
	dir := t.TempDir()
	// A surviving ordinary descendant would write after the helper returns.
	_, err := runProcess(runRequest{Binary: "/bin/sh", Args: []string{"-c", "(sleep 0.3; printf leaked > marker) & wait"}, Dir: dir, Timeout: 50 * time.Millisecond, OutputLimit: 32})
	if err == nil || !strings.Contains(err.Error(), "process timeout") {
		t.Fatalf("expected timeout, got %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 32})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files}); err != nil {
		t.Fatal(err)
	}
}
