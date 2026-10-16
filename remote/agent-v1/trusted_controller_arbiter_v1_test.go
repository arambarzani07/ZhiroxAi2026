package remoteagent

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryRevocationChecker struct {
	revoked map[string]bool
	fail    bool
}

func (m *memoryRevocationChecker) IsTrustedControllerRevoked(controllerID, fingerprint string) (bool, error) {
	if m.fail {
		return false, errors.New("revocation backend unavailable")
	}
	return m.revoked[controllerID+":"+fingerprint], nil
}

func arbiterLease(controllerID, sessionID string, now time.Time) TrustedActivationLease {
	return TrustedActivationLease{
		ControllerID: controllerID,
		AgentID:      "Server-c24b5c0f",
		SessionID:    sessionID,
		Fingerprint:  strings.Repeat("a", 64),
		Screen:       true,
		Mouse:        true,
		Keyboard:     true,
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}
}

func TestTrustedArbiterAllowsOnlyOneLiveSession(t *testing.T) {
	now := time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)
	arbiter, err := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	if err != nil {
		t.Fatal(err)
	}
	first := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(first, now); err != nil {
		t.Fatal(err)
	}
	second := arbiterLease("controller-2", "session-000000000000000002", now)
	if _, err := arbiter.Acquire(second, now.Add(time.Second)); err == nil {
		t.Fatal("expected second live controller session to be rejected")
	}
	if err := arbiter.ValidateActive(first.ControllerID, first.AgentID, first.SessionID, first.Fingerprint, now.Add(2*time.Second)); err != nil {
		t.Fatalf("first holder should remain active: %v", err)
	}
}

func TestTrustedArbiterReleasesExactHolder(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(lease, now); err != nil {
		t.Fatal(err)
	}
	if err := arbiter.Release("controller-1", lease.AgentID, "session-wrong"); err == nil {
		t.Fatal("expected mismatched release to fail")
	}
	if err := arbiter.Release(lease.ControllerID, lease.AgentID, lease.SessionID); err != nil {
		t.Fatal(err)
	}
	if arbiter.Status(now.Add(time.Second)).Active != nil {
		t.Fatal("expected active holder to be cleared")
	}
}

func TestTrustedArbiterExpiresOldLeaseBeforeNewAcquire(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	first := arbiterLease("controller-1", "session-000000000000000001", now)
	first.ExpiresAt = now.Add(2 * time.Second)
	if _, err := arbiter.Acquire(first, now); err != nil {
		t.Fatal(err)
	}
	second := arbiterLease("controller-2", "session-000000000000000002", now.Add(3*time.Second))
	if _, err := arbiter.Acquire(second, now.Add(3*time.Second)); err != nil {
		t.Fatalf("expected expired holder to make room for new session: %v", err)
	}
}

func TestTrustedArbiterEmergencyStopClearsAndLatches(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(lease, now); err != nil {
		t.Fatal(err)
	}
	stop, err := arbiter.EngageEmergencyStop("owner requested stop", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !stop.Engaged || arbiter.Status(now.Add(2*time.Second)).Active != nil {
		t.Fatal("emergency stop must clear the active session and latch closed")
	}
	if _, err := arbiter.Acquire(lease, now.Add(3*time.Second)); err == nil {
		t.Fatal("acquisition must remain blocked while emergency stop is engaged")
	}
	if _, err := arbiter.ResetEmergencyStop("owner-auth-1", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.Acquire(lease, now.Add(5*time.Second)); err != nil {
		t.Fatalf("fresh acquisition should be possible after authorized reset: %v", err)
	}
}

func TestTrustedArbiterEmergencyResetRequiresOwnerAuthorization(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	if _, err := arbiter.EngageEmergencyStop("manual kill switch", now); err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.ResetEmergencyStop("", now.Add(time.Second)); err == nil {
		t.Fatal("expected reset without owner authorization to fail")
	}
}

func TestTrustedArbiterFailsClosedOnRevocationLookupFailure(t *testing.T) {
	now := time.Now().UTC()
	checker := &memoryRevocationChecker{fail: true}
	arbiter, _ := NewTrustedSessionArbiter(checker)
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(lease, now); err == nil {
		t.Fatal("expected revocation lookup failure to block acquisition")
	}
}

func TestTrustedArbiterRevocationInvalidatesActiveLease(t *testing.T) {
	now := time.Now().UTC()
	checker := &memoryRevocationChecker{revoked: map[string]bool{}}
	arbiter, _ := NewTrustedSessionArbiter(checker)
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(lease, now); err != nil {
		t.Fatal(err)
	}
	checker.revoked[lease.ControllerID+":"+lease.Fingerprint] = true
	if err := arbiter.ValidateActive(lease.ControllerID, lease.AgentID, lease.SessionID, lease.Fingerprint, now.Add(time.Second)); err == nil {
		t.Fatal("expected revoked active holder to fail validation")
	}
	if arbiter.Status(now.Add(2*time.Second)).Active != nil {
		t.Fatal("revoked active lease must be cleared")
	}
}

func TestTrustedArbiterExplicitInvalidationClearsMatchingController(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	if _, err := arbiter.Acquire(lease, now); err != nil {
		t.Fatal(err)
	}
	cleared, err := arbiter.InvalidateController(lease.ControllerID, lease.Fingerprint, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !cleared || arbiter.Status(now.Add(2*time.Second)).Active != nil {
		t.Fatal("matching invalidation must clear the active lease")
	}
}
