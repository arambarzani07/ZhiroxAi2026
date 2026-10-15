package remoteagent

import (
	"context"
	"testing"
	"time"
)

func TestPrepareAttendedAgentSessionComposesApprovedPathWithoutActivation(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	dispatcher, err := NewControlDispatcher(&fakeControlSink{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bound, err := PrepareAttendedAgentSession(AttendedSessionOrchestratorConfig{
		Context:         ctx,
		Arbiter:         arbiter,
		Permit:          permit,
		Consent:         consent,
		ControllerLabel: "Owner browser",
		ApprovalID:      "approval-full-orchestrator-1",
		Dispatcher:      dispatcher,
		Session: Config{
			Signal: func(string, any) error { return nil },
		},
		Now: now.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if consent.approveCalls != 1 || consent.indicatorCalls != 1 {
		t.Fatalf("expected one approval and one visible indicator, got approval=%d indicator=%d", consent.approveCalls, consent.indicatorCalls)
	}
	if bound.Session == nil || bound.Runtime == nil {
		t.Fatal("attended session orchestration did not bind runtime and WebRTC session")
	}
	if bound.Session.controlDispatch != dispatcher {
		t.Fatal("WebRTC session must use the orchestrator dispatcher")
	}
	if bound.Session.controlGate == nil {
		t.Fatal("WebRTC session must use the runtime-owned attended gate")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("full attended orchestration must still leave dispatcher inactive")
	}
}

func TestPrepareAttendedAgentSessionDenialFailsBeforeIndicator(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: false}
	dispatcher, _ := NewControlDispatcher(&fakeControlSink{})

	_, err := PrepareAttendedAgentSession(AttendedSessionOrchestratorConfig{
		Context:         context.Background(),
		Arbiter:         arbiter,
		Permit:          permit,
		Consent:         consent,
		ControllerLabel: "Owner browser",
		ApprovalID:      "approval-denied-full-orchestrator",
		Dispatcher:      dispatcher,
		Session: Config{Signal: func(string, any) error { return nil }},
		Now: now.Add(3 * time.Second),
	})
	if err == nil {
		t.Fatal("local denial must fail the attended session orchestration")
	}
	if consent.indicatorCalls != 0 {
		t.Fatal("visible active-control indicator must not start after local denial")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("dispatcher must remain inactive after local denial")
	}
}

func TestPrepareAttendedAgentSessionSessionConstructionFailureFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	dispatcher, _ := NewControlDispatcher(&fakeControlSink{})

	_, err := PrepareAttendedAgentSession(AttendedSessionOrchestratorConfig{
		Context:         context.Background(),
		Arbiter:         arbiter,
		Permit:          permit,
		Consent:         consent,
		ControllerLabel: "Owner browser",
		ApprovalID:      "approval-session-failure",
		Dispatcher:      dispatcher,
		Session:         Config{}, // Missing Signal forces NewAgentSession to fail.
		Now:             now.Add(3 * time.Second),
	})
	if err == nil {
		t.Fatal("invalid WebRTC session config must fail closed")
	}
	if consent.approveCalls != 1 || consent.indicatorCalls != 1 {
		t.Fatal("session construction failure should occur only after attended approval and indicator preparation")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("session construction failure must leave dispatcher inactive")
	}
}

func TestPrepareAttendedAgentSessionEmergencyStopFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	arbiter, permit, _ := attendedFixture(t, now)
	consent := &fakeRemoteInputConsent{approve: true}
	dispatcher, _ := NewControlDispatcher(&fakeControlSink{})
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	_, err := PrepareAttendedAgentSession(AttendedSessionOrchestratorConfig{
		Context:         context.Background(),
		Arbiter:         arbiter,
		Permit:          permit,
		Consent:         consent,
		ControllerLabel: "Owner browser",
		ApprovalID:      "approval-emergency-full-orchestrator",
		Dispatcher:      dispatcher,
		Session: Config{Signal: func(string, any) error { return nil }},
		Now: now.Add(3 * time.Second),
	})
	if err == nil {
		t.Fatal("emergency stop must block attended session orchestration")
	}
	if consent.approveCalls != 0 || consent.indicatorCalls != 0 {
		t.Fatal("invalid trusted permit must fail before local consent or indicator")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("emergency stop must leave dispatcher inactive")
	}
}
