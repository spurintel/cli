package spur

import (
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
)

// ValidateIP ensures the supplied value is a syntactically valid IP address
// and does not contain control characters that could distort prompt text.
func ValidateIP(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "", fmt.Errorf("ip is required")
	}
	if containsControl(ip) {
		return "", fmt.Errorf("ip must not contain control characters")
	}
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("ip must be a valid IPv4 or IPv6 address")
	}
	return ip, nil
}

// ValidateHistoricalDate accepts an empty date or a valid YYYYmmdd value.
func ValidateHistoricalDate(dt string) (string, error) {
	dt = strings.TrimSpace(dt)
	if dt == "" {
		return "", nil
	}
	if containsControl(dt) {
		return "", fmt.Errorf("dt must not contain control characters")
	}
	if _, err := time.Parse("20060102", dt); err != nil {
		return "", fmt.Errorf("dt must be a valid date in YYYYmmdd format")
	}
	return dt, nil
}

func containsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
