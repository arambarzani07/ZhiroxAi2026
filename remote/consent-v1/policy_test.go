package main

import "testing"

func TestConsentHelperModes(t *testing.T) {
	for _, mode := range []string{"approve", "indicator", " APPROVE "} {
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
