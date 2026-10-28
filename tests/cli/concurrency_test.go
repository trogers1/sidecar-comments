package cli_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Independent executable processes start behind a shared barrier. No response
// order or wall-clock overlap is assumed: either legal serialization is exact,
// and the final independently declared combined document must survive both.
func TestSubjectIndependentWriters(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "body", Mode: 0600, Bytes: []byte("x\n")}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640}}
	for _, f := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{threadID, "018f0000-0000-7000-8000-000000000004"}
	messages := []string{messageID, "018f0000-0000-7000-8000-000000000005"}
	documents := make([]string, 2)
	threads := make([]string, 2)
	for i := range ids {
		documents[i] = "[@th_" + ids[i] + " lost]\n---user <!-- sidecar-meta: {\"id\":\"" + messages[i] + "\"} -->\nx\n[/@th_" + ids[i] + "]\n"
		threads[i] = fmt.Sprintf(`{"id":%q,"location":{"kind":"lost"},"messages":[{"id":%q,"author":{"kind":"human"},"body":"x"}]}`, ids[i], messages[i])
	}
	combined := []byte(documents[0] + "\n" + documents[1])
	combinedRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(combined))
	start := make(chan struct{})
	results := make([]chan processResult, 2)
	failures := make([]chan error, 2)
	for i := range ids {
		results[i] = make(chan processResult, 1)
		failures[i] = make(chan error, 1)
		go func() {
			<-start
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "create", "--file", "note.txt.comments", "--location", "lost", "--author-kind", "human", "--body-file", "body", "--id", ids[i], "--message-id", messages[i]}, Dir: dir, Timeout: 10 * time.Second, OutputLimit: 8192})
			results[i] <- got
			failures[i] <- err
		}()
	}
	close(start)
	observed := make([]processResult, 2)
	var processErrors []error
	for i := range ids {
		observed[i] = <-results[i]
		processErrors = append(processErrors, <-failures[i])
	}
	if err := errors.Join(processErrors...); err != nil {
		t.Fatal(err)
	}
	for i := range ids {
		if err := validateSchema(schemaRequest{JSON: observed[i].Stdout}); err != nil {
			t.Fatal(err)
		}
	}
	// Each candidate serialization fixes both complete process observations.
	legal := false
	for _, first := range []int{0, 1} {
		matches := true
		for i := range ids {
			revision := combinedRevision
			if i == first {
				revision = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(documents[i])))
			}
			want := fmt.Sprintf(`{"protocol":1,"command":"thread-create","changed":true,"result":{"thread":%s,"storage_revision":%q,"canonical_revision":%q}}`+"\n", threads[i], revision, revision)
			if compareProcess(processExpectation{Got: observed[i], Want: processResult{Stdout: []byte(want)}}) != nil {
				matches = false
			}
		}
		if matches {
			legal = true
		}
	}
	if !legal {
		t.Fatalf("responses do not match either legal serialization: %#v", observed)
	}
	initial[3].Bytes = combined
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}
}
