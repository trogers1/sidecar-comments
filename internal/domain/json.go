package domain

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

type ValidateJSONRequest struct{ Bytes []byte }
type scanner struct {
	b        []byte
	i, depth int
}

func ValidateJSON(r ValidateJSONRequest) error {
	if !utf8.Valid(r.Bytes) {
		return invalid(invalidRequest{})
	}
	s := scanner{b: r.Bytes}
	if e := s.value(scanRequest{}); e != nil {
		return e
	}
	s.space(scanRequest{})
	if s.i != len(s.b) {
		return invalid(invalidRequest{})
	}
	return nil
}

type scanRequest struct{}

func (s *scanner) space(r scanRequest) {
	for s.i < len(s.b) && (s.b[s.i] == ' ' || s.b[s.i] == '\t' || s.b[s.i] == '\r' || s.b[s.i] == '\n') {
		s.i++
	}
}
func (s *scanner) str(r scanRequest) (string, error) {
	start := s.i
	s.i++
	for s.i < len(s.b) {
		c := s.b[s.i]
		if c == '"' {
			s.i++
			var v string
			if json.Unmarshal(s.b[start:s.i], &v) != nil {
				return "", invalid(invalidRequest{})
			}
			return v, nil
		}
		if c < 32 {
			return "", invalid(invalidRequest{})
		}
		if c == '\\' {
			s.i++
			if s.i >= len(s.b) {
				return "", invalid(invalidRequest{})
			}
			if s.b[s.i] == 'u' {
				if s.i+4 >= len(s.b) {
					return "", invalid(invalidRequest{})
				}
				n, e := strconv.ParseUint(string(s.b[s.i+1:s.i+5]), 16, 16)
				if e != nil || n >= 0xd800 && n <= 0xdfff {
					return "", invalid(invalidRequest{})
				}
				s.i += 4
			}
		}
		s.i++
	}
	return "", invalid(invalidRequest{})
}
func (s *scanner) value(r scanRequest) error {
	s.space(scanRequest{})
	if s.i >= len(s.b) || s.depth > 128 {
		return invalid(invalidRequest{})
	}
	s.depth++
	defer func(r scanRequest) { s.depth-- }(scanRequest{})
	switch s.b[s.i] {
	case '"':
		_, e := s.str(scanRequest{})
		return e
	case '{':
		s.i++
		s.space(scanRequest{})
		seen := map[string]bool{}
		if s.i < len(s.b) && s.b[s.i] == '}' {
			s.i++
			return nil
		}
		for {
			if s.i >= len(s.b) || s.b[s.i] != '"' {
				return invalid(invalidRequest{})
			}
			k, e := s.str(scanRequest{})
			if e != nil || seen[k] {
				return invalid(invalidRequest{})
			}
			seen[k] = true
			s.space(scanRequest{})
			if s.i >= len(s.b) || s.b[s.i] != ':' {
				return invalid(invalidRequest{})
			}
			s.i++
			if e := s.value(scanRequest{}); e != nil {
				return e
			}
			s.space(scanRequest{})
			if s.i >= len(s.b) {
				return invalid(invalidRequest{})
			}
			c := s.b[s.i]
			s.i++
			if c == '}' {
				return nil
			}
			if c != ',' {
				return invalid(invalidRequest{})
			}
			s.space(scanRequest{})
		}
	case '[':
		s.i++
		s.space(scanRequest{})
		if s.i < len(s.b) && s.b[s.i] == ']' {
			s.i++
			return nil
		}
		for {
			if e := s.value(scanRequest{}); e != nil {
				return e
			}
			s.space(scanRequest{})
			if s.i >= len(s.b) {
				return invalid(invalidRequest{})
			}
			c := s.b[s.i]
			s.i++
			if c == ']' {
				return nil
			}
			if c != ',' {
				return invalid(invalidRequest{})
			}
		}
	case 't', 'f', 'n':
		for _, v := range []string{"true", "false", "null"} {
			if len(s.b)-s.i >= len(v) && string(s.b[s.i:s.i+len(v)]) == v {
				s.i += len(v)
				return nil
			}
		}
		return invalid(invalidRequest{})
	default:
		start := s.i
		for s.i < len(s.b) && s.b[s.i] >= '0' && s.b[s.i] <= '9' {
			s.i++
		}
		if start == s.i || s.b[start] == '0' && s.i-start > 1 {
			return invalid(invalidRequest{})
		}
		_, e := strconv.ParseInt(string(s.b[start:s.i]), 10, 64)
		if e != nil {
			return invalid(invalidRequest{})
		}
		return nil
	}
}
