package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSubjectHelpAfterFlagValues(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, prefix := range [][]string{{"--version"}, {"--output", "json", "--version"}, {"thread", "create", "--body-file", "-"}, {"--output", "json", "thread", "create", "--body-file", "-"}} {
		for _, help := range []string{"--help", "-h"} {
			t.Run(strings.Join(prefix, "_")+help, func(t *testing.T) {
				dir := t.TempDir()
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				args := append(append([]string{}, prefix...), help)
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Dir = dir
				cmd.Stdin = reader
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("help must not wait for stdin or require flags: %v, stderr=%q", err, stderr.String())
				}
				if stderr.Len() != 0 || !strings.Contains(strings.ToLower(stdout.String()), "usage:") || bytes.Contains(stdout.Bytes(), []byte(`"protocol":`)) {
					t.Fatalf("expected plain stdout help: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
				files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSubjectHelpLikeFlagValuesAreLiteral(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, value := range []string{"--help", "-h"} {
		t.Run(value, func(t *testing.T) {
			dir := t.TempDir()
			got, err := runProcess(runRequest{Binary: binary, Dir: dir, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--author-name", value, "--body-file", "-", "--id", threadID, "--message-id", messageID}, Stdin: []byte("literal flag value"), Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != 0 || len(got.Stderr) != 0 {
				t.Fatalf("literal author name must execute create: %+v", got)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got.Stdout, []byte(`"name":"`+value+`"`)) || !bytes.Contains(got.Stdout, []byte(`"command":"thread-create"`)) {
				t.Fatalf("literal author name did not execute: %s", got.Stdout)
			}
		})
	}
	// A help-looking filename is a value, not an informational help request.
	got, err := runProcess(runRequest{Binary: binary, Dir: t.TempDir(), Args: []string{"--output", "json", "validate", "--file", "--help"}, Timeout: 5 * time.Second, OutputLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == 0 || len(got.Stdout) != 0 || !bytes.Contains(got.Stderr, []byte(`"error":`)) {
		t.Fatalf("--file --help must take normal validation/error path: %+v", got)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
		t.Fatal(err)
	}
}
