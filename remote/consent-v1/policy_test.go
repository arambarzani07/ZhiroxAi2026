package main

import "testing"

func TestConsentHelperModes(t *testing.T) {
	for _, mode := range []string{"approve", "indicator", "pairing", " APPROVE "} {
		if _, err := validateMode(mode); err != nil {
			t.Fatalf("valid mode %q rejected: %v", mode, err)
		}
	}
	for _, mode := range []string{"hidden", "silent", "unattended", ""} {
		if _, err := validateMode(mode); err == nil {
			t.Fatalf("unsafe/invalid mode accepted: %q", mode)
		}
	}
}

func TestConsentDisplaySanitization(t *testing.T) {
	got := sanitizeDisplayText("  Browser\x00\nController   ", 120)
	if got != "Browser Controller" {
		t.Fatalf("unexpected sanitized label: %q", got)
	}
	long := "abcdefghijklmnopqrstuvwxyz"
	if got := sanitizeDisplayText(long, 5); got != "abcde" {
		t.Fatalf("length bound failed: %q", got)
	}
}

func TestPairingCodeSanitization(t *testing.T) {
	if got := sanitizePairingCode(" abcd-0123-ef45 "); got != "ABCD-0123-EF45" {
		t.Fatalf("unexpected sanitized pairing code: %q", got)
	}
	for _, bad := range []string{"", "PAIR-1234", "ABCD_1234", "ABCD 1234", "<script>"} {
		if got := sanitizePairingCode(bad); got != "" {
			t.Fatalf("invalid pairing code accepted: %q -> %q", bad, got)
		}
	}
}
