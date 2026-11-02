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

func role(r roleRequest) (out roleResult, err error) {
	context := contentErrorRequest{Bytes: []byte(r.Text), Code: "role_delimiter", Message: "The role delimiter is missing or unrecognized.", Hint: "Use ---user, ---bot, or ---external with sidecar-meta JSON."}
	defer func() {
		if err != nil {
			context.Error = err
			err = contentError(context)
		}
	}()
	r.Text = trimRoleWhitespace(roleWhitespaceRequest{Text: r.Text})
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
	context.Offset = len(prefix)
	context.Code, context.Message, context.Hint = "message_metadata_missing", "The role delimiter has no message metadata.", "Restore sidecar-meta JSON containing the message id; for user and bot messages, sidecar fix --add-missing-message-ids can add missing IDs."
	if r.Text == prefix {
		if kind == domain.AuthorExternal || r.Mode != "repair" {
			return out, invalid(invalidRequest{})
		}
		out.Missing = true
		return out, nil
	}
	context.Code, context.Message, context.Hint = "message_metadata_delimiter", "The message metadata comment is malformed.", "Follow the role with a space and <!-- sidecar-meta: JSON -->."
	start := prefix + " <!-- sidecar-meta: "
	if !strings.HasPrefix(r.Text, start) || !strings.HasSuffix(r.Text, " -->") {
		return out, invalid(invalidRequest{})
	}
	context.Offset = len(start)
	context.Code, context.Message, context.Hint = "message_metadata", "Message metadata must be a valid JSON object with a valid id and only supported fields.", "Correct the JSON and id; supported fields are id, name, external_author, and gitlab."
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
	context.Code, context.Message, context.Hint = "message_author", "Message metadata has invalid author or GitLab fields for this role.", "Use external_author only for external messages and provide the required author identity and GitLab fields."
	if domain.ValidateID(domain.ValidateIDRequest{ID: dto.ID}) != nil || domain.ValidateAuthor(domain.ValidateAuthorRequest{Author: out.Message.Author}) != nil || domain.ValidateGitLab(domain.ValidateGitLabRequest{GitLab: dto.GitLab}) != nil {
		return out, invalid(invalidRequest{})
	}
	context.Code, context.Message, context.Hint = "message_metadata_noncanonical", "The message metadata is not canonically formatted.", "Run sidecar format to normalize metadata field order and spacing."
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

func parseDocument(r ParseRequest) (out RepairDocument, err error) {
	out = RepairDocument{Threads: []domain.Thread{}, Diagnostics: []domain.Diagnostic{}}
	context := contentErrorRequest{Bytes: r.Bytes, Code: "invalid_content", Message: "The sidecar content is invalid.", Hint: "Restore the damaged content from version control."}
	defer func() {
		if err != nil {
			context.Error = err
			err = contentError(context)
		}
	}()
	rawOffsets := []int{0}
	for n, b := range r.Bytes {
		if b == '\n' {
			rawOffsets = append(rawOffsets, n+1)
		}
	}
	at := func(line int, code, message, hint string) {
		context.Offset = len(r.Bytes)
		if line < len(rawOffsets) {
			context.Offset = rawOffsets[line]
		}
		context.Code, context.Message, context.Hint = code, message, hint
	}
	if r.Mode != "normal" && r.Mode != "format" && r.Mode != "repair" {
		return out, invalid(invalidRequest{})
	}
	context.Code, context.Message, context.Hint = "target_size", "The sidecar exceeds the target size limit.", "Split the sidecar into smaller files or remove unneeded threads."
	context.Offset = domain.MaxTargetBytes
	if len(r.Bytes) > domain.MaxTargetBytes {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_size", Reason: "too_large"})
	}
	if len(r.Bytes) == 0 {
		return out, nil
	}
	s := string(r.Bytes)
	at(0, "encoding_bom", "The sidecar starts with a UTF-8 byte-order mark.", "Remove the byte-order mark and save as plain UTF-8.")
	if strings.HasPrefix(s, "\ufeff") {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "bom"})
	}
	context.Code, context.Message, context.Hint = "encoding_nul", "The sidecar contains a NUL byte.", "Remove the NUL byte and save as plain UTF-8."
	context.Offset = strings.IndexByte(s, 0)
	if strings.ContainsRune(s, 0) {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "nul"})
	}
	context.Code, context.Message, context.Hint = "encoding_utf8", "The sidecar contains invalid UTF-8.", "Replace the invalid byte sequence and save as UTF-8."
	if !utf8.ValidString(s) {
		for n := 0; n < len(s); {
			_, size := utf8.DecodeRuneInString(s[n:])
			if size == 1 && s[n] >= utf8.RuneSelf {
				context.Offset = n
				break
			}
			n += size
		}
		return out, domain.Validation(domain.ValidationRequest{Path: "target_encoding", Reason: "utf8"})
	}
	context.Code, context.Message, context.Hint = "line_endings", "The sidecar has unsupported line endings.", "Use LF line endings; sidecar format can convert consistent CRLF endings."
	context.Offset = strings.IndexByte(s, '\r')
	if strings.Contains(s, "\r") {
		if r.Mode != "format" || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\r") || strings.Contains(strings.ReplaceAll(s, "\r\n", ""), "\n") {
			return out, domain.Validation(domain.ValidationRequest{Path: "target_endings", Reason: "endings"})
		}
		s = strings.ReplaceAll(s, "\r\n", "\n")
	}
	s = trimTrailingPadding(trailingPaddingRequest{Text: s})
	if r.Mode == "format" {
		if !strings.HasSuffix(s, "\n") {
			s += "\n"
		} else if strings.HasSuffix(s, "\n\n") {
			s = strings.TrimSuffix(s, "\n")
		}
	}
	context.Offset = len(r.Bytes)
	context.Code, context.Message, context.Hint = "missing_final_newline", "The sidecar ends without a final newline.", "Add a newline after the final thread closing delimiter."
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
	at(0, "invalid_header", "The format header is missing or malformed.", "Restore the format header from a sidecar written by this version.")
	if lines[0] != formatHeader {
		if strings.HasPrefix(lines[0], domain.ThreadOpenPrefix) || unsupportedHeader(unsupportedHeaderRequest{Text: lines[0]}) {
			return out, domain.Unsupported(domain.UnsupportedRequest{Received: "format"})
		}
		return out, invalid(invalidRequest{})
	}
	i := 1
	seenT := map[string]bool{}
	seenM := map[string]bool{}
	total := 0
	trackingBytes := 0
	for i < len(lines)-1 {
		threadLine := i
		at(i, "thread_open", "Expected a thread opening delimiter.", "Use [@th_ID LOCATION] with a valid thread ID and location.")
		line := lines[i]
		if !strings.HasPrefix(line, domain.ThreadOpenPrefix) || !strings.HasSuffix(line, "]") {
			return out, invalid(invalidRequest{})
		}
		content := strings.TrimSuffix(strings.TrimPrefix(line, domain.ThreadOpenPrefix), "]")
		id, loc, ok := strings.Cut(content, " ")
		at(i, "thread_id", "The thread ID is invalid or duplicated, or its location is missing.", "Use a unique valid thread ID followed by a space and location.")
		if !ok || domain.ValidateID(domain.ValidateIDRequest{ID: id}) != nil || seenT[id] {
			return out, invalid(invalidRequest{})
		}
		seenT[id] = true
		at(i, "thread_location", "The thread location is invalid.", "Use a canonical file, line, range, or lost location.")
		context.Offset += len(domain.ThreadOpenPrefix) + len(id) + 1
		l, e := domain.ParseLocation(domain.ParseLocationRequest{Text: loc})
		if e != nil {
			return out, e
		}
		t := domain.Thread{ID: id, Location: l, Messages: []domain.Message{}}
		i++
		at(i, "missing_tracking", "Expected thread tracking metadata after the opening delimiter.", "Add the thread metadata line; use an empty metadata object when there is no tracking record.")
		if i >= len(lines)-1 {
			return out, invalid(invalidRequest{})
		}
		at(i, "tracking_metadata", "Thread tracking metadata is malformed, noncanonical, or inconsistent with the location.", "Restore the tracking line from version control, or correct its JSON fields and location; use sidecar format for canonical spacing.")
		t.Tracking, e = parseTracking(parseTrackingRequest{Text: lines[i], Mode: r.Mode, Location: l})
		if e != nil {
			return out, e
		}
		if t.Tracking != nil {
			trackingBytes += len(t.Tracking.SelectedText) + len(t.Tracking.Prefix) + len(t.Tracking.Suffix)
			if trackingBytes > domain.MaxTrackingBytes {
				context.Code, context.Message, context.Hint = "tracking_limit", "Tracking excerpts exceed the document size limit.", "Split this sidecar into smaller documents or remove unneeded threads."
				return out, domain.Validation(domain.ValidationRequest{Path: "tracking", Reason: "too_large"})
			}
		}
		i++
		end := domain.ThreadClosePrefix + id + "]"
		for {
			at(i, "unexpected_eof", "The thread ended before a message or closing delimiter was found.", "Add a message and the matching thread closing delimiter.")
			if i >= len(lines)-1 {
				return out, invalid(invalidRequest{})
			}
			roleLine := i
			at(i, "role_metadata", "Expected a valid role delimiter with message metadata.", "Use ---user, ---bot, or ---external followed by canonical sidecar-meta JSON with a valid unique id and role-appropriate author fields.")
			rr, e := role(roleRequest{Text: lines[i], Mode: r.Mode})
			if e != nil {
				return out, e
			}
			i++
			bodyStart := i
			for i < len(lines)-1 && !reserved(reservedRequest{Text: lines[i]}) {
				i++
			}
			at(i, "thread_close", "The message has no closing delimiter.", "Add the matching thread closing delimiter after the message body.")
			if i >= len(lines)-1 {
				return out, invalid(invalidRequest{})
			}
			isEnd := lines[i] == end
			if !isEnd {
				if strings.HasPrefix(lines[i], domain.ThreadClosePrefix) {
					at(i, "unexpected_delimiter", "The thread closing delimiter does not match the opening thread ID.", "Replace this delimiter with "+end+".")
					return out, invalid(invalidRequest{})
				}
				at(i, "unexpected_delimiter", "Expected another role delimiter or the matching thread closing delimiter.", "Correct the closing thread ID or replace this reserved line with a valid role delimiter.")
				if _, e := role(roleRequest{Text: lines[i], Mode: r.Mode}); e != nil {
					return out, e
				}
			}
			bodyLines := lines[bodyStart:i]
			at(bodyStart, "empty_body", "The message body is missing.", "Add a nonempty message body before the next delimiter.")
			if len(bodyLines) == 0 {
				return out, invalid(invalidRequest{})
			}
			if !isEnd {
				if bodyLines[len(bodyLines)-1] == "" {
					bodyLines = bodyLines[:len(bodyLines)-1]
				} else if r.Mode != "format" {
					at(i, "message_separator", "A blank line is required before the next message.", "Insert a blank line before this role delimiter, or run sidecar format.")
					return out, invalid(invalidRequest{})
				}
			}
			at(bodyStart, "message_body", "The message body is invalid or exceeds the size limit.", "Provide a nonempty body within the message size limit.")
			body := strings.Join(bodyLines, "\n")
			if e := domain.ValidateBody(domain.ValidateBodyRequest{Body: body}); e != nil {
				return out, e
			}
			rr.Message.Body = body
			if rr.Missing {
				out.Diagnostics = append(out.Diagnostics, domain.Diagnostic{Code: "missing_message_id", ByteOffset: offsets[roleLine], Line: roleLine + 1, ThreadID: id, MessageOrdinal: len(t.Messages) + 1})
			} else {
				at(roleLine, "duplicate_message_id", "This message ID is already used in the document.", "Replace it with a new unique message ID.")
				if seenM[rr.Message.ID] {
					return out, invalid(invalidRequest{})
				}
				seenM[rr.Message.ID] = true
			}
			t.Messages = append(t.Messages, rr.Message)
			total++
			at(roleLine, "message_limit", "The document or thread has too many messages.", "Split the comments into smaller documents or remove unneeded messages.")
			if len(t.Messages) > domain.MaxMessagesThread || total > domain.MaxMessagesDocument {
				return out, invalid(invalidRequest{})
			}
			if isEnd {
				i++
				break
			}
		}
		at(threadLine, "thread_order", "Threads are not in canonical location order.", "Run sidecar format to sort the threads.")
		if len(out.Threads) > 0 && r.Mode != "format" && less(lessRequest{A: t, B: out.Threads[len(out.Threads)-1]}) {
			return out, invalid(invalidRequest{})
		}
		out.Threads = append(out.Threads, t)
		at(threadLine, "thread_limit", "The sidecar has too many threads.", "Split the comments into smaller documents or remove unneeded threads.")
		if len(out.Threads) > domain.MaxThreads {
			return out, invalid(invalidRequest{})
		}
		if i < len(lines)-1 {
			if lines[i] == "" {
				i++
			} else if r.Mode != "format" {
				at(i, "thread_separator", "A blank line is required between threads.", "Insert a blank line before this thread, or run sidecar format.")
				return out, invalid(invalidRequest{})
			}
		}
	}
	at(1, "missing_thread", "The nonempty sidecar contains no threads.", "Add a complete thread or make the sidecar empty.")
	if len(out.Threads) == 0 {
		return out, invalid(invalidRequest{})
	}
	at(1, "document_tracking", "Tracking records in the document are inconsistent.", "Restore consistent tracking records from version control or correct the conflicting source revisions.")
	if err := domain.ValidateDocumentTracking(domain.ValidateDocumentTrackingRequest{Threads: out.Threads}); err != nil {
		return out, err
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
		comparisonLines := strings.Split(trimTrailingPadding(trailingPaddingRequest{Text: string(r.Bytes)}), "\n")
		for n := range comparisonLines {
			comparisonLines[n] = trimRoleWhitespace(roleWhitespaceRequest{Text: comparisonLines[n]})
		}
		comparison := strings.Join(comparisonLines, "\n")
		if canonical != comparison {
			context.Code, context.Message, context.Hint = "noncanonical_content", "The sidecar is not canonically formatted.", "Run sidecar format to normalize this document."
			context.Offset = 0
			for context.Offset < len(comparison) && context.Offset < len(canonical) && comparison[context.Offset] == canonical[context.Offset] {
				context.Offset++
			}
			prefix := comparison[:context.Offset]
			line := strings.Count(prefix, "\n")
			column := len(prefix) - strings.LastIndex(prefix, "\n") - 1
			at(line, context.Code, context.Message, context.Hint)
			context.Offset += column
			return out, invalid(invalidRequest{})
		}
	}
	return out, nil
}
