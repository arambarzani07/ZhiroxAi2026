package remoteagent

import (
	"strings"
	"testing"
	"time"
)

func attendedFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, TrustedControlPermit, LocalInputApproval) {
	t.Helper()
	arbiter, err := NewTrustedSessionArbiter(&memoryRevocationChecker{})
	if err != nil {
		t.Fatal(err)
	}
	lease := arbiterLease("controller-1", "session-000000000000000001", now)
	active, err := arbiter.Acquire(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := NewTrustedControlPermit(arbiter, active, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	approval := LocalInputApproval{
		ApprovalID:      "local-approval-1",
		ControllerID:    permit.ControllerID,
		AgentID:         permit.AgentID,
		SessionID:       permit.SessionID,
		Fingerprint:     permit.Fingerprint,
		Mouse:           permit.Mouse,
		Keyboard:        permit.Keyboard,
		InteractiveUser: true,
		ApprovedAt:      now.Add(2 * time.Second),
		ExpiresAt:       now.Add(90 * time.Second),
	}
	return arbiter, permit, approval
}

func TestAttendedGateRequiresInteractiveLocalApproval(t *testing.T) {
	now := time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)
	arbiter, permit, approval := attendedFixture(t, now)
	approval.InteractiveUser = false
	if _, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected explicit interactive local approval to be required")
	}
}

func TestAttendedGateAuthorizesExactApprovedSession(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	auth, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Mouse || !auth.Keyboard || auth.ApprovalID != approval.ApprovalID {
		t.Fatalf("unexpected attended authorization: %+v", auth)
	}
	if !auth.ExpiresAt.Equal(approval.ExpiresAt) {
		t.Fatalf("expected shorter local approval expiry to win: %+v", auth)
	}
	if err := auth.Validate(arbiter, permit, approval, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestAttendedGateRejectsIdentityDrift(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	approval.SessionID = "session-000000000000000099"
	if _, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected local approval identity mismatch to fail")
	}
}

func TestAttendedGateRejectsCapabilityDrift(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	approval.Keyboard = false
	if _, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected missing matching keyboard approval to fail")
	}
}

func TestAttendedGateRejectsApprovalOverTTL(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	approval.ExpiresAt = approval.ApprovedAt.Add(LocalInputApprovalTTL + 10*time.Second)
	if _, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected excessive local approval TTL to fail")
	}
}

func TestAttendedGateExpiresWithLocalApproval(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	auth, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Validate(arbiter, permit, approval, approval.ExpiresAt); err == nil {
		t.Fatal("expected expired local approval to invalidate authorization")
	}
}

func TestAttendedGateFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	auth, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local user requested stop", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := auth.Validate(arbiter, permit, approval, now.Add(5*time.Second)); err == nil {
		t.Fatal("expected emergency stop to invalidate attended authorization")
	}
}

func TestAttendedGateRejectsFingerprintDrift(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval := attendedFixture(t, now)
	approval.Fingerprint = strings.Repeat("b", 64)
	if _, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second)); err == nil {
		t.Fatal("expected local approval fingerprint mismatch to fail")
	}
}
