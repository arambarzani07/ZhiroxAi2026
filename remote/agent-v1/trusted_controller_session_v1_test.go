package remoteagent

import (
	"testing"
	"time"
)

func testTrustedAuthorization(now time.Time) TrustedAuthorization {
	return TrustedAuthorization{
		ControllerID: "controller-1",
		AgentID:      "agent-1",
		SessionID:    "session-1",
		Fingerprint:  "fingerprint-1",
		Screen:       true,
		Mouse:        true,
		Keyboard:     true,
		IssuedAt:     now,
		ExpiresAt:    now.Add(TrustedSessionTTL),
	}
}

func TestGrantTrustedSessionPreservesChallengeBinding(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 45, 0, 0, time.UTC)
	auth := testTrustedAuthorization(now)
	grant, err := GrantTrustedSession(auth, TrustedSessionCapabilityRequest{
		Screen: true, Mouse: true, Keyboard: true,
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if grant.ControllerID != auth.ControllerID || grant.AgentID != auth.AgentID || grant.SessionID != auth.SessionID {
		t.Fatalf("session binding changed: %+v", grant)
	}
	if !grant.Screen || !grant.Mouse || !grant.Keyboard {
		t.Fatalf("expected exact requested capabilities: %+v", grant)
	}
}

func TestGrantTrustedSessionRejectsCapabilityEscalation(t *testing.T) {
	now := time.Now().UTC()
	auth := testTrustedAuthorization(now)
	auth.Mouse = false
	if _, err := GrantTrustedSession(auth, TrustedSessionCapabilityRequest{Screen: true, Mouse: true}, now.Add(time.Second)); err == nil {
		t.Fatal("expected mouse capability escalation to be rejected")
	}
}

func TestGrantTrustedSessionRejectsExpiredAuthorization(t *testing.T) {
	now := time.Now().UTC()
	auth := testTrustedAuthorization(now)
	if _, err := GrantTrustedSession(auth, TrustedSessionCapabilityRequest{Screen: true}, auth.ExpiresAt); err == nil {
		t.Fatal("expected expired authorization to be rejected")
	}
}

func TestGrantTrustedSessionRequiresScreen(t *testing.T) {
	now := time.Now().UTC()
	auth := testTrustedAuthorization(now)
	if _, err := GrantTrustedSession(auth, TrustedSessionCapabilityRequest{Mouse: true}, now.Add(time.Second)); err == nil {
		t.Fatal("expected screen-less session request to be rejected")
	}
}

func TestGrantTrustedSessionRejectsUnboundAuthorization(t *testing.T) {
	now := time.Now().UTC()
	auth := testTrustedAuthorization(now)
	auth.SessionID = ""
	if _, err := GrantTrustedSession(auth, TrustedSessionCapabilityRequest{Screen: true}, now.Add(time.Second)); err == nil {
		t.Fatal("expected unbound authorization to be rejected")
	}
}
