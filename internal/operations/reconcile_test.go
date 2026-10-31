package operations

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestReconcileSupportedMappings(t *testing.T) {
	prefix := strings.Repeat("p", domain.MaxContextBytes)
	suffix := strings.Repeat("s", domain.MaxContextBytes)
	selection := "chosen text"
	for _, tc := range []struct{ name, replacement, outcome, reason string }{
		{"unchanged", selection, domain.OutcomeUnchanged, domain.ReasonSourceUnchanged},
		{"replacement", "replacement", domain.OutcomeModified, domain.ReasonContextReplacement},
		{"start expansion", "NEW" + selection, domain.OutcomeModified, domain.ReasonBoundaryExpansion},
		{"end expansion", selection + "NEW", domain.OutcomeModified, domain.ReasonBoundaryExpansion},
		{"deletion", "", domain.OutcomeUnresolved, domain.ReasonNoMatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + selection + suffix)})
			if err != nil {
				t.Fatal(err)
			}
			loc, err := old.Location(domain.SourceOffsetsRequest{Start: len(prefix), End: len(prefix) + len(selection)})
			if err != nil {
				t.Fatal(err)
			}
			record, err := old.Capture(domain.SourceLocationRequest{Location: loc})
			if err != nil {
				t.Fatal(err)
			}
			doc, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: "01900000-0000-7000-8000-000000000001", Location: loc, Tracking: &record, Messages: []domain.Message{{ID: "01900000-0000-7000-8000-000000000002", Author: domain.Author{Kind: "human"}, Body: "conversation"}}}}})
			if err != nil {
				t.Fatal(err)
			}
			current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + tc.replacement + suffix)})
			if err != nil {
				t.Fatal(err)
			}
			result, err := Reconcile(ReconcileRequest{Document: doc, Source: &current})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Outcomes) != 1 || result.Outcomes[0].Outcome != tc.outcome || result.Outcomes[0].Reason != tc.reason {
				t.Fatalf("unexpected outcomes: %+v", result.Outcomes)
			}
			before := doc.Threads(domain.DocumentThreadsRequest{})[0]
			after := result.Document.Threads(domain.DocumentThreadsRequest{})[0]
			if before.ID != after.ID || !reflect.DeepEqual(before.Messages, after.Messages) {
				t.Fatal("conversation changed")
			}
			if tc.outcome == domain.OutcomeUnresolved {
				if !reflect.DeepEqual(before.Tracking, after.Tracking) {
					t.Fatal("unresolved evidence changed")
				}
			} else if after.Tracking.SelectedText != tc.replacement {
				t.Fatal("wrong inferred selection")
			}
			repeated, err := Reconcile(ReconcileRequest{Document: result.Document, Source: &current})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Document, repeated.Document) {
				t.Fatal("repeat not stable")
			}
		})
	}
}

// These lower-level checks exercise precise comparison/candidate boundaries
// that public CLI lifecycle fixtures cannot economically observe.
func TestReconcileSearchAccounting(t *testing.T) {
	pattern := "x"
	text := strings.Repeat(pattern, domain.MaxApplyOperations)
	m := syncMeter{}
	if got := m.occurrences(syncSearchRequest{Text: text, Pattern: pattern}); len(got) != domain.MaxApplyOperations || m.thread != len(text) || m.global != len(text) || m.exhausted {
		t.Fatalf("exact bound: count/work=%d/%d", len(got), m.thread)
	}
	m = syncMeter{}
	m.occurrences(syncSearchRequest{Text: text + pattern, Pattern: pattern})
	if !m.exhausted {
		t.Fatal("candidate overflow accepted")
	}
	m = syncMeter{thread: domain.MaxSourceBytes - len(text), global: domain.MaxSyncWorkBytes - len(text)}
	if got := m.occurrences(syncSearchRequest{Text: text, Pattern: pattern}); len(got) != domain.MaxApplyOperations || m.exhausted {
		t.Fatal("exact work budget rejected")
	}
	if m.spend(syncWorkRequest{Bytes: 1}) || !m.exhausted {
		t.Fatal("work overflow accepted")
	}
	for _, full := range []syncMeter{{thread: domain.MaxSourceBytes}, {global: domain.MaxSyncWorkBytes}} {
		if full.spend(syncWorkRequest{Bytes: 1}) || !full.exhausted {
			t.Fatal("independent budget bypassed")
		}
	}
}

func TestReconcileRepetitiveSearchLinear(t *testing.T) {
	for n := 1; n <= domain.MaxAnchorBytes; n *= 2 {
		text := strings.Repeat("a", n) + "b"
		pattern := strings.Repeat("a", n/2) + "c"
		m := syncMeter{}
		got := m.occurrences(syncSearchRequest{Text: text, Pattern: pattern})
		if len(got) != 0 || m.exhausted || m.thread > 2*(len(text)+len(pattern)) {
			t.Fatalf("nonlinear work for %d: %d", n, m.thread)
		}
	}
}

func TestReconcileContextAndLineConstraints(t *testing.T) {
	prefix := strings.Repeat("p", domain.MaxContextBytes)
	suffix := strings.Repeat("s", domain.MaxContextBytes)
	selection := "chosen"
	for _, tc := range []struct {
		name, old, current string
		location           domain.Location
		reason, evidence   string
	}{
		{"short replacement context", "left chosen right", "left NEW right", domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 6, EndLine: 1, EndColumn: 12}, domain.ReasonInsufficientContext, domain.EvidenceNone},
		{"unicode moved", "α🙂 chosen ω", "heading\nα🙂 chosen ω", domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len("α🙂 ") + 1, EndLine: 1, EndColumn: len("α🙂 chosen") + 1}, domain.ReasonExactMatch, domain.EvidenceExact},
		{"duplicate exact", prefix + selection + suffix, prefix + selection + suffix + "\n" + prefix + selection + suffix, domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len(prefix) + 1, EndLine: 1, EndColumn: len(prefix) + len(selection) + 1}, domain.ReasonAmbiguous, domain.EvidenceNone},
		{"duplicate replacement", prefix + selection + suffix, prefix + "changed" + suffix + "\n" + prefix + "changed" + suffix, domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len(prefix) + 1, EndLine: 1, EndColumn: len(prefix) + len(selection) + 1}, domain.ReasonAmbiguous, domain.EvidenceNone},
		{"empty line moved", "before\n\nafter", "new\nbefore\n\nafter", domain.Location{Kind: domain.LocationLine, Line: 2}, domain.ReasonExactMatch, domain.EvidenceExact},
		{"empty line ambiguous", "before\n\nafter", "before\n\nafter\nbefore\n\nafter", domain.Location{Kind: domain.LocationLine, Line: 2}, domain.ReasonAmbiguous, domain.EvidenceNone},
		{"line split", prefix + "\n" + selection + "\n" + suffix, prefix + "\nchanged\nline\n" + suffix, domain.Location{Kind: domain.LocationLine, Line: 2}, domain.ReasonUnsupportedTransformation, domain.EvidenceNone},
		{"line replacement", prefix + "\n" + selection + "\n" + suffix, prefix + "\nchanged\n" + suffix, domain.Location{Kind: domain.LocationLine, Line: 2}, domain.ReasonContextReplacement, domain.EvidenceInferred},
		{"oversized", prefix + selection + suffix, prefix + strings.Repeat("N", domain.MaxAnchorBytes+1) + suffix, domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: len(prefix) + 1, EndLine: 1, EndColumn: len(prefix) + len(selection) + 1}, domain.ReasonSelectionTooLarge, domain.EvidenceNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(tc.old)})
			if err != nil {
				t.Fatal(err)
			}
			record, err := old.Capture(domain.SourceLocationRequest{Location: tc.location})
			if err != nil {
				t.Fatal(err)
			}
			current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(tc.current)})
			if err != nil {
				t.Fatal(err)
			}
			meter := syncMeter{}
			_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
			if reason != tc.reason || evidence != tc.evidence {
				t.Fatalf("got %s/%s, want %s/%s", reason, evidence, tc.reason, tc.evidence)
			}
		})
	}
}

func TestReconcileUnchangedAttachmentEvidenceIsMetered(t *testing.T) {
	prefix := strings.Repeat("p", domain.MaxContextBytes)
	suffix := strings.Repeat("s", domain.MaxContextBytes)
	selection := "chosen"
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + selection + suffix)})
	if err != nil {
		t.Fatal(err)
	}
	location, err := source.Location(domain.SourceOffsetsRequest{Start: len(prefix), End: len(prefix) + len(selection)})
	if err != nil {
		t.Fatal(err)
	}
	record, err := source.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	meter := syncMeter{}
	if !meter.validAttachment(syncAttachmentRequest{Source: &source, Location: location, Tracking: &record}) {
		t.Fatal("valid attachment rejected")
	}
	// Two range-coordinate probes, one start-window boundary probe, and
	// each selected/context byte. The end-window boundary is source EOF.
	want := len(prefix) + len(selection) + len(suffix) + 3
	if meter.thread != want || meter.global != want {
		t.Fatalf("attachment accounting: got %d/%d want %d", meter.thread, meter.global, want)
	}
	meter = syncMeter{global: domain.MaxSyncWorkBytes - want + 1}
	if meter.validAttachment(syncAttachmentRequest{Source: &source, Location: location, Tracking: &record}) || !meter.exhausted {
		t.Fatal("unmetered unchanged evidence accepted")
	}
	invalid := record
	invalid.SelectedText = strings.ToUpper(selection)
	meter = syncMeter{}
	if meter.validAttachment(syncAttachmentRequest{Source: &source, Location: location, Tracking: &invalid}) {
		t.Fatal("fingerprint accepted without valid active evidence")
	}
}

func TestReconcileAggregateOverflowIsAtomic(t *testing.T) {
	prefix := strings.Repeat("p", domain.MaxContextBytes)
	suffix := strings.Repeat("s", domain.MaxContextBytes)
	selection := "X"
	old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + selection + suffix)})
	if err != nil {
		t.Fatal(err)
	}
	location, err := old.Location(domain.SourceOffsetsRequest{Start: len(prefix), End: len(prefix) + len(selection)})
	if err != nil {
		t.Fatal(err)
	}
	record, err := old.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	count := domain.MaxTrackingBytes/(domain.MaxAnchorBytes+2*domain.MaxContextBytes) + 1
	threads := make([]domain.Thread, count)
	const idFormat = "01900000-0000-7000-8000-%012x"
	for i := range threads {
		threads[i] = domain.Thread{ID: fmt.Sprintf(idFormat, 2*i+1), Location: location, Tracking: &record, Messages: []domain.Message{{ID: fmt.Sprintf(idFormat, 2*i+2), Author: domain.Author{Kind: "human"}, Body: "conversation"}}}
	}
	doc, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	before := doc.Threads(domain.DocumentThreadsRequest{})
	current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + strings.Repeat("N", domain.MaxAnchorBytes) + suffix)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Reconcile(ReconcileRequest{Document: doc, Source: &current})
	failure, ok := err.(*domain.Error)
	if !ok || failure.Code != domain.CodePrecondition || failure.Message != "precondition:sync:limit_tracking" {
		t.Fatalf("expected atomic tracking overflow, got %v", err)
	}
	if !reflect.DeepEqual(before, doc.Threads(domain.DocumentThreadsRequest{})) {
		t.Fatal("input mutated on overflow")
	}
}

func TestReconcileGlobalBudgetNeverAcceptsUncheckedUniqueness(t *testing.T) {
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("left chosen right")})
	if err != nil {
		t.Fatal(err)
	}
	record, err := source.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 6, EndLine: 1, EndColumn: 12}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := syncMeter{}
	_, reason, evidence := matchTracking(syncMatchRequest{Source: &source, Tracking: &record, Meter: &baseline})
	if reason != domain.ReasonExactMatch || evidence != domain.EvidenceExact {
		t.Fatalf("baseline %s/%s", reason, evidence)
	}
	for _, remaining := range []int{0, baseline.global - 1, baseline.global} {
		meter := syncMeter{global: domain.MaxSyncWorkBytes - remaining}
		_, reason, evidence := matchTracking(syncMatchRequest{Source: &source, Tracking: &record, Meter: &meter})
		if remaining < baseline.global {
			if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone {
				t.Fatalf("accepted incomplete search with %d units", remaining)
			}
		} else if reason != domain.ReasonExactMatch {
			t.Fatalf("exact budget rejected: %s", reason)
		}
		if meter.global > domain.MaxSyncWorkBytes || meter.thread > domain.MaxSourceBytes {
			t.Fatal("budget overflow")
		}
		meter.thread = 0
		meter.exhausted = false
		_, reason, evidence = matchTracking(syncMatchRequest{Source: &source, Tracking: &record, Meter: &meter})
		if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone {
			t.Fatal("later thread accepted after global exhaustion")
		}
	}
}

func TestReconcileSmallSearchProperty(t *testing.T) {
	// Exhaust all short binary sources/patterns, including overlapping matches.
	type wordRequest struct{ Length, Bits int }
	word := func(r wordRequest) string {
		b := make([]byte, r.Length)
		for i := range b {
			b[i] = 'a' + byte((r.Bits>>i)&1)
		}
		return string(b)
	}
	for n := 0; n <= 8; n++ {
		for bits := 0; bits < (1 << n); bits++ {
			text := word(wordRequest{Length: n, Bits: bits})
			for size := 1; size <= 4; size++ {
				for p := 0; p < (1 << size); p++ {
					pattern := word(wordRequest{Length: size, Bits: p})
					want := []int{}
					for i := 0; i+len(pattern) <= len(text); i++ {
						if text[i:i+len(pattern)] == pattern {
							want = append(want, i)
						}
					}
					meter := syncMeter{}
					got := meter.occurrences(syncSearchRequest{Text: text, Pattern: pattern})
					if !reflect.DeepEqual(got, want) || meter.exhausted {
						t.Fatalf("search property failed for %q/%q: %v versus %v", text, pattern, got, want)
					}
				}
			}
		}
	}
}

func TestReconcileEmptyDocumentWithoutSource(t *testing.T) {
	result, err := Reconcile(ReconcileRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outcomes) != 0 {
		t.Fatal("unexpected outcomes")
	}
}
