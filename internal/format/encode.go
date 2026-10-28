package format

import (
	"encoding/base64"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"sort"
	"strconv"
	"strings"
)

type Document struct {
	Value  domain.DocumentValue
	Repair *RepairDocument
}
type RepairDocument struct {
	Threads     []domain.Thread
	Diagnostics []domain.Diagnostic
}
type EncodeRequest struct{ Document domain.DocumentValue }
type encodeThreadsRequest struct{ Threads []domain.Thread }
type quoteRequest struct{ Text string }

func quote(r quoteRequest) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range r.Text {
		switch c {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(c)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 32 {
				b.WriteString(`\u00`)
				const h = "0123456789abcdef"
				b.WriteByte(h[c>>4])
				b.WriteByte(h[c&15])
			} else {
				b.WriteRune(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

type metadataRequest struct{ Message domain.Message }

func metadata(r metadataRequest) string {
	m := r.Message
	s := `{"id":` + quote(quoteRequest{Text: m.ID})
	if m.Author.Name != nil {
		s += `,"name":` + quote(quoteRequest{Text: *m.Author.Name})
	}
	if a := m.Author.ExternalAuthor; a != nil {
		s += `,"external_author":{"id":` + quote(quoteRequest{Text: a.ID}) + `,"name":` + quote(quoteRequest{Text: a.Name}) + `}`
	}
	if g := m.GitLab; g != nil {
		s += `,"gitlab":{"kind":` + quote(quoteRequest{Text: g.Kind}) + `,"project_id":` + strconv.FormatInt(g.ProjectID, 10)
		if g.DiscussionID != nil {
			s += `,"discussion_id":` + quote(quoteRequest{Text: *g.DiscussionID})
		}
		s += `,"note_id":` + strconv.FormatInt(g.NoteID, 10) + `}`
	}
	return s + `}`
}

type lessRequest struct{ A, B domain.Thread }

func less(r lessRequest) bool {
	a, b := r.A.Location, r.B.Location
	ar := domain.LocationRank(domain.LocationRankRequest{Kind: a.Kind})
	br := domain.LocationRank(domain.LocationRankRequest{Kind: b.Kind})
	if ar != br {
		return ar < br
	}
	av := []int{a.Line, a.StartLine, a.StartColumn, a.EndLine, a.EndColumn}
	bv := []int{b.Line, b.StartLine, b.StartColumn, b.EndLine, b.EndColumn}
	for i := range av {
		if av[i] != bv[i] {
			return av[i] < bv[i]
		}
	}
	if (a.Anchor == nil) != (b.Anchor == nil) {
		return a.Anchor == nil
	}
	if a.Anchor != nil {
		aa, e := base64.StdEncoding.DecodeString(*a.Anchor)
		if e != nil {
			panic(e)
		}
		bb, e := base64.StdEncoding.DecodeString(*b.Anchor)
		if e != nil {
			panic(e)
		}
		if string(aa) != string(bb) {
			return string(aa) < string(bb)
		}
	}
	return r.A.ID < r.B.ID
}
func Encode(r EncodeRequest) []byte {
	return encodeThreads(encodeThreadsRequest{Threads: r.Document.Threads(domain.DocumentThreadsRequest{})})
}
func encodeThreads(r encodeThreadsRequest) []byte {
	ts := append([]domain.Thread(nil), r.Threads...)
	sort.Slice(ts, func(i, j int) bool { return less(lessRequest{A: ts[i], B: ts[j]}) })
	var b strings.Builder
	for i, t := range ts {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(domain.ThreadOpenPrefix + t.ID + " " + domain.LocationText(domain.LocationTextRequest{Location: t.Location}) + "]\n")
		for j, m := range t.Messages {
			if j > 0 {
				b.WriteByte('\n')
			}
			role := domain.AuthorRole(domain.AuthorRoleRequest{Kind: m.Author.Kind})
			b.WriteString("---" + role + " <!-- sidecar-meta: " + metadata(metadataRequest{Message: m}) + " -->\n" + m.Body + "\n")
		}
		b.WriteString(domain.ThreadClosePrefix + t.ID + "]\n")
	}
	return []byte(b.String())
}
