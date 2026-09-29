package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// scanJSON rejects duplicate object members without decoding untrusted values
// into an untyped representation. It is intentionally a lexical structural
// check; semantic DTO validation remains the executable's responsibility.
func decodeJSONCorpus(b []byte, out *corpus, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%s: trailing JSON", label)
	}
	return nil
}
func decodeJSONGolden(b []byte, out *golden, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%s: trailing JSON", label)
	}
	return nil
}

type scanner struct {
	b []byte
	i int
}

func uniqueJSON(b []byte) error {
	s := scanner{b: b}
	s.space()
	if e := s.value(); e != nil {
		return e
	}
	s.space()
	if s.i != len(b) {
		return errors.New("trailing JSON")
	}
	return nil
}
func (s *scanner) space() {
	for s.i < len(s.b) && bytes.ContainsRune([]byte(" \t\r\n"), rune(s.b[s.i])) {
		s.i++
	}
}
func (s *scanner) value() error {
	s.space()
	if s.i >= len(s.b) {
		return errors.New("short JSON")
	}
	switch s.b[s.i] {
	case '{':
		return s.object()
	case '[':
		return s.array()
	case '"':
		_, e := s.str()
		return e
	case 't':
		return s.word("true")
	case 'f':
		return s.word("false")
	case 'n':
		return s.word("null")
	default:
		if s.b[s.i] == '-' || (s.b[s.i] >= '0' && s.b[s.i] <= '9') {
			return s.number()
		}
		return errors.New("invalid JSON token")
	}
}
func (s *scanner) word(w string) error {
	if len(s.b)-s.i < len(w) || string(s.b[s.i:s.i+len(w)]) != w {
		return errors.New("invalid literal")
	}
	s.i += len(w)
	return nil
}
func (s *scanner) str() (string, error) {
	if s.i >= len(s.b) || s.b[s.i] != '"' {
		return "", errors.New("expected string")
	}
	start := s.i
	s.i++
	for s.i < len(s.b) {
		c := s.b[s.i]
		if c == '"' {
			s.i++
			var v string
			if e := json.Unmarshal(s.b[start:s.i], &v); e != nil {
				return "", e
			}
			return v, nil
		}
		if c == '\\' {
			s.i += 2
			if s.i > len(s.b) {
				break
			}
			continue
		}
		if c < 0x20 {
			return "", errors.New("control in string")
		}
		s.i++
	}
	return "", errors.New("unterminated string")
}
func (s *scanner) object() error {
	s.i++
	s.space()
	seen := make(map[string]bool)
	if s.i < len(s.b) && s.b[s.i] == '}' {
		s.i++
		return nil
	}
	for {
		s.space()
		k, e := s.str()
		if e != nil {
			return e
		}
		if seen[k] {
			return fmt.Errorf("duplicate JSON key %q", k)
		}
		seen[k] = true
		s.space()
		if s.i >= len(s.b) || s.b[s.i] != ':' {
			return errors.New("expected colon")
		}
		s.i++
		if e = s.value(); e != nil {
			return e
		}
		s.space()
		if s.i >= len(s.b) {
			return errors.New("unterminated object")
		}
		if s.b[s.i] == '}' {
			s.i++
			return nil
		}
		if s.b[s.i] != ',' {
			return errors.New("expected comma")
		}
		s.i++
	}
}
func (s *scanner) array() error {
	s.i++
	s.space()
	if s.i < len(s.b) && s.b[s.i] == ']' {
		s.i++
		return nil
	}
	for {
		if e := s.value(); e != nil {
			return e
		}
		s.space()
		if s.i >= len(s.b) {
			return errors.New("unterminated array")
		}
		if s.b[s.i] == ']' {
			s.i++
			return nil
		}
		if s.b[s.i] != ',' {
			return errors.New("expected comma")
		}
		s.i++
	}
}
func (s *scanner) number() error {
	start := s.i
	if s.b[s.i] == '-' {
		s.i++
	}
	if s.i >= len(s.b) {
		return errors.New("bad number")
	}
	if s.b[s.i] == '0' {
		s.i++
	} else {
		if s.b[s.i] < '1' || s.b[s.i] > '9' {
			return errors.New("bad number")
		}
		for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
			s.i++
		}
	}
	if s.i < len(s.b) && (s.b[s.i] == '.' || s.b[s.i] == 'e' || s.b[s.i] == 'E') {
		return errors.New("non-integer number")
	}
	if start == s.i {
		return errors.New("bad number")
	}
	return nil
}

type envelope struct {
	Protocol int             `json:"protocol"`
	Command  string          `json:"command"`
	Changed  *bool           `json:"changed"`
	Result   json.RawMessage `json:"result"`
}
type errorEnvelope struct {
	Protocol int         `json:"protocol"`
	Error    publicError `json:"error"`
}
type publicError struct {
	Code, Message string
	Details       json.RawMessage `json:"details"`
}

func decodeEnvelope(b []byte) error {
	if len(b) == 0 || b[len(b)-1] != '\n' || bytes.Count(b, []byte("\n")) != 1 {
		return errors.New("JSON output must be one LF-terminated object")
	}
	b = b[:len(b)-1]
	if len(b) == 0 || b[0] != '{' || b[len(b)-1] != '}' {
		return errors.New("JSON output must be one compact object")
	}
	if e := uniqueJSON(b); e != nil {
		return e
	}
	var x envelope
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&x); e != nil {
		return e
	}
	if x.Protocol != 1 || x.Command == "" || x.Changed == nil || !isObject(x.Result) {
		return errors.New("invalid success envelope")
	}
	return uniqueJSON(x.Result)
}
func decodeErrorEnvelope(b []byte) error {
	if len(b) == 0 || b[len(b)-1] != '\n' || bytes.Count(b, []byte("\n")) != 1 {
		return errors.New("JSON error must be one LF-terminated object")
	}
	b = b[:len(b)-1]
	if len(b) == 0 || b[0] != '{' || b[len(b)-1] != '}' {
		return errors.New("JSON error must be one compact object")
	}
	if e := uniqueJSON(b); e != nil {
		return e
	}
	var x errorEnvelope
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&x); e != nil {
		return e
	}
	if x.Protocol != 1 || x.Error.Code == "" || x.Error.Message == "" || !isObject(x.Error.Details) {
		return errors.New("invalid error envelope")
	}
	return uniqueJSON(x.Error.Details)
}
func isObject(b []byte) bool {
	b = bytes.TrimSpace(b)
	return len(b) >= 2 && b[0] == '{' && b[len(b)-1] == '}'
}

type invocation struct {
	command string
	output  string
}

func parseInvocation(argv []string) (invocation, error) {
	output := "text"
	commandAt := 0
	if len(argv) >= 2 && argv[0] == "--output" {
		output, commandAt = argv[1], 2
	}
	if output != "text" && output != "json" && output != "id" && output != "none" {
		return invocation{}, fmt.Errorf("invalid output mode %q", output)
	}
	if commandAt >= len(argv) {
		return invocation{}, errors.New("missing command")
	}
	for _, argument := range argv[commandAt:] {
		if argument == "--output" {
			return invocation{}, errors.New("--output must precede the command")
		}
	}
	command := argv[commandAt]
	if command == "thread" || command == "message" {
		if commandAt+1 >= len(argv) {
			return invocation{}, fmt.Errorf("missing %s subcommand", command)
		}
		command += "-" + argv[commandAt+1]
	}
	switch command {
	case "version", "validate", "format", "fix", "apply", "thread-list", "thread-get", "thread-create", "thread-move", "thread-delete", "message-add", "message-update", "message-delete":
		return invocation{command: command, output: output}, nil
	default:
		return invocation{}, fmt.Errorf("unknown command %q", command)
	}
}

func jsonOutput(argv []string) bool {
	invocation, err := parseInvocation(argv)
	return err == nil && invocation.output == "json"
}

func mutationCommand(command string) bool {
	switch command {
	case "format", "fix", "apply", "thread-create", "thread-move", "thread-delete", "message-add", "message-update", "message-delete":
		return true
	}
	return false
}

func idCommand(command string) bool {
	return command == "thread-create" || command == "message-add" || command == "fix"
}

func errorStatus(b []byte) (int, error) {
	var value errorEnvelope
	if err := json.Unmarshal(bytes.TrimSuffix(b, []byte("\n")), &value); err != nil {
		return 0, err
	}
	switch value.Error.Code {
	case "usage":
		return 2, nil
	case "validation", "repair":
		return 3, nil
	case "conflict":
		return 4, nil
	case "not_found":
		return 5, nil
	case "precondition":
		return 6, nil
	case "io":
		return 7, nil
	case "unsupported":
		return 8, nil
	default:
		return 0, fmt.Errorf("unknown error code %q", value.Error.Code)
	}
}

func textError(b []byte) bool {
	_, err := textErrorStatus(b)
	return err == nil
}

// textErrorStatus validates the text transport and binds its public error code
// to the required process exit status, just as errorStatus does for JSON.
func textErrorStatus(b []byte) (int, error) {
	if bytes.Count(b, []byte("\n")) != 1 || !bytes.HasSuffix(b, []byte("\n")) {
		return 0, errors.New("text error must be one LF-terminated line")
	}
	parts := strings.Split(strings.TrimSuffix(string(b), "\n"), " ")
	if len(parts) != 3 || parts[0] != "error" || parts[1] == "" || parts[2] == "" {
		return 0, errors.New("invalid text error")
	}
	code, message := parts[1], parts[2]
	valid := false
	switch code {
	case "usage":
		field, reason, ok := strings.Cut(strings.TrimPrefix(message, "usage:"), ":")
		valid = ok && message == "usage:"+field+":"+reason && oneOf(field, "argv", "output", "body", "metadata", "operations", "revision", "stdin") && oneOf(reason, "missing", "required", "repeated", "malformed", "incompatible", "too_large", "stdin_multiple")
	case "validation":
		valid = strings.HasPrefix(message, "validation:")
	case "repair":
		valid = message == "repair:document"
	case "conflict":
		valid = oneOf(message, "conflict:revision", "conflict:lock_timeout", "conflict:identity_changed")
	case "not_found":
		valid = oneOf(message, "not_found:thread", "not_found:message")
	case "precondition":
		valid = strings.HasPrefix(message, "precondition:")
	case "io":
		valid = strings.HasPrefix(message, "io:")
	case "unsupported":
		valid = strings.HasPrefix(message, "unsupported:")
	}
	if !valid {
		return 0, errors.New("invalid text error")
	}
	switch code {
	case "usage":
		return 2, nil
	case "validation", "repair":
		return 3, nil
	case "conflict":
		return 4, nil
	case "not_found":
		return 5, nil
	case "precondition":
		return 6, nil
	case "io":
		return 7, nil
	case "unsupported":
		return 8, nil
	}
	return 0, errors.New("unknown text error code")
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

// textSuccessEnvelope turns the specified text transport into the same JSON
// envelope used by JSON mode so result DTOs receive schema validation too.
func textSuccessEnvelope(b []byte, command string) ([]byte, error) {
	if !textSuccess(b, command) {
		return nil, errors.New("invalid text success")
	}
	line := strings.TrimSuffix(string(b), "\n")
	rest := strings.TrimPrefix(line, command+" changed=")
	changedText, result, _ := strings.Cut(rest, " result=")
	changed := changedText == "true"
	return json.Marshal(envelope{Protocol: 1, Command: command, Changed: &changed, Result: json.RawMessage(result)})
}

func textSuccess(b []byte, command string) bool {
	prefix := command + " changed="
	line := strings.TrimSuffix(string(b), "\n")
	if bytes.Count(b, []byte("\n")) != 1 || !strings.HasPrefix(line, prefix) {
		return false
	}
	rest := strings.TrimPrefix(line, prefix)
	changed, result, ok := strings.Cut(rest, " result=")
	if !ok || (changed != "true" && changed != "false") || !isObject([]byte(result)) || uniqueJSON([]byte(result)) != nil {
		return false
	}
	return true
}

// validateEnvelopes makes fixture artifacts state the command/output contract,
// not just arbitrary matching bytes. Status 3 has both legal shapes: a check
// result is success output, while an invalid target is an error.
func validateEnvelopes(g golden, status int, out, err []byte) error {
	invocation, parseErr := parseInvocation(g.Argv)
	if g.Expect.InvocationError != "" {
		textStatus, textErr := textErrorStatus(err)
		if g.Expect.InvocationError != "argv" || parseErr == nil || status != 2 || len(out) != 0 || textErr != nil || textStatus != status {
			return errors.New("invalid argv-error fixture declaration")
		}
		return nil
	}
	if parseErr != nil {
		return parseErr
	}
	if invocation.output == "none" && !mutationCommand(invocation.command) {
		textStatus, textErr := textErrorStatus(err)
		if status == 2 && len(out) == 0 && textErr == nil && textStatus == status {
			return nil
		}
		return fmt.Errorf("--output none is invalid for %s", invocation.command)
	}
	if invocation.output == "id" && !idCommand(invocation.command) {
		textStatus, textErr := textErrorStatus(err)
		if status == 2 && len(out) == 0 && textErr == nil && textStatus == status {
			return nil
		}
		return fmt.Errorf("--output id is invalid for %s", invocation.command)
	}
	if invocation.output != "json" {
		if status == 0 || status == 3 {
			if status == 3 && len(err) == 0 && !checkSuccessCommand(invocation.command, g.Argv) {
				return fmt.Errorf("success status 3 is invalid for %s", invocation.command)
			}
			if invocation.output == "none" && len(out) == 0 && len(err) == 0 {
				return nil
			}
			// Explicit-ID create/add and an already-normal fix have no generated
			// IDs to print. The public id mode therefore permits an empty success.
			if invocation.output == "id" && len(out) == 0 && len(err) == 0 {
				return nil
			}
			if len(err) == 0 && invocation.output == "text" && textSuccess(out, invocation.command) {
				return nil
			}
			if len(err) == 0 && invocation.output == "id" && len(out) > 0 {
				if status != 0 {
					return errors.New("id output is invalid for a nonzero exit status")
				}
				if !validIDOutput(out) {
					return errors.New("id output must contain canonical UUIDv7 values, one per LF-terminated line")
				}
				return nil
			}
			if status == 3 && len(out) == 0 {
				textStatus, textErr := textErrorStatus(err)
				if textErr == nil && textStatus == status {
					return nil
				}
			}
			return fmt.Errorf("non-JSON status %d used illegal streams", status)
		}
		textStatus, textErr := textErrorStatus(err)
		if len(out) != 0 || textErr != nil || textStatus != status {
			return fmt.Errorf("non-JSON failure status %d must write its matching text error to stderr", status)
		}
		return nil
	}
	if len(out) > 0 && len(err) > 0 {
		return errors.New("JSON response wrote both stdout and stderr")
	}
	if len(out) > 0 {
		if err := decodeEnvelope(out); err != nil {
			return fmt.Errorf("stdout success envelope: %w", err)
		}
		var value envelope
		if err := json.Unmarshal(bytes.TrimSuffix(out, []byte("\n")), &value); err != nil {
			return err
		}
		if value.Command != invocation.command {
			return fmt.Errorf("success envelope command %q want %q", value.Command, invocation.command)
		}
		if status != 0 && status != 3 {
			return fmt.Errorf("success envelope has illegal status %d", status)
		}
		if status == 3 && !checkSuccessCommand(invocation.command, g.Argv) {
			return fmt.Errorf("success status 3 is invalid for %s", invocation.command)
		}
		return nil
	}
	if len(err) > 0 {
		if err := decodeErrorEnvelope(err); err != nil {
			return fmt.Errorf("stderr error envelope: %w", err)
		}
		wantStatus, err := errorStatus(err)
		if err != nil {
			return err
		}
		if status != wantStatus {
			return fmt.Errorf("error envelope status %d want %d", status, wantStatus)
		}
		return nil
	}
	return fmt.Errorf("JSON response for status %d wrote neither stdout nor stderr", status)
}

func checkSuccessCommand(command string, argv []string) bool {
	if command == "validate" {
		return true
	}
	if command != "format" && command != "fix" {
		return false
	}
	for _, argument := range argv {
		if argument == "--check" {
			return true
		}
	}
	return false
}

func validIDOutput(out []byte) bool {
	if len(out) == 0 || !bytes.HasSuffix(out, []byte("\n")) || bytes.Count(out, []byte("\n")) == 0 {
		return false
	}
	for _, id := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		if !canonicalUUIDV7(id) {
			return false
		}
	}
	return true
}
