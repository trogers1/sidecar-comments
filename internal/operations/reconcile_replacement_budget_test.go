package operations

import (
	"fmt"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestReconcileReplacementBoundaryExactFunding(t *testing.T) {
	n := domain.MinMatchContextBytes
	prefix, suffix := strings.Repeat("p", n), strings.Repeat("s", n)
	old, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + "Y" + suffix)})
	if err != nil {
		t.Fatal(err)
	}
	loc, err := old.Location(domain.SourceOffsetsRequest{Start: n, End: n + 1})
	if err != nil {
		t.Fatal(err)
	}
	record, err := old.Capture(domain.SourceLocationRequest{Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + "X" + suffix)})
	if err != nil {
		t.Fatal(err)
	}
	// Missing Y: 2n+1 comparisons. Strong windows: 2 inspections.
	// Repeated-p search: n-1 preprocessing and 3n search comparisons.
	// Repeated-s search: n-1 preprocessing and 2n+1 search comparisons.
	// Finally, both range endpoints need one RuneStart inspection each.
	work := (2*n + 1) + 2 + (n - 1 + 3*n) + (n - 1 + 2*n + 1) + 2
	for _, global := range []bool{false, true} {
		for _, enough := range []bool{false, true} {
			t.Run(fmt.Sprintf("global=%t/enough=%t", global, enough), func(t *testing.T) {
				remaining := work
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
					if reason != domain.ReasonContextReplacement || evidence != domain.EvidenceInferred || meter.exhausted || meter.thread-initial.thread != work || meter.global-initial.global != work {
						t.Fatalf("exact inferred funding rejected/miscounted: %s/%s %+v", reason, evidence, meter)
					}
				} else if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted {
					t.Fatalf("unfunded inferred mapping accepted: %s/%s %+v", reason, evidence, meter)
				}
			})
		}
	}
}
