package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestSubjectDeleteOutputModes(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	previous := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalNote)))
	empty := fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	result := fmt.Sprintf(`{"deleted_thread_id":%q,"previous_storage_revision":%q,"storage_revision":null,"canonical_revision":%q}`, threadID, previous, empty)
	envelope := `{"protocol":1,"command":"thread-delete","changed":true,"result":` + result + "}\n"
	if err := validateSchema(schemaRequest{JSON: []byte(envelope)}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"json", "text", "none"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(canonicalNote)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", mode, "thread", "delete", "--if-revision", previous, "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if mode == "json" {
				want = envelope
			} else if mode == "text" {
				want = "thread-delete changed=true result=" + result + "\n"
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
					t.Fatal(err)
				}
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial[:2]}); err != nil {
				t.Fatal(err)
			}
			got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", mode, "thread", "delete", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			failure := "error not_found not_found:thread\n"
			if mode == "json" {
				failure = `{"protocol":1,"error":{"code":"not_found","message":"not_found:thread","details":{"entity":"thread","id":"` + threadID + `"}}}` + "\n"
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 5, Stderr: []byte(failure)}}); err != nil {
				t.Fatal(err)
			}
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
					t.Fatal(err)
				}
			}
			files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial[:2]}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
