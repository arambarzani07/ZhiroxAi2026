package remoteagent

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingAttendedAuditSink struct {
	mu        sync.Mutex
	events    []AttendedExecutionAuditEvent
	failOpen  bool
	failClose bool
}

func (s *recordingAttendedAuditSink) RecordAttendedExecutionAudit(event AttendedExecutionAuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.EventType == AttendedMockAuditSessionOpened && s.failOpen {
		return errors.New("open audit failed")
	}
	if event.EventType == AttendedMockAuditSessionClosed && s.failClose {
		return errors.New("close audit failed")
	}
	s.events = append(s.events, event)
	return nil
}

func (s *recordingAttendedAuditSink) snapshot() []AttendedExecutionAuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AttendedExecutionAuditEvent(nil), s.events...)
}

func mockObservationFixture(t *testing.T, now time.Time, audit AttendedExecutionAuditSink) (*TrustedSessionArbiter, *ControlDispatcher, *fakeControlSink, *AttendedControlRuntime, AttendedActivationGrant, AttendedExecutionReview, *MockReviewedAttendedExecutor, *AttendedExecutionSession) {
	t.Helper()
	arbiter, dispatcher, sink, runtime, grant, _, review := executionRequestFixture(t, now)
	adapter, err := NewMockReviewedAttendedExecutor("reviewed-windows-input-v1", true, true, audit)
	if err != nil {
		t.Fatal(err)
	}
	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, adapter, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewAttendedExecutionRequestConsumer().Consume(runtime, grant, review, adapter, request, now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, dispatcher, sink, runtime, grant, review, adapter, session
}

func TestMockAttendedExecutionObservationAuditsLifecycleAndKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{}
	_, dispatcher, sink, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.Validate(now.Add(10 * time.Second)); err != nil {
		t.Fatalf("fresh mock observation should validate: %v", err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("mock executor lifecycle must not activate dispatcher")
	}
	if sink.batches != 0 {
		t.Fatal("mock executor lifecycle must not reach control sink")
	}
	if err := observation.Close(now.Add(11 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := observation.Close(now.Add(12 * time.Second)); err != nil {
		t.Fatal("close must be idempotent")
	}

	events := audit.snapshot()
	if len(events) != 2 {
		t.Fatalf("expected open and close audit events, got %d", len(events))
	}
	if events[0].EventType != AttendedMockAuditSessionOpened || events[1].EventType != AttendedMockAuditSessionClosed {
		t.Fatal("unexpected mock execution audit lifecycle")
	}
	if events[0].ExecutionSessionID != session.ExecutionSessionID || events[1].ExecutionSessionID != session.ExecutionSessionID {
		t.Fatal("audit events lost execution-session identity")
	}
	if events[0].ControllerID != session.ControllerID || events[0].AgentID != session.AgentID || events[0].SessionID != session.SessionID {
		t.Fatal("audit event lost attended identity binding")
	}
}

func TestMockAttendedExecutionOpenAuditFailureClosesSession(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{failOpen: true}
	_, dispatcher, _, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	if _, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second)); err == nil {
		t.Fatal("audit failure must fail closed")
	}
	if err := session.Validate(runtime, grant, review, adapter, now.Add(10*time.Second)); err == nil {
		t.Fatal("audit failure must close the execution session")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("audit failure must leave dispatcher inactive")
	}
}

func TestMockAttendedExecutionCloseAuditFailureStillClosesSession(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{failClose: true}
	_, dispatcher, _, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.Close(now.Add(11 * time.Second)); err == nil {
		t.Fatal("close audit failure should be surfaced")
	}
	if err := session.Validate(runtime, grant, review, adapter, now.Add(11*time.Second)); err == nil {
		t.Fatal("session must remain closed even when close audit fails")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("close audit failure must leave dispatcher inactive")
	}
}

func TestMockAttendedExecutionRejectsExecutorSubstitution(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{}
	_, dispatcher, _, runtime, grant, review, _, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	substitute, err := NewMockReviewedAttendedExecutor("different-executor", true, true, audit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMockAttendedExecutionObservation(runtime, grant, review, substitute, session, now.Add(10*time.Second)); err == nil {
		t.Fatal("executor substitution must fail closed")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("executor substitution must leave dispatcher inactive")
	}
}

func TestMockAttendedExecutionFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{}
	arbiter, dispatcher, _, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := observation.Validate(now.Add(11 * time.Second)); err == nil {
		t.Fatal("emergency stop must invalidate mock execution observation")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("emergency stop must leave dispatcher inactive")
	}
}

func TestNewMockReviewedAttendedExecutorRejectsUnsafeMetadataConfiguration(t *testing.T) {
	audit := &recordingAttendedAuditSink{}
	if _, err := NewMockReviewedAttendedExecutor("", true, false, audit); err == nil {
		t.Fatal("empty executor id must be rejected")
	}
	if _, err := NewMockReviewedAttendedExecutor("mock", false, false, audit); err == nil {
		t.Fatal("executor with no attended input capability must be rejected")
	}
	if _, err := NewMockReviewedAttendedExecutor("mock", true, false, nil); err == nil {
		t.Fatal("missing audit sink must be rejected")
	}
}
