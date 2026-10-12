package main

import (
	"strings"
	"testing"
)

func TestKurdishVoiceEventAllowlist(t *testing.T) {
	for name, want := range pcOSKurdishVoiceEntries {
		got, err := pcOSKurdishVoiceLookup(name)
		if err != nil {
			t.Fatalf("lookup %q failed: %v", name, err)
		}
		if got != want {
			t.Fatalf("lookup %q mismatch: got %#v want %#v", name, got, want)
		}
		if strings.TrimSpace(got.Phrase) == "" {
			t.Fatalf("lookup %q has empty Kurdish phrase", name)
		}
		if !strings.HasSuffix(strings.ToLower(got.Asset), ".wav") {
			t.Fatalf("lookup %q asset is not WAV: %q", name, got.Asset)
		}
		if strings.ContainsAny(got.Asset, `/\\:`) {
			t.Fatalf("lookup %q asset contains a path component: %q", name, got.Asset)
		}
	}
}

func TestKurdishVoiceRejectsUnknownEvent(t *testing.T) {
	if _, err := pcOSKurdishVoiceLookup("../../arbitrary.wav"); err == nil {
		t.Fatal("unknown/path-like event must be rejected")
	}
}

func TestKurdishVoiceDefaultPolicyIsPrivateAndZeroUI(t *testing.T) {
	p := pcOSKurdishVoicePolicy()
	checks := map[string]interface{}{
		"default_enabled":       false,
		"zero_ui":               true,
		"microphone_enabled":    false,
		"continuous_listening":  false,
		"network_required":      false,
		"shell_allowed":         false,
		"free_form_tts_enabled": false,
	}
	for key, want := range checks {
		if got := p[key]; got != want {
			t.Fatalf("policy %s: got %#v want %#v", key, got, want)
		}
	}
}
