package remoteagent

import (
	"strings"
	"testing"
)

func TestGenerateCanaryPairingCode(t *testing.T) {
	first, err := GenerateCanaryPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateCanaryPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent canary pairing codes unexpectedly matched")
	}
	parts := strings.Split(first, "-")
	if len(parts) != 6 {
		t.Fatalf("expected 6 groups, got %q", first)
	}
	for _, part := range parts {
		if len(part) != 4 {
			t.Fatalf("unexpected pairing group %q", part)
		}
	}
	if _, err := NormalizeCanaryPairingCode(first); err != nil {
		t.Fatalf("generated code rejected: %v", err)
	}
}

func TestCanaryPairingNormalizationAndHash(t *testing.T) {
	code := "ABCD-0123-EF45-6789-A0B1-C2D3"
	normalized, err := NormalizeCanaryPairingCode(" abcd 0123-ef45 6789-a0b1-c2d3 ")
	if err != nil {
		t.Fatal(err)
	}
	if normalized != "ABCD0123EF456789A0B1C2D3" {
		t.Fatalf("unexpected normalized code %q", normalized)
	}
	h1, err := CanaryPairingHash(code)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := CanaryPairingHash(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("unexpected pairing hash equality/size: %q %q", h1, h2)
	}
	for _, bad := range []string{"", "ABCD", "ZZZZ-0123-EF45-6789-A0B1-C2D3", "ABCD-0123-EF45-6789-A0B1-C2D3-FFFF"} {
		if _, err := NormalizeCanaryPairingCode(bad); err == nil {
			t.Fatalf("invalid code accepted: %q", bad)
		}
	}
}
