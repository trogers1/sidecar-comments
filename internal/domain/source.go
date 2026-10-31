package domain

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"
)

// SourceValue owns validated, normalized logical source. Its zero value is invalid.
// Strings and private line indexes keep copies immutable; Bytes returns a copy.
type SourceValue struct {
	text        string
	fingerprint string
	starts      []int
	ends        []int
}

type NewSourceValueRequest struct{ Bytes []byte }
type ValidateSourceRevisionRequest struct{ Text string }

// InspectByte is optional search accounting. Refusal aborts coordinate
// validation before reading the indicated byte; ordinary callers omit it.
type SourceByteInspectionRequest struct{ Offset int }
type SourceLocationRequest struct {
	Location    Location
	InspectByte func(SourceByteInspectionRequest) bool
}
type SourcePositionRequest struct {
	Line, Column int
	InspectByte  func(SourceByteInspectionRequest) bool
}
type SourceOffsetRequest struct {
	Offset      int
	InspectByte func(SourceByteInspectionRequest) bool
}
type SourceOffsetsRequest struct {
	Start, End  int
	InspectByte func(SourceByteInspectionRequest) bool
}
type SourceOffsets struct{ Start, End int }
type SourcePosition struct{ Line, Column int }

func ValidateSourceRevision(r ValidateSourceRevisionRequest) error {
	if r.Text == RevisionMissing {
		return invalid(invalidRequest{})
	}
	return ValidateRevision(ValidateRevisionRequest{Text: r.Text})
}

func NewSourceValue(r NewSourceValueRequest) (SourceValue, error) {
	b := r.Bytes
	if len(b) > MaxSourceBytes {
		return SourceValue{}, Validation(ValidationRequest{Path: "source_size", Reason: "too_large"})
	}
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		return SourceValue{}, Validation(ValidationRequest{Path: "source_encoding", Reason: "bom"})
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return SourceValue{}, Validation(ValidationRequest{Path: "source_encoding", Reason: "nul"})
	}
	if !utf8.Valid(b) {
		return SourceValue{}, Validation(ValidationRequest{Path: "source_encoding", Reason: "utf8"})
	}
	crlf := bytes.Count(b, []byte("\r\n"))
	if bytes.Count(b, []byte("\r")) != crlf || (crlf > 0 && bytes.Count(b, []byte("\n")) != crlf) {
		return SourceValue{}, Validation(ValidationRequest{Path: "source_endings", Reason: "endings"})
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	s := SourceValue{text: text, fingerprint: HashRevision(HashRevisionRequest{Bytes: []byte(text)})}
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			s.starts = append(s.starts, start)
			s.ends = append(s.ends, i)
			start = i + 1
		}
	}
	// A final LF terminates the preceding line, not a phantom empty line.
	if start < len(text) || len(text) == 0 {
		s.starts = append(s.starts, start)
		s.ends = append(s.ends, len(text))
	}
	return s, nil
}

func (s SourceValue) requireValid(r ValueRequest) {
	if s.fingerprint == "" {
		panic("domain: uninitialized SourceValue")
	}
}
func (s SourceValue) Text(r ValueRequest) string {
	s.requireValid(r)
	return s.text
}
func (s SourceValue) Bytes(r ValueRequest) []byte { return []byte(s.Text(r)) }
func (s SourceValue) Fingerprint(r ValueRequest) string {
	s.requireValid(r)
	return s.fingerprint
}
func (s SourceValue) ByteCount(r ValueRequest) int { return len(s.Text(r)) }
func (s SourceValue) LineCount(r ValueRequest) int {
	s.requireValid(r)
	return len(s.starts)
}

func sourceCoordinateError(r ValueRequest) error {
	return Validation(ValidationRequest{Path: "source_coordinate", Reason: "location"})
}

// Offset maps a one-based logical byte position to a zero-based byte offset.
// Columns may identify the end of a line, but never the inside of a UTF-8 rune.
func (s SourceValue) Offset(r SourcePositionRequest) (int, error) {
	if r.Line < 1 || r.Line > len(s.starts) || r.Column < 1 {
		return 0, sourceCoordinateError(ValueRequest{})
	}
	start, end := s.starts[r.Line-1], s.ends[r.Line-1]
	if r.Column-1 > end-start {
		return 0, sourceCoordinateError(ValueRequest{})
	}
	offset := start + r.Column - 1
	if offset < end {
		if r.InspectByte != nil && !r.InspectByte(SourceByteInspectionRequest{Offset: offset}) {
			return 0, sourceCoordinateError(ValueRequest{})
		}
		if !utf8.RuneStart(s.text[offset]) {
			return 0, sourceCoordinateError(ValueRequest{})
		}
	}
	return offset, nil
}

// Position rejects offsets without a logical coordinate, including the byte
// after a final LF. The LF itself is the preceding line's end column.
func (s SourceValue) Position(r SourceOffsetRequest) (SourcePosition, error) {
	if len(s.starts) == 0 || r.Offset < 0 || r.Offset > len(s.text) {
		return SourcePosition{}, sourceCoordinateError(ValueRequest{})
	}
	line := sort.Search(len(s.starts), func(i int) bool { return s.starts[i] > r.Offset }) - 1
	if line < 0 || r.Offset > s.ends[line] {
		return SourcePosition{}, sourceCoordinateError(ValueRequest{})
	}
	if r.Offset < s.ends[line] {
		if r.InspectByte != nil && !r.InspectByte(SourceByteInspectionRequest{Offset: r.Offset}) {
			return SourcePosition{}, sourceCoordinateError(ValueRequest{})
		}
		if !utf8.RuneStart(s.text[r.Offset]) {
			return SourcePosition{}, sourceCoordinateError(ValueRequest{})
		}
	}
	return SourcePosition{Line: line + 1, Column: r.Offset - s.starts[line] + 1}, nil
}

// Offsets returns the selection bounds for line/range locations. Whole lines
// exclude their terminator. File and lost locations have no selection.
// Legacy anchors are intentionally not interpreted here.
func (s SourceValue) Offsets(r SourceLocationRequest) (SourceOffsets, error) {
	l := r.Location
	switch l.Kind {
	case LocationLine:
		if l.Line < 1 || l.Line > len(s.starts) {
			return SourceOffsets{}, sourceCoordinateError(ValueRequest{})
		}
		return SourceOffsets{Start: s.starts[l.Line-1], End: s.ends[l.Line-1]}, nil
	case LocationRange:
		start, err := s.Offset(SourcePositionRequest{Line: l.StartLine, Column: l.StartColumn, InspectByte: r.InspectByte})
		if err != nil {
			return SourceOffsets{}, err
		}
		end, err := s.Offset(SourcePositionRequest{Line: l.EndLine, Column: l.EndColumn, InspectByte: r.InspectByte})
		if err != nil {
			return SourceOffsets{}, err
		}
		if start >= end {
			return SourceOffsets{}, sourceCoordinateError(ValueRequest{})
		}
		return SourceOffsets{Start: start, End: end}, nil
	default:
		return SourceOffsets{}, Validation(ValidationRequest{Path: "target_content", Reason: "location"})
	}
}

func (s SourceValue) ValidateLocation(r SourceLocationRequest) error {
	if len(s.starts) == 0 {
		return sourceCoordinateError(ValueRequest{})
	}
	if r.Location.Kind == LocationFile || r.Location.Kind == LocationLost {
		return nil
	}
	_, err := s.Offsets(r)
	return err
}

func (s SourceValue) SelectedText(r SourceLocationRequest) (string, error) {
	bounds, err := s.Offsets(r)
	if err != nil {
		return "", err
	}
	return s.text[bounds.Start:bounds.End], nil
}

// Location constructs a nonempty range from half-open logical byte offsets.
func (s SourceValue) Location(r SourceOffsetsRequest) (Location, error) {
	if r.Start >= r.End {
		return Location{}, sourceCoordinateError(ValueRequest{})
	}
	start, err := s.Position(SourceOffsetRequest{Offset: r.Start, InspectByte: r.InspectByte})
	if err != nil {
		return Location{}, err
	}
	end, err := s.Position(SourceOffsetRequest{Offset: r.End, InspectByte: r.InspectByte})
	if err != nil {
		return Location{}, err
	}
	return Location{Kind: LocationRange, StartLine: start.Line, StartColumn: start.Column, EndLine: end.Line, EndColumn: end.Column}, nil
}
