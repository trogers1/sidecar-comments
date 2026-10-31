package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

const contextTarget = "note.txt.comments"
const contextSource = "note.txt"

type contextFixtureRequest struct {
	T                *testing.T
	Source, Location string
}
type contextFixture struct {
	t           *testing.T
	binary, dir string
}
type contextReadRequest struct{}
type contextSourceRequest struct{ Text string }
type contextThreadResult struct {
	Thread          domain.Thread `json:"thread"`
	StorageRevision string        `json:"storage_revision"`
}

func newContextFixture(r contextFixtureRequest) contextFixture {
	r.T.Helper()
	f := contextFixture{t: r.T, binary: subjectBinary(subjectRequest{T: r.T}), dir: r.T.TempDir()}
	if err := writeFile(fileRequest{Path: filepath.Join(f.dir, contextSource), Mode: 0640, Bytes: []byte(r.Source)}); err != nil {
		r.T.Fatal(err)
	}
	f.call(contextCallRequest{Args: []string{"thread", "create", "--file", contextTarget, "--location", r.Location, "--author-kind", "human", "--body-file", "-", "--id", threadID, "--message-id", messageID}, Stdin: []byte("Keep this conversation.")})
	return f
}
func (f contextFixture) prepare(r contextCallRequest) contextCallRequest {
	r.T, r.Binary, r.Dir = f.t, f.binary, f.dir
	return r
}
func (f contextFixture) call(r contextCallRequest) contextEnvelope {
	f.t.Helper()
	return contextCall(f.prepare(r))
}
func (f contextFixture) observe(r contextCallRequest) processResult {
	f.t.Helper()
	return contextObserve(f.prepare(r))
}
func (f contextFixture) read(r contextReadRequest) contextThreadResult {
	f.t.Helper()
	response := f.call(contextCallRequest{Args: []string{"thread", "get", "--file", contextTarget, "--id", threadID, "--include-tracking"}})
	var result contextThreadResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		f.t.Fatal(err)
	}
	return result
}
func (f contextFixture) source(r contextSourceRequest) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, contextSource), []byte(r.Text), 0640); err != nil {
		f.t.Fatal(err)
	}
}
func (f contextFixture) snapshot(r contextReadRequest) []fileRequest {
	f.t.Helper()
	files, err := observeFiles(filesRequest{Dir: f.dir, ByteLimit: domain.MaxBodyBytes})
	if err != nil {
		f.t.Fatal(err)
	}
	return files
}
func (f contextFixture) sourceRevision(r contextReadRequest) string {
	f.t.Helper()
	response := f.call(contextCallRequest{Args: []string{"source", "inspect", "--file", contextTarget}})
	var result struct {
		SourceRevision string `json:"source_revision"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		f.t.Fatal(err)
	}
	return result.SourceRevision
}

type contextOutcomesResult struct {
	WouldChange bool                     `json:"would_change"`
	Outcomes    []domain.TrackingOutcome `json:"outcomes"`
}
type contextOutcomesRequest struct {
	T        *testing.T
	Response contextEnvelope
}

func contextOutcomes(r contextOutcomesRequest) contextOutcomesResult {
	r.T.Helper()
	var result contextOutcomesResult
	if err := json.Unmarshal(r.Response.Result, &result); err != nil {
		r.T.Fatal(err)
	}
	return result
}

func TestSubjectContextExactRelocation(t *testing.T) {
	for _, tc := range []struct {
		name, source, edited, location string
		want                           domain.Location
	}{
		{name: "lines_above", source: "Intro\nchosen phrase\nEnd.\n", edited: "Heading\nIntro\nchosen phrase\nEnd.\n", location: "range:2:1-2:14", want: domain.Location{Kind: domain.LocationRange, StartLine: 3, StartColumn: 1, EndLine: 3, EndColumn: 14}},
		{name: "same_line", source: "prefix chosen phrase suffix\n", edited: "longer prefix chosen phrase suffix\n", location: "range:1:8-1:21", want: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 15, EndLine: 1, EndColumn: 28}},
		{name: "utf8_bytes", source: "é chosen phrase tail\n", edited: "🌍 chosen phrase tail\n", location: "range:1:4-1:17", want: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 6, EndLine: 1, EndColumn: 19}},
		{name: "multiline", source: "before\nfirst\nsecond\nafter\n", edited: "heading\nbefore\nfirst\nsecond\nafter\n", location: "range:2:1-3:7", want: domain.Location{Kind: domain.LocationRange, StartLine: 3, StartColumn: 1, EndLine: 4, EndColumn: 7}},
		{name: "whole_line", source: "before\nchosen phrase\nafter\n", edited: "heading\nbefore\nchosen phrase\nafter\n", location: "line:2", want: domain.Location{Kind: domain.LocationLine, Line: 3}},
		{name: "crlf", source: "Intro\r\nchosen phrase\r\nEnd.\r\n", edited: "Heading\r\nIntro\r\nchosen phrase\r\nEnd.\r\n", location: "range:2:1-2:14", want: domain.Location{Kind: domain.LocationRange, StartLine: 3, StartColumn: 1, EndLine: 3, EndColumn: 14}},
		{name: "combining_bytes", source: "é chosen phrase tail\n", edited: "e\u0301 chosen phrase tail\n", location: "range:1:4-1:17", want: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 5, EndLine: 1, EndColumn: 18}},
		{name: "document_start", source: "chosen phrase tail\n", edited: "heading\nchosen phrase tail\n", location: "range:1:1-1:14", want: domain.Location{Kind: domain.LocationRange, StartLine: 2, StartColumn: 1, EndLine: 2, EndColumn: 14}},
		{name: "passage_move", source: "Intro\nchosen phrase\nEnd.\n", edited: "Other\nEnd.\nIntro\nchosen phrase", location: "range:2:1-2:14", want: domain.Location{Kind: domain.LocationRange, StartLine: 4, StartColumn: 1, EndLine: 4, EndColumn: 14}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: tc.source, Location: tc.location})
			before := f.read(contextReadRequest{})
			f.source(contextSourceRequest{Text: tc.edited})
			checkBefore := f.snapshot(contextReadRequest{})
			checked := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget, "--check"}, ExpectedStatus: domain.ExitValidation})
			proposed := contextOutcomes(contextOutcomesRequest{T: t, Response: checked})
			if checked.Changed || !proposed.WouldChange || len(proposed.Outcomes) != 1 || proposed.Outcomes[0].Location != tc.want || proposed.Outcomes[0].Outcome != domain.OutcomeMoved {
				t.Fatalf("incorrect proposal: changed=%t result=%+v", checked.Changed, proposed)
			}
			if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: checkBefore}); err != nil {
				t.Fatal(err)
			}
			synced := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
			if !synced.Changed {
				t.Fatal("relocation was not persisted")
			}
			after := f.read(contextReadRequest{})
			if after.Thread.Location != tc.want || after.Thread.Tracking == nil || before.Thread.Tracking == nil || after.Thread.Tracking.SelectedText != before.Thread.Tracking.SelectedText || after.Thread.Messages[0].ID != messageID {
				t.Fatalf("attachment/conversation: %+v", after.Thread)
			}
			if next := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}}); next.Changed {
				t.Fatal("repeated sync rewrote unchanged state")
			}
			sourceBytes, err := os.ReadFile(filepath.Join(f.dir, contextSource))
			if err != nil || string(sourceBytes) != tc.edited {
				t.Fatalf("source bytes changed: bytes=%q error=%v", sourceBytes, err)
			}
		})
	}
}

func TestSubjectContextInteriorAndBoundaryInference(t *testing.T) {
	const prefix = "Distinct introductory prose provides enough evidence before the selected passage: "
	const selected = "old words"
	const suffix = " The following paragraph provides sufficiently distinct trailing evidence.\n"
	for _, replacement := range []string{"new and expanded words", "NEW " + selected, selected + " NEW"} {
		t.Run(replacement, func(t *testing.T) {
			location := domain.LocationText(domain.LocationTextRequest{Location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len(prefix) + 1, EndLine: 1, EndColumn: len(prefix) + len(selected) + 1}})
			f := newContextFixture(contextFixtureRequest{T: t, Source: prefix + selected + suffix, Location: location})
			f.source(contextSourceRequest{Text: prefix + replacement + suffix})
			response := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
			outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: response})
			if len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Outcome != domain.OutcomeModified || outcomes.Outcomes[0].Evidence != domain.EvidenceInferred {
				t.Fatalf("expected inferred modification: %+v", outcomes)
			}
			got := f.read(contextReadRequest{})
			want := domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len(prefix) + 1, EndLine: 1, EndColumn: len(prefix) + len(replacement) + 1}
			if got.Thread.Location != want || got.Thread.Tracking.SelectedText != replacement {
				t.Fatalf("range=%+v selected=%q", got.Thread.Location, got.Thread.Tracking.SelectedText)
			}
		})
	}
}

func TestSubjectContextUnresolvedRecoveryAndPurge(t *testing.T) {
	for _, tc := range []struct{ name, edited, reason string }{
		{name: "deleted", edited: "unrelated sentence\n", reason: domain.ReasonNoMatch},
		{name: "ambiguous", edited: "prefix chosen suffix\nprefix chosen suffix\n", reason: domain.ReasonAmbiguous},
		{name: "candidate_limit", edited: strings.Repeat("prefix chosen suffix\n", domain.MaxApplyOperations+1), reason: domain.ReasonSearchLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			before := f.read(contextReadRequest{})
			f.source(contextSourceRequest{Text: tc.edited})
			response := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
			outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: response})
			if len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Outcome != domain.OutcomeUnresolved || outcomes.Outcomes[0].Reason != tc.reason {
				t.Fatalf("unexpected unresolved reason: %+v", outcomes)
			}
			after := f.read(contextReadRequest{})
			if after.Thread.Location.Kind != domain.LocationUnresolved || after.Thread.Tracking == nil || *after.Thread.Tracking != *before.Thread.Tracking {
				t.Fatalf("lost evidence: %+v", after.Thread)
			}
			stable := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
			if stable.Changed {
				t.Fatal("unresolved repetition changed retained evidence")
			}
			check := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget, "--check"}, ExpectedStatus: domain.ExitValidation})
			if contextOutcomes(contextOutcomesRequest{T: t, Response: check}).WouldChange {
				t.Fatal("stable unresolved check claimed storage drift")
			}
			f.call(contextCallRequest{Args: []string{"thread", "move", "--file", contextTarget, "--id", threadID, "--location", "line:1"}})
			if f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}}).Changed {
				t.Fatal("explicit reattachment did not establish current tracking")
			}
			f.call(contextCallRequest{Args: []string{"thread", "move", "--file", contextTarget, "--id", threadID, "--location", "lost"}})
			lost := f.read(contextReadRequest{})
			if lost.Thread.Tracking != nil || lost.Thread.Location.Kind != domain.LocationLost || lost.Thread.Messages[0].ID != messageID {
				t.Fatalf("lost did not purge tracking: %+v", lost.Thread)
			}
			storage, err := os.ReadFile(filepath.Join(f.dir, contextTarget))
			if err != nil || bytes.Contains(storage, []byte(`"selected_text"`)) {
				t.Fatalf("tracking bytes remain: %q error=%v", storage, err)
			}
			if err := os.Remove(filepath.Join(f.dir, contextSource)); err != nil {
				t.Fatal(err)
			}
			f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
			f.call(contextCallRequest{Args: []string{"message", "delete", "--file", contextTarget, "--thread", threadID, "--id", messageID}})
			if _, err := os.Stat(filepath.Join(f.dir, contextTarget)); !os.IsNotExist(err) {
				t.Fatalf("last-message deletion retained sidecar: %v", err)
			}
		})
	}
}
