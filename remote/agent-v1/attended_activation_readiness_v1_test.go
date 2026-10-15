package remoteagent

import (
	"context"
	"testing"
	"time"
)

func readinessRuntimeFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, *ControlDispatcher, *fakeControlSink, *AttendedControlRuntime) {
	t.Helper()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	sink := &fakeControlSink{}
	dispatcher, err := NewControlDispatcher(sink)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := PrepareAttendedControlRuntime(context.Background(), AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, dispatcher, sink, runtime
}

func TestPrepareAttendedActivationReadinessKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, sink, runtime := readinessRuntimeFixture(t, now)
	defer runtime.Close()

	readiness, err := PrepareAttendedActivationReadiness(runtime, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if readiness.ControllerID == "" || readiness.AgentID == "" || readiness.SessionID == "" || readiness.ApprovalID == "" {
		t.Fatal("readiness proof identity is incomplete")
	}
	if !readiness.Mouse && !readiness.Keyboard {
		t.Fatal("readiness proof must preserve approved input capability")
	}
	if err := readiness.Validate(runtime, now.Add(6*time.Second)); err != nil {
		t.Fatalf("fresh readiness proof should validate: %v", err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("readiness proof must not activate dispatcher")
	}
	if sink.batches != 0 {
		t.Fatal("readiness proof must not reach control sink")
	}
}

func TestAttendedActivationReadinessFailsAfterRuntimeClose(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, _, runtime := readinessRuntimeFixture(t, now)
	readiness, err := PrepareAttendedActivationReadiness(runtime, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := readiness.Validate(runtime, now.Add(6*time.Second)); err == nil {
		t.Fatal("closed attended runtime must invalidate readiness proof")
	}
	if _, err := PrepareAttendedActivationReadiness(runtime, now.Add(6*time.Second)); err == nil {
		t.Fatal("closed attended runtime must not mint a new readiness proof")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("runtime close must leave dispatcher inactive")
	}
}

func TestAttendedActivationReadinessFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, dispatcher, _, runtime := readinessRuntimeFixture(t, now)
	defer runtime.Close()
	readiness, err := PrepareAttendedActivationReadiness(runtime, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := readiness.Validate(runtime, now.Add(7*time.Second)); err == nil {
		t.Fatal("emergency stop must invalidate activation readiness")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("emergency stop path must leave dispatcher inactive")
	}
}

func TestAttendedActivationReadinessUsesShorterExpiry(t *testing.T) {
	now := time.Now().UTC()
	_, _, _, runtime := readinessRuntimeFixture(t, now)
	defer runtime.Close()
	readiness, err := PrepareAttendedActivationReadiness(runtime, now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if readiness.ExpiresAt.Sub(readiness.IssuedAt) > AttendedActivationReadinessTTL {
		t.Fatal("readiness proof exceeded bounded TTL")
	}
	if err := readiness.Validate(runtime, readiness.ExpiresAt); err == nil {
		t.Fatal("readiness proof must be invalid at expiry")
	}
}
