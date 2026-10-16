package remoteagent

import (
	"strings"
	"testing"
	"time"
)

func trustedHandshakeResultForActivation(now time.Time) TrustedHandshakeResult {
	fingerprint := strings.Repeat("a", 64)
	return TrustedHandshakeResult{
		Authorization: TrustedAuthorization{
			ControllerID: "controller-1",
			AgentID:      "agent-1",
			SessionID:    "session-activation-000001",
			Fingerprint:  fingerprint,
			Screen:       true,
			Mouse:        true,
			Keyboard:     true,
			IssuedAt:     now,
			ExpiresAt:    now.Add(15 * time.Minute),
		},
		Grant: TrustedSessionGrant{
			ControllerID: "controller-1",
			AgentID:      "agent-1",
			SessionID:    "session-activation-000001",
			Fingerprint:  fingerprint,
			Screen:       true,
			Mouse:        true,
			Keyboard:     true,
			IssuedAt:     now,
			ExpiresAt:    now.Add(15 * time.Minute),
		},
		Binding: TrustedSignalingBinding{
			ControllerID: "controller-1",
			SessionID:    "session-activation-000001",
			Fingerprint:  fingerprint,
			Screen:       true,
			Mouse:        true,
			Keyboard:     true,
			ExpiresAt:    now.Add(5 * time.Minute),
		},
	}
}

func TestTrustedActivationLeaseExactBinding(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 30, 0, 0, time.UTC)
	lease, err := NewTrustedActivationLease(trustedHandshakeResultForActivation(now), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !lease.AllowsInput() || !lease.Screen || !lease.Mouse || !lease.Keyboard {
		t.Fatalf("expected input-capable trusted lease: %+v", lease)
	}
	if !lease.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected shortest upstream expiry to win: %+v", lease)
	}
	if err := lease.ValidateFor("controller-1", "agent-1", "session-activation-000001", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestTrustedActivationLeaseRejectsIdentityDrift(t *testing.T) {
	now := time.Now().UTC()
	result := trustedHandshakeResultForActivation(now)
	result.Binding.SessionID = "session-activation-000002"
	if _, err := NewTrustedActivationLease(result, now.Add(time.Second)); err == nil {
		t.Fatal("expected session identity drift to be rejected")
	}
}

func TestTrustedActivationLeaseRejectsCapabilityDrift(t *testing.T) {
	now := time.Now().UTC()
	result := trustedHandshakeResultForActivation(now)
	result.Binding.Keyboard = false
	if _, err := NewTrustedActivationLease(result, now.Add(time.Second)); err == nil {
		t.Fatal("expected capability drift to be rejected")
	}
}

func TestTrustedActivationLeaseRejectsExpiredBinding(t *testing.T) {
	now := time.Now().UTC()
	result := trustedHandshakeResultForActivation(now)
	result.Binding.ExpiresAt = now
	if _, err := NewTrustedActivationLease(result, now.Add(time.Second)); err == nil {
		t.Fatal("expected expired signaling binding to be rejected")
	}
}

func TestTrustedActivationLeaseValidateForRejectsWrongController(t *testing.T) {
	now := time.Now().UTC()
	lease, err := NewTrustedActivationLease(trustedHandshakeResultForActivation(now), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.ValidateFor("controller-2", "agent-1", "session-activation-000001", now.Add(2*time.Second)); err == nil {
		t.Fatal("expected wrong controller to be rejected")
	}
}

func TestTrustedActivationLeaseScreenOnlyDoesNotAllowInput(t *testing.T) {
	now := time.Now().UTC()
	result := trustedHandshakeResultForActivation(now)
	result.Authorization.Mouse = false
	result.Authorization.Keyboard = false
	result.Grant.Mouse = false
	result.Grant.Keyboard = false
	result.Binding.Mouse = false
	result.Binding.Keyboard = false
	lease, err := NewTrustedActivationLease(result, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if lease.AllowsInput() {
		t.Fatal("screen-only lease must not authorize input")
	}
}
