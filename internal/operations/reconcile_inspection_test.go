package operations

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestReconcileStrongFundsEveryOriginalByteBeforeInspection(t *testing.T) {
	for _, tc := range []struct{ name, head string }{
		{name: "three_byte_space_four_byte_nonspace", head: "\u2003🌍"},
		{name: "two_byte_space_two_byte_nonspace", head: "\u00a0é"},
		{name: "valid_replacement_character", head: " \ufffd"},
		{name: "ascii", head: "\tX"},
		{name: "two_byte_nonspace", head: "é"},
	} {
		text := tc.head + strings.Repeat(" ", domain.MinMatchContextBytes)
		work := len(tc.head)
		for _, global := range []bool{false, true} {
			for remaining := 0; remaining <= work; remaining++ {
				t.Run(fmt.Sprintf("%s/global=%t/remaining=%d", tc.name, global, remaining), func(t *testing.T) {
					meter := syncMeter{}
					if global {
						meter.global = domain.MaxSyncWorkBytes - remaining
					} else {
						meter.thread = domain.MaxSourceBytes - remaining
					}
					initial := meter
					observed := []int{}
					accepted := meter.strong(syncTextRequest{Text: text, ObserveInspection: func(r domain.SourceByteInspectionRequest) { observed = append(observed, r.Offset) }})
					want := make([]int, remaining)
					for i := range want {
						want[i] = i
					}
					if !reflect.DeepEqual(observed, want) {
						t.Fatalf("inspection escaped funded prefix: got=%v want=%v", observed, want)
					}
					if meter.thread-initial.thread != len(observed) || meter.global-initial.global != len(observed) {
						t.Fatalf("unaccounted actual inspection: initial=%+v final=%+v observed=%v", initial, meter, observed)
					}
					enough := remaining == work
					if accepted != enough || meter.exhausted == enough {
						t.Fatalf("accepted=%t exhausted=%t enough=%t", accepted, meter.exhausted, enough)
					}
					if !enough {
						before := meter
						if meter.spend(syncWorkRequest{Bytes: 1}) || meter != before {
							t.Fatal("refusal was not sticky")
						}
					}
				})
			}
		}
	}
}

func TestReconcileStrongAlreadyExhaustedDoesNotInspect(t *testing.T) {
	meter := syncMeter{exhausted: true}
	before := meter
	observations := 0
	if meter.strong(syncTextRequest{Text: "\u2003🌍" + strings.Repeat(" ", domain.MinMatchContextBytes), ObserveInspection: func(r domain.SourceByteInspectionRequest) { observations++ }}) || observations != 0 || meter != before {
		t.Fatalf("exhausted entry inspected/spent: observations=%d meter=%+v", observations, meter)
	}
}

func TestReconcileStrongWhitespaceOnlyExactFunding(t *testing.T) {
	text := strings.Repeat("\u2003", domain.MinMatchContextBytes)
	meter := syncMeter{global: domain.MaxSyncWorkBytes - len(text)}
	observations := 0
	if meter.strong(syncTextRequest{Text: text, ObserveInspection: func(r domain.SourceByteInspectionRequest) {
		if r.Offset != observations {
			t.Fatal("nonsequential whitespace inspection")
		}
		observations++
	}}) || observations != len(text) || meter.thread != len(text) || meter.exhausted {
		t.Fatalf("whitespace scan miscounted: observations=%d meter=%+v", observations, meter)
	}
}

func TestReconcileStrongInvalidContextFailsLoudly(t *testing.T) {
	for _, tc := range []struct{ name, prefix, panicText string }{
		{name: "invalid_leading", prefix: "\xff", panicText: "reconcile: invalid UTF-8 context leading byte"},
		{name: "bad_continuation", prefix: "\xe2Xx", panicText: "reconcile: invalid UTF-8 context encoding"},
		{name: "surrogate", prefix: "\xed\xa0\x80", panicText: "reconcile: invalid UTF-8 context encoding"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if got := recover(); got != tc.panicText {
					t.Fatalf("unexpected panic: %v", got)
				}
			}()
			meter := syncMeter{}
			meter.strong(syncTextRequest{Text: tc.prefix + strings.Repeat(" ", domain.MinMatchContextBytes)})
		})
	}
}
