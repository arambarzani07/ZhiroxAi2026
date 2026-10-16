package remoteagent

import (
	"testing"
	"time"
)

type fakeReviewedAttendedExecutor struct {
	id       string
	mouse    bool
	keyboard bool
}

func (f fakeReviewedAttendedExecutor) ExecutorID() string { return f.id }

func (f fakeReviewedAttendedExecutor) SupportsAttendedCapabilities(mouse, keyboard bool) bool {
	if mouse && !f.mouse {
		return false
	}
	if keyboard && !f.keyboard {
		return false
	}
	return mouse || keyboard
}

func executionReviewFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, *ControlDispatcher, *fakeControlSink, *AttendedControlRuntime, AttendedActivationGrant) {
	t.Helper()
	arbiter, dispatcher, sink, runtime := readinessRuntimeFixture(t, now)
	readiness, err := PrepareAttendedActivationReadiness(runtime, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	grant, err := NewAttendedActivationConsumer().Consume(runtime, readiness, now.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, dispatcher, sink, runtime, grant
}

func TestPrepareReviewedAttendedExecutionKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, sink, runtime, grant := executionReviewFixture(t, now)
	defer runtime.Close()

	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if review.ReviewID == "" || review.ExecutorID != executor.id || review.ContractVersion != AttendedExecutionContractV1 {
		t.Fatal("reviewed execution binding is incomplete")
	}
	if review.ControllerID != grant.ControllerID || review.AgentID != grant.AgentID || review.SessionID != grant.SessionID || review.GrantReadinessID != grant.ReadinessID {
		t.Fatal("reviewed execution binding lost grant identity")
	}
	if err := review.Validate(runtime, grant, executor, now.Add(8*time.Second)); err != nil {
		t.Fatalf("fresh reviewed execution binding should validate: %v", err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("reviewed execution binding must not activate dispatcher")
	}
	if sink.batches != 0 {
		t.Fatal("reviewed execution binding must not reach control sink")
	}
}

func TestPrepareReviewedAttendedExecutionRejectsExecutorCapabilityMismatch(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, _, runtime, grant := executionReviewFixture(t, now)
	defer runtime.Close()

	executor := fakeReviewedAttendedExecutor{id: "mouse-only-executor", mouse: true, keyboard: false}
	if grant.Keyboard {
		if _, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second)); err == nil {
			t.Fatal("executor missing keyboard capability must be rejected")
		}
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("capability mismatch must leave dispatcher inactive")
	}
}

func TestReviewedAttendedExecutionRejectsExecutorSubstitution(t *testing.T) {
	now := time.Now().UTC()
	_, _, _, runtime, grant := executionReviewFixture(t, now)
	defer runtime.Close()

	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	substitute := fakeReviewedAttendedExecutor{id: "different-executor", mouse: true, keyboard: true}
	if err := review.Validate(runtime, grant, substitute, now.Add(8*time.Second)); err == nil {
		t.Fatal("executor identity substitution must be rejected")
	}
}

func TestReviewedAttendedExecutionFailsAfterRuntimeClose(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, _, runtime, grant := executionReviewFixture(t, now)
	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := review.Validate(runtime, grant, executor, now.Add(8*time.Second)); err == nil {
		t.Fatal("closed runtime must invalidate reviewed execution binding")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("runtime close must leave dispatcher inactive")
	}
}

func TestReviewedAttendedExecutionUsesShorterExpiry(t *testing.T) {
	now := time.Now().UTC()
	_, _, _, runtime, grant := executionReviewFixture(t, now)
	defer runtime.Close()

	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if review.ExpiresAt.Sub(review.IssuedAt) > AttendedExecutionReviewTTL {
		t.Fatal("reviewed execution binding exceeded bounded TTL")
	}
	if err := review.Validate(runtime, grant, executor, review.ExpiresAt); err == nil {
		t.Fatal("reviewed execution binding must be invalid at expiry")
	}
}

func TestReviewedAttendedExecutionFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, dispatcher, _, runtime, grant := executionReviewFixture(t, now)
	defer runtime.Close()

	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := review.Validate(runtime, grant, executor, now.Add(9*time.Second)); err == nil {
		t.Fatal("emergency stop must invalidate reviewed execution binding")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("emergency stop must leave dispatcher inactive")
	}
}
