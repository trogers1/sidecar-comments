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

func TestSubjectDocumentLimits(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, kind := range []string{"threads", "messages_thread", "messages_document"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			var document strings.Builder
			args := []string{"--output", "json"}
			operation := ""
			if kind == "threads" {
				for i := 1; i <= 1024; i++ {
					id := fmt.Sprintf("018f0000-0000-7000-8000-%012x", i)
					mid := fmt.Sprintf("018f0001-0000-7000-8000-%012x", i)
					if i > 1 {
						document.WriteByte('\n')
					}
					document.WriteString("[@th_" + id + " lost]\n---user <!-- sidecar-meta: {\"id\":\"" + mid + "\"} -->\nx\n[/@th_" + id + "]\n")
				}
				args = append(args, "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", "018f0002-0000-7000-8000-000000000001", "--message-id", "018f0002-0000-7000-8000-000000000002")
				operation = "thread-create"
			} else if kind == "messages_document" {
				for i := 1; i <= 64; i++ {
					id := fmt.Sprintf("018f0000-0000-7000-8000-%012x", i)
					if i > 1 {
						document.WriteByte('\n')
					}
					document.WriteString("[@th_" + id + " lost]\n")
					for j := 1; j <= 256; j++ {
						if j > 1 {
							document.WriteByte('\n')
						}
						document.WriteString(fmt.Sprintf("---user <!-- sidecar-meta: {\"id\":\"018f0001-0000-7000-8000-%012x\"} -->\nx\n", (i-1)*256+j))
					}
					document.WriteString("[/@th_" + id + "]\n")
				}
				args = append(args, "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", "018f0002-0000-7000-8000-000000000001", "--message-id", messageID)
				operation = "thread-create"
			} else {
				document.WriteString("[@th_" + threadID + " lost]\n")
				for i := 1; i <= 256; i++ {
					if i > 1 {
						document.WriteByte('\n')
					}
					document.WriteString(fmt.Sprintf("---user <!-- sidecar-meta: {\"id\":\"018f0001-0000-7000-8000-%012x\"} -->\nx\n", i))
				}
				document.WriteString(endThread)
				args = append(args, "message", "add", "--file", "note.txt.comments", "--thread", threadID, "--author-kind", "human", "--body-file", "body", "--id", messageID)
				operation = "message-add"
			}
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(document.String())}}
			for _, f := range initial {
				if len(f.Bytes) <= 1<<20 {
					if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
						t.Fatal(err)
					}
				} else {
					file, err := os.OpenFile(filepath.Join(dir, f.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := file.Write(f.Bytes); err != nil {
						t.Fatal(err)
					}
					if err := file.Chmod(f.Mode); err != nil {
						t.Fatal(err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			// The exact maximum itself must validate, not merely reject the next write.
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(document.String())))
			want := fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			{
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != len(initial) {
					t.Fatal("unexpected directory entries")
				}
				for i, f := range initial {
					if entries[i].Name() != f.Path {
						t.Fatal("unexpected entry name")
					}
					info, err := os.Lstat(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					if info.Mode() != f.Mode || !bytes.Equal(data, f.Bytes) {
						t.Fatalf("fixture changed: %s", f.Path)
					}
				}
			}
			got, err = runProcess(runRequest{Binary: binary, Args: args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want = fmt.Sprintf(`{"protocol":1,"error":{"code":"precondition","message":"precondition:%s:limit_%s","details":{"operation":%q,"reason":"limit_%s"}}}`+"\n", operation, kind, operation, kind)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 6, Stderr: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
				t.Fatal(err)
			}
			{
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				if len(entries) != len(initial) {
					t.Fatal("unexpected directory entries")
				}
				for i, f := range initial {
					if entries[i].Name() != f.Path {
						t.Fatal("unexpected entry name")
					}
					info, err := os.Lstat(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					data, err := os.ReadFile(filepath.Join(dir, f.Path))
					if err != nil {
						t.Fatal(err)
					}
					if info.Mode() != f.Mode || !bytes.Equal(data, f.Bytes) {
						t.Fatalf("fixture changed: %s", f.Path)
					}
				}
			}
		})
	}
}
