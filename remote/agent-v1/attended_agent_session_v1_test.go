package remoteagent

import (
	"context"
	"testing"
	"time"
)

func preparedAttendedRuntimeFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, *ControlDispatcher, *AttendedControlRuntime) {
	t.Helper()
	arbiter, permit, result, consent := runtimeFixture(t, now)
	dispatcher, err := NewControlDispatcher(&fakeControlSink{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runtime, err := PrepareAttendedControlRuntime(ctx, AttendedControlRuntimeConfig{
		Arbiter: arbiter, Permit: permit, ApprovalResult: result,
		Consent: consent, ControllerLabel: "Owner browser", Dispatcher: dispatcher,
		Now: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return arbiter, dispatcher, runtime
}

func TestNewAttendedAgentSessionBindsRuntimeGateAndKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, runtime := preparedAttendedRuntimeFixture(t, now)

	bound, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: Config{Signal: func(string, any) error { return nil }},
		Now:     now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if bound.Session == nil || bound.Runtime != runtime {
		t.Fatal("attended session binding did not preserve prepared runtime")
	}
	if bound.Session.controlDispatch != dispatcher {
		t.Fatal("agent session dispatcher is not the attended runtime dispatcher")
	}
	if bound.Session.controlGate == nil {
		t.Fatal("agent session must inherit attended runtime control gate")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("session binding must not activate dispatcher")
	}
}

func TestNewAttendedAgentSessionRejectsForeignDispatcher(t *testing.T) {
	now := time.Now().UTC()
	_, _, runtime := preparedAttendedRuntimeFixture(t, now)
	foreign, _ := NewControlDispatcher(&fakeControlSink{})
	_, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: Config{
			Signal:  func(string, any) error { return nil },
			Control: foreign,
		},
		Now: now.Add(5 * time.Second),
	})
	if err == nil {
		t.Fatal("foreign dispatcher substitution must fail closed")
	}
}

func TestNewAttendedAgentSessionRejectsCallerSuppliedGate(t *testing.T) {
	now := time.Now().UTC()
	_, _, runtime := preparedAttendedRuntimeFixture(t, now)
	_, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: Config{
			Signal:      func(string, any) error { return nil },
			ControlGate: runtime.ControlGate(),
		},
		Now: now.Add(5 * time.Second),
	})
	if err == nil {
		t.Fatal("caller-supplied control gate must be rejected; runtime owns the gate")
	}
}

func TestNewAttendedAgentSessionRejectsInvalidatedRuntime(t *testing.T) {
	now := time.Now().UTC()
	arbiter, dispatcher, runtime := preparedAttendedRuntimeFixture(t, now)
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: Config{Signal: func(string, any) error { return nil }},
		Now:     now.Add(6 * time.Second),
	})
	if err == nil {
		t.Fatal("invalidated attended runtime must not bind to an agent session")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("invalidated runtime must leave dispatcher inactive")
	}
}

func TestAttendedAgentSessionCloseKeepsDispatcherFailClosed(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, runtime := preparedAttendedRuntimeFixture(t, now)
	bound, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: Config{Signal: func(string, any) error { return nil }},
		Now:     now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bound.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("attended session close must leave dispatcher inactive")
	}
}
