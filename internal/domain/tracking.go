package domain

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// TrackingRecord is an untrusted persistence DTO. Validated values own copies.
type TrackingRecord struct {
	Version        int      `json:"version"`
	SourceRevision string   `json:"source_revision"`
	LastLocation   Location `json:"last_location"`
	SelectedText   string   `json:"selected_text"`
	Prefix         string   `json:"prefix"`
	Suffix         string   `json:"suffix"`
	PrefixAtStart  bool     `json:"prefix_at_start"`
	SuffixAtEnd    bool     `json:"suffix_at_end"`
}
type TrackingSummary struct {
	Version        int      `json:"version"`
	SourceRevision string   `json:"source_revision"`
	LastLocation   Location `json:"last_location"`
	PrefixAtStart  bool     `json:"prefix_at_start"`
	SuffixAtEnd    bool     `json:"suffix_at_end"`
}
type PublicThread struct {
	ID       string           `json:"id"`
	Location Location         `json:"location"`
	Messages []Message        `json:"messages"`
	Tracking *TrackingSummary `json:"tracking,omitempty"`
}
type ThreadMetadata struct {
	Tracking *TrackingRecord `json:"tracking,omitempty"`
}
type trackingErrorRequest struct{ Reason string }

func trackingError(r trackingErrorRequest) error {
	return Validation(ValidationRequest{Path: "tracking", Reason: r.Reason})
}

type ValidateThreadTrackingRequest struct {
	Location Location
	Tracking *TrackingRecord
}

func ValidateThreadTracking(r ValidateThreadTrackingRequest) error {
	fail := trackingError(trackingErrorRequest{Reason: "invalid"})
	if ValidateLocation(ValidateLocationRequest{Location: r.Location}) != nil {
		return fail
	}
	attached := r.Location.Kind == LocationLine || r.Location.Kind == LocationRange
	if attached || r.Location.Kind == LocationUnresolved {
		if r.Tracking == nil {
			return fail
		}
		if err := ValidateTrackingRecord(ValidateTrackingRecordRequest{Tracking: *r.Tracking}); err != nil {
			return err
		}
		if attached && r.Location != r.Tracking.LastLocation {
			return fail
		}
	} else if r.Tracking != nil {
		return fail
	}
	return nil
}

type ValidateDocumentTrackingRequest struct{ Threads []Thread }

func ValidateDocumentTracking(r ValidateDocumentTrackingRequest) error {
	total := 0
	for _, t := range r.Threads {
		if err := ValidateThreadTracking(ValidateThreadTrackingRequest{Location: t.Location, Tracking: t.Tracking}); err != nil {
			return err
		}
		if t.Tracking != nil {
			total += len(t.Tracking.SelectedText) + len(t.Tracking.Prefix) + len(t.Tracking.Suffix)
		}
		if total > MaxTrackingBytes {
			return trackingError(trackingErrorRequest{Reason: "too_large"})
		}
	}
	return nil
}

type ValidateTrackingRecordRequest struct{ Tracking TrackingRecord }
type CloneTrackingRequest struct{ Tracking *TrackingRecord }

func CloneTracking(r CloneTrackingRequest) *TrackingRecord {
	if r.Tracking == nil {
		return nil
	}
	t := *r.Tracking
	t.LastLocation = CloneLocation(CloneLocationRequest{Location: t.LastLocation})
	return &t
}
func ValidateTrackingRecord(r ValidateTrackingRecordRequest) error {
	t := r.Tracking
	fail := trackingError(trackingErrorRequest{Reason: "invalid"})
	if t.Version != TrackingVersion || ValidateSourceRevision(ValidateSourceRevisionRequest{Text: t.SourceRevision}) != nil || (t.LastLocation.Kind != LocationLine && t.LastLocation.Kind != LocationRange) || ValidateLocation(ValidateLocationRequest{Location: t.LastLocation}) != nil {
		return fail
	}
	if len(t.SelectedText) > MaxAnchorBytes || len(t.Prefix) > MaxContextBytes || len(t.Suffix) > MaxContextBytes {
		return trackingError(trackingErrorRequest{Reason: "too_large"})
	}
	for _, s := range []string{t.SelectedText, t.Prefix, t.Suffix} {
		if !utf8.ValidString(s) || strings.ContainsAny(s, "\r\x00") {
			return fail
		}
	}
	if t.LastLocation.Kind == LocationRange && t.SelectedText == "" || t.LastLocation.Kind == LocationLine && strings.Contains(t.SelectedText, "\n") {
		return fail
	}
	// A non-edge window can be short only when the next rune exceeds the cap.
	if !t.PrefixAtStart && len(t.Prefix) <= MaxContextBytes-utf8.UTFMax || !t.SuffixAtEnd && len(t.Suffix) <= MaxContextBytes-utf8.UTFMax {
		return fail
	}
	return nil
}

// CaptureLimitError is source-independent. The orchestrator maps it to its
// explicit operation's precondition:limit_selection (including batch index).
type CaptureLimitError struct{}

func (*CaptureLimitError) Error() string { return "capture:limit_selection" }
func (s SourceValue) Capture(r SourceLocationRequest) (TrackingRecord, error) {
	if err := ValidateLocation(ValidateLocationRequest{Location: r.Location}); err != nil {
		return TrackingRecord{}, err
	}
	b, err := s.Offsets(r)
	if err != nil {
		return TrackingRecord{}, err
	}
	if b.End-b.Start > MaxAnchorBytes {
		return TrackingRecord{}, &CaptureLimitError{}
	}
	start := b.Start - MaxContextBytes
	if start < 0 {
		start = 0
	}
	for start < b.Start && !utf8.RuneStart(s.text[start]) {
		start++
	}
	end := b.End + MaxContextBytes
	if end > len(s.text) {
		end = len(s.text)
	}
	for end < len(s.text) && end > b.End && !utf8.RuneStart(s.text[end]) {
		end--
	}
	t := TrackingRecord{Version: TrackingVersion, SourceRevision: s.Fingerprint(ValueRequest{}), LastLocation: CloneLocation(CloneLocationRequest{Location: r.Location}), SelectedText: s.text[b.Start:b.End], Prefix: s.text[start:b.Start], Suffix: s.text[b.End:end], PrefixAtStart: start == 0, SuffixAtEnd: end == len(s.text)}
	if err := ValidateTrackingRecord(ValidateTrackingRecordRequest{Tracking: t}); err != nil {
		return TrackingRecord{}, err
	}
	return t, nil
}

type UnsupportedRequest struct{ Received string }

func Unsupported(r UnsupportedRequest) error {
	b, err := json.Marshal(struct {
		Received string `json:"received"`
	}{r.Received})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeUnsupported, Message: "unsupported:" + r.Received, Details: b, Exit: ExitUnsupported}
}

type SourceRevisionConflictRequest struct{ Expected, CurrentSourceRevision string }
type SourceRevisionConflictDetails struct {
	Kind                  string `json:"kind"`
	Expected              string `json:"expected"`
	CurrentSourceRevision string `json:"current_source_revision"`
}

func SourceRevisionConflict(r SourceRevisionConflictRequest) error {
	b, err := json.Marshal(SourceRevisionConflictDetails{Kind: "source_revision", Expected: r.Expected, CurrentSourceRevision: r.CurrentSourceRevision})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeConflict, Message: "conflict:source_revision", Details: b, Exit: ExitConflict}
}
