package remoteagent

import (
	"context"
	"testing"
	"time"
)

type runtimeIndicator struct {
	done      chan error
	stopCalls int
}

func (i *runtimeIndicator) Done() <-chan error { return i.done }
func (i *runtimeIndicator) Stop() error {
	i.stopCalls++
	return nil
}

func runtimeFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, TrustedControlPermit, AttendedInputApprovalResult, *fakeRemoteInputConsent) {
	t.Helper()
	arbiter, permit, _ := attendedFixture(t, now)
	indicator := &runtimeIndicator{done: make(chan error, 1)}
	consent := &fakeRemoteInputConsent{approve: true, indicator: indicator}
	result, err := RequestAttendedInputAuthorization(
		arbiter,
		permit,
		consent,
		"Owner browser",
		"approval-runtime-1",
		now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, permit, result, consent
}

func TestPrepareAttendedControlRuntimeKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	sink := &fakeControlSink{}
	dispatcher, err := NewControlDispatcher(sink)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := PrepareAttendedControlRuntime(ctx, AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if runtime.ControlGate() == nil {
		t.Fatal("runtime must expose attended per-message gate")
	}
	if err := runtime.ControlGate().ValidateControlMessage(now.Add(5 * time.Second)); err != nil {
		t.Fatalf("prepared runtime gate should be valid: %v", err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("prepared runtime must keep dispatcher inactive")
	}
	if sink.batches != 0 {
		t.Fatal("inactive runtime reached control sink")
	}
}

func TestPrepareAttendedControlRuntimeDeactivatesReusedDispatcher(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	dispatcher, _ := NewControlDispatcher(&fakeControlSink{})
	dispatcher.Activate()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime, err := PrepareAttendedControlRuntime(ctx, AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("runtime preparation must fail closed a reused active dispatcher")
	}
}

func TestAttendedControlRuntimeIndicatorExitDeactivatesDispatcher(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	sink := &fakeControlSink{}
	dispatcher, _ := NewControlDispatcher(sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := PrepareAttendedControlRuntime(ctx, AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	indicator := consent.indicator.(*runtimeIndicator)

	// Simulate a separately reviewed future activation phase. The runtime must
	// still tear the dispatcher down as soon as the visible indicator exits.
	dispatcher.Activate()
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err != nil {
		t.Fatalf("test setup activation failed: %v", err)
	}
	indicator.done <- nil
	close(indicator.done)
	select {
	case err := <-runtime.Done():
		if err != nil {
			t.Fatalf("unexpected runtime shutdown error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime did not stop after indicator exit")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 2)); err == nil {
		t.Fatal("indicator exit must deactivate dispatcher")
	}
	if dispatcher.LastSequence() != 0 {
		t.Fatal("runtime teardown must reset dispatcher sequence")
	}
}

func TestAttendedControlRuntimeGateFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	dispatcher, _ := NewControlDispatcher(&fakeControlSink{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := PrepareAttendedControlRuntime(ctx, AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ControlGate().ValidateControlMessage(now.Add(6 * time.Second)); err == nil {
		t.Fatal("emergency stop must invalidate runtime control gate")
	}
}
