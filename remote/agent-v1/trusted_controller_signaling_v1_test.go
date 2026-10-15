package remoteagent

import (
	"strings"
	"testing"
	"time"
)

func trustedPending(now time.Time) PendingSession {
	return PendingSession{
		ID:                    "session-1",
		State:                 "pending",
		ExpiresAt:             now.Add(5 * time.Minute).Format(time.RFC3339Nano),
		ControllerID:          "controller-1",
		ControllerFingerprint: strings.Repeat("a", 64),
		RequestedCapabilities: map[string]any{
			"screen_stream": true,
			"mouse":         true,
			"keyboard":      true,
			"clipboard":     false,
			"file_transfer": false,
			"audio":         false,
		},
	}
}

func trustedGrantForSignaling(now time.Time) TrustedSessionGrant {
	return TrustedSessionGrant{
		ControllerID: "controller-1",
		AgentID:      "agent-1",
		SessionID:    "session-1",
		Fingerprint:  strings.Repeat("a", 64),
		Screen:       true,
		Mouse:        true,
		Keyboard:     true,
		IssuedAt:     now,
		ExpiresAt:    now.Add(TrustedSessionTTL),
	}
}

func TestTrustedSignalingBindingExactMatch(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)
	intent, err := ParseTrustedSignalingIntent(trustedPending(now), now)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindTrustedGrantToSignaling(trustedGrantForSignaling(now), intent, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !binding.Screen || !binding.Mouse || !binding.Keyboard {
		t.Fatalf("expected exact signaling capabilities: %+v", binding)
	}
	if !binding.ExpiresAt.Equal(intent.ExpiresAt) {
		t.Fatalf("expected shorter pending expiry to win: %+v", binding)
	}
}

func TestTrustedSignalingRejectsCapabilityDrift(t *testing.T) {
	now := time.Now().UTC()
	pending := trustedPending(now)
	pending.RequestedCapabilities["keyboard"] = false
	intent, err := ParseTrustedSignalingIntent(pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindTrustedGrantToSignaling(trustedGrantForSignaling(now), intent, now.Add(time.Second)); err == nil {
		t.Fatal("expected signaling capability mismatch to be rejected")
	}
}

func TestTrustedSignalingRejectsFingerprintMismatch(t *testing.T) {
	now := time.Now().UTC()
	pending := trustedPending(now)
	pending.ControllerFingerprint = strings.Repeat("b", 64)
	intent, err := ParseTrustedSignalingIntent(pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindTrustedGrantToSignaling(trustedGrantForSignaling(now), intent, now.Add(time.Second)); err == nil {
		t.Fatal("expected signaling fingerprint mismatch to be rejected")
	}
}

func TestTrustedSignalingRejectsMissingControllerID(t *testing.T) {
	now := time.Now().UTC()
	pending := trustedPending(now)
	pending.ControllerID = ""
	if _, err := ParseTrustedSignalingIntent(pending, now); err == nil {
		t.Fatal("expected missing controller id to be rejected")
	}
}

func TestTrustedSignalingRejectsUnsupportedCapabilities(t *testing.T) {
	now := time.Now().UTC()
	pending := trustedPending(now)
	pending.RequestedCapabilities["clipboard"] = true
	if _, err := ParseTrustedSignalingIntent(pending, now); err == nil {
		t.Fatal("expected disabled clipboard capability to be rejected")
	}
}

func TestTrustedSignalingRejectsExpiredPendingSession(t *testing.T) {
	now := time.Now().UTC()
	pending := trustedPending(now)
	pending.ExpiresAt = now.Format(time.RFC3339Nano)
	if _, err := ParseTrustedSignalingIntent(pending, now); err == nil {
		t.Fatal("expected expired pending session to be rejected")
	}
}
