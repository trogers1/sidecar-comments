package cli_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The parent holds an actual POSIX advisory lock before launching the subject;
// no scheduling delay or guessed overlap is used to induce the timeout.
func TestSubjectLockTimeout(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := os.OpenFile(filepath.Join(dir, initial[0].Path), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
			t.Error(err)
		}
	}()
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "delete", "--lock-timeout", "1ms", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"protocol":1,"error":{"code":"conflict","message":"conflict:lock_timeout","details":{"kind":"lock_timeout"}}}` + "\n"
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 4, Stderr: []byte(want)}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
