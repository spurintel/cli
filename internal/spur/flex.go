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
// quoted string during unmarshal and always emits a bare number on marshal.
// It preserves the raw textual representation so integers do not gain a
// trailing ".0" on round-trip and floats keep their precision.
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
		// String form: unquote and keep the content.
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*n = FlexNumber(strings.TrimSpace(str))
		return nil
	}
	// Bare number.
	*n = FlexNumber(s)
	return nil
}

// MarshalJSON implements json.Marshaler.
func (n FlexNumber) MarshalJSON() ([]byte, error) {
	if string(n) == "" {
		return []byte("0"), nil
	}
	return []byte(string(n)), nil
}

// String returns the textual representation of the number.
func (n FlexNumber) String() string { return string(n) }
