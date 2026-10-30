package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Help must work before any required flags, file access, or stdin consumption.
// An open, empty pipe never reaches EOF: reading it would hang this invocation.
func TestSubjectHelp(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	commands := [][]string{nil, {"thread"}, {"message"}, {"version"}, {"validate"}, {"format"}, {"fix"}, {"apply"}, {"thread", "list"}, {"thread", "get"}, {"thread", "create"}, {"thread", "move"}, {"thread", "delete"}, {"message", "add"}, {"message", "update"}, {"message", "delete"}}
	for _, command := range commands {
		for _, flag := range []string{"--help", "-h"} {
			t.Run(strings.Join(command, "_")+flag, func(t *testing.T) {
				dir := t.TempDir()
				fixture := fileRequest{Path: "sentinel", Mode: 0640, Bytes: []byte("untouched\n")}
				if err := writeFile(fileRequest{Path: filepath.Join(dir, fixture.Path), Mode: fixture.Mode, Bytes: fixture.Bytes}); err != nil {
					t.Fatal(err)
				}
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, append(append([]string{}, command...), flag)...)
				cmd.Dir = dir
				cmd.Stdin = reader
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("help must exit successfully without reading stdin: %v; stderr=%s", err, stderr.String())
				}
				if stderr.Len() != 0 {
					t.Fatalf("help stderr=%q", stderr.String())
				}
				text := strings.ToLower(stdout.String())
				for _, term := range []string{"usage:", "--help", "--output"} {
					if !strings.Contains(text, term) {
						t.Errorf("help missing %q: %s", term, text)
					}
				}
				if len(command) > 0 && command[0] != "thread" && command[0] != "message" || len(command) == 2 {
					if !strings.Contains(text, "example:") {
						t.Errorf("command help lacks example: %s", text)
					}
					if command[0] != "version" && (!strings.Contains(text, "--file") || !strings.Contains(text, "required")) {
						t.Errorf("command help lacks required file description: %s", text)
					}
				}
				files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files, Want: []fileRequest{fixture}}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSubjectRootNoArguments(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	got, err := runProcess(runRequest{Binary: binary, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 16384})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != 0 || len(got.Stderr) != 0 || !bytes.Contains(bytes.ToLower(got.Stdout), []byte("usage:")) {
		t.Fatalf("root must show successful stdout help: %+v", got)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectHelpExplainsRequiredInputs(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct {
		args  []string
		terms []string
	}{
		{[]string{"thread", "create"}, []string{"--location", "line:", "--author-kind", "human", "--body-file", "stdin", "required"}},
		{[]string{"message", "update"}, []string{"--thread", "--id", "--body-file", "required"}},
		{[]string{"apply"}, []string{"--operations-file", "json", "stdin", "required"}},
		{[]string{"fix"}, []string{"--add-missing-message-ids", "--check", "required"}},
	} {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Args: append(tc.args, "--help"), Dir: t.TempDir(), Timeout: 5 * time.Second, OutputLimit: 16384})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != 0 || len(got.Stderr) != 0 {
				t.Fatalf("help failed: %+v", got)
			}
			for _, term := range tc.terms {
				if !strings.Contains(strings.ToLower(string(got.Stdout)), term) {
					t.Errorf("help missing %q: %s", term, got.Stdout)
				}
			}
		})
	}
}
