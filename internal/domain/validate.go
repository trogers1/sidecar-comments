package domain

import (
	"encoding/base64"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type ValidateIDRequest struct{ ID string }

func ValidateID(r ValidateIDRequest) error {
	if !idPattern.MatchString(r.ID) {
		return invalid(invalidRequest{})
	}
	return nil
}

type invalidRequest struct{}

func invalid(r invalidRequest) error {
	return Validation(ValidationRequest{Path: "target_content", Reason: "invalid"})
}

type ParseLocationRequest struct{ Text string }

var locationPattern = regexp.MustCompile(`^range:([1-9][0-9]*):([1-9][0-9]*)-([1-9][0-9]*):([1-9][0-9]*)(?: \| ([A-Za-z0-9+/]*={0,2}))?$`)

func ParseLocation(r ParseLocationRequest) (Location, error) {
	fail := Validation(ValidationRequest{Path: "target_content", Reason: "location"})
	if r.Text == LocationFile || r.Text == LocationLost {
		return Location{Kind: r.Text}, nil
	}
	if strings.HasPrefix(r.Text, "line:") {
		s := strings.TrimPrefix(r.Text, "line:")
		n, e := strconv.Atoi(s)
		if e == nil && n > 0 && strconv.Itoa(n) == s {
			return Location{Kind: LocationLine, Line: n}, nil
		}
		return Location{}, fail
	}
	m := locationPattern.FindStringSubmatch(r.Text)
	if m == nil {
		return Location{}, fail
	}
	v := make([]int, 4)
	for i := range v {
		n, e := strconv.Atoi(m[i+1])
		if e != nil {
			return Location{}, fail
		}
		v[i] = n
	}
	l := Location{Kind: LocationRange, StartLine: v[0], StartColumn: v[1], EndLine: v[2], EndColumn: v[3]}
	if l.StartLine > l.EndLine || (l.StartLine == l.EndLine && l.StartColumn >= l.EndColumn) {
		return Location{}, fail
	}
	if strings.Contains(r.Text, " | ") {
		a := m[5]
		b, e := base64.StdEncoding.Strict().DecodeString(a)
		if e != nil || len(b) == 0 || len(b) > MaxAnchorBytes || !utf8.Valid(b) || base64.StdEncoding.EncodeToString(b) != a {
			return Location{}, fail
		}
		l.Anchor = &a
	}
	return l, nil
}

type LocationTextRequest struct{ Location Location }

func LocationText(r LocationTextRequest) string {
	l := r.Location
	switch l.Kind {
	case LocationFile, LocationLost:
		return l.Kind
	case LocationLine:
		return fmt.Sprintf("line:%d", l.Line)
	case LocationRange:
		s := fmt.Sprintf("range:%d:%d-%d:%d", l.StartLine, l.StartColumn, l.EndLine, l.EndColumn)
		if l.Anchor != nil {
			s += " | " + *l.Anchor
		}
		return s
	default:
		panic("invalid location")
	}
}

type ValidateBodyRequest struct{ Body string }

func ValidateBody(r ValidateBodyRequest) error {
	b := r.Body
	if !utf8.ValidString(b) || len(b) > MaxBodyBytes || strings.ContainsAny(b, "\r\x00") {
		return invalid(invalidRequest{})
	}
	nonwhite := false
	for _, c := range b {
		if !white(whiteRequest{Scalar: c}) {
			nonwhite = true
		}
	}
	if !nonwhite {
		return invalid(invalidRequest{})
	}
	for _, line := range strings.Split(b, "\n") {
		if ReservedLine(ReservedLineRequest{Text: line}) {
			return invalid(invalidRequest{})
		}
	}
	return nil
}

type whiteRequest struct{ Scalar rune }

func white(r whiteRequest) bool {
	c := r.Scalar
	return c >= 9 && c <= 13 || c == 32 || c == 0x85 || c == 0xa0 || c == 0x1680 || c >= 0x2000 && c <= 0x200a || c == 0x2028 || c == 0x2029 || c == 0x202f || c == 0x205f || c == 0x3000
}

type textRequest struct {
	Text string
	Max  int
	NFC  bool
}

func validText(r textRequest) bool {
	if !utf8.ValidString(r.Text) || utf8.RuneCountInString(r.Text) < 1 || utf8.RuneCountInString(r.Text) > r.Max || r.NFC && !norm.NFC.IsNormalString(r.Text) {
		return false
	}
	for _, c := range r.Text {
		if c < 32 || c >= 127 && c <= 159 {
			return false
		}
	}
	return true
}

type ValidateAuthorRequest struct{ Author Author }

func ValidateAuthor(r ValidateAuthorRequest) error {
	a := r.Author
	switch a.Kind {
	case AuthorHuman, AuthorAgent:
		if a.ExternalAuthor != nil {
			return invalid(invalidRequest{})
		}
		if a.Name != nil && ValidateName(ValidateNameRequest{Text: *a.Name}) != nil {
			return invalid(invalidRequest{})
		}
	case AuthorExternal:
		if a.Name != nil || a.ExternalAuthor == nil {
			return invalid(invalidRequest{})
		}
		if ValidateIdentity(ValidateIdentityRequest{Text: a.ExternalAuthor.ID}) != nil || ValidateName(ValidateNameRequest{Text: a.ExternalAuthor.Name}) != nil {
			return invalid(invalidRequest{})
		}
	default:
		return invalid(invalidRequest{})
	}
	return nil
}

type ValidateGitLabRequest struct{ GitLab *GitLab }

func ValidateGitLab(r ValidateGitLabRequest) error {
	g := r.GitLab
	if g == nil {
		return nil
	}
	if g.ProjectID <= 0 || g.NoteID <= 0 {
		return invalid(invalidRequest{})
	}
	switch g.Kind {
	case GitLabNote:
		if g.DiscussionID == nil || ValidateIdentity(ValidateIdentityRequest{Text: *g.DiscussionID}) != nil {
			return invalid(invalidRequest{})
		}
	case GitLabSystemNote:
		if g.DiscussionID != nil {
			return invalid(invalidRequest{})
		}
	default:
		return invalid(invalidRequest{})
	}
	return nil
}

type ValidateLocationRequest struct{ Location Location }

// ValidateLocation checks the closed union before invoking its trusted formatter.
func ValidateLocation(r ValidateLocationRequest) error {
	l := r.Location
	fail := Validation(ValidationRequest{Path: "target_content", Reason: "location"})
	switch l.Kind {
	case LocationFile, LocationLost:
		if l.Line != 0 || l.StartLine != 0 || l.StartColumn != 0 || l.EndLine != 0 || l.EndColumn != 0 || l.Anchor != nil {
			return fail
		}
	case LocationLine:
		if l.StartLine != 0 || l.StartColumn != 0 || l.EndLine != 0 || l.EndColumn != 0 || l.Anchor != nil {
			return fail
		}
	case LocationRange:
		if l.Line != 0 {
			return fail
		}
	default:
		return fail
	}
	_, err := ParseLocation(ParseLocationRequest{Text: LocationText(LocationTextRequest{Location: l})})
	return err
}

type ValidateDocumentRequest struct{ Threads []Thread }

func ValidateDocument(r ValidateDocumentRequest) error {
	_, err := NewDocumentValue(NewDocumentValueRequest{Threads: r.Threads})
	return err
}
