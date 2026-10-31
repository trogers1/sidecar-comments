package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

// Matrix journeys not covered elsewhere. Each prose paragraph is longer than
// the retained context window (checked by requireMatrixProse), so a
// selection's context never reaches text a whole paragraph away.
const matrixLead = "The committee reviewed the harbour survey, the archive inventory, and the orchard budget before lunch. " +
	"Afterwards, the treasurer described seven unrelated invoices while the secretary recorded questions about lighting. " +
	"Finally, everyone agreed that the bridge inspection deserved its own separate meeting next spring.\n"
const matrixTail = "Later that week, volunteers repainted the library railings and catalogued donated maps from the attic. " +
	"A visiting engineer measured the old culvert and promised a written estimate before the autumn storms arrived. " +
	"Nobody expected the bakery on the corner to sponsor the lantern parade, yet it happily did so again.\n"

const contextLockFile = "." + contextTarget + ".sidecar.lock"

type matrixProseRequest struct{ T *testing.T }

func requireMatrixProse(r matrixProseRequest) {
	r.T.Helper()
	for _, paragraph := range []string{matrixLead, matrixTail} {
		if len(paragraph) <= domain.MaxContextBytes {
			r.T.Fatalf("fixture paragraph (%d bytes) must exceed the %d-byte context window", len(paragraph), domain.MaxContextBytes)
		}
	}
}

type contextRangeRequest struct {
	T            *testing.T
	Text, Needle string
	Occurrence   int
}

// contextRange is an independent coordinate oracle: 1-based line and UTF-8
// byte column of the chosen occurrence, with a half-open end.
func contextRange(r contextRangeRequest) domain.Location {
	r.T.Helper()
	start := -1
	for i, from := 0, 0; i <= r.Occurrence; i++ {
		index := strings.Index(r.Text[from:], r.Needle)
		if index < 0 {
			r.T.Fatalf("occurrence %d of %q not found", r.Occurrence, r.Needle)
		}
		start = from + index
		from = start + len(r.Needle)
	}
	position := func(offset int) (int, int) {
		return 1 + strings.Count(r.Text[:offset], "\n"), offset - (strings.LastIndex(r.Text[:offset], "\n") + 1) + 1
	}
	startLine, startColumn := position(start)
	endLine, endColumn := position(start + len(r.Needle))
	return domain.Location{Kind: domain.LocationRange, StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}
}

type contextCreateRequest struct {
	ID, MessageID, Body string
	Location            domain.Location
}

func (f contextFixture) create(r contextCreateRequest) {
	f.t.Helper()
	f.call(contextCallRequest{Args: []string{"thread", "create", "--file", contextTarget, "--location", domain.LocationText(domain.LocationTextRequest{Location: r.Location}), "--author-kind", "human", "--body-file", "-", "--id", r.ID, "--message-id", r.MessageID}, Stdin: []byte(r.Body)})
}

type contextReadIDRequest struct{ ID string }

func (f contextFixture) readID(r contextReadIDRequest) contextThreadResult {
	f.t.Helper()
	response := f.call(contextCallRequest{Args: []string{"thread", "get", "--file", contextTarget, "--id", r.ID, "--include-tracking"}})
	var result contextThreadResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f contextFixture) sidecar(r contextReadRequest) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, contextTarget))
	if err != nil {
		f.t.Fatal(err)
	}
	return data
}

type contextErrorRequest struct {
	Result  processResult
	Message string
}

func (f contextFixture) requireError(r contextErrorRequest) {
	f.t.Helper()
	if !bytes.Contains(r.Result.Stderr, []byte(`"message":"`+r.Message+`"`)) {
		f.t.Fatalf("want error %s, got %q", r.Message, r.Result.Stderr)
	}
}

type contextSyncOneRequest struct {
	Outcome, Reason, Evidence string
	Location                  domain.Location
}

// syncOne runs a write sync over a single-thread fixture and checks its outcome.
func (f contextFixture) syncOne(r contextSyncOneRequest) contextEnvelope {
	f.t.Helper()
	response := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
	outcomes := contextOutcomes(contextOutcomesRequest{T: f.t, Response: response}).Outcomes
	want := domain.TrackingOutcome{ThreadID: threadID, Outcome: r.Outcome, Reason: r.Reason, Evidence: r.Evidence, Location: r.Location}
	if len(outcomes) != 1 || outcomes[0] != want {
		f.t.Fatalf("outcomes %+v; want %+v", outcomes, want)
	}
	return response
}

func rangeText(r contextRangeRequest) string {
	return domain.LocationText(domain.LocationTextRequest{Location: contextRange(r)})
}

// CTX-02 (deleting lines above) and CTX-05 (edits above combined with an edit
// inside the selection).
func TestSubjectContextEditsAboveAndInside(t *testing.T) {
	requireMatrixProse(matrixProseRequest{T: t})
	const head = "Heading one\nHeading two\n"
	const old, changed = "selected old wording", "selected new wording"
	body := matrixLead + "The " + old + " remains here.\n" + matrixTail
	for _, tc := range []struct {
		name, edited, selection   string
		outcome, reason, evidence string
	}{
		{name: "delete_above", edited: "Heading one\n" + body, selection: old, outcome: domain.OutcomeMoved, reason: domain.ReasonExactMatch, evidence: domain.EvidenceExact},
		{name: "insert_above_and_edit_inside", edited: "Inserted line\n" + head + strings.Replace(body, old, changed, 1), selection: changed, outcome: domain.OutcomeModified, reason: domain.ReasonContextReplacement, evidence: domain.EvidenceInferred},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := head + body
			f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: old})})
			f.source(contextSourceRequest{Text: tc.edited})
			want := contextRange(contextRangeRequest{T: t, Text: tc.edited, Needle: tc.selection})
			f.syncOne(contextSyncOneRequest{Outcome: tc.outcome, Reason: tc.reason, Evidence: tc.evidence, Location: want})
			got := f.read(contextReadRequest{})
			if got.Thread.Location != want || got.Thread.Tracking.SelectedText != tc.selection || got.Thread.ID != threadID || len(got.Thread.Messages) != 1 || got.Thread.Messages[0].ID != messageID {
				t.Fatalf("attachment/conversation: %+v", got.Thread)
			}
		})
	}
}

// CTX-09: a repeated selection is matched by its own unique context. Swapping
// the passages makes coordinate proximity and occurrence order both point at
// the wrong copy.
func TestSubjectContextUniqueContextDisambiguatesDuplicates(t *testing.T) {
	requireMatrixProse(matrixProseRequest{T: t})
	const needle = "chosen phrase"
	alpha := matrixLead + "alpha " + needle + " omega\n"
	beta := matrixTail + "beta " + needle + " gamma\n"
	source, edited := alpha+beta, beta+alpha
	for occurrence := 0; occurrence < 2; occurrence++ {
		t.Run(fmt.Sprint(occurrence), func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: needle, Occurrence: occurrence})})
			f.source(contextSourceRequest{Text: edited})
			want := contextRange(contextRangeRequest{T: t, Text: edited, Needle: needle, Occurrence: 1 - occurrence})
			f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeMoved, Reason: domain.ReasonExactMatch, Evidence: domain.EvidenceExact, Location: want})
		})
	}
}

// CTX-06 offline: supported two-sided expansion, competing envelopes, and the
// exact fallback when context is too short to expand.
func TestSubjectContextBoundaryExpansion(t *testing.T) {
	requireMatrixProse(matrixProseRequest{T: t})
	const selected = "chosen"
	prefix := strings.TrimSuffix(matrixLead, "\n") + " "
	suffix := " " + matrixTail
	source := prefix + selected + suffix
	expanded := prefix + "NEW " + selected + suffix
	unresolved := domain.Location{Kind: domain.LocationUnresolved}
	newFixture := func(t *testing.T) (contextFixture, contextThreadResult) {
		f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: selected})})
		return f, f.read(contextReadRequest{})
	}
	t.Run("supported_expansion", func(t *testing.T) {
		f, _ := newFixture(t)
		f.source(contextSourceRequest{Text: expanded})
		f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeModified, Reason: domain.ReasonBoundaryExpansion, Evidence: domain.EvidenceInferred, Location: contextRange(contextRangeRequest{T: t, Text: expanded, Needle: "NEW " + selected})})
	})
	t.Run("two_supported_envelopes", func(t *testing.T) {
		f, before := newFixture(t)
		if len(prefix) < domain.MaxContextBytes || len(suffix) < domain.MaxContextBytes {
			t.Fatal("fixture: context must fill the retained window")
		}
		window := prefix[len(prefix)-domain.MaxContextBytes:]
		// Drift check of the fixture premise, not an expected value.
		if before.Thread.Tracking.Prefix != window || before.Thread.Tracking.Suffix != suffix[:domain.MaxContextBytes] {
			t.Fatal("fixture: retained context windows differ from the computed windows")
		}
		// One exact copy, two envelopes supported by the repeated prefix window.
		f.source(contextSourceRequest{Text: window + "zzz " + window + "NEW " + selected + suffix})
		if !f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeUnresolved, Reason: domain.ReasonAmbiguous, Evidence: domain.EvidenceNone, Location: unresolved}).Changed {
			t.Fatal("unresolved state was not persisted")
		}
		if after := f.read(contextReadRequest{}); *after.Thread.Tracking != *before.Thread.Tracking {
			t.Fatal("ambiguous expansion changed retained evidence")
		}
	})
	t.Run("envelope_and_exact_copy", func(t *testing.T) {
		// Regression only: two context-supported exact copies are ambiguous
		// without envelope logic, so this does not isolate envelope ambiguity.
		f, before := newFixture(t)
		f.source(contextSourceRequest{Text: expanded + source})
		f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeUnresolved, Reason: domain.ReasonAmbiguous, Evidence: domain.EvidenceNone, Location: unresolved})
		if after := f.read(contextReadRequest{}); *after.Thread.Tracking != *before.Thread.Tracking {
			t.Fatal("ambiguous expansion changed retained evidence")
		}
	})
	t.Run("short_context_exact_fallback", func(t *testing.T) {
		// Too little context to expand, but the exact selection is still uniquely supported.
		f := newContextFixture(contextFixtureRequest{T: t, Source: "left chosen right\n", Location: "range:1:6-1:12"})
		f.source(contextSourceRequest{Text: "left NEW chosen right\n"})
		f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeMoved, Reason: domain.ReasonExactMatch, Evidence: domain.EvidenceExact, Location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 10, EndLine: 1, EndColumn: 16}})
	})
}

// CTX-04 offline unsupported and CTX-06 insufficient: short context cannot
// support an interior or boundary change, so the thread stays unresolved.
func TestSubjectContextInsufficientEvidence(t *testing.T) {
	for _, tc := range []struct{ name, source, location, edited string }{
		{name: "boundary_insertion", source: "left chosen right\n", location: "range:1:6-1:12", edited: "left NEW chosen RIGHT\n"},
		{name: "interior_edit", source: "prefix chosen suffix\n", location: "range:1:8-1:14", edited: "prefix chXosen suffix\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: tc.source, Location: tc.location})
			before := f.read(contextReadRequest{})
			f.source(contextSourceRequest{Text: tc.edited})
			f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeUnresolved, Reason: domain.ReasonInsufficientContext, Evidence: domain.EvidenceNone, Location: domain.Location{Kind: domain.LocationUnresolved}})
			if after := f.read(contextReadRequest{}); *after.Thread.Tracking != *before.Thread.Tracking {
				t.Fatal("insufficient evidence changed retained tracking")
			}
		})
	}
}

// CTX-04/06 editor side: the CLI captures client-supplied coordinates exactly
// and classifies them. Boundary exclusion itself is the client's job and is
// not proven by supplied coordinates.
func TestSubjectContextEditorSuppliedCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name, edited, selected string
		location               domain.Location
		outcome                string
	}{
		{name: "interior", edited: "prefix chXosen suffix\n", selected: "chXosen", location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 15}, outcome: domain.OutcomeModified},
		{name: "start_insertion", edited: "prefix NEW chosen suffix\n", selected: "chosen", location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 12, EndLine: 1, EndColumn: 18}, outcome: domain.OutcomeMoved},
		{name: "end_insertion", edited: "prefix chosen NEW suffix\n", selected: "chosen", location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 8, EndLine: 1, EndColumn: 14}, outcome: domain.OutcomeUnchanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			read := f.read(contextReadRequest{})
			f.source(contextSourceRequest{Text: tc.edited})
			location := tc.location
			request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: read.StorageRevision, SourceRevision: f.sourceRevision(contextReadRequest{}), Updates: []domain.EditorUpdate{{ThreadID: threadID, Location: &location}}}
			response := f.call(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: contextEditorInput(contextEditorInputRequest{T: t, Request: request})})
			outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: response}).Outcomes
			want := domain.TrackingOutcome{ThreadID: threadID, Outcome: tc.outcome, Reason: domain.ReasonEditorLocation, Evidence: domain.EvidenceEditor, Location: tc.location}
			if !response.Changed || len(outcomes) != 1 || outcomes[0] != want {
				t.Fatalf("editor outcome: changed=%t %+v; want %+v", response.Changed, outcomes, want)
			}
			after := f.read(contextReadRequest{})
			if after.Thread.Location != tc.location || after.Thread.Tracking.SelectedText != tc.selected || after.Thread.Tracking.SourceRevision == read.Thread.Tracking.SourceRevision {
				t.Fatalf("editor capture: %+v", after.Thread)
			}
			if f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}}).Changed {
				t.Fatal("editor capture is not stable under sync")
			}
		})
	}
}

// CTX-11: an explicit move to equal coordinates recaptures changed context
// (unresolved reattachment is covered in context_lifecycle_test.go).
// CTX-14: a stale ordinary move source guard conflicts before locking,
// without mutation.
func TestSubjectContextEqualCoordinateMoveAndGuard(t *testing.T) {
	f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
	before := f.read(contextReadRequest{})
	f.source(contextSourceRequest{Text: "prefix chosen SUFFIX\n"})
	snapshot := f.snapshot(contextReadRequest{})
	stale := f.observe(contextCallRequest{Args: []string{"thread", "move", "--if-source-revision", before.Thread.Tracking.SourceRevision, "--file", contextTarget, "--id", threadID, "--location", "range:1:8-1:14"}, ExpectedStatus: domain.ExitConflict, Error: true})
	f.requireError(contextErrorRequest{Result: stale, Message: "conflict:source_revision"})
	if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: snapshot}); err != nil {
		t.Fatal(err)
	}
	moved := f.call(contextCallRequest{Args: []string{"thread", "move", "--if-source-revision", f.sourceRevision(contextReadRequest{}), "--file", contextTarget, "--id", threadID, "--location", "range:1:8-1:14"}})
	if !moved.Changed {
		t.Fatal("equal-coordinate move did not refresh tracking")
	}
	after := f.read(contextReadRequest{})
	if after.Thread.Location != before.Thread.Location || after.Thread.Tracking.Suffix != " SUFFIX\n" || after.Thread.Tracking.SourceRevision == before.Thread.Tracking.SourceRevision {
		t.Fatalf("tracking not refreshed: %+v", after.Thread.Tracking)
	}
	if f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeUnchanged, Reason: domain.ReasonSourceUnchanged, Evidence: domain.EvidenceUnchanged, Location: before.Thread.Location}).Changed {
		t.Fatal("refreshed tracking not stable")
	}
}

type mixedCheckRequest struct {
	T           *testing.T
	Response    contextEnvelope
	WouldChange bool
	Want        []domain.TrackingOutcome
}

func checkMixedOutcomes(r mixedCheckRequest) {
	r.T.Helper()
	got := contextOutcomes(contextOutcomesRequest{T: r.T, Response: r.Response})
	if got.WouldChange != r.WouldChange || len(got.Outcomes) != len(r.Want) {
		r.T.Fatalf("would_change=%t outcomes %+v; want would_change=%t %+v", got.WouldChange, got.Outcomes, r.WouldChange, r.Want)
	}
	for i, w := range r.Want {
		if got.Outcomes[i] != w {
			r.T.Fatalf("outcome %d = %+v; want %+v", i, got.Outcomes[i], w)
		}
	}
}

// CTX-15: one sync over attached, previously unresolved, file and lost threads.
// Threads are created out of ID order so outcome order must come from IDs.
func TestSubjectContextMixedThreadSync(t *testing.T) {
	requireMatrixProse(matrixProseRequest{T: t})
	const (
		attachedID, attachedMessage = "018f0000-0000-7000-8000-0000000000a1", "018f0000-0000-7000-8000-0000000000b1"
		doomedID, doomedMessage     = "018f0000-0000-7000-8000-0000000000a2", "018f0000-0000-7000-8000-0000000000b2"
		lostID, lostMessage         = "018f0000-0000-7000-8000-0000000000a4", "018f0000-0000-7000-8000-0000000000b4"
		attached, doomed, lost      = "anchor sentence for alpha", "doomed sentence for beta", "lost sentence for delta"
	)
	doomedLine := doomed + ".\n"
	source := matrixLead + attached + ".\n" + matrixTail + doomedLine + matrixLead + lost + ".\n"
	// The fixture's own thread (lowest ID) is the file-scoped participant.
	f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: "file"})
	f.create(contextCreateRequest{ID: lostID, MessageID: lostMessage, Body: "Lost discussion.", Location: contextRange(contextRangeRequest{T: t, Text: source, Needle: lost})})
	f.create(contextCreateRequest{ID: doomedID, MessageID: doomedMessage, Body: "Doomed discussion.", Location: contextRange(contextRangeRequest{T: t, Text: source, Needle: doomed})})
	f.create(contextCreateRequest{ID: attachedID, MessageID: attachedMessage, Body: "Attached discussion.", Location: contextRange(contextRangeRequest{T: t, Text: source, Needle: attached})})
	f.call(contextCallRequest{Args: []string{"thread", "move", "--file", contextTarget, "--id", lostID, "--location", "lost"}})
	doomedBefore := f.readID(contextReadIDRequest{ID: doomedID})

	unresolved := domain.Location{Kind: domain.LocationUnresolved}
	for round, heading := range []string{"First heading\n", "Second heading\n"} {
		source = heading + strings.Replace(source, doomedLine, "", 1)
		f.source(contextSourceRequest{Text: source})
		want := []domain.TrackingOutcome{
			{ThreadID: threadID, Outcome: domain.OutcomeSkipped, Reason: domain.ReasonFile, Evidence: domain.EvidenceNone, Location: domain.Location{Kind: domain.LocationFile}},
			{ThreadID: attachedID, Outcome: domain.OutcomeMoved, Reason: domain.ReasonExactMatch, Evidence: domain.EvidenceExact, Location: contextRange(contextRangeRequest{T: t, Text: source, Needle: attached})},
			{ThreadID: doomedID, Outcome: domain.OutcomeUnresolved, Reason: domain.ReasonNoMatch, Evidence: domain.EvidenceNone, Location: unresolved},
			{ThreadID: lostID, Outcome: domain.OutcomeSkipped, Reason: domain.ReasonLost, Evidence: domain.EvidenceNone, Location: domain.Location{Kind: domain.LocationLost}},
		}
		snapshot := f.snapshot(contextReadRequest{})
		checkMixedOutcomes(mixedCheckRequest{T: t, Response: f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget, "--check"}, ExpectedStatus: domain.ExitValidation}), WouldChange: true, Want: want})
		if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: snapshot}); err != nil {
			t.Fatalf("round %d check wrote: %v", round, err)
		}
		synced := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
		if !synced.Changed {
			t.Fatalf("round %d did not persist the moved attachment", round)
		}
		checkMixedOutcomes(mixedCheckRequest{T: t, Response: synced, WouldChange: synced.Changed, Want: want})
	}
	if f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}}).Changed {
		t.Fatal("repeated mixed sync was not a no-op")
	}
	if got := f.readID(contextReadIDRequest{ID: doomedID}); got.Thread.Location != unresolved || *got.Thread.Tracking != *doomedBefore.Thread.Tracking || got.Thread.Messages[0].ID != doomedMessage {
		t.Fatalf("unresolved thread lost evidence: %+v", got.Thread)
	}
	// The lost thread's text is still uniquely present; it must not reattach.
	if got := f.readID(contextReadIDRequest{ID: lostID}); got.Thread.Location.Kind != domain.LocationLost || got.Thread.Tracking != nil {
		t.Fatalf("lost thread automatically reattached: %+v", got.Thread)
	}
}

// CTX-16: explicit file/lost moves purge excerpts and never recover, even when
// the original selection remains uniquely present.
func TestSubjectContextFileAndLostNeverRecover(t *testing.T) {
	const source = "quokka-left distinctive-selection wombat-right\n"
	private := []string{"quokka-left", "distinctive-selection", "wombat-right"}
	for _, tc := range []struct{ location, reason string }{
		{location: domain.LocationFile, reason: domain.ReasonFile},
		{location: domain.LocationLost, reason: domain.ReasonLost},
	} {
		t.Run(tc.location, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: "distinctive-selection"})})
			for _, text := range private {
				if !bytes.Contains(f.sidecar(contextReadRequest{}), []byte(text)) {
					t.Fatalf("fixture: %q not retained before the move", text)
				}
			}
			f.call(contextCallRequest{Args: []string{"thread", "move", "--file", contextTarget, "--id", threadID, "--location", tc.location}})
			stored := f.sidecar(contextReadRequest{})
			for _, text := range append(private, `"selected_text"`) {
				if bytes.Contains(stored, []byte(text)) {
					t.Fatalf("tracking excerpt %q remains: %q", text, stored)
				}
			}
			if f.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeSkipped, Reason: tc.reason, Evidence: domain.EvidenceNone, Location: domain.Location{Kind: tc.location}}).Changed {
				t.Fatal("skipped thread changed")
			}
			if got := f.read(contextReadRequest{}); got.Thread.Location.Kind != tc.location || got.Thread.Tracking != nil || got.Thread.Messages[0].ID != messageID {
				t.Fatalf("thread recovered or lost conversation: %+v", got.Thread)
			}
		})
	}
}

// CTX-17: deleting one thread removes only its conversation and tracking.
// CTX-18: deleting the final thread leaves no sidecar, cache or snapshot.
func TestSubjectContextDeleteThreads(t *testing.T) {
	requireMatrixProse(matrixProseRequest{T: t})
	const otherID, otherMessage = "018f0000-0000-7000-8000-0000000000a1", "018f0000-0000-7000-8000-0000000000b1"
	source := "first zebra passage\n" + matrixLead + matrixTail + "second yak passage\n"
	f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: "zebra"})})
	f.create(contextCreateRequest{ID: otherID, MessageID: otherMessage, Body: "Second discussion.", Location: contextRange(contextRangeRequest{T: t, Text: source, Needle: "yak"})})
	other := f.readID(contextReadIDRequest{ID: otherID})
	if !strings.Contains(f.read(contextReadRequest{}).Thread.Tracking.Suffix, "harbour survey") {
		t.Fatal("fixture: deleted thread must retain attributable context")
	}
	f.call(contextCallRequest{Args: []string{"thread", "delete", "--file", contextTarget, "--id", threadID}})
	stored := f.sidecar(contextReadRequest{})
	for _, private := range []string{"zebra", "harbour survey", "Keep this conversation.", threadID} {
		if bytes.Contains(stored, []byte(private)) {
			t.Fatalf("deleted thread data %q remains: %q", private, stored)
		}
	}
	remaining := f.readID(contextReadIDRequest{ID: otherID})
	if remaining.Thread.Location != other.Thread.Location || *remaining.Thread.Tracking != *other.Thread.Tracking || len(remaining.Thread.Messages) != 1 || remaining.Thread.Messages[0] != other.Thread.Messages[0] {
		t.Fatalf("surviving thread changed: %+v", remaining.Thread)
	}
	f.call(contextCallRequest{Args: []string{"thread", "delete", "--file", contextTarget, "--id", otherID}})
	sources := 0
	for _, file := range f.snapshot(contextReadRequest{}) {
		switch file.Path {
		case contextSource:
			sources++
			if string(file.Bytes) != source {
				t.Fatal("source changed")
			}
		case contextLockFile:
		default:
			t.Fatalf("unexpected file after final deletion: %s", file.Path)
		}
	}
	if sources != 1 {
		t.Fatalf("source missing after deletion")
	}
}

// CTX-19: a source and sidecar copied alone to a new directory still sync
// after the original is gone. HOME/XDG isolation is not covered: runRequest
// does not control the environment.
func TestSubjectContextFreshCopyPortability(t *testing.T) {
	const source = "Intro\nchosen phrase\nEnd.\n"
	original := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: "range:2:1-2:14"})
	copied := original
	copied.dir = filepath.Join(t.TempDir(), "elsewhere", "checkout")
	if err := os.MkdirAll(copied.dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{contextSource, contextTarget} {
		data, err := os.ReadFile(filepath.Join(original.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFile(fileRequest{Path: filepath.Join(copied.dir, name), Mode: 0640, Bytes: data}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(original.dir); err != nil {
		t.Fatal(err)
	}
	copied.source(contextSourceRequest{Text: "Heading\n" + source})
	copied.syncOne(contextSyncOneRequest{Outcome: domain.OutcomeMoved, Reason: domain.ReasonExactMatch, Evidence: domain.EvidenceExact, Location: domain.Location{Kind: domain.LocationRange, StartLine: 3, StartColumn: 1, EndLine: 3, EndColumn: 14}})
	for _, file := range copied.snapshot(contextReadRequest{}) {
		if file.Path != contextSource && file.Path != contextTarget && file.Path != contextLockFile {
			t.Fatalf("unexpected machine-local file: %s", file.Path)
		}
	}
}

type directoryStateRequest struct {
	T   *testing.T
	Dir string
}

// directoryState records names, regular-file bytes and link targets without
// observeFiles, which rejects symbolic links.
func directoryState(r directoryStateRequest) string {
	r.T.Helper()
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		r.T.Fatal(err)
	}
	var b strings.Builder
	for _, entry := range entries {
		path := filepath.Join(r.Dir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				r.T.Fatal(err)
			}
			fmt.Fprintf(&b, "%s -> %s\n", entry.Name(), link)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			r.T.Fatal(err)
		}
		fmt.Fprintf(&b, "%s %q\n", entry.Name(), data)
	}
	return b.String()
}

// CTX-20 (partial): missing and symlinked sources and corrupt stored tracking
// fail with typed errors and write nothing; discussion works without source.
// Malformed, oversized and unreadable sources are not covered here.
func TestSubjectContextSourceAndTrackingFailures(t *testing.T) {
	for _, tc := range []struct {
		name, message, path string
		status              int
		damage              func(f contextFixture)
	}{
		{name: "missing", message: "io:open", path: "source", status: domain.ExitIO, damage: func(f contextFixture) {
			if err := os.Remove(filepath.Join(f.dir, contextSource)); err != nil {
				f.t.Fatal(err)
			}
		}},
		{name: "symlink", message: "validation:source_type:symlink", path: "source_type", status: domain.ExitValidation, damage: func(f contextFixture) {
			if err := os.Rename(filepath.Join(f.dir, contextSource), filepath.Join(f.dir, "referent")); err != nil {
				f.t.Fatal(err)
			}
			if err := os.Symlink("referent", filepath.Join(f.dir, contextSource)); err != nil {
				f.t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			tc.damage(f)
			before := directoryState(directoryStateRequest{T: t, Dir: f.dir})
			for _, args := range [][]string{{"sync", "--file", contextTarget}, {"sync", "--file", contextTarget, "--check"}} {
				result := f.observe(contextCallRequest{Args: args, ExpectedStatus: tc.status, Error: true})
				f.requireError(contextErrorRequest{Result: result, Message: tc.message})
				if !bytes.Contains(result.Stderr, []byte(`"path":"`+tc.path+`"`)) {
					t.Fatalf("%v: missing path %s detail: %q", args, tc.path, result.Stderr)
				}
				if after := directoryState(directoryStateRequest{T: t, Dir: f.dir}); after != before {
					t.Fatalf("%v changed the directory:\n%s\nwant:\n%s", args, after, before)
				}
			}
			f.call(contextCallRequest{Args: []string{"message", "add", "--file", contextTarget, "--thread", threadID, "--author-kind", "agent", "--body-file", "-"}, Stdin: []byte("Reply without source.")})
			if got := f.read(contextReadRequest{}); len(got.Thread.Messages) != 2 || got.Thread.Tracking == nil {
				t.Fatalf("discussion failed without source: %+v", got.Thread)
			}
		})
	}
	t.Run("corrupt_tracking", func(t *testing.T) {
		f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
		stored := f.sidecar(contextReadRequest{})
		corrupt := bytes.Replace(stored, []byte(`"tracking":{"version":1,`), []byte(`"tracking":{"version":9,`), 1)
		if bytes.Equal(corrupt, stored) {
			t.Fatal("fixture: tracking version not found")
		}
		if err := os.WriteFile(filepath.Join(f.dir, contextTarget), corrupt, 0640); err != nil {
			t.Fatal(err)
		}
		before := f.snapshot(contextReadRequest{})
		for _, args := range [][]string{{"thread", "list", "--file", contextTarget}, {"sync", "--file", contextTarget}} {
			f.requireError(contextErrorRequest{Result: f.observe(contextCallRequest{Args: args, ExpectedStatus: domain.ExitValidation, Error: true}), Message: "validation:tracking:invalid"})
			if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		}
	})
}

type editorBatchRequest struct{ Revision, Source string }

// CTX-21 (editor batch): shape and count bounds reject the whole request; a
// batch of exactly MaxApplyOperations updates passes the count check.
func TestSubjectContextEditorBatchBounds(t *testing.T) {
	update := fmt.Sprintf(`{"thread_id":%q,"location":{"kind":"line","line":1}}`, threadID)
	updates := func(n int) string { return strings.TrimSuffix(strings.Repeat(update+",", n), ",") }
	for _, tc := range []struct {
		name, message string
		status        int
		request       func(r editorBatchRequest) string
	}{
		{name: "no_updates", message: "usage:operations:malformed", status: domain.ExitUsage, request: func(r editorBatchRequest) string {
			return fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[]}`, domain.ProtocolVersion, r.Revision, r.Source)
		}},
		{name: "too_many_updates", message: "usage:operations:malformed", status: domain.ExitUsage, request: func(r editorBatchRequest) string {
			return fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[%s]}`, domain.ProtocolVersion, r.Revision, r.Source, updates(domain.MaxApplyOperations+1))
		}},
		{name: "at_limit_reaches_semantics", message: "precondition:editor-update:duplicate_thread", status: domain.ExitPrecondition, request: func(r editorBatchRequest) string {
			return fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"source_revision":%q,"updates":[%s]}`, domain.ProtocolVersion, r.Revision, r.Source, updates(domain.MaxApplyOperations))
		}},
		{name: "missing_source_guard", message: "usage:operations:malformed", status: domain.ExitUsage, request: func(r editorBatchRequest) string {
			return fmt.Sprintf(`{"protocol":%d,"if_revision":%q,"updates":[%s]}`, domain.ProtocolVersion, r.Revision, update)
		}},
		{name: "missing_sidecar_guard", message: "usage:operations:malformed", status: domain.ExitUsage, request: func(r editorBatchRequest) string {
			return fmt.Sprintf(`{"protocol":%d,"source_revision":%q,"updates":[%s]}`, domain.ProtocolVersion, r.Source, update)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			request := tc.request(editorBatchRequest{Revision: f.read(contextReadRequest{}).StorageRevision, Source: f.sourceRevision(contextReadRequest{})})
			before := f.snapshot(contextReadRequest{})
			result := f.observe(contextCallRequest{Args: []string{"editor-update", "--file", contextTarget, "--operations-file", "-"}, Stdin: []byte(request), ExpectedStatus: tc.status, Error: true})
			f.requireError(contextErrorRequest{Result: result, Message: tc.message})
			if tc.status == domain.ExitPrecondition && !bytes.Contains(result.Stderr, []byte(`"index":1`)) {
				t.Fatalf("duplicate not reported at index 1: %q", result.Stderr)
			}
			if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// CTX-23 (partial): v1 sidecars fail closed for reads, mutations, repair and
// sync without rewriting. Running an old binary against v2 is not covered.
func TestSubjectContextRejectsV1Sidecar(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	v1 := "[@th_" + threadID + " file]\n" + role + "x\n" + endThread
	for _, tc := range []struct{ name, stored string }{
		{name: "headerless", stored: v1},
		{name: "v1_header", stored: "<!-- sidecar-format: 1 -->\n" + v1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range []fileRequest{{Path: contextSource, Mode: 0640, Bytes: []byte("x\n")}, {Path: contextTarget, Mode: 0640, Bytes: []byte(tc.stored)}} {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, file.Path), Mode: file.Mode, Bytes: file.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			for _, call := range []contextCallRequest{
				{Args: []string{"thread", "list", "--file", contextTarget}},
				{Args: []string{"thread", "get", "--file", contextTarget, "--id", threadID}},
				{Args: []string{"message", "add", "--file", contextTarget, "--thread", threadID, "--author-kind", "agent", "--body-file", "-"}, Stdin: []byte("reply")},
				{Args: []string{"format", "--file", contextTarget}},
				{Args: []string{"fix", "--file", contextTarget, "--add-missing-message-ids"}},
				{Args: []string{"sync", "--file", contextTarget}},
				{Args: []string{"sync", "--file", contextTarget, "--check"}},
			} {
				call.T, call.Binary, call.Dir, call.ExpectedStatus, call.Error = t, binary, dir, domain.ExitUnsupported, true
				result := contextObserve(call)
				if !bytes.Contains(result.Stderr, []byte(`"message":"unsupported:format"`)) {
					t.Fatalf("%v: %q", call.Args, result.Stderr)
				}
				data, err := os.ReadFile(filepath.Join(dir, contextTarget))
				if err != nil || string(data) != tc.stored {
					t.Fatalf("%v rewrote v1 sidecar: %q error=%v", call.Args, data, err)
				}
			}
		})
	}
}

// CTX-24 (partial): human and quiet output never dump retained excerpts.
// Terminal-control escaping is covered in terminal_text_test.go.
func TestSubjectContextHumanOutputHidesExcerpts(t *testing.T) {
	const secret, context = "xylophone-secret", "quokka-context"
	source := context + " " + secret + " tail\n"
	f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: rangeText(contextRangeRequest{T: t, Text: source, Needle: secret})})
	f.source(contextSourceRequest{Text: "Heading\n" + source})
	// Positive control: the excerpts are retained and exposed only on request.
	full := f.call(contextCallRequest{Args: []string{"thread", "get", "--file", contextTarget, "--id", threadID, "--include-tracking"}})
	for _, private := range []string{secret, context} {
		if !bytes.Contains(full.Result, []byte(private)) {
			t.Fatalf("fixture: %q not retained", private)
		}
	}
	for _, tc := range []struct {
		args   []string
		status int
		quiet  bool
	}{
		{args: []string{"thread", "get", "--file", contextTarget, "--id", threadID, "--include-tracking"}},
		{args: []string{"thread", "list", "--file", contextTarget, "--include-tracking"}},
		{args: []string{"sync", "--file", contextTarget, "--check"}, status: domain.ExitValidation},
		{args: []string{"--output", "none", "sync", "--file", contextTarget, "--check"}, status: domain.ExitValidation, quiet: true},
		{args: []string{"sync", "--file", contextTarget}},
	} {
		got, err := runProcess(runRequest{Binary: f.binary, Dir: f.dir, Args: tc.args, Timeout: 5 * time.Second, OutputLimit: domain.MaxBodyBytes})
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != tc.status {
			t.Fatalf("%v: status=%d stdout=%q stderr=%q", tc.args, got.Status, got.Stdout, got.Stderr)
		}
		for _, private := range []string{secret, context} {
			if bytes.Contains(got.Stdout, []byte(private)) || bytes.Contains(got.Stderr, []byte(private)) {
				t.Fatalf("%v exposed %q: stdout=%q stderr=%q", tc.args, private, got.Stdout, got.Stderr)
			}
		}
		if tc.quiet && (len(got.Stdout) != 0 || len(got.Stderr) != 0) {
			t.Fatalf("quiet output wrote: stdout=%q stderr=%q", got.Stdout, got.Stderr)
		}
		if !tc.quiet && !bytes.Contains(got.Stdout, []byte(threadID)) {
			t.Fatalf("%v: human output does not describe the thread: %q", tc.args, got.Stdout)
		}
	}
}
