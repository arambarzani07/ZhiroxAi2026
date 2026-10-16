package remoteagent

import (
	"testing"
	"time"
)

func attendedMessageGateFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, TrustedControlPermit, LocalInputApproval, AttendedControlAuthorization) {
	t.Helper()
	arbiter, permit, _ := attendedFixture(t, now)
	approval, err := NewLocalInputApproval(permit, "approval-message-gate-1", true, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := AuthorizeAttendedControl(arbiter, permit, approval, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, permit, approval, authorization
}

func TestAttendedControlMessageGateAllowsCurrentApprovedSession(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval, authorization := attendedMessageGateFixture(t, now)
	gate := &AttendedControlMessageGate{
		Arbiter: arbiter, Permit: permit, Approval: approval, Authorization: authorization,
	}
	if err := gate.ValidateControlMessage(now.Add(4 * time.Second)); err != nil {
		t.Fatalf("expected current attended authorization to validate: %v", err)
	}
}

func TestAttendedControlMessageGateFailsClosedAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval, authorization := attendedMessageGateFixture(t, now)
	gate := &AttendedControlMessageGate{
		Arbiter: arbiter, Permit: permit, Approval: approval, Authorization: authorization,
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := gate.ValidateControlMessage(now.Add(5 * time.Second)); err == nil {
		t.Fatal("expected emergency stop to invalidate per-message authorization")
	}
}

func TestAttendedControlMessageGateFailsClosedWhenLocalApprovalExpires(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, approval, authorization := attendedMessageGateFixture(t, now)
	approval.ExpiresAt = now.Add(4 * time.Second)
	gate := &AttendedControlMessageGate{
		Arbiter: arbiter, Permit: permit, Approval: approval, Authorization: authorization,
	}
	if err := gate.ValidateControlMessage(now.Add(5 * time.Second)); err == nil {
		t.Fatal("expected expired local approval to invalidate per-message authorization")
	}
}

func TestAttendedControlMessageGateRejectsMissingArbiter(t *testing.T) {
	gate := &AttendedControlMessageGate{}
	if err := gate.ValidateControlMessage(time.Now().UTC()); err == nil {
		t.Fatal("expected missing arbiter to fail closed")
	}
}
