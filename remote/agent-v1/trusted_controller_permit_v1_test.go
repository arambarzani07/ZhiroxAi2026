package remoteagent

import (
	"testing"
	"time"
)

func TestTrustedControlPermitRequiresLiveInputLease(t *testing.T) {
	now := time.Date(2026, 10, 15, 15, 0, 0, 0, time.UTC)
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	active, err := arbiter.Acquire(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !permit.Mouse || !permit.Keyboard || permit.ArbiterGeneration != active.Generation {
		t.Fatalf("expected input-bound permit: %+v", permit)
	}
	if err := permit.Validate(arbiter, now.Add(2*time.Second)); err != nil {
		t.Fatalf("expected live permit validation: %v", err)
	}
}

func TestTrustedControlPermitRejectsScreenOnlyLease(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	lease.Mouse = false
	lease.Keyboard = false
	active, err := arbiter.Acquire(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second)); err == nil {
		t.Fatal("expected screen-only lease to be denied an input permit")
	}
}

func TestTrustedControlPermitInvalidAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	active, _ := arbiter.Acquire(lease, now)
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("owner emergency stop", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := permit.Validate(arbiter, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected emergency stop to invalidate control permit")
	}
}

func TestTrustedControlPermitInvalidAfterReleaseAndReplacement(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	active, _ := arbiter.Acquire(lease, now)
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := arbiter.Release(lease.ControllerID, lease.AgentID, lease.SessionID); err != nil {
		t.Fatal(err)
	}
	replacement := arbiterLease("controller-2", "session-000000000000000002", now.Add(2*time.Second))
	if _, err := arbiter.Acquire(replacement, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := permit.Validate(arbiter, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected released/replaced generation to invalidate old permit")
	}
}

func TestTrustedControlPermitInvalidAfterRevocation(t *testing.T) {
	now := time.Now().UTC()
	checker := &memoryRevocationChecker{revoked: map[string]bool{}}
	arbiter, _ := NewTrustedSessionArbiter(checker)
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	active, _ := arbiter.Acquire(lease, now)
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	checker.revoked[lease.ControllerID+":"+lease.Fingerprint] = true
	if err := permit.Validate(arbiter, now.Add(2*time.Second)); err == nil {
		t.Fatal("expected controller revocation to invalidate permit")
	}
}

func TestTrustedControlPermitExpiresWithLease(t *testing.T) {
	now := time.Now().UTC()
	arbiter, _ := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	lease.ExpiresAt = now.Add(2 * time.Second)
	active, _ := arbiter.Acquire(lease, now)
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.Validate(arbiter, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected expired lease to invalidate permit")
	}
}
