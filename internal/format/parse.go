package format

import (
	"bytes"
	"encoding/json"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"strings"
	"unicode/utf8"
)

type ParseRequest struct {
	Bytes []byte
	Mode  string
}

type invalidRequest struct{}

func invalid(r invalidRequest) error {
	return domain.Validation(domain.ValidationRequest{Path: "target_content", Reason: "invalid"})
}

type roleRequest struct {
	Text string
	Mode string
}
type roleResult struct {
	Message domain.Message
	Missing bool
}

func role(r roleRequest) (roleResult, error) {
	var out roleResult
	kind := ""
	prefix := ""
	for _, name := range []string{domain.RoleUser, domain.RoleBot, domain.RoleExternal} {
		candidate := "---" + name
		if strings.HasPrefix(r.Text, candidate) {
			prefix = candidate
			kind = domain.RoleAuthor(domain.RoleAuthorRequest{Role: name})
			break
		}
	}
	if prefix == "" {
		return out, invalid(invalidRequest{})
	}
	out.Message.Author.Kind = kind
	if r.Text == prefix {
		if kind == domain.AuthorExternal || r.Mode != "repair" {
			return out, invalid(invalidRequest{})
		}
		out.Missing = true
		return out, nil
	}
	start := prefix + " <!-- sidecar-meta: "
	if !strings.HasPrefix(r.Text, start) || !strings.HasSuffix(r.Text, " -->") {
		return out, invalid(invalidRequest{})
	}
	raw := []byte(strings.TrimSuffix(strings.TrimPrefix(r.Text, start), " -->"))
	if len(raw) > domain.MaxMetadataBytes || domain.ValidateDTO(domain.ValidateDTORequest{Bytes: raw, Kind: "metadata"}) != nil {
		return out, invalid(invalidRequest{})
	}
	var dto domain.MessageMetadata
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&dto) != nil {
		return out, invalid(invalidRequest{})
	}
	out.Message.ID = dto.ID
	out.Message.Author.Name = dto.Name
	out.Message.Author.ExternalAuthor = dto.ExternalAuthor
	out.Message.GitLab = dto.GitLab
	if domain.ValidateID(domain.ValidateIDRequest{ID: dto.ID}) != nil || domain.ValidateAuthor(domain.ValidateAuthorRequest{Author: out.Message.Author}) != nil || domain.ValidateGitLab(domain.ValidateGitLabRequest{GitLab: dto.GitLab}) != nil {
		return out, invalid(invalidRequest{})
	}
	if r.Mode != "format" && string(raw) != metadata(metadataRequest{Message: out.Message}) {
		return out, invalid(invalidRequest{})
	}
	return out, nil
}

type reservedRequest struct{ Text string }

func reserved(r reservedRequest) bool {
	return domain.ReservedLine(domain.ReservedLineRequest{Text: r.Text})
}
func Parse(r ParseRequest) (Document, error) {
	ast, err := parseDocument(r)
	out := Document{}
	if err == nil {
		if len(ast.Diagnostics) > 0 {
			out.Repair = &ast
			return out, nil
		}
		value, validationErr := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: ast.Threads})
		if validationErr != nil {
			return out, validationErr
		}
		out.Value = value
		return out, nil
	}
	if r.Mode == "repair" || (r.Mode != "normal" && r.Mode != "format") {
		return out, err
	}
	repaired, repairErr := parseDocument(ParseRequest{Bytes: r.Bytes, Mode: "repair"})
	if repairErr == nil && len(repaired.Diagnostics) > 0 {
		details, marshalErr := json.Marshal(domain.RepairDetails{Diagnostics: repaired.Diagnostics})
		if marshalErr != nil {
			return out, marshalErr
		}
		return out, &domain.Error{Code: domain.CodeRepair, Message: domain.CodeRepair + ":document", Details: details, Exit: domain.ExitValidation}
	}
	return out, err
}

func parseDocument(r ParseRequest) (RepairDocument, error) {
	out := RepairDocument{Threads: []domain.Thread{}, Diagnostics: []domain.Diagnostic{}}
	if r.Mode != "normal" && r.Mode != "format" && r.Mode != "repair" {
		return out, invalid(invalidRequest{})
	}
	if len(r.Bytes) > domain.MaxTargetBytes {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_size", Reason: "too_large"})
	}
	if len(r.Bytes) == 0 {
		return out, nil
	}
	s := string(r.Bytes)
	if strings.HasPrefix(s, "\ufeff") {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "bom"})
	}
	if strings.ContainsRune(s, 0) {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "nul"})
	}
	if !utf8.ValidString(s) {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "utf8"})
	}
	if strings.Contains(s, "\r") {
		if r.Mode != "format" || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\r") || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
			return out, domain.Validation(domain.ValidationRequest{Path: "target_endings", Reason: "endings"})
		}
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	if r.Mode == "format" {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		} else if strings.HasSuffix(s, "\n\n") {
			s = strings.TrimSuffix(s, "\n")
		}
	}
	if !strings.HasSuffix(s, "\n") {
		return out, invalid(invalidRequest{})
	}
	lines := strings.Split(s, "\n")
	offsets := make([]int, len(lines))
	n := 1
	for i, line := range lines {
		offsets[i] = n
		n += len(line) + 1
	}
	i := 0
	seenT := map[string]bool{}
	seenM := map[string]bool{}
	total := 0
	for i < len(lines)-1 {
		line := lines[i]
		if !strings.HasPrefix(line, domain.ThreadOpenPrefix) || !strings.HasSuffix(line, "]") {
			return out, invalid(invalidRequest{})
		}
		content := strings.TrimSuffix(strings.TrimPrefix(line, domain.ThreadOpenPrefix), "]")
		id, loc, ok := strings.Cut(content, " ")
		if !ok || domain.ValidateID(domain.ValidateIDRequest{ID: id}) != nil || seenT[id] {
			return out, invalid(invalidRequest{})
		}
		seenT[id] = true
		l, e := domain.ParseLocation(domain.ParseLocationRequest{Text: loc})
		if e != nil {
			return out, e
		}
		t := domain.Thread{ID: id, Location: l, Messages: []domain.Message{}}
		i++
		end := domain.ThreadClosePrefix + id + "]"
		for {
			if i >= len(lines)-1 {
				return out, invalid(invalidRequest{})
			}
			roleLine := i
			rr, e := role(roleRequest{Text: lines[i], Mode: r.Mode})
			if e != nil {
				return out, e
			}
			i++
			bodyStart := i
			for i < len(lines)-1 && !reserved(reservedRequest{Text: lines[i]}) {
				i++
			}
			if i >= len(lines)-1 {
				return out, invalid(invalidRequest{})
			}
			isEnd := lines[i] == end
			if !isEnd {
				if _, e := role(roleRequest{Text: lines[i], Mode: r.Mode}); e != nil {
					return out, e
				}
			}
			bodyLines := lines[bodyStart:i]
			if len(bodyLines) == 0 {
				return out, invalid(invalidRequest{})
			}
			if !isEnd {
				if bodyLines[len(bodyLines)-1] == "" {
					bodyLines = bodyLines[:len(bodyLines)-1]
				} else if r.Mode != "format" {
					return out, invalid(invalidRequest{})
				}
			}
			body := strings.Join(bodyLines, "\n")
			if e := domain.ValidateBody(domain.ValidateBodyRequest{Body: body}); e != nil {
				return out, e
			}
			rr.Message.Body = body
			if rr.Missing {
				out.Diagnostics = append(out.Diagnostics, domain.Diagnostic{Code: "missing_message_id", ByteOffset: offsets[roleLine], Line: roleLine + 1, ThreadID: id, MessageOrdinal: len(t.Messages) + 1})
			} else {
				if seenM[rr.Message.ID] {
					return out, invalid(invalidRequest{})
				}
				seenM[rr.Message.ID] = true
			}
			t.Messages = append(t.Messages, rr.Message)
			total++
			if len(t.Messages) > domain.MaxMessagesThread || total > domain.MaxMessagesDocument {
				return out, invalid(invalidRequest{})
			}
			if isEnd {
				i++
				break
			}
		}
		if len(out.Threads) > 0 && r.Mode != "format" && less(lessRequest{A: t, B: out.Threads[len(out.Threads)-1]}) {
			return out, invalid(invalidRequest{})
		}
		out.Threads = append(out.Threads, t)
		if len(out.Threads) > domain.MaxThreads {
			return out, invalid(invalidRequest{})
		}
		if i < len(lines)-1 {
			if lines[i] == "" {
				i++
			} else if r.Mode != "format" {
				return out, invalid(invalidRequest{})
			}
		}
	}
	if len(out.Threads) == 0 {
		return out, invalid(invalidRequest{})
	}
	if r.Mode != "format" {
		var canonical string
		if r.Mode == "repair" {
			canonical = string(encodeRepair(encodeRepairRequest{Document: out}))
		} else {
			value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: out.Threads})
			if err != nil {
				return out, err
			}
			canonical = string(Encode(EncodeRequest{Document: value}))
		}
		if r.Mode == "repair" {
			// Only missing IDs are repairable; all remaining bytes must be canonical.
			canonical = strings.ReplaceAll(canonical, `---`+domain.RoleUser+` <!-- sidecar-meta: {"id":""} -->`, "---"+domain.RoleUser)
			canonical = strings.ReplaceAll(canonical, `---`+domain.RoleBot+` <!-- sidecar-meta: {"id":""} -->`, "---"+domain.RoleBot)
		}
		if canonical != string(r.Bytes) {
			return out, invalid(invalidRequest{})
		}
	}
	return out, nil
}
