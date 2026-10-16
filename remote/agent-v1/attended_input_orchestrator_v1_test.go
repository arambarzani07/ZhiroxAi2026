package remoteagent

import (
	"testing"
	"time"
)

type fakeRemoteInputConsent struct {
	approve        bool
	approveErr     error
	approveCalls   int
	indicatorCalls int
	lastMouse      bool
	lastKeyboard   bool
	lastFingerprint string
	indicator      SessionIndicator
}

func (f *fakeRemoteInputConsent) ApproveInput(controllerLabel, controllerFingerprint string, mouse, keyboard bool) (bool, error) {
	f.approveCalls++
	f.lastMouse = mouse
	f.lastKeyboard = keyboard
	f.lastFingerprint = controllerFingerprint
	if f.approveErr != nil {
		return false, f.approveErr
	}
	return f.approve, nil
}

func (f *fakeRemoteInputConsent) StartInputIndicator(controllerLabel string, mouse, keyboard bool) (SessionIndicator, error) {
	f.indicatorCalls++
	f.lastMouse = mouse
	f.lastKeyboard = keyboard
	if f.indicator != nil {
		return f.indicator, nil
	}
	return &fakeInputIndicator{done: make(chan error)}, nil
}

type fakeInputIndicator struct {
	done chan error
}

func (f *fakeInputIndicator) Done() <-chan error { return f.done }
func (f *fakeInputIndicator) Stop() error        { return nil }

func TestAttendedInputOrchestratorBindsExplicitConsentToPermit(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	result, err := RequestAttendedInputAuthorization(
		arbiter,
		permit,
		consent,
		"Owner browser",
		"local-approval-orchestrated-1",
		now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if consent.approveCalls != 1 {
		t.Fatalf("expected one explicit local approval prompt, got %d", consent.approveCalls)
	}
	if consent.lastMouse != permit.Mouse || consent.lastKeyboard != permit.Keyboard {
		t.Fatalf("consent capabilities drifted: mouse=%v keyboard=%v", consent.lastMouse, consent.lastKeyboard)
	}
	if consent.lastFingerprint != permit.Fingerprint {
		t.Fatalf("consent fingerprint mismatch: %q != %q", consent.lastFingerprint, permit.Fingerprint)
	}
	if result.Approval.SessionID != permit.SessionID || result.Authorization.SessionID != permit.SessionID {
		t.Fatal("approval and authorization must remain bound to the trusted session")
	}
	if !result.Approval.InteractiveUser {
		t.Fatal("orchestrated approval must record explicit interactive local consent")
	}
}

func TestAttendedInputOrchestratorRejectsLocalDenial(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: false}
	if _, err := RequestAttendedInputAuthorization(arbiter, permit, consent, "Owner browser", "approval-denied", now.Add(3*time.Second)); err == nil {
		t.Fatal("expected local denial to fail closed")
	}
}

func TestAttendedInputIndicatorStartsOnlyAfterAuthorizationRevalidation(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	result, err := RequestAttendedInputAuthorization(arbiter, permit, consent, "Owner browser", "approval-indicator-1", now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StartAttendedInputIndicator(arbiter, permit, consent, result, "Owner browser", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if consent.indicatorCalls != 1 {
		t.Fatalf("expected one visible input indicator, got %d", consent.indicatorCalls)
	}
}

func TestAttendedInputIndicatorFailsClosedAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	result, err := RequestAttendedInputAuthorization(arbiter, permit, consent, "Owner browser", "approval-emergency-1", now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := StartAttendedInputIndicator(arbiter, permit, consent, result, "Owner browser", now.Add(5*time.Second)); err == nil {
		t.Fatal("expected emergency stop to block attended input indicator")
	}
	if consent.indicatorCalls != 0 {
		t.Fatal("indicator must not start after authorization becomes invalid")
	}
}

func TestNewLocalInputApprovalUsesShorterPermitExpiry(t *testing.T) {
	now := time.Now().UTC()
	_, permit, _ := attendedFixture(t, now)
	permit.ExpiresAt = now.Add(30 * time.Second)
	approval, err := NewLocalInputApproval(permit, "approval-short-expiry", true, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !approval.ExpiresAt.Equal(permit.ExpiresAt) {
		t.Fatalf("expected permit expiry to bound local approval: %v != %v", approval.ExpiresAt, permit.ExpiresAt)
	}
}
