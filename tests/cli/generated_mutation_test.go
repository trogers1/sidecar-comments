package cli_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectGeneratedCreateAdd(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, mode := range []string{"json", "id"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x\n")}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", mode, "thread", "create", "--file", "note.txt.comments", "--location", "file", "--author-kind", "human", "--body-file", "body"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			var tid, mid string
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
					t.Fatal(err)
				}
				var envelope struct {
					Result struct {
						GeneratedIDs struct {
							ThreadID  string `json:"thread_id"`
							MessageID string `json:"message_id"`
						} `json:"generated_ids"`
					} `json:"result"`
				}
				if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
					t.Fatal(err)
				}
				tid = envelope.Result.GeneratedIDs.ThreadID
				mid = envelope.Result.GeneratedIDs.MessageID
			} else {
				ids := strings.Split(string(got.Stdout), "\n")
				if len(ids) != 3 || ids[2] != "" {
					t.Fatalf("want two LF-terminated IDs: %q", got.Stdout)
				}
				tid = ids[0]
				mid = ids[1]
			}
			// Validate extracted unpredictable values using the existing closed schema.
			qt, err := json.Marshal(tid)
			if err != nil {
				t.Fatal(err)
			}
			qm, err := json.Marshal(mid)
			if err != nil {
				t.Fatal(err)
			}
			request := `{"protocol":1,"operations":[{"op":"message_delete","thread_id":` + string(qt) + `,"message_id":` + string(qm) + `}]}`
			if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
				t.Fatal(err)
			}
			if tid == mid {
				t.Fatal("create generated duplicate UUID values within one process")
			}
			message := fmt.Sprintf(`{"id":%q,"author":{"kind":"human"},"body":"x"}`, mid)
			thread := fmt.Sprintf(`{"id":%q,"location":{"kind":"file"},"messages":[%s]}`, tid, message)
			document := []byte("[@th_" + tid + " file]\n---user <!-- sidecar-meta: {\"id\":\"" + mid + "\"} -->\nx\n[/@th_" + tid + "]\n")
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
			want := fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q,"generated_ids":{"thread_id":%q,"message_id":%q}}}`+"\n", thread, revision, revision, tid, mid)
			if mode == "id" {
				want = tid + "\n" + mid + "\n"
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			initial[3].Bytes = document
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
			if mode == "id" {
				checked, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
				if err != nil {
					t.Fatal(err)
				}
				expected := fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
				if err := compareProcess(processExpectation{Got: checked, Want: processResult{Stdout: []byte(expected)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: checked.Stdout}); err != nil {
					t.Fatal(err)
				}
				observed, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: observed, Want: initial}); err != nil {
					t.Fatal(err)
				}
			}
			got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", mode, "message", "add", "--file", "note.txt.comments", "--thread", tid, "--author-kind", "agent", "--body-file", "body"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			var replyID string
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
					t.Fatal(err)
				}
				var envelope struct {
					Result struct {
						GeneratedID string `json:"generated_id"`
					} `json:"result"`
				}
				if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
					t.Fatal(err)
				}
				replyID = envelope.Result.GeneratedID
			} else {
				replyID = strings.TrimSuffix(string(got.Stdout), "\n")
			}
			qr, err := json.Marshal(replyID)
			if err != nil {
				t.Fatal(err)
			}
			request = `{"protocol":1,"operations":[{"op":"message_delete","thread_id":` + string(qt) + `,"message_id":` + string(qr) + `}]}`
			if err := validateSchema(schemaRequest{JSON: []byte(request)}); err != nil {
				t.Fatal(err)
			}
			if replyID == mid {
				t.Fatal("generated add ID collides with existing message")
			}
			// No UUID ordering comparison across these separate command processes.
			reply := fmt.Sprintf(`{"id":%q,"author":{"kind":"agent"},"body":"x"}`, replyID)
			thread = fmt.Sprintf(`{"id":%q,"location":{"kind":"file"},"messages":[%s,%s]}`, tid, message, reply)
			document = []byte("[@th_" + tid + " file]\n---user <!-- sidecar-meta: {\"id\":\"" + mid + "\"} -->\nx\n\n---bot <!-- sidecar-meta: {\"id\":\"" + replyID + "\"} -->\nx\n[/@th_" + tid + "]\n")
			revision = fmt.Sprintf("sha256:%x", sha256.Sum256(document))
			want = fmt.Sprintf(`{"protocol":1,"command":"message-add","changed":true,"result":{"thread":%s,"message":%s,"storage_revision":%q,"canonical_revision":%q,"generated_id":%q}}`+"\n", thread, reply, revision, revision, replyID)
			if mode == "id" {
				want = replyID + "\n"
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			initial[3].Bytes = document
			files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
			if mode == "id" {
				checked, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
				if err != nil {
					t.Fatal(err)
				}
				expected := fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision)
				if err := compareProcess(processExpectation{Got: checked, Want: processResult{Stdout: []byte(expected)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: checked.Stdout}); err != nil {
					t.Fatal(err)
				}
				observed, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: observed, Want: initial}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
