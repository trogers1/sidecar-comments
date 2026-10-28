package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Both public body transports remove exactly one final LF after consistent
// CRLF normalization. Explicit-ID retry compares normalized stored fields.
func TestSubjectBodyCreateRetry(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, transport := range []struct{ name, raw, logical string }{
		{"no_terminator", "x", "x"}, {"lf", "x\n", "x"}, {"crlf", "x\r\n", "x"}, {"two_lfs", "x\n\n", "x\n"}, {"two_crlfs", "x\r\n\r\n", "x\n"},
	} {
		for _, stdin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stdin=%t", transport.name, stdin), func(t *testing.T) {
				dir := t.TempDir()
				initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body.txt", Mode: 0600, Bytes: []byte(transport.raw)}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640}}
				for _, f := range initial {
					if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
						t.Fatal(err)
					}
				}
				bodyPath := "body.txt"
				var input []byte
				if stdin {
					bodyPath = "-"
					input = []byte(transport.raw)
				}
				args := []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", "file", "--author-kind", "human", "--body-file", bodyPath, "--id", threadID, "--message-id", messageID}
				document := []byte("[@th_" + threadID + " file]\n" + role + transport.logical + "\n" + endThread)
				revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
				thread := fmt.Sprintf(`{"id":%q,"location":{"kind":"file"},"messages":[{"id":%q,"author":{"kind":"human"},"body":%q}]}`, threadID, messageID, transport.logical)
				got, err := runProcess(runRequest{Binary: binary, Args: args, Dir: dir, Stdin: input, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
				if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
					t.Fatal(err)
				}
				initial[3].Bytes = document
				files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
					t.Fatal(err)
				}
				got, err = runProcess(runRequest{Binary: binary, Args: args, Dir: dir, Stdin: input, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				want = fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":false,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", thread, revision, revision)
				if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
					t.Fatal(err)
				}
				files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
					t.Fatal(err)
				}
				// Revision mismatch wins over the absent entity semantic failure.
				got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "delete", "--if-revision", "missing", "--file", "note.txt.comments", "--id", "018f0000-0000-7000-8000-000000000003"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				want = fmt.Sprintf(`{"protocol":1,"error":{"code":"conflict","message":"conflict:revision","details":{"kind":"revision","expected":"missing","current_storage_revision":%q}}}`+"\n", revision)
				if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 4, Stderr: []byte(want)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
					t.Fatal(err)
				}
				files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
