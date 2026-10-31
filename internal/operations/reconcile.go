package operations

import (
	"encoding/json"
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type ReconcileRequest struct {
	Document domain.DocumentValue
	Source   *domain.SourceValue
}
type ReconcileResponse struct {
	Document domain.DocumentValue
	Outcomes []domain.TrackingOutcome
}

// Every byte comparison in search (including KMP preprocessing and context
// verification) consumes one unit. Failed comparisons count too. Coordinate
// lookups use SourceValue's indexed API, never another source scan.
type syncMeter struct {
	thread, global int
	exhausted      bool
}

type syncWorkRequest struct{ Bytes int }
type syncEqualRequest struct{ Left, Right string }
type syncSearchRequest struct{ Text, Pattern string }
type syncTextRequest struct {
	Text              string
	ObserveInspection func(domain.SourceByteInspectionRequest)
}
type syncReadRequest struct {
	syncTextRequest
	Offset int
}
type syncLocationRequest struct {
	Source      *domain.SourceValue
	Kind        string
	Span        syncSpan
	InspectByte func(domain.SourceByteInspectionRequest) bool
}
type syncSupportRequest struct {
	Text     string
	Tracking *domain.TrackingRecord
	Span     syncSpan
}
type syncAttachmentRequest struct {
	Source   *domain.SourceValue
	Location domain.Location
	Tracking *domain.TrackingRecord
}
type syncMatchRequest struct {
	Source   *domain.SourceValue
	Tracking *domain.TrackingRecord
	Meter    *syncMeter
}

func (m *syncMeter) beginThread(r domain.ValueRequest) {
	m.thread = 0
	m.exhausted = m.global >= domain.MaxSyncWorkBytes
}
func (m *syncMeter) inspectByte(r domain.SourceByteInspectionRequest) bool {
	return m.spend(syncWorkRequest{Bytes: 1})
}
func (m *syncMeter) spend(r syncWorkRequest) bool {
	if m.exhausted {
		return false
	}
	n := r.Bytes
	if n > domain.MaxSourceBytes-m.thread || n > domain.MaxSyncWorkBytes-m.global {
		m.exhausted = true
		return false
	}
	m.thread += n
	m.global += n
	return true
}
func (m *syncMeter) equal(r syncEqualRequest) bool {
	a, b := r.Left, r.Right
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if !m.spend(syncWorkRequest{Bytes: 1}) || a[i] != b[i] {
			return false
		}
	}
	return true
}

// KMP avoids quadratic rescanning of repetitive source. All occurrences must
// be enumerated before any mapping can be accepted.
func (m *syncMeter) occurrences(r syncSearchRequest) []int {
	text, pattern := r.Text, r.Pattern
	if len(pattern) == 0 {
		panic("reconcile: empty search pattern")
	}
	table := make([]int, len(pattern))
	j := 0
	for i := 1; i < len(pattern); i++ {
		for {
			if !m.spend(syncWorkRequest{Bytes: 1}) {
				return nil
			}
			if pattern[i] == pattern[j] {
				j++
				break
			}
			if j == 0 {
				break
			}
			j = table[j-1]
		}
		table[i] = j
	}
	result := []int{}
	j = 0
	for i := 0; i < len(text); i++ {
		for {
			if !m.spend(syncWorkRequest{Bytes: 1}) {
				return nil
			}
			if text[i] == pattern[j] {
				j++
				break
			}
			if j == 0 {
				break
			}
			j = table[j-1]
		}
		if j == len(pattern) {
			if len(result) == domain.MaxApplyOperations {
				m.exhausted = true
				return nil
			}
			result = append(result, i+1-j)
			j = table[j-1]
		}
	}
	return result
}

type syncSpan struct{ start, end int }

func syncLocation(r syncLocationRequest) (domain.Location, bool) {
	s, kind, b := r.Source, r.Kind, r.Span
	if kind == domain.LocationLine {
		p, err := s.Position(domain.SourceOffsetRequest{Offset: b.start, InspectByte: r.InspectByte})
		if err != nil {
			return domain.Location{}, false
		}
		l := domain.Location{Kind: domain.LocationLine, Line: p.Line}
		bounds, err := s.Offsets(domain.SourceLocationRequest{Location: l, InspectByte: r.InspectByte})
		return l, err == nil && bounds.Start == b.start && bounds.End == b.end
	}
	l, err := s.Location(domain.SourceOffsetsRequest{Start: b.start, End: b.end, InspectByte: r.InspectByte})
	return l, err == nil
}
func (m *syncMeter) readByte(r syncReadRequest) (byte, bool) {
	if !m.spend(syncWorkRequest{Bytes: 1}) {
		return 0, false
	}
	if r.ObserveInspection != nil {
		r.ObserveInspection(domain.SourceByteInspectionRequest{Offset: r.Offset})
	}
	return r.Text[r.Offset], true
}

func (m *syncMeter) strong(request syncTextRequest) bool {
	if m.exhausted || len(request.Text) < domain.MinMatchContextBytes {
		return false
	}
	for offset := 0; offset < len(request.Text); {
		var captured [utf8.UTFMax]byte
		first, ok := m.readByte(syncReadRequest{syncTextRequest: request, Offset: offset})
		if !ok {
			return false
		}
		captured[0] = first
		width := 1
		switch {
		case first < utf8.RuneSelf:
		case first >= 0xc2 && first <= 0xdf:
			width = 2
		case first >= 0xe0 && first <= 0xef:
			width = 3
		case first >= 0xf0 && first <= 0xf4:
			width = utf8.UTFMax
		default:
			panic("reconcile: invalid UTF-8 context leading byte")
		}
		if width > len(request.Text)-offset {
			panic("reconcile: truncated UTF-8 context")
		}
		for i := 1; i < width; i++ {
			b, ok := m.readByte(syncReadRequest{syncTextRequest: request, Offset: offset + i})
			if !ok {
				return false
			}
			captured[i] = b
		}
		// Decode only the funded local copy, never the original context.
		r, decoded := utf8.DecodeRune(captured[:width])
		if decoded != width {
			panic("reconcile: invalid UTF-8 context encoding")
		}
		if !unicode.IsSpace(r) {
			return true
		}
		offset += width
	}
	return false
}
func (m *syncMeter) supported(r syncSupportRequest) bool {
	text, t, b := r.Text, r.Tracking, r.Span
	left := b.start >= len(t.Prefix) && len(t.Prefix) > 0 && m.equal(syncEqualRequest{Left: text[b.start-len(t.Prefix) : b.start], Right: t.Prefix})
	right := b.end+len(t.Suffix) <= len(text) && len(t.Suffix) > 0 && m.equal(syncEqualRequest{Left: text[b.end : b.end+len(t.Suffix)], Right: t.Suffix})
	return left || right || t.PrefixAtStart && len(t.Prefix) == 0 && b.start == 0 || t.SuffixAtEnd && len(t.Suffix) == 0 && b.end == len(text)
}

// validAttachment checks the longest rune-aligned windows without capturing or
// rescanning them through an unmetered string operation.
func (m *syncMeter) validAttachment(r syncAttachmentRequest) bool {
	if m.exhausted {
		return false
	}
	s, loc, t := r.Source, r.Location, r.Tracking
	b, err := s.Offsets(domain.SourceLocationRequest{Location: loc, InspectByte: m.inspectByte})
	if err != nil {
		return false
	}
	text := s.Text(domain.ValueRequest{})
	start := b.Start - domain.MaxContextBytes
	if start < 0 {
		start = 0
	}
	for start < b.Start {
		if !m.spend(syncWorkRequest{Bytes: 1}) {
			return false
		}
		if utf8.RuneStart(text[start]) {
			break
		}
		start++
	}
	end := b.End + domain.MaxContextBytes
	if end > len(text) {
		end = len(text)
	}
	for end < len(text) && end > b.End {
		if !m.spend(syncWorkRequest{Bytes: 1}) {
			return false
		}
		if utf8.RuneStart(text[end]) {
			break
		}
		end--
	}
	return t.PrefixAtStart == (start == 0) && t.SuffixAtEnd == (end == len(text)) &&
		m.equal(syncEqualRequest{Left: text[b.Start:b.End], Right: t.SelectedText}) && m.equal(syncEqualRequest{Left: text[start:b.Start], Right: t.Prefix}) && m.equal(syncEqualRequest{Left: text[b.End:end], Right: t.Suffix})
}

func Reconcile(r ReconcileRequest) (ReconcileResponse, error) {
	if err := domain.ValidateDocumentTracking(domain.ValidateDocumentTrackingRequest{Threads: r.Document.Threads(domain.DocumentThreadsRequest{})}); err != nil {
		return ReconcileResponse{}, err
	}
	threads := r.Document.Entries(domain.DocumentEntriesRequest{})
	order := make([]int, len(threads))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return threads[order[i]].ID(domain.ValueRequest{}).Text(domain.IDTextRequest{}) < threads[order[j]].ID(domain.ValueRequest{}).Text(domain.IDTextRequest{})
	})
	response := ReconcileResponse{Outcomes: make([]domain.TrackingOutcome, 0, len(threads))}
	meter := syncMeter{}
	for _, i := range order {
		thread := threads[i]
		loc := thread.Location(domain.ValueRequest{})
		record := thread.Tracking(domain.ValueRequest{})
		out := domain.TrackingOutcome{ThreadID: thread.ID(domain.ValueRequest{}).Text(domain.IDTextRequest{}), Location: loc, Evidence: domain.EvidenceNone}
		if record == nil {
			out.Outcome = domain.OutcomeSkipped
			out.Reason = loc.Kind
			response.Outcomes = append(response.Outcomes, out)
			continue
		}
		if r.Source == nil {
			return ReconcileResponse{}, fmt.Errorf("reconcile: source required for tracked threads")
		}
		meter.beginThread(domain.ValueRequest{})
		// Fingerprints alone never establish an attachment. Compare the current
		// selection and both complete retained windows at the active coordinates.
		if !meter.exhausted && loc.Kind != domain.LocationUnresolved && record.SourceRevision == r.Source.Fingerprint(domain.ValueRequest{}) && meter.validAttachment(syncAttachmentRequest{Source: r.Source, Location: loc, Tracking: record}) {
			out.Outcome = domain.OutcomeUnchanged
			out.Reason = domain.ReasonSourceUnchanged
			out.Evidence = domain.EvidenceUnchanged
			response.Outcomes = append(response.Outcomes, out)
			continue
		}
		span, reason, evidence := matchTracking(syncMatchRequest{Source: r.Source, Tracking: record, Meter: &meter})
		if meter.exhausted {
			reason = domain.ReasonSearchLimit
			evidence = domain.EvidenceNone
		}
		nextRecord := record
		nextLoc := domain.Location{Kind: domain.LocationUnresolved}
		if evidence != domain.EvidenceNone {
			var ok bool
			nextLoc, ok = syncLocation(syncLocationRequest{Source: r.Source, Kind: record.LastLocation.Kind, Span: span})
			if !ok {
				return ReconcileResponse{}, fmt.Errorf("reconcile: accepted invalid mapping")
			}
			// Search and all matching comparisons are finished before capture.
			// Capture/structural validation are not matching passes and cannot
			// establish uniqueness or consume an estimated search allowance.
			captured, err := r.Source.Capture(domain.SourceLocationRequest{Location: nextLoc})
			if err != nil {
				return ReconcileResponse{}, err
			}
			nextRecord = &captured
		}
		out.Location = nextLoc
		out.Reason = reason
		out.Evidence = evidence
		out.Outcome = domain.OutcomeUnresolved
		if evidence != domain.EvidenceNone {
			out.Outcome = domain.OutcomeUnchanged
			if nextLoc != loc {
				out.Outcome = domain.OutcomeMoved
			}
			if evidence == domain.EvidenceInferred {
				out.Outcome = domain.OutcomeModified
			}
		}
		updated, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: thread.ID(domain.ValueRequest{}), Location: nextLoc, Messages: thread.Messages(domain.ValueRequest{}), Tracking: nextRecord})
		if err != nil {
			return ReconcileResponse{}, err
		}
		threads[i] = updated
		response.Outcomes = append(response.Outcomes, out)
	}
	dtos := make([]domain.Thread, len(threads))
	for i, t := range threads {
		dtos[i] = t.DTO(domain.ValueRequest{})
	}
	if err := domain.ValidateDocumentTracking(domain.ValidateDocumentTrackingRequest{Threads: dtos}); err != nil {
		details, e := json.Marshal(PreconditionDetails{Operation: "sync", Reason: "limit_tracking"})
		if e != nil {
			return ReconcileResponse{}, e
		}
		return ReconcileResponse{}, &domain.Error{Code: domain.CodePrecondition, Message: "precondition:sync:limit_tracking", Details: details, Exit: domain.ExitPrecondition}
	}
	doc, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: threads})
	if err != nil {
		return ReconcileResponse{}, err
	}
	response.Document = doc
	return response, nil
}

func matchTracking(r syncMatchRequest) (syncSpan, string, string) {
	s, t, m := r.Source, r.Tracking, r.Meter
	if m.exhausted || m.global >= domain.MaxSyncWorkBytes {
		m.exhausted = true
		return syncSpan{}, domain.ReasonSearchLimit, domain.EvidenceNone
	}
	text := s.Text(domain.ValueRequest{})
	raw := []int{}
	if t.SelectedText != "" {
		raw = m.occurrences(syncSearchRequest{Text: text, Pattern: t.SelectedText})
	} else {
		if len(text) == 0 {
			raw = append(raw, 0)
		}
		atLineStart := true
		for i := 0; i < len(text); i++ {
			if !m.spend(syncWorkRequest{Bytes: 1}) {
				break
			}
			newline := text[i] == '\n'
			if atLineStart && newline {
				if len(raw) == domain.MaxApplyOperations {
					m.exhausted = true
					break
				}
				raw = append(raw, i)
			}
			atLineStart = newline
		}
	}
	exact := []syncSpan{}
	unsupported := false
	insufficient := false
	for _, start := range raw {
		b := syncSpan{start, start + len(t.SelectedText)}
		if m.supported(syncSupportRequest{Text: text, Tracking: t, Span: b}) {
			if _, ok := syncLocation(syncLocationRequest{Source: s, Kind: t.LastLocation.Kind, Span: b, InspectByte: m.inspectByte}); ok {
				exact = append(exact, b)
			} else {
				unsupported = true
			}
		} else {
			insufficient = true
		}
	}
	strong := m.strong(syncTextRequest{Text: t.Prefix}) && m.strong(syncTextRequest{Text: t.Suffix})
	envelopes := []syncSpan{}
	expansion := false
	if strong && !m.exhausted {
		left := m.occurrences(syncSearchRequest{Text: text, Pattern: t.Prefix})
		right := m.occurrences(syncSearchRequest{Text: text, Pattern: t.Suffix})
		for _, a := range left {
			for _, z := range right {
				b := syncSpan{a + len(t.Prefix), z}
				if b.end <= b.start {
					continue
				}
				eligible := len(raw) == 0
				if !eligible && b.end-b.start > len(t.SelectedText) && t.SelectedText != "" {
					eligible = m.equal(syncEqualRequest{Left: text[b.start : b.start+len(t.SelectedText)], Right: t.SelectedText}) || m.equal(syncEqualRequest{Left: text[b.end-len(t.SelectedText) : b.end], Right: t.SelectedText})
				}
				if eligible {
					if len(envelopes) == domain.MaxApplyOperations {
						m.exhausted = true
						break
					}
					envelopes = append(envelopes, b)
					if len(raw) > 0 {
						expansion = true
					}
				}
			}
		}
	}
	// Weak windows can explain why a potential replacement is unsupported,
	// but can never authorize attaching that replacement. Enumerate them with
	// the same caps rather than using an unmetered Contains/Index fallback.
	if !strong && len(raw) == 0 && !m.exhausted && len(t.Prefix) > 0 && len(t.Suffix) > 0 {
		left := m.occurrences(syncSearchRequest{Text: text, Pattern: t.Prefix})
		right := m.occurrences(syncSearchRequest{Text: text, Pattern: t.Suffix})
		for _, a := range left {
			for _, z := range right {
				if a+len(t.Prefix) < z {
					insufficient = true
				}
			}
		}
	}
	if m.exhausted {
		return syncSpan{}, domain.ReasonSearchLimit, domain.EvidenceNone
	}
	if len(envelopes) > 1 || len(exact) > 1 {
		return syncSpan{}, domain.ReasonAmbiguous, domain.EvidenceNone
	}
	if len(envelopes) == 1 {
		b := envelopes[0]
		// A competing supported exact mapping outside the expansion is ambiguous.
		if expansion && len(exact) == 1 && (exact[0].start < b.start || exact[0].end > b.end) {
			return syncSpan{}, domain.ReasonAmbiguous, domain.EvidenceNone
		}
		if b.end-b.start > domain.MaxAnchorBytes {
			return syncSpan{}, domain.ReasonSelectionTooLarge, domain.EvidenceNone
		}
		if _, ok := syncLocation(syncLocationRequest{Source: s, Kind: t.LastLocation.Kind, Span: b, InspectByte: m.inspectByte}); !ok {
			if m.exhausted {
				return syncSpan{}, domain.ReasonSearchLimit, domain.EvidenceNone
			}
			return syncSpan{}, domain.ReasonUnsupportedTransformation, domain.EvidenceNone
		}
		reason := domain.ReasonContextReplacement
		if expansion {
			reason = domain.ReasonBoundaryExpansion
		}
		return b, reason, domain.EvidenceInferred
	}
	if len(exact) == 1 {
		return exact[0], domain.ReasonExactMatch, domain.EvidenceExact
	}
	if unsupported {
		return syncSpan{}, domain.ReasonUnsupportedTransformation, domain.EvidenceNone
	}
	if insufficient || len(raw) > 0 {
		return syncSpan{}, domain.ReasonInsufficientContext, domain.EvidenceNone
	}
	return syncSpan{}, domain.ReasonNoMatch, domain.EvidenceNone
}
