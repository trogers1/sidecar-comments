package format

import (
	"encoding/json"
	"errors"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type contentErrorRequest struct {
	Bytes               []byte
	Offset              int // zero based; len(Bytes) denotes EOF
	Error               error
	Code, Message, Hint string
}

// Add context without changing the existing error identity or repair protocol.
func contentError(r contentErrorRequest) error {
	var e *domain.Error
	if !errors.As(r.Error, &e) || e.Code != domain.CodeValidation {
		return r.Error
	}
	var details domain.ValidationDetails
	if err := json.Unmarshal(e.Details, &details); err != nil {
		return err
	}
	if len(details.Diagnostics) > 0 {
		d := details.Diagnostics[0]
		r.Code, r.Message, r.Hint = d.Code, d.Message, d.Hint
		r.Offset += d.ByteOffset - 1
	}
	offset := r.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(r.Bytes) {
		offset = len(r.Bytes)
	}
	line, column := 1, 1
	for _, b := range r.Bytes[:offset] {
		if b == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	details.Diagnostics = []domain.ContentDiagnostic{{Code: r.Code, Message: r.Message, Hint: r.Hint, Line: line, Column: column, ByteOffset: offset + 1}}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	return &domain.Error{Code: e.Code, Message: e.Message, Details: raw, Exit: e.Exit}
}
