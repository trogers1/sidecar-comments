package domain_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

// Location grammar and revision hashing are production domain boundaries. No
// CLI recovery or mocked validation should mask a panic or invalid result.
func FuzzLocationRoundTrip(f *testing.F) {
	anchor := "YQ=="
	for _, location := range []domain.Location{
		{Kind: domain.LocationFile}, {Kind: domain.LocationLost},
		{Kind: domain.LocationLine, Line: 1},
		{Kind: domain.LocationRange, StartLine: 1, StartColumn: 1, EndLine: 2, EndColumn: 3, Anchor: &anchor},
	} {
		if err := domain.ValidateLocation(domain.ValidateLocationRequest{Location: location}); err != nil {
			f.Fatal(err)
		}
		f.Add(domain.LocationText(domain.LocationTextRequest{Location: location}))
	}
	for _, malformed := range []string{"", "line:0", "line:01", "range:1:2-1:1", "range:1:1-2:2 | /w==", "line:99999999999999999999999999999999", "\xff"} {
		f.Add(malformed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > domain.MaxTargetBytes+1 {
			t.Skip()
		}
		location, err := domain.ParseLocation(domain.ParseLocationRequest{Text: text})
		if err != nil {
			return
		}
		if err := domain.ValidateLocation(domain.ValidateLocationRequest{Location: location}); err != nil {
			t.Fatalf("parser produced invalid location: %v", err)
		}
		canonical := domain.LocationText(domain.LocationTextRequest{Location: location})
		if canonical != text {
			t.Fatalf("accepted noncanonical location %q -> %q", text, canonical)
		}
		again, err := domain.ParseLocation(domain.ParseLocationRequest{Text: canonical})
		if err != nil || !reflect.DeepEqual(location, again) {
			t.Fatalf("location changed: %+v -> %+v (%v)", location, again, err)
		}
	})
}

func FuzzRevisionDeterministic(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("body"), []byte("body\n"), []byte("body\n\n"), {0, 255}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > domain.MaxTargetBytes+1 {
			t.Skip()
		}
		before := bytes.Clone(data)
		revision := domain.HashRevision(domain.HashRevisionRequest{Bytes: data})
		if revision != domain.HashRevision(domain.HashRevisionRequest{Bytes: bytes.Clone(data)}) {
			t.Fatal("revision is nondeterministic")
		}
		if err := domain.ValidateRevision(domain.ValidateRevisionRequest{Text: revision}); err != nil {
			t.Fatalf("hash produced invalid revision: %v", err)
		}
		if !bytes.Equal(data, before) {
			t.Fatal("hash mutated input")
		}
		// Revisions are byte-based, not normalized text: final LF and blank lines
		// must affect them even when the visible text would look identical.
		withLF := append(bytes.Clone(data), '\n')
		if revision == domain.HashRevision(domain.HashRevisionRequest{Bytes: withLF}) {
			t.Fatal("revision ignored appended LF")
		}
	})
}
