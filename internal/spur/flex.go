package spur

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FlexBool is a boolean that accepts bare JSON booleans (true/false) and
// quoted string forms ("true", "false", "1", "0", "yes", "no") during
// unmarshal. It always marshals back as a bare JSON boolean. This exists
// because the Spur API returns boolean metadata fields as bare booleans
// while the upstream docs show them as strings; this makes the client
// tolerant of either shape.
type FlexBool bool

// UnmarshalJSON implements json.Unmarshaler.
func (b *FlexBool) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" || s == "" {
		return nil
	}
	switch s {
	case "true":
		*b = true
		return nil
	case "false":
		*b = false
		return nil
	}
	// Quoted string form.
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "true", "1", "yes", "y":
			*b = true
			return nil
		case "false", "0", "no", "n", "":
			*b = false
			return nil
		}
	}
	return fmt.Errorf("cannot unmarshal %s into FlexBool", s)
}

// MarshalJSON implements json.Marshaler. FlexBool always emits a bare JSON
// boolean.
func (b FlexBool) MarshalJSON() ([]byte, error) {
	if b {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

// FlexNumber is a numeric value that accepts either a bare JSON number or a
// quoted string during unmarshal and emits a bare number on marshal. It
// preserves the raw textual representation so integers do not gain a trailing
// ".0" on round-trip and floats keep their precision.
//
// The upstream docs type some numeric fields as strings, so the string form
// tolerates non-numeric content (e.g. "N/A"); such a value round-trips as a
// quoted string on marshal — never as a bare token, which would corrupt the
// enclosing document. A bare value that is not a number is rejected.
//
// A zero value encodes as the bare number 0.
type FlexNumber string

// UnmarshalJSON implements json.Unmarshaler.
func (n *FlexNumber) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" || s == "" {
		*n = ""
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		// String form: unquote and keep the content, numeric or not.
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*n = FlexNumber(strings.TrimSpace(str))
		return nil
	}
	if !isJSONNumber(s) {
		return fmt.Errorf("cannot unmarshal %s into FlexNumber", s)
	}
	*n = FlexNumber(s)
	return nil
}

// MarshalJSON implements json.Marshaler. Numeric content is emitted as a bare
// JSON number; anything else (tolerated from the string form) is quoted so the
// value can never render the enclosing document invalid.
func (n FlexNumber) MarshalJSON() ([]byte, error) {
	if string(n) == "" {
		return []byte("0"), nil
	}
	if isJSONNumber(string(n)) {
		return []byte(string(n)), nil
	}
	return json.Marshal(string(n))
}

// isJSONNumber reports whether s is a syntactically valid bare JSON number.
func isJSONNumber(s string) bool {
	if s == "" {
		return false
	}
	if c := s[0]; c != '-' && (c < '0' || c > '9') {
		return false
	}
	return json.Valid([]byte(s))
}

// String returns the textual representation of the number.
func (n FlexNumber) String() string { return string(n) }
