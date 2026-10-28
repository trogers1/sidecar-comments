package domain

import (
	"bytes"
	"encoding/json"
	"strconv"
)

type ValidateDTORequest struct {
	Bytes []byte
	Kind  string
}
type dtoField struct {
	Name, Kind string
	Required   bool
}
type dtoObjectRequest struct {
	Bytes  []byte
	Fields []dtoField
}
type dtoValueRequest struct {
	Bytes []byte
	Kind  string
}

// ValidateDTO checks lexical JSON and exact recursive wire shape before concrete decoding.
func ValidateDTO(r ValidateDTORequest) error {
	if err := ValidateJSON(ValidateJSONRequest{Bytes: r.Bytes}); err != nil {
		return err
	}
	if err := dtoValue(dtoValueRequest{Bytes: bytes.TrimSpace(r.Bytes), Kind: r.Kind}); err != nil {
		return invalid(invalidRequest{})
	}
	return nil
}
func dtoObject(r dtoObjectRequest) error {
	var fields map[string]json.RawMessage
	if len(r.Bytes) == 0 || r.Bytes[0] != '{' || json.Unmarshal(r.Bytes, &fields) != nil || fields == nil {
		return invalid(invalidRequest{})
	}
	for key := range fields {
		found := false
		for _, f := range r.Fields {
			if f.Name == key {
				found = true
				break
			}
		}
		if !found {
			return invalid(invalidRequest{})
		}
	}
	for _, f := range r.Fields {
		v, ok := fields[f.Name]
		if !ok {
			if f.Required {
				return invalid(invalidRequest{})
			}
			continue
		}
		if err := dtoValue(dtoValueRequest{Bytes: bytes.TrimSpace(v), Kind: f.Kind}); err != nil {
			return err
		}
	}
	return nil
}
func dtoTag(r dtoValueRequest) (string, error) {
	var v struct {
		Kind string `json:"kind"`
		Op   string `json:"op"`
	}
	if json.Unmarshal(r.Bytes, &v) != nil {
		return "", invalid(invalidRequest{})
	}
	if r.Kind == "operation" {
		return v.Op, nil
	}
	return v.Kind, nil
}

func dtoValue(r dtoValueRequest) error {
	fail := invalid(invalidRequest{})
	if len(r.Bytes) == 0 || bytes.Equal(r.Bytes, []byte("null")) {
		return fail
	}
	var fields []dtoField
	switch r.Kind {
	case "string", "name", "identity", "id", "body", "revision", "author_kind", "anchor":
		var s string
		if r.Bytes[0] != '"' || json.Unmarshal(r.Bytes, &s) != nil {
			return fail
		}
		switch r.Kind {
		case "string":
			return nil
		case "name":
			return ValidateName(ValidateNameRequest{Text: s})
		case "identity":
			return ValidateIdentity(ValidateIdentityRequest{Text: s})
		case "id":
			return ValidateID(ValidateIDRequest{ID: s})
		case "body":
			return ValidateBody(ValidateBodyRequest{Body: s})
		case "revision":
			return ValidateRevision(ValidateRevisionRequest{Text: s})
		case "author_kind":
			return ValidateAuthorKind(ValidateAuthorKindRequest{Kind: s})
		case "anchor": // Decoded anchor checks are centralized in ParseLocation.
			_, err := ParseLocation(ParseLocationRequest{Text: "range:1:1-1:2 | " + s})
			return err
		}
		return nil
	case "positive":
		n, e := strconv.ParseInt(string(r.Bytes), 10, 64)
		if e != nil || n <= 0 {
			return fail
		}
		return nil
	case "author":
		tag, e := dtoTag(r)
		if e != nil {
			return e
		}
		switch tag {
		case AuthorHuman, AuthorAgent:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "name", Kind: "name", Required: false}}
		case AuthorExternal:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "external_author", Kind: "external_author", Required: true}}
		default:
			return fail
		}
	case "external_author":
		fields = []dtoField{{Name: "id", Kind: "identity", Required: true}, {Name: "name", Kind: "name", Required: true}}
	case "gitlab":
		tag, e := dtoTag(r)
		if e != nil {
			return e
		}
		switch tag {
		case GitLabNote:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "project_id", Kind: "positive", Required: true}, {Name: "discussion_id", Kind: "identity", Required: true}, {Name: "note_id", Kind: "positive", Required: true}}
		case GitLabSystemNote:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "project_id", Kind: "positive", Required: true}, {Name: "note_id", Kind: "positive", Required: true}}
		default:
			return fail
		}
	case "location":
		tag, e := dtoTag(r)
		if e != nil {
			return e
		}
		switch tag {
		case LocationFile, LocationLost:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}}
		case LocationLine:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "line", Kind: "positive", Required: true}}
		case LocationRange:
			fields = []dtoField{{Name: "kind", Kind: "string", Required: true}, {Name: "start_line", Kind: "positive", Required: true}, {Name: "start_column", Kind: "positive", Required: true}, {Name: "end_line", Kind: "positive", Required: true}, {Name: "end_column", Kind: "positive", Required: true}, {Name: "anchor", Kind: "anchor", Required: false}}
		default:
			return fail
		}
	case "metadata":
		fields = []dtoField{{Name: "id", Kind: "id", Required: true}, {Name: "name", Kind: "name", Required: false}, {Name: "external_author", Kind: "external_author", Required: false}, {Name: "gitlab", Kind: "gitlab", Required: false}}
	case "apply":
		fields = []dtoField{{Name: "protocol", Kind: "positive", Required: true}, {Name: "if_revision", Kind: "revision", Required: false}, {Name: "operations", Kind: "operations", Required: true}}
	case "operations":
		if r.Bytes[0] != '[' {
			return fail
		}
		var ops []json.RawMessage
		if json.Unmarshal(r.Bytes, &ops) != nil || len(ops) < 1 || len(ops) > MaxApplyOperations {
			return fail
		}
		for _, op := range ops {
			if e := dtoValue(dtoValueRequest{Bytes: bytes.TrimSpace(op), Kind: "operation"}); e != nil {
				return e
			}
		}
		return nil
	case "operation":
		tag, e := dtoTag(r)
		if e != nil {
			return e
		}
		fields = []dtoField{{Name: "op", Kind: "string", Required: true}, {Name: "thread_id", Kind: "id", Required: true}}
		switch tag {
		case OpThreadCreate:
			fields = append(fields, dtoField{Name: "message_id", Kind: "id", Required: true}, dtoField{Name: "location", Kind: "location", Required: true}, dtoField{Name: "author", Kind: "author", Required: true}, dtoField{Name: "body", Kind: "body", Required: true}, dtoField{Name: "gitlab", Kind: "gitlab", Required: false})
		case OpThreadMove:
			fields = append(fields, dtoField{Name: "location", Kind: "location", Required: true})
		case OpMessageAdd:
			fields = append(fields, dtoField{Name: "message_id", Kind: "id", Required: true}, dtoField{Name: "author", Kind: "author", Required: true}, dtoField{Name: "body", Kind: "body", Required: true}, dtoField{Name: "gitlab", Kind: "gitlab", Required: false}, dtoField{Name: "if_last_author_kind", Kind: "author_kind", Required: false})
		case OpMessageUpdate:
			fields = append(fields, dtoField{Name: "message_id", Kind: "id", Required: true}, dtoField{Name: "body", Kind: "body", Required: true})
		case OpThreadDelete:
		case OpMessageDelete:
			fields = append(fields, dtoField{Name: "message_id", Kind: "id", Required: true})
		default:
			return fail
		}
	default:
		return fail
	}
	if e := dtoObject(dtoObjectRequest{Bytes: r.Bytes, Fields: fields}); e != nil {
		return e
	}
	if r.Kind == "location" {
		var l Location
		if json.Unmarshal(r.Bytes, &l) != nil {
			return fail
		}
		return ValidateLocation(ValidateLocationRequest{Location: l})
	}
	if r.Kind == "author" {
		var a Author
		if json.Unmarshal(r.Bytes, &a) != nil {
			return fail
		}
		return ValidateAuthor(ValidateAuthorRequest{Author: a})
	}
	if r.Kind == "gitlab" {
		var g GitLab
		if json.Unmarshal(r.Bytes, &g) != nil {
			return fail
		}
		return ValidateGitLab(ValidateGitLabRequest{GitLab: &g})
	}
	if r.Kind == "metadata" {
		var m MessageMetadata
		if json.Unmarshal(r.Bytes, &m) != nil || m.Name != nil && m.ExternalAuthor != nil {
			return fail
		}
	}
	return nil
}
