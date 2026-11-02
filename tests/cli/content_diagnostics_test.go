package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type locatedValidationExpectation struct {
	Got      processResult
	Want     string
	Document []byte
}

// This specialized oracle allows only the new diagnostics extension, while
// retaining the original validation envelope and the strict process oracle.
func compareLocatedValidation(r locatedValidationExpectation) error {
	if err := compareProcess(processExpectation{Got: r.Got, Want: processResult{Status: domain.ExitValidation, Stderr: r.Got.Stderr}}); err != nil {
		return err
	}
	if err := validateSchema(schemaRequest{JSON: r.Got.Stderr}); err != nil {
		return err
	}
	type envelope struct {
		Protocol int `json:"protocol"`
		Error    struct {
			Code    string                   `json:"code"`
			Message string                   `json:"message"`
			Details domain.ValidationDetails `json:"details"`
		} `json:"error"`
	}
	var got, want envelope
	if err := json.Unmarshal(r.Got.Stderr, &got); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(r.Want), &want); err != nil {
		return err
	}
	diagnostics := got.Error.Details.Diagnostics
	got.Error.Details.Diagnostics = nil
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("validation envelope changed: got %+v; want %+v", got, want)
	}
	if len(diagnostics) != 1 {
		return fmt.Errorf("want one located diagnostic, got %d", len(diagnostics))
	}
	for _, d := range diagnostics {
		if strings.TrimSpace(d.Code) == "" || strings.TrimSpace(d.Message) == "" || strings.TrimSpace(d.Hint) == "" {
			return fmt.Errorf("diagnostic lacks substantive code, message, or hint: %+v", d)
		}
		if d.ByteOffset < 1 || d.ByteOffset > len(r.Document)+1 {
			return fmt.Errorf("diagnostic offset outside input: %+v", d)
		}
		prefix := r.Document[:d.ByteOffset-1]
		line := bytes.Count(prefix, []byte("\n")) + 1
		column := len(prefix) - bytes.LastIndexByte(prefix, '\n')
		if d.Line != line || d.Column != column {
			return fmt.Errorf("diagnostic coordinates %+v; want line %d column %d", d, line, column)
		}
	}
	return nil
}
