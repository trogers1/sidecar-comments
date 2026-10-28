// Package store owns bounded filesystem reads and locked, durable transactions.
package store

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type ReadKind string

const (
	ReadTarget ReadKind = "target"
	ReadSource ReadKind = "source"
)

type ReadRequest struct {
	Path string
	Kind ReadKind
}
type TransactionReadRequest struct{}
type TransactionCloseRequest struct{}

func readLimit(r ReadRequest) (int64, error) {
	switch r.Kind {
	case ReadTarget:
		if err := validateTargetPath(pathRequest{Path: r.Path}); err != nil {
			return 0, err
		}
		return domain.MaxTargetBytes, nil
	case ReadSource:
		return domain.MaxSourceBytes, nil
	default:
		return 0, fmt.Errorf("store: invalid read kind %q", r.Kind)
	}
}

type Snapshot struct {
	Bytes    []byte
	Exists   bool
	Revision *string
}
type BeginRequest struct {
	Path    string
	Timeout time.Duration
}
type CommitRequest struct{ Bytes []byte }
type CheckWriterCapabilityRequest struct{}
type sourceCoordinateRequest struct{}
type cleanupRequest struct{}
type pathRequest struct{ Path string }
type columnRequest struct {
	Line   string
	Column int
}
type anchorMatchRequest struct {
	Lines    []string
	Location domain.Location
	Anchor   []byte
}
type ValidateSourceRequest struct {
	Path      string
	Locations []domain.Location
}

func validation(r domain.ValidationRequest) error { return domain.Validation(r) }

func ValidateSource(r ValidateSourceRequest) error {
	required := len(r.Locations) == 0
	for _, loc := range r.Locations {
		if loc.Kind != domain.LocationLost {
			required = true
		}
	}
	if !required {
		return nil
	}
	s, err := Read(ReadRequest{Path: r.Path, Kind: ReadSource})
	if err != nil {
		return err
	}
	b := s.Bytes
	if bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) {
		return validation(domain.ValidationRequest{Path: "source_encoding", Reason: "bom"})
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return validation(domain.ValidationRequest{Path: "source_encoding", Reason: "nul"})
	}
	if !utf8.Valid(b) {
		return validation(domain.ValidationRequest{Path: "source_encoding", Reason: "utf8"})
	}
	crlf := bytes.Count(b, []byte("\r\n"))
	if bytes.Count(b, []byte("\r")) != crlf || (crlf > 0 && bytes.Count(b, []byte("\n")) != crlf) {
		return validation(domain.ValidationRequest{Path: "source_endings", Reason: "endings"})
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	// A final terminator ends the preceding line; it does not create a phantom line.
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, loc := range r.Locations {
		switch loc.Kind {
		case domain.LocationLost, domain.LocationFile:
		case domain.LocationLine:
			if loc.Line < 1 || loc.Line > len(lines) {
				return sourceCoordinate(sourceCoordinateRequest{})
			}
		case domain.LocationRange:
			sl, el, sc, ec := loc.StartLine, loc.EndLine, loc.StartColumn, loc.EndColumn
			if sl < 1 || el < sl || el > len(lines) || (sl == el && sc >= ec) {
				return sourceCoordinate(sourceCoordinateRequest{})
			}
			if !column(columnRequest{Line: lines[sl-1], Column: sc}) || !column(columnRequest{Line: lines[el-1], Column: ec}) {
				return sourceCoordinate(sourceCoordinateRequest{})
			}
			if loc.Anchor != nil {
				if len(*loc.Anchor) > base64.StdEncoding.EncodedLen(domain.MaxAnchorBytes) {
					return validation(domain.ValidationRequest{Path: "target_content", Reason: "location"})
				}
				anchor, e := base64.StdEncoding.Strict().DecodeString(*loc.Anchor)
				if e != nil || len(anchor) == 0 || !utf8.Valid(anchor) || len(anchor) > domain.MaxAnchorBytes {
					return validation(domain.ValidationRequest{Path: "target_content", Reason: "location"})
				}
				if !anchorMatches(anchorMatchRequest{Lines: lines, Location: loc, Anchor: anchor}) {
					return validation(domain.ValidationRequest{Path: "source_anchor", Reason: "location"})
				}
			}
		default:
			return validation(domain.ValidationRequest{Path: "target_content", Reason: "location"})
		}
	}
	return nil
}

// anchorMatches consumes bounded anchor bytes without constructing the selected
// source range. Each crossed line consumes a logical LF, including CRLF input.
func anchorMatches(r anchorMatchRequest) bool {
	remaining := r.Anchor
	for line := r.Location.StartLine; line <= r.Location.EndLine; line++ {
		fragment := r.Lines[line-1]
		start, end := 0, len(fragment)
		if line == r.Location.StartLine {
			start = r.Location.StartColumn - 1
		}
		if line == r.Location.EndLine {
			end = r.Location.EndColumn - 1
		}
		fragment = fragment[start:end]
		if len(fragment) > len(remaining) || string(remaining[:len(fragment)]) != fragment {
			return false
		}
		remaining = remaining[len(fragment):]
		if line < r.Location.EndLine {
			if len(remaining) == 0 || remaining[0] != '\n' {
				return false
			}
			remaining = remaining[1:]
		}
	}
	return len(remaining) == 0
}
func validateTargetPath(r pathRequest) error {
	name := filepath.Base(r.Path)
	if r.Path == "" || strings.HasSuffix(r.Path, string(filepath.Separator)) || name == ".comments" || !strings.HasSuffix(name, ".comments") {
		return validation(domain.ValidationRequest{Path: "target_name", Reason: "invalid"})
	}
	return nil
}
func sourceCoordinate(r sourceCoordinateRequest) error {
	return validation(domain.ValidationRequest{Path: "source_coordinate", Reason: "location"})
}
func column(r columnRequest) bool {
	return r.Column >= 1 && r.Column <= len(r.Line)+1 && (r.Column == len(r.Line)+1 || utf8.RuneStart(r.Line[r.Column-1]))
}
