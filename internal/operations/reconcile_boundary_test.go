package operations

import (
	"fmt"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestReconcileCandidateExactFunding(t *testing.T) {
	for _, tc := range []struct {
		name, original, current, kind string
		work                          int
	}{
		{name: "range", original: "X", current: "Xa", kind: domain.LocationRange, work: 4},
		{name: "utf8", original: "é", current: "éX", kind: domain.LocationRange, work: 6},
		{name: "whole_line", original: "X", current: "X\n", kind: domain.LocationLine, work: 3},
	} {
		for _, global := range []bool{false, true} {
			for _, enough := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/global=%t/enough=%t", tc.name, global, enough), func(t *testing.T) {
					old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(tc.original)})
					if err != nil {
						t.Fatal(err)
					}
					loc := domain.Location{Kind: domain.LocationLine, Line: 1}
					if tc.kind == domain.LocationRange {
						loc, err = old.Location(domain.SourceOffsetsRequest{Start: 0, End: len(tc.original)})
						if err != nil {
							t.Fatal(err)
						}
					}
					record, err := old.Capture(domain.SourceLocationRequest{Location: loc})
					if err != nil {
						t.Fatal(err)
					}
					current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(tc.current)})
					if err != nil {
						t.Fatal(err)
					}
					remaining := tc.work
					if !enough {
						remaining--
					}
					meter := syncMeter{}
					if global {
						meter.global = domain.MaxSyncWorkBytes - remaining
					} else {
						meter.thread = domain.MaxSourceBytes - remaining
					}
					initial := meter
					_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
					if enough {
						if reason != domain.ReasonExactMatch || evidence != domain.EvidenceExact || meter.exhausted || meter.thread-initial.thread != tc.work || meter.global-initial.global != tc.work {
							t.Fatalf("exact funding rejected/miscounted: %s/%s %+v initial=%+v", reason, evidence, meter, initial)
						}
					} else if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted {
						t.Fatalf("unfunded validation accepted: %s/%s %+v", reason, evidence, meter)
					}
				})
			}
		}
	}
}

func TestReconcileUnchangedExactFunding(t *testing.T) {
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("éX界")})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		location domain.Location
		work     int
	}{
		{name: "range", location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 3, EndLine: 1, EndColumn: 4}, work: 9},
		{name: "whole_line", location: domain.Location{Kind: domain.LocationLine, Line: 1}, work: len("éX界")},
	} {
		for _, global := range []bool{false, true} {
			for _, enough := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/global=%t/enough=%t", tc.name, global, enough), func(t *testing.T) {
					record, err := source.Capture(domain.SourceLocationRequest{Location: tc.location})
					if err != nil {
						t.Fatal(err)
					}
					remaining := tc.work
					if !enough {
						remaining--
					}
					meter := syncMeter{}
					if global {
						meter.global = domain.MaxSyncWorkBytes - remaining
					} else {
						meter.thread = domain.MaxSourceBytes - remaining
					}
					initial := meter
					accepted := meter.validAttachment(syncAttachmentRequest{Source: &source, Location: tc.location, Tracking: &record})
					if accepted != enough || meter.exhausted == enough {
						t.Fatalf("accepted=%t meter=%+v enough=%t", accepted, meter, enough)
					}
					if enough && (meter.thread-initial.thread != tc.work || meter.global-initial.global != tc.work) {
						t.Fatal("incorrect exact unchanged work")
					}
				})
			}
		}
	}
}

func TestReconcileLaterZeroByteEvidenceCannotBypassGlobalGuard(t *testing.T) {
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{})
	if err != nil {
		t.Fatal(err)
	}
	loc := domain.Location{Kind: domain.LocationLine, Line: 1}
	record, err := source.Capture(domain.SourceLocationRequest{Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	before := record
	meter := syncMeter{global: domain.MaxSyncWorkBytes}
	meter.beginThread(domain.ValueRequest{})
	if meter.validAttachment(syncAttachmentRequest{Source: &source, Location: loc, Tracking: &record}) {
		t.Fatal("zero-byte unchanged evidence bypassed global exhaustion")
	}
	_, reason, evidence := matchTracking(syncMatchRequest{Source: &source, Tracking: &record, Meter: &meter})
	if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted || record != before || meter.thread != 0 {
		t.Fatalf("later zero-byte thread: %s/%s %+v", reason, evidence, meter)
	}
}

func TestReconcileLateEmptyLineRequiresFullScanAndContext(t *testing.T) {
	oldText := "before\n\nafter\n"
	old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(oldText)})
	if err != nil {
		t.Fatal(err)
	}
	record, err := old.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationLine, Line: 2}})
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("occupied\n", domain.MaxApplyOperations+1) + oldText
	current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	work := len(text) + len(record.Prefix) + len(record.Suffix)
	for _, enough := range []bool{false, true} {
		t.Run(fmt.Sprintf("enough=%t", enough), func(t *testing.T) {
			remaining := work
			if !enough {
				remaining--
			}
			meter := syncMeter{global: domain.MaxSyncWorkBytes - remaining}
			_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
			if enough {
				if reason != domain.ReasonExactMatch || evidence != domain.EvidenceExact || meter.thread != work || meter.exhausted {
					t.Fatalf("late unique empty line rejected/miscounted: %s/%s %+v", reason, evidence, meter)
				}
			} else if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted {
				t.Fatal("incomplete empty-line scan/context accepted")
			}
		})
	}
	crowded, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(strings.Repeat("\n", domain.MaxApplyOperations+1))})
	if err != nil {
		t.Fatal(err)
	}
	meter := syncMeter{}
	_, reason, evidence := matchTracking(syncMatchRequest{Source: &crowded, Tracking: &record, Meter: &meter})
	if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted || meter.thread != domain.MaxApplyOperations+1 {
		t.Fatalf("empty-line cap not metered: %s/%s %+v", reason, evidence, meter)
	}
}
