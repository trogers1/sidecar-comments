package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type trackingMetadataRequest struct{ Tracking *domain.TrackingRecord }

// Use the same raw UTF-8 quoting as message metadata, including U+2028/U+2029.
func trackingMetadata(r trackingMetadataRequest) string {
	if r.Tracking == nil {
		return `{}`
	}
	t := r.Tracking
	l := t.LastLocation
	location := `{"kind":` + quote(quoteRequest{Text: l.Kind})
	if l.Kind == domain.LocationLine {
		location += `,"line":` + strconv.Itoa(l.Line)
	} else {
		location += `,"start_line":` + strconv.Itoa(l.StartLine) + `,"start_column":` + strconv.Itoa(l.StartColumn) + `,"end_line":` + strconv.Itoa(l.EndLine) + `,"end_column":` + strconv.Itoa(l.EndColumn)
	}
	location += `}`
	return `{"tracking":{"version":` + strconv.Itoa(t.Version) + `,"source_revision":` + quote(quoteRequest{Text: t.SourceRevision}) + `,"last_location":` + location + `,"selected_text":` + quote(quoteRequest{Text: t.SelectedText}) + `,"prefix":` + quote(quoteRequest{Text: t.Prefix}) + `,"suffix":` + quote(quoteRequest{Text: t.Suffix}) + `,"prefix_at_start":` + strconv.FormatBool(t.PrefixAtStart) + `,"suffix_at_end":` + strconv.FormatBool(t.SuffixAtEnd) + `}}`
}

type parseTrackingRequest struct {
	Text, Mode string
	Location   domain.Location
}

func parseTracking(r parseTrackingRequest) (*domain.TrackingRecord, error) {
	if !strings.HasPrefix(r.Text, trackingPrefix) || !strings.HasSuffix(r.Text, commentSuffix) {
		return nil, invalid(invalidRequest{})
	}
	// Only the terminal suffix frames JSON. An excerpt may contain any number of
	// earlier comment suffixes and delimiter-looking strings.
	raw := []byte(strings.TrimSuffix(strings.TrimPrefix(r.Text, trackingPrefix), commentSuffix))
	if len(raw) > domain.MaxTrackingMetadataBytes {
		return nil, domain.Validation(domain.ValidationRequest{Path: "tracking", Reason: "too_large"})
	}
	if err := domain.ValidateDTO(domain.ValidateDTORequest{Bytes: raw, Kind: "tracking_metadata"}); err != nil {
		var typed *domain.Error
		if errors.As(err, &typed) && strings.HasPrefix(typed.Message, "validation:tracking:") {
			return nil, err
		}
		return nil, domain.Validation(domain.ValidationRequest{Path: "tracking", Reason: "invalid"})
	}
	var dto domain.ThreadMetadata
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&dto); err != nil {
		return nil, domain.Validation(domain.ValidationRequest{Path: "tracking", Reason: "invalid"})
	}
	if err := domain.ValidateThreadTracking(domain.ValidateThreadTrackingRequest{Location: r.Location, Tracking: dto.Tracking}); err != nil {
		return nil, err
	}
	if r.Mode != "format" && string(raw) != trackingMetadata(trackingMetadataRequest{Tracking: dto.Tracking}) {
		return nil, invalid(invalidRequest{})
	}
	return dto.Tracking, nil
}
