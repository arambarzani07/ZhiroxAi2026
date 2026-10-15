package main

import (
	"strings"
	"testing"
)

func TestConsentCapabilityModeDefaultsScreenOnly(t *testing.T) {
	mode, err := parseConsentCapabilityMode("")
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if mode != consentScreenOnly { t.Fatalf("unexpected default mode: %q", mode) }
}

func TestConsentCapabilityModeScreenInputDisclosure(t *testing.T) {
	mode, err := parseConsentCapabilityMode("screen-input")
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	approval := strings.ToLower(consentApprovalNotice(mode))
	active := strings.ToLower(consentActiveNotice(mode))
	for _, required := range []string{"view", "mouse", "keyboard"} {
		if !strings.Contains(approval, required) { t.Fatalf("approval notice missing %q: %q", required, approval) }
	}
	for _, required := range []string{"control", "mouse", "keyboard"} {
		if !strings.Contains(active, required) { t.Fatalf("active notice missing %q: %q", required, active) }
	}
}

func TestConsentCapabilityModeMouseOnlyDisclosure(t *testing.T) {
	mode, err := parseConsentCapabilityMode("screen-mouse")
	if err != nil { t.Fatal(err) }
	approval := strings.ToLower(consentApprovalNotice(mode))
	if !strings.Contains(approval, "control the mouse") || !strings.Contains(approval, "keyboard control is disabled") {
		t.Fatalf("mouse-only disclosure is not exact: %q", approval)
	}
}

func TestConsentCapabilityModeKeyboardOnlyDisclosure(t *testing.T) {
	mode, err := parseConsentCapabilityMode("screen-keyboard")
	if err != nil { t.Fatal(err) }
	approval := strings.ToLower(consentApprovalNotice(mode))
	if !strings.Contains(approval, "control the keyboard") || !strings.Contains(approval, "mouse control is disabled") {
		t.Fatalf("keyboard-only disclosure is not exact: %q", approval)
	}
}

func TestConsentCapabilityModeRejectsUnknown(t *testing.T) {
	if _, err := parseConsentCapabilityMode("unattended"); err == nil {
		t.Fatal("unknown capability mode should fail closed")
	}
}
