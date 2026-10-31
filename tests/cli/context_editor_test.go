package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type contextEditorInputRequest struct {
	T       *testing.T
	Request domain.EditorRequest
}

func contextEditorInput(r contextEditorInputRequest) []byte {
	r.T.Helper()
	body, err := json.Marshal(r.Request)
	if err != nil {
		r.T.Fatal(err)
	}
	if err := validateSchema(schemaRequest{JSON: body, Path: contextSchemaPath}); err != nil {
		r.T.Fatalf("invalid machine request fixture: %v", err)
	}
	return body
}

func TestSubjectContextEditorTrackedBoundary(t *testing.T) {
	f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
	before := f.read(contextReadRequest{})
	f.source(contextSourceRequest{Text: "prefix NEW chosen suffix\n"})
	location := domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 12, EndLine: 1, EndColumn: 18}
	request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: before.StorageRevision, SourceRevision: f.sourceRevision(contextReadRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Location: &location}}}
	response := f.call(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request})})
	outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: response})
	if !response.Changed || len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Evidence != domain.EvidenceEditor || outcomes.Outcomes[0].Outcome != domain.OutcomeMoved {
		t.Fatalf("tracked result: %+v", outcomes)
	}
	after := f.read(contextReadRequest{})
	if after.Thread.Location != location || after.Thread.Tracking.SelectedText != "chosen" || after.Thread.Messages[0].ID != messageID {
		t.Fatalf("boundary insertion was not excluded: %+v", after.Thread)
	}
	if f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}}).Changed {
		t.Fatal("editor mapping did not establish current context")
	}
}

func TestSubjectContextEditorRevisionConflicts(t *testing.T) {
	for _, conflict := range []string{"source_revision", "revision"} {
		t.Run(conflict, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			read := f.read(contextReadRequest{})
			location := read.Thread.Location
			request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: read.StorageRevision, SourceRevision: f.sourceRevision(contextReadRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Location: &location}}}
			if conflict == "source_revision" {
				// Coordinates remain valid; only fingerprint checking can detect this.
				f.source(contextSourceRequest{Text: "PREFIX chosen suffix\n"})
			} else {
				f.call(contextCallRequest{Args: []string{"message", "add", "--file", contextTarget, "--thread", threadID, "--author-kind", "agent", "--body-file", "-"}, Stdin: []byte("External reply must survive.")})
			}
			before := f.snapshot(contextReadRequest{})
			result := f.observe(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request}), ExpectedStatus: domain.ExitConflict, Error: true})
			if !bytes.Contains(result.Stderr, []byte(`"message":"conflict:`+conflict+`"`)) {
				t.Fatalf("wrong conflict: %q", result.Stderr)
			}
			if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
				t.Fatal(err)
			}
			if conflict == "revision" {
				reread := f.read(contextReadRequest{})
				if len(reread.Thread.Messages) != 2 || reread.Thread.Messages[1].Body != "External reply must survive." {
					t.Fatal("external reply lost")
				}
				request.IfRevision = reread.StorageRevision
				f.call(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request})})
			}
		})
	}
}

func TestSubjectContextEditorAtomicBatchAndDeletion(t *testing.T) {
	f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
	read := f.read(contextReadRequest{})
	location := domain.Location{Kind: domain.LocationLine, Line: 1}
	request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: read.StorageRevision, SourceRevision: f.sourceRevision(contextReadRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Location: &location}, {ThreadID: threadID, Location: &location}}}
	before := f.snapshot(contextReadRequest{})
	result := f.observe(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request}), ExpectedStatus: domain.ExitPrecondition, Error: true})
	if !bytes.Contains(result.Stderr, []byte(`"reason":"duplicate_thread","index":1`)) {
		t.Fatalf("missing indexed duplicate rejection: %q", result.Stderr)
	}
	if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
		t.Fatal(err)
	}
	f.source(contextSourceRequest{Text: "prefix suffix\n"})
	deleted := true
	request.SourceRevision = f.sourceRevision(contextReadRequest{})
	request.Updates = []domain.EditorUpdate{{ThreadID: threadID, Deleted: &deleted}}
	f.call(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request})})
	after := f.read(contextReadRequest{})
	if after.Thread.Location.Kind != domain.LocationUnresolved || *after.Thread.Tracking != *read.Thread.Tracking || after.Thread.Messages[0].ID != messageID {
		t.Fatalf("deleted selection lost evidence/conversation: %+v", after.Thread)
	}
}

func TestSubjectContextEditorOversizeCapture(t *testing.T) {
	f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
	read := f.read(contextReadRequest{})
	f.source(contextSourceRequest{Text: strings.Repeat("x", domain.MaxAnchorBytes+1)})
	location := domain.Location{Kind: domain.LocationLine, Line: 1}
	request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: read.StorageRevision, SourceRevision: f.sourceRevision(contextReadRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Location: &location}}}
	before := f.snapshot(contextReadRequest{})
	result := f.observe(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request}), ExpectedStatus: domain.ExitPrecondition, Error: true})
	if !bytes.Contains(result.Stderr, []byte(`"reason":"limit_selection","index":0`)) {
		t.Fatalf("wrong capture limit: %q", result.Stderr)
	}
	if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
		t.Fatal(err)
	}
}
