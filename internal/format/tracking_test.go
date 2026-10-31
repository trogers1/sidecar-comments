package format_test

import (
	"bytes"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
)

func TestV2TrackingFraming(t *testing.T) {
	corpus := corpusDocument(corpusRequest{T: t})
	threads := fixture(fixtureRequest{T: t, Body: "conversation\n<!-- sidecar-tracking: ordinary body -->", Corpus: corpus}).Threads(domain.DocumentThreadsRequest{})
	selected := "é --> <!-- sidecar-tracking: {} --> <&>\u2028"
	loc := domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: len(selected) + 1}
	threads[0].Location = loc
	threads[0].Tracking = &domain.TrackingRecord{Version: domain.TrackingVersion, SourceRevision: domain.HashRevision(domain.HashRevisionRequest{Bytes: []byte(selected)}), LastLocation: loc, SelectedText: selected, PrefixAtStart: true, SuffixAtEnd: true}
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	checkRoundtrip(roundtripRequest{T: t, Value: value})
	canonical := format.Encode(format.EncodeRequest{Document: value})
	if bytes.Contains(canonical, []byte(`\u003c`)) || bytes.Contains(canonical, []byte(`\u2028`)) {
		t.Fatal("tracking is not raw UTF-8")
	}
	if !bytes.Contains(canonical, []byte(`"prefix_at_start":true,"suffix_at_end":true`)) {
		t.Fatal("tracking key order changed")
	}
	for _, mode := range []string{"normal", "format", "repair"} {
		t.Run(mode, func(t *testing.T) {
			parsed, err := format.Parse(format.ParseRequest{Bytes: canonical, Mode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(parsed.Value.Threads(domain.DocumentThreadsRequest{}), threads) {
				t.Fatal("tracking changed")
			}
			for _, bad := range []string{
				strings.Replace(string(canonical), `"version":1`, `"version":1,"version":1`, 1),
				strings.Replace(string(canonical), `"version":1`, `"version":1e0`, 1),
				strings.Replace(string(canonical), `"prefix":""`, `"prefix":null`, 1),
				strings.Replace(string(canonical), `"prefix":""`, `"prefix":"\ud800"`, 1),
				strings.Replace(string(canonical), `"tracking":{`, `"unknown":{`, 1),
				strings.Replace(string(canonical), "<!-- sidecar-tracking: ", "<!-- invalid: ", 1),
			} {
				if _, err := format.Parse(format.ParseRequest{Bytes: []byte(bad), Mode: mode}); err == nil {
					t.Fatal("accepted malformed tracking")
				}
			}
		})
	}
	variant := bytes.Replace(canonical, []byte(`"version":1`), []byte(`"version" : 1`), 1)
	variant = bytes.ReplaceAll(variant, []byte("\n"), []byte("\r\n"))
	formatted, err := format.Parse(format.ParseRequest{Bytes: variant, Mode: "format"})
	if err != nil || !bytes.Equal(format.Encode(format.EncodeRequest{Document: formatted.Value}), canonical) {
		t.Fatalf("tracking formatter allowance: %v", err)
	}
	// Control-byte escaping can exceed message metadata's limit while every
	// decoded tracking excerpt remains within its independent domain budget.
	threads[0].Tracking.SelectedText = strings.Repeat("\x01", domain.MaxAnchorBytes)
	threads[0].Location.EndColumn = domain.MaxAnchorBytes + 1
	threads[0].Tracking.LastLocation = threads[0].Location
	value, err = domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	encoded := format.Encode(format.EncodeRequest{Document: value})
	if len(encoded) <= domain.MaxMetadataBytes {
		t.Fatal("fixture does not exceed message metadata cap")
	}
	checkRoundtrip(roundtripRequest{T: t, Value: value})
}

func TestV2ThreadOrder(t *testing.T) {
	corpus := corpusDocument(corpusRequest{T: t})
	base := corpus.Threads(domain.DocumentThreadsRequest{})[0]
	locations := []domain.Location{
		{Kind: domain.LocationFile},
		{Kind: domain.LocationLine, Line: 1},
		{Kind: domain.LocationRange, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2},
		{Kind: domain.LocationLost},
		{Kind: domain.LocationUnresolved},
		{Kind: domain.LocationUnresolved},
	}
	var threads []domain.Thread
	var want []string
	for i, location := range locations {
		id := base.ID[:len(base.ID)-1] + strconv.Itoa(i+1)
		message := base.Messages[0]
		message.ID = id
		thread := domain.Thread{ID: id, Location: location, Messages: []domain.Message{message}}
		if i == 1 || i == 2 || i >= 4 {
			last := location
			if i >= 4 {
				// Reverse historical coordinates relative to ID order.
				last = domain.Location{Kind: domain.LocationLine, Line: len(locations) - i}
			}
			thread.Tracking = &domain.TrackingRecord{Version: domain.TrackingVersion, SourceRevision: domain.HashRevision(domain.HashRevisionRequest{Bytes: []byte("x")}), LastLocation: last, SelectedText: "x", PrefixAtStart: true, SuffixAtEnd: true}
		}
		threads = append([]domain.Thread{thread}, threads...)
		want = append(want, id)
	}
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	encoded := format.Encode(format.EncodeRequest{Document: value})
	previous := -1
	for _, id := range want {
		position := bytes.Index(encoded, []byte(domain.ThreadOpenPrefix+id+" "))
		if position <= previous {
			t.Fatal("serialized threads not in expected order")
		}
		previous = position
	}
	parsed, err := format.Parse(format.ParseRequest{Bytes: encoded, Mode: "normal"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, thread := range parsed.Value.Threads(domain.DocumentThreadsRequest{}) {
		got = append(got, thread.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("thread order: got %v, want %v", got, want)
	}
}

func TestV2VersionAndRepair(t *testing.T) {
	corpus := corpusDocument(corpusRequest{T: t})
	canonical := format.Encode(format.EncodeRequest{Document: fixture(fixtureRequest{T: t, Body: "body", Corpus: corpus})})
	header, rest, _ := strings.Cut(string(canonical), "\n")
	for _, mode := range []string{"normal", "format", "repair"} {
		for _, data := range []string{rest, strings.Replace(string(canonical), header, "<!-- sidecar-format: 1 -->", 1), strings.Replace(string(canonical), header, "<!-- sidecar-format: 999999999999999999999999 -->", 1)} {
			_, err := format.Parse(format.ParseRequest{Bytes: []byte(data), Mode: mode})
			var typed *domain.Error
			if !errors.As(err, &typed) || typed.Code != domain.CodeUnsupported || typed.Exit != domain.ExitUnsupported {
				t.Fatalf("%s: wanted unsupported, got %v", mode, err)
			}
		}
		for _, data := range []string{header + "\n", header + "\n" + string(canonical), strings.Replace(string(canonical), header, "<!-- sidecar-format: nope -->", 1), strings.Replace(string(canonical), "<!-- sidecar-tracking: {} -->\n", "", 1)} {
			if _, err := format.Parse(format.ParseRequest{Bytes: []byte(data), Mode: mode}); err == nil {
				t.Fatalf("%s accepted malformed framing", mode)
			}
		}
		parsed, err := format.Parse(format.ParseRequest{Mode: mode})
		if err != nil || len(format.Encode(format.EncodeRequest{Document: parsed.Value})) != 0 {
			t.Fatalf("empty %s: %v", mode, err)
		}
	}
	lines := strings.Split(string(canonical), "\n")
	lines[3] = "---" + domain.RoleUser
	repairedBytes := []byte(strings.Join(lines, "\n"))
	parsed, err := format.Parse(format.ParseRequest{Bytes: repairedBytes, Mode: "repair"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Repair == nil || len(parsed.Repair.Diagnostics) != 1 {
		t.Fatal("missing repair diagnostic")
	}
	diagnostic := parsed.Repair.Diagnostics[0]
	if diagnostic.Line != 4 || diagnostic.ByteOffset != len(strings.Join(lines[:3], "\n"))+2 {
		t.Fatalf("incorrect v2 repair coordinates: %+v", diagnostic)
	}
}
