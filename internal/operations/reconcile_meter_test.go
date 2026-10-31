package operations

import (
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestReconcileBoundaryInspectionBudget(t *testing.T) {
	original, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("X")})
	if err != nil {
		t.Fatal(err)
	}
	record, err := original.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("X" + strings.Repeat("a", domain.MaxSourceBytes-1))})
	if err != nil {
		t.Fatal(err)
	}
	meter := syncMeter{}
	_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
	if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted {
		t.Fatalf("unfunded boundary inspection accepted: reason=%s evidence=%s meter=%+v", reason, evidence, meter)
	}
}

func TestReconcileEmptyLineSearchNeedsBudget(t *testing.T) {
	original, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: nil})
	if err != nil {
		t.Fatal(err)
	}
	record, err := original.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationLine, Line: 1}})
	if err != nil {
		t.Fatal(err)
	}
	current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("a\nb\n")})
	if err != nil {
		t.Fatal(err)
	}
	meter := syncMeter{global: domain.MaxSyncWorkBytes}
	_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
	if reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone || !meter.exhausted {
		t.Fatalf("free index evidence used after exhaustion: reason=%s evidence=%s meter=%+v", reason, evidence, meter)
	}
}
