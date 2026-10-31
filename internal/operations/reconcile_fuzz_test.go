package operations

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type expectedWindowRequest struct {
	Text   string
	Prefix bool
}

// Independent rune-sequence oracle: production capture trims byte offsets.
func expectedWindow(r expectedWindowRequest) string {
	runes := []rune(r.Text)
	used := 0
	if r.Prefix {
		start := len(runes)
		for start > 0 && used+utf8.RuneLen(runes[start-1]) <= domain.MaxContextBytes {
			start--
			used += utf8.RuneLen(runes[start])
		}
		return string(runes[start:])
	}
	end := 0
	for end < len(runes) && used+utf8.RuneLen(runes[end]) <= domain.MaxContextBytes {
		used += utf8.RuneLen(runes[end])
		end++
	}
	return string(runes[:end])
}

func FuzzTrackingReconcile(f *testing.F) {
	tid, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		f.Fatal(err)
	}
	mid, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		f.Fatal(err)
	}
	for _, mode := range []uint8{0, 1, 2, 3, 4} {
		f.Add("before", "chosen", "after", "replacement", mode, uint16(40))
		f.Add(strings.Repeat("é", domain.MaxContextBytes), "🌍\t\"", strings.Repeat("界", domain.MaxContextBytes/utf8.RuneLen('界')+2), "new\ntext", mode, uint16(1))
		f.Add("", "", "", "", mode, uint16(0))
	}
	for _, size := range []int{domain.MaxAnchorBytes - 1, domain.MaxAnchorBytes, domain.MaxAnchorBytes + 1} {
		f.Add("", strings.Repeat("x", size), "", "", uint8(0), uint16(16))
	}
	f.Fuzz(func(t *testing.T, prefix, selection, suffix, edit string, mode uint8, remaining uint16) {
		if len(prefix) > 2*domain.MaxContextBytes || len(suffix) > 2*domain.MaxContextBytes || len(selection) > domain.MaxAnchorBytes+1 || len(edit) > domain.MaxAnchorBytes+1 {
			return
		}
		for _, text := range []string{prefix, selection, suffix, edit} {
			if !utf8.ValidString(text) || strings.ContainsAny(text, "\x00\r") {
				return
			}
		}
		oldText := prefix + "\n" + selection + "\n" + suffix
		if strings.HasPrefix(oldText, "\ufeff") {
			return
		}
		original, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(oldText)})
		if err != nil {
			t.Fatal(err)
		}
		location := domain.Location{Kind: domain.LocationLine, Line: strings.Count(prefix, "\n") + 2}
		if selection != "" {
			location, err = original.Location(domain.SourceOffsetsRequest{Start: len(prefix) + 1, End: len(prefix) + 1 + len(selection)})
			if err != nil {
				t.Fatal(err)
			}
		}
		record, err := original.Capture(domain.SourceLocationRequest{Location: location})
		if len(selection) > domain.MaxAnchorBytes {
			if err == nil {
				t.Fatal("oversize capture accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		wantPrefix := expectedWindow(expectedWindowRequest{Text: prefix + "\n", Prefix: true})
		wantSuffix := expectedWindow(expectedWindowRequest{Text: "\n" + suffix})
		if record.SelectedText != selection || record.Prefix != wantPrefix || record.Suffix != wantSuffix || record.PrefixAtStart != (len(wantPrefix) == len(prefix)+1) || record.SuffixAtEnd != (len(wantSuffix) == len(suffix)+1) {
			t.Fatal("capture is not the full selection and longest rune-aligned windows")
		}
		thread := domain.Thread{ID: tid, Location: location, Tracking: &record, Messages: []domain.Message{{ID: mid, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "discussion"}}}
		document, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{thread}})
		if err != nil {
			t.Fatal(err)
		}
		currentText := oldText
		switch mode % 5 {
		case 1:
			currentText = "heading\n" + oldText
		case 2:
			currentText = "erased\n"
		case 3:
			currentText = oldText + oldText
		case 4:
			currentText = prefix + "\n" + edit + "\n" + suffix
		}
		current, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(currentText)})
		if err != nil {
			t.Fatal(err)
		}
		result, err := Reconcile(ReconcileRequest{Document: document, Source: &current})
		if err != nil {
			t.Fatal(err)
		}
		after := result.Document.Threads(domain.DocumentThreadsRequest{})[0]
		if after.ID != thread.ID || !reflect.DeepEqual(after.Messages, thread.Messages) {
			t.Fatal("conversation changed")
		}
		if after.Location.Kind == domain.LocationUnresolved {
			if !reflect.DeepEqual(after.Tracking, thread.Tracking) {
				t.Fatal("unresolved evidence changed")
			}
		} else {
			if err := current.ValidateLocation(domain.SourceLocationRequest{Location: after.Location}); err != nil {
				t.Fatal(err)
			}
			selected, err := current.SelectedText(domain.SourceLocationRequest{Location: after.Location})
			if err != nil || selected != after.Tracking.SelectedText {
				t.Fatal("accepted location does not describe retained selection")
			}
		}
		if mode%5 == 0 && result.Outcomes[0].Outcome != domain.OutcomeUnchanged {
			t.Fatal("unchanged source lost attachment")
		}
		if mode%5 == 3 && (result.Outcomes[0].Outcome != domain.OutcomeUnresolved || (result.Outcomes[0].Reason != domain.ReasonAmbiguous && result.Outcomes[0].Reason != domain.ReasonSearchLimit)) {
			t.Fatal("duplicate supported passages accepted")
		}
		repeated, err := Reconcile(ReconcileRequest{Document: result.Document, Source: &current})
		if err != nil || !reflect.DeepEqual(result.Document, repeated.Document) {
			t.Fatalf("repeat unstable: %v", err)
		}
		meter := syncMeter{global: domain.MaxSyncWorkBytes - int(remaining)}
		_, reason, evidence := matchTracking(syncMatchRequest{Source: &current, Tracking: &record, Meter: &meter})
		if meter.thread > domain.MaxSourceBytes || meter.global > domain.MaxSyncWorkBytes || (meter.exhausted && (reason != domain.ReasonSearchLimit || evidence != domain.EvidenceNone)) {
			t.Fatal("search violated work cap or accepted unchecked evidence")
		}
	})
}
