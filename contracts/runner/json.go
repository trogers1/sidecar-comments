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
	output, at := "text", 0
	if len(argv) > 0 && strings.HasPrefix(argv[0], "--output=") {
		output, at = strings.TrimPrefix(argv[0], "--output="), 1
	} else if len(argv) >= 2 && argv[0] == "--output" {
		output, at = argv[1], 2
	}
	if at < len(argv) && (argv[at] == "--output" || strings.HasPrefix(argv[at], "--output=")) {
		return invocation{}, errors.New("repeated or missing --output")
	}
	help := invocation{command: "help", output: "text"}
	if at == len(argv) {
		if output == "text" {
			return help, nil
		}
		return invocation{}, errors.New("missing command")
	}
	command := argv[at]
	at++
	if oneOf(command, "--help", "-h") {
		return help, nil
	}
	if command == "--version" {
		command = "version"
	} else if strings.Contains(command, "-") {
		return invocation{}, fmt.Errorf("unknown command %q", command)
	}
	if command == "thread" || command == "message" {
		if at == len(argv) {
			if output == "text" {
				return help, nil
			}
			return invocation{}, fmt.Errorf("missing %s subcommand", command)
		}
		if oneOf(argv[at], "--help", "-h") {
			return help, nil
		}
		command += "-" + argv[at]
		at++
	}
	// Independent contract model of the public flags in docs/v1/cli.md.
	// This only determines whether help is reached, not flag value validity.
	flags := ""
	switch command {
	case "version":
	case "validate":
		flags = "file source"
	case "format":
		flags = "file check"
	case "fix":
		flags = "file check add-missing-message-ids"
	case "thread-list":
		flags = "file"
	case "thread-get", "thread-delete":
		flags = "file id"
	case "thread-create":
		flags = "file location author-kind author-name body-file external-author-id id message-id gitlab-json"
	case "thread-move":
		flags = "file id location"
	case "message-add":
		flags = "file thread author-kind author-name body-file external-author-id id gitlab-json if-last-author-kind"
	case "message-update":
		flags = "file thread id body-file"
	case "message-delete":
		flags = "file thread id"
	case "apply":
		flags = "file operations-file"
	default:
		return invocation{}, fmt.Errorf("unknown command %q", command)
	}
	if !oneOf(output, "text", "json", "id", "none") {
		return invocation{}, fmt.Errorf("invalid output mode %q", output)
	}
	parsed := invocation{command: command, output: output}
	if output == "none" && !mutationCommand(command) || output == "id" && !idCommand(command) {
		return parsed, nil
	}
	if mutationCommand(command) {
		flags += " if-revision lock-timeout"
	}
	seen := map[string]bool{}
	ordinary := false
	for ; at < len(argv); at++ {
		argument := argv[at]
		if oneOf(argument, "--help", "-h") {
			return help, nil
		}
		if !strings.HasPrefix(argument, "-") {
			return parsed, nil
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(argument, "-"), "=")
		if name == "output" {
			return invocation{}, errors.New("--output must precede the command")
		}
		guard := oneOf(name, "if-revision", "lock-timeout")
		if seen[name] || guard && ordinary || !strings.Contains(" "+flags+" ", " "+name+" ") {
			return parsed, nil
		}
		seen[name] = true
		ordinary = ordinary || !guard
		if !hasValue && !oneOf(name, "check", "add-missing-message-ids") {
			at++ // Values named --help or -h are not help requests.
		}
	}
	return parsed, nil
}

func jsonOutput(argv []string) bool {
	invocation, err := parseInvocation(argv)
	if err == nil {
		return invocation.output == "json"
	}
	return len(argv) >= 2 && argv[0] == "--output" && argv[1] == "json" || len(argv) > 0 && argv[0] == "--output=json"
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

// textErrorStatus validates the id/none error transport and binds its public error code
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

// validateEnvelopes makes fixture artifacts state the command/output contract,
// not just arbitrary matching bytes. Status 3 has both legal shapes: a check
// result is success output, while an invalid target is an error.
func validateEnvelopes(g golden, status int, out, err []byte) error {
	invocation, parseErr := parseInvocation(g.Argv)
	if g.Expect.InvocationError != "" {
		if g.Expect.InvocationError != "argv" || parseErr == nil || status != 2 || len(out) != 0 {
			return errors.New("invalid argv-error fixture declaration")
		}
		if jsonOutput(g.Argv) {
			if decodeErr := decodeErrorEnvelope(err); decodeErr != nil {
				return decodeErr
			}
			if errorStatus, statusErr := errorStatus(err); statusErr != nil || errorStatus != status {
				return errors.New("argv JSON error must match usage exit status")
			}
			return nil
		}
		if len(g.Argv) >= 2 && g.Argv[0] == "--output" && oneOf(g.Argv[1], "id", "none") || len(g.Argv) > 0 && oneOf(g.Argv[0], "--output=id", "--output=none") {
			machineStatus, machineErr := textErrorStatus(err)
			if machineErr != nil || machineStatus != status {
				return errors.New("argv id/none error must match usage exit status")
			}
			return nil
		}
		if !bytes.HasSuffix(err, []byte("\n")) {
			return errors.New("argv error must write human stderr")
		}
		return nil
	}
	if parseErr != nil {
		return parseErr
	}
	// Human prose is presentation, not a JSON transport. Exact golden byte
	// matching checks its wording; only stream placement and status belong here.
	if invocation.output == "text" {
		if status == 0 || (status == 3 && len(err) == 0 && checkSuccessCommand(invocation.command, g.Argv)) {
			if len(err) == 0 && bytes.HasSuffix(out, []byte("\n")) {
				return nil
			}
		} else if status >= 2 && status <= 8 && len(out) == 0 && bytes.HasSuffix(err, []byte("\n")) {
			return nil
		}
		return errors.New("human output used illegal streams or status")
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
