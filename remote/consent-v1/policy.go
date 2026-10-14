package main

import (
	"errors"
	"strings"
)

const (
	maxDisplayLabelChars = 120
	maxFingerprintChars  = 160
	maxPairingCodeChars  = 40
)

func sanitizeDisplayText(v string, max int) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, v)
	v = strings.Join(strings.Fields(v), " ")
	if max > 0 && len([]rune(v)) > max {
		r := []rune(v)
		v = string(r[:max])
	}
	return v
}

func sanitizePairingCode(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v == "" || len(v) > maxPairingCodeChars {
		return ""
	}
	for _, r := range v {
		if (r >= 'A' && r <= 'F') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return ""
	}
	return v
}

func validateMode(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "approve", "indicator", "pairing":
		return v, nil
	default:
		return "", errors.New("unsupported consent-helper mode")
	}
}
