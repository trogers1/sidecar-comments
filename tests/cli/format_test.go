package cli_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// One public lifecycle, not a parser/serializer oracle supplied by the subject.
// format.md defines framing and metadata id,name order; cli.md defines argv
// and check status; protocol.md and schema/v1.json define envelope field order.
func TestFormatLifecycle(t *testing.T) {
	binary := os.Getenv("SIDECAR_BIN")
	if binary == "" {
		t.Fatal("SIDECAR_BIN is required; helper/schema success is not Sidecar behavioral proof")
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		t.Fatalf("SIDECAR_BIN must identify an executable: %v", err)
	}
	// Resolve relative supplied paths before running the child in its fixture.
	binary, err = filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}

	original := []byte("[@th_018f0000-0000-7000-8000-000000000001 file]\n" +
		"---user <!-- sidecar-meta: { \"name\" : \"Taylor\", \"id\" : \"018f0000-0000-7000-8000-000000000002\" } -->\n" +
		"Please clarify this note.\n" +
		"[/@th_018f0000-0000-7000-8000-000000000001]\n")
	canonical := []byte("[@th_018f0000-0000-7000-8000-000000000001 file]\n" +
		"---user <!-- sidecar-meta: {\"id\":\"018f0000-0000-7000-8000-000000000002\",\"name\":\"Taylor\"} -->\n" +
		"Please clarify this note.\n" +
		"[/@th_018f0000-0000-7000-8000-000000000001]\n")
	// index.md: exact declared disk bytes vs independently declared canonical
	// bytes. Never derive either revision from subject output or rewritten files.
	originalRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(original))
	canonicalRevision := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	dir := t.TempDir()
	// storage-security.md permits reusable persistent locks. Precreating this
	// user-approved empty lock avoids prescribing new-lock contents or mode.
	initial := []fileRequest{
		{Path: ".note.txt.comments.sidecar.lock", Mode: 0600, Bytes: []byte{}},
		{Path: "note.txt", Mode: 0640, Bytes: []byte("A source note.\n")},
		{Path: "note.txt.comments", Mode: 0640, Bytes: original},
	}
	for _, file := range initial {
		if err := writeFile(fileRequest{Path: filepath.Join(dir, file.Path), Mode: file.Mode, Bytes: file.Bytes}); err != nil {
			t.Fatal(err)
		}
	}

	// Noncanonical metadata is formatter input; check reports work without
	// changing bytes or taking a write lock (cli.md, format.md).
	got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stdout: []byte(fmt.Sprintf("{\"protocol\":1,\"command\":\"format\",\"changed\":false,\"result\":{\"formatted\":false,\"storage_revision\":%q,\"canonical_revision\":%q}}\n", originalRevision, canonicalRevision))}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
		t.Fatal(err)
	}

	// Atomic replacement preserves the existing target mode; only metadata
	// spelling changes. Revisions describe the resulting canonical document.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 0, Stdout: []byte(fmt.Sprintf("{\"protocol\":1,\"command\":\"format\",\"changed\":true,\"result\":{\"formatted\":true,\"storage_revision\":%q,\"canonical_revision\":%q}}\n", canonicalRevision, canonicalRevision))}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	formatted := []fileRequest{initial[0], initial[1], {Path: "note.txt.comments", Mode: initial[2].Mode, Bytes: canonical}}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: formatted}); err != nil {
		t.Fatal(err)
	}

	// File location requires a valid readable supplied source, but no
	// coordinate check (source-and-location.md). Validation never writes.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 0, Stdout: []byte(fmt.Sprintf("{\"protocol\":1,\"command\":\"validate\",\"changed\":false,\"result\":{\"valid\":true,\"warnings\":[],\"storage_revision\":%q,\"canonical_revision\":%q}}\n", canonicalRevision, canonicalRevision))}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: formatted}); err != nil {
		t.Fatal(err)
	}

	// The same check now succeeds unchanged: canonicalization is idempotent.
	got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 0, Stdout: []byte(fmt.Sprintf("{\"protocol\":1,\"command\":\"format\",\"changed\":false,\"result\":{\"formatted\":false,\"storage_revision\":%q,\"canonical_revision\":%q}}\n", canonicalRevision, canonicalRevision))}}); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
		t.Fatal(err)
	}
	files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: files, Want: formatted}); err != nil {
		t.Fatal(err)
	}
}
