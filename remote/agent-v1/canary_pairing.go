package remoteagent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	CanaryPairingEntropyBytes = 12
	CanaryPairingTTL          = 2 * time.Minute
	canaryPairingHexChars     = CanaryPairingEntropyBytes * 2
)

// GenerateCanaryPairingCode returns a human-readable 96-bit one-time code.
// The code is displayed only on the attended target PC and is never intended
// to be persisted. Servers should retain only CanaryPairingHash(code).
func GenerateCanaryPairingCode() (string, error) {
	buf := make([]byte, CanaryPairingEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := strings.ToUpper(hex.EncodeToString(buf))
	parts := make([]string, 0, 6)
	for i := 0; i < len(raw); i += 4 {
		parts = append(parts, raw[i:i+4])
	}
	return strings.Join(parts, "-"), nil
}

func NormalizeCanaryPairingCode(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	code = strings.NewReplacer("-", "", " ", "").Replace(code)
	if len(code) != canaryPairingHexChars {
		return "", errors.New("canary pairing code has invalid length")
	}
	if _, err := hex.DecodeString(code); err != nil {
		return "", errors.New("canary pairing code contains invalid characters")
	}
	return code, nil
}

func CanaryPairingHash(code string) (string, error) {
	normalized, err := NormalizeCanaryPairingCode(code)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:]), nil
}
