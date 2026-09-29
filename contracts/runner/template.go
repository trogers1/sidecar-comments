package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// UUIDV7 templates are intentionally limited to expected streams and expected
// regular-file bytes. They bind an observed, canonical UUIDv7 once per fixture
// and let later expected fields refer to that exact value. They are never an
// input-substitution mechanism.
type uuidBindings struct{ values map[string]string }

func newUUIDBindings() *uuidBindings { return &uuidBindings{values: make(map[string]string)} }

func validateUUIDTemplates(g golden) error {
	for _, e := range g.Initial.Entries {
		if e.BytesTemplate != "" {
			return errors.New("UUIDV7 templates are forbidden in initial_fs")
		}
	}
	declared := newUUIDBindings()
	// Base64 expectations are literal bytes. Only the dedicated template fields
	// participate in template parsing or expansion.
	for _, value := range []*string{g.Expect.StdoutTemplate, g.Expect.StderrTemplate} {
		if value == nil {
			continue
		}
		expanded, err := expand([]byte(*value), expandRequest{Initial: g.Initial, Final: g.Expect.Final, CLIVersion: "artifact"})
		if err != nil {
			return err
		}
		if err := declareUUIDTemplate(expanded, declared); err != nil {
			return err
		}
	}
	for _, e := range g.Expect.Final.Entries {
		if e.BytesTemplate != "" {
			if e.Kind != "file" || e.Bytes != "" {
				return errors.New("bytes_template is only valid for expected regular-file bytes")
			}
			if err := declareUUIDTemplate([]byte(e.BytesTemplate), declared); err != nil {
				return err
			}
		}
	}
	return rejectUUIDTemplatesInInputs(g)
}

func declareUUIDTemplate(input []byte, bindings *uuidBindings) error {
	for len(input) != 0 {
		i := bytes.Index(input, []byte("{{"))
		if i < 0 {
			return nil
		}
		input = input[i:]
		end := bytes.Index(input, []byte("}}"))
		if end < 0 {
			return errors.New("unterminated template")
		}
		token := string(input[:end+2])
		kind, name, ok := uuidToken(token)
		if !ok {
			return fmt.Errorf("unapproved template %q", token)
		}
		if kind == "bind" {
			if _, exists := bindings.values[name]; exists {
				return fmt.Errorf("UUIDV7 name rebound %q", name)
			}
			bindings.values[name] = ""
		} else if _, exists := bindings.values[name]; !exists {
			return fmt.Errorf("UUIDV7 name undefined %q", name)
		}
		input = input[end+2:]
	}
	return nil
}

func validateUUIDTokenSyntax(input []byte) error {
	for len(input) != 0 {
		i := bytes.Index(input, []byte("{{"))
		if i < 0 {
			return nil
		}
		input = input[i:]
		end := bytes.Index(input, []byte("}}"))
		if end < 0 {
			return errors.New("unterminated template")
		}
		if _, _, ok := uuidToken(string(input[:end+2])); !ok {
			return fmt.Errorf("unapproved template %q", input[:end+2])
		}
		input = input[end+2:]
	}
	return nil
}

func uuidToken(token string) (kind, name string, ok bool) {
	if !strings.HasPrefix(token, "{{UUIDV7:") || !strings.HasSuffix(token, "}}") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(token, "{{UUIDV7:"), "}}"), ":")
	if len(parts) != 2 || (parts[0] != "bind" && parts[0] != "ref") || !validUUIDName(parts[1]) {
		return "", "", false
	}
	return parts[0], parts[1], true
}
func validUUIDName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if !(r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' || i > 0 && (r == '_' || r == '-')) {
			return false
		}
	}
	return true
}

func matchUUIDTemplate(expected, actual []byte, bindings *uuidBindings) error {
	for {
		i := bytes.Index(expected, []byte("{{"))
		if i < 0 {
			if !bytes.Equal(expected, actual) {
				return errors.New("bytes differ")
			}
			return nil
		}
		if len(actual) < i || !bytes.Equal(expected[:i], actual[:i]) {
			return errors.New("bytes differ")
		}
		expected, actual = expected[i:], actual[i:]
		end := bytes.Index(expected, []byte("}}"))
		if end < 0 {
			return errors.New("unterminated template")
		}
		kind, name, ok := uuidToken(string(expected[:end+2]))
		if !ok {
			return fmt.Errorf("unapproved template %q", expected[:end+2])
		}
		if len(actual) < 36 {
			return errors.New("UUIDV7 bytes truncated")
		}
		value := string(actual[:36])
		if kind == "bind" {
			if _, exists := bindings.values[name]; exists {
				return fmt.Errorf("UUIDV7 name rebound %q", name)
			}
			if !canonicalUUIDV7(value) {
				return fmt.Errorf("UUIDV7 bind %q is not canonical lowercase UUIDv7", name)
			}
			bindings.values[name] = value
		} else if value, exists := bindings.values[name]; !exists || string(actual[:36]) != value {
			return fmt.Errorf("UUIDV7 reference %q differs or is undefined", name)
		}
		expected, actual = expected[end+2:], actual[36:]
	}
}

// matchRuntimeTemplate resolves SHA-256 tokens at the point they occur. UUID
// bindings therefore render final bytes before their revisions are hashed.
func matchRuntimeTemplate(expected, actual []byte, bindings *uuidBindings, initial, final fsSpec) error {
	for {
		i := bytes.Index(expected, []byte("{{"))
		if i < 0 {
			if !bytes.Equal(expected, actual) {
				return errors.New("bytes differ")
			}
			return nil
		}
		if len(actual) < i || !bytes.Equal(expected[:i], actual[:i]) {
			return errors.New("bytes differ")
		}
		expected, actual = expected[i:], actual[i:]
		end := bytes.Index(expected, []byte("}}"))
		if end < 0 {
			return errors.New("unterminated template")
		}
		token := string(expected[:end+2])
		if kind, name, ok := uuidToken(token); ok {
			if len(actual) < 36 {
				return errors.New("UUIDV7 bytes truncated")
			}
			value := string(actual[:36])
			if kind == "bind" {
				if _, exists := bindings.values[name]; exists || !canonicalUUIDV7(value) {
					return fmt.Errorf("UUIDV7 bind %q is invalid or rebound", name)
				}
				bindings.values[name] = value
			} else if value, exists := bindings.values[name]; !exists || string(actual[:36]) != value {
				return fmt.Errorf("UUIDV7 reference %q differs or is undefined", name)
			}
			expected, actual = expected[end+2:], actual[36:]
			continue
		}
		if strings.HasPrefix(token, "{{sha256:") {
			value, err := runtimeSHA256Token(token, bindings, initial, final)
			if err != nil {
				return err
			}
			if len(actual) < len(value) || !bytes.Equal(actual[:len(value)], value) {
				return errors.New("SHA-256 bytes differ")
			}
			expected, actual = expected[end+2:], actual[len(value):]
			continue
		}
		return fmt.Errorf("unapproved template %q", token)
	}
}

func runtimeSHA256Token(token string, bindings *uuidBindings, initial, final fsSpec) ([]byte, error) {
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(token, "{{sha256:"), "}}"), ":")
	if len(parts) != 2 || (parts[0] != "initial" && parts[0] != "final") {
		return nil, fmt.Errorf("unapproved template %q", token)
	}
	files := initial.Entries
	if parts[0] == "final" {
		files = final.Entries
	}
	for _, entry := range files {
		if entry.Path != parts[1] || entry.Kind != "file" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(entry.Bytes)
		if err != nil {
			return nil, err
		}
		if entry.BytesTemplate != "" {
			data = []byte(entry.BytesTemplate)
			for name, value := range bindings.values {
				data = bytes.ReplaceAll(data, []byte("{{UUIDV7:ref:"+name+"}}"), []byte(value))
			}
			if bytes.Contains(data, []byte("{{UUIDV7:")) {
				return nil, errors.New("revision references an unbound UUIDV7")
			}
		}
		sum := sha256.Sum256(data)
		return []byte("sha256:" + fmt.Sprintf("%x", sum)), nil
	}
	return nil, fmt.Errorf("revision template references unknown file %q", parts[1])
}

func canonicalUUIDV7(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' || !strings.ContainsRune("89ab", rune(value[19])) {
		return false
	}
	for i := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((value[i] >= '0' && value[i] <= '9') || (value[i] >= 'a' && value[i] <= 'f')) {
			return false
		}
	}
	return true
}

func renderUUIDTemplatesForValidation(input []byte) ([]byte, error) {
	if err := validateUUIDTokenSyntax(input); err != nil {
		return nil, err
	}
	out := append([]byte(nil), input...)
	for bytes.Contains(out, []byte("{{")) {
		i, end := bytes.Index(out, []byte("{{")), -1
		end = i + bytes.Index(out[i:], []byte("}}"))
		if end < i {
			return nil, errors.New("unterminated template")
		}
		out = append(append(out[:i:i], []byte("018f0000-0000-7000-8000-000000000001")...), out[end+2:]...)
	}
	return out, nil
}

func rejectUUIDTemplatesInInputs(g golden) error {
	for _, argument := range g.Argv {
		if strings.Contains(argument, "{{UUIDV7:") {
			return errors.New("UUIDV7 templates are forbidden in argv")
		}
	}
	for _, variable := range g.Env {
		if strings.Contains(variable.Value, "{{UUIDV7:") {
			return errors.New("UUIDV7 templates are forbidden in environment")
		}
	}
	// stdin_b64 and initial_fs bytes_b64 are literal byte payloads. They may
	// contain template-looking body text without creating a binding.
	return nil
}
