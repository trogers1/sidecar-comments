package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type contextCallRequest struct {
	T              *testing.T
	Binary, Dir    string
	Args           []string
	Stdin          []byte
	ExpectedStatus int
	Error          bool
	Timeout        time.Duration
}
type contextEnvelope struct {
	Changed bool            `json:"changed"`
	Result  json.RawMessage `json:"result"`
}

func contextObserve(r contextCallRequest) processResult {
	r.T.Helper()
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	got, err := runProcess(runRequest{Binary: r.Binary, Dir: r.Dir, Args: append([]string{"--output", "json"}, r.Args...), Stdin: r.Stdin, Timeout: timeout, OutputLimit: domain.MaxBodyBytes})
	if err != nil {
		r.T.Fatal(err)
	}
	if got.Status != r.ExpectedStatus || (!r.Error && len(got.Stderr) != 0) || (r.Error && len(got.Stdout) != 0) {
		r.T.Fatalf("command %v: status=%d want=%d stdout=%q stderr=%q", r.Args, got.Status, r.ExpectedStatus, got.Stdout, got.Stderr)
	}
	wire := got.Stdout
	if r.Error {
		wire = got.Stderr
	}
	if err := validateSchema(schemaRequest{JSON: wire, Path: contextSchemaPath}); err != nil {
		r.T.Fatalf("command %v schema: %v", r.Args, err)
	}
	return got
}

func contextCall(r contextCallRequest) contextEnvelope {
	r.T.Helper()
	got := contextObserve(r)
	var envelope contextEnvelope
	if err := json.Unmarshal(got.Stdout, &envelope); err != nil {
		r.T.Fatal(err)
	}
	return envelope
}

// CTX-01 exercises capture/privacy, a source-independent reply and no-op sync
// through the executable. Expected evidence is authored independently of capture.
func TestSubjectContextCaptureReplyAndUnchangedSync(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	dir := t.TempDir()
	const source = "Introduction\nTarget phrase.\nConclusion\n"
	const target = "note.txt.comments"
	if err := writeFile(fileRequest{Path: filepath.Join(dir, "note.txt"), Mode: 0640, Bytes: []byte(source)}); err != nil {
		t.Fatal(err)
	}
	created := contextCall(contextCallRequest{T: t, Binary: binary, Dir: dir, Args: []string{"thread", "create", "--file", target, "--location", "range:2:1-2:15", "--author-kind", "human", "--body-file", "-", "--id", threadID, "--message-id", messageID}, Stdin: []byte("Please revise.")})
	if !created.Changed {
		t.Fatal("creation must change state")
	}
	for _, key := range []string{`"selected_text"`, `"prefix"`, `"suffix"`} {
		if bytes.Contains(created.Result, []byte(key)) {
			t.Fatalf("ordinary create exposed retained excerpt key %s", key)
		}
	}
	full := contextCall(contextCallRequest{T: t, Binary: binary, Dir: dir, Args: []string{"thread", "get", "--file", target, "--id", threadID, "--include-tracking"}})
	var got struct {
		Thread domain.Thread `json:"thread"`
	}
	if err := json.Unmarshal(full.Result, &got); err != nil {
		t.Fatal(err)
	}
	want := domain.TrackingRecord{
		Version: domain.TrackingVersion, SourceRevision: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(source))),
		LastLocation: domain.Location{Kind: domain.LocationRange, StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 15},
		SelectedText: "Target phrase.", Prefix: "Introduction\n", Suffix: "\nConclusion\n", PrefixAtStart: true, SuffixAtEnd: true,
	}
	if got.Thread.Tracking == nil || *got.Thread.Tracking != want {
		t.Fatalf("tracking=%+v; want %+v", got.Thread.Tracking, want)
	}
	if got.Thread.ID != threadID || len(got.Thread.Messages) != 1 || got.Thread.Messages[0].ID != messageID || got.Thread.Messages[0].Body != "Please revise." {
		t.Fatalf("conversation changed: %+v", got.Thread)
	}
	contextCall(contextCallRequest{T: t, Binary: binary, Dir: dir, Args: []string{"message", "add", "--file", target, "--thread", threadID, "--author-kind", "agent", "--body-file", "-"}, Stdin: []byte("I will revise it.")})
	before, err := observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxBodyBytes})
	if err != nil {
		t.Fatal(err)
	}
	synced := contextCall(contextCallRequest{T: t, Binary: binary, Dir: dir, Args: []string{"sync", "--file", target}})
	if synced.Changed {
		t.Fatal("unchanged-source sync must be a no-op")
	}
	after, err := observeFiles(filesRequest{Dir: dir, ByteLimit: domain.MaxBodyBytes})
	if err != nil {
		t.Fatal(err)
	}
	if err := compareFiles(filesExpectation{Got: after, Want: before}); err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 {
		t.Fatalf("expected source, sidecar and documented lock only: %+v", after)
	}
	final := contextCall(contextCallRequest{T: t, Binary: binary, Dir: dir, Args: []string{"thread", "get", "--file", target, "--id", threadID}})
	var conversation struct {
		Thread domain.PublicThread `json:"thread"`
	}
	if err := json.Unmarshal(final.Result, &conversation); err != nil {
		t.Fatal(err)
	}
	if conversation.Thread.ID != threadID || len(conversation.Thread.Messages) != 2 || conversation.Thread.Messages[0].ID != messageID || conversation.Thread.Messages[1].Body != "I will revise it." {
		t.Fatalf("reply not preserved: %+v", conversation.Thread)
	}
	unchanged, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil || string(unchanged) != source {
		t.Fatalf("source changed: bytes=%q error=%v", unchanged, err)
	}
}
