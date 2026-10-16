package remoteagent

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transientAttendedAuditSink struct {
	mu                 sync.Mutex
	events             []AttendedExecutionAuditEvent
	closeAttempts       []string
	failCloseRemaining int
}

func (s *transientAttendedAuditSink) RecordAttendedExecutionAudit(event AttendedExecutionAuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.EventType == AttendedMockAuditSessionClosed {
		s.closeAttempts = append(s.closeAttempts, event.EventID)
		if s.failCloseRemaining > 0 {
			s.failCloseRemaining--
			return errors.New("transient close audit failure")
		}
	}
	s.events = append(s.events, event)
	return nil
}

func (s *transientAttendedAuditSink) snapshot() ([]AttendedExecutionAuditEvent, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AttendedExecutionAuditEvent(nil), s.events...), append([]string(nil), s.closeAttempts...)
}

func auditIntegrityFixture(t *testing.T, now time.Time) (*AttendedControlRuntime, *AttendedExecutionSession) {
	t.Helper()
	audit := &recordingAttendedAuditSink{}
	_, _, _, runtime, _, _, _, session := mockObservationFixture(t, now, audit)
	return runtime, session
}

func TestAttendedAuditIntegrityRejectsCloseBeforeOpen(t *testing.T) {
	now := time.Now().UTC()
	runtime, session := auditIntegrityFixture(t, now)
	defer runtime.Close()

	downstream := &recordingAttendedAuditSink{}
	guard, err := NewAttendedExecutionAuditIntegrityGuard(downstream)
	if err != nil {
		t.Fatal(err)
	}
	closeEvent, err := newAttendedMockAuditEvent(AttendedMockAuditSessionClosed, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordAttendedExecutionAudit(closeEvent); err == nil {
		t.Fatal("close without matching open must be rejected")
	}
	if len(downstream.snapshot()) != 0 {
		t.Fatal("invalid close must not reach downstream audit sink")
	}
}

func TestAttendedAuditIntegrityRejectsBindingDriftAndTimestampRegression(t *testing.T) {
	now := time.Now().UTC()
	runtime, session := auditIntegrityFixture(t, now)
	defer runtime.Close()

	downstream := &recordingAttendedAuditSink{}
	guard, err := NewAttendedExecutionAuditIntegrityGuard(downstream)
	if err != nil {
		t.Fatal(err)
	}
	openEvent, err := newAttendedMockAuditEvent(AttendedMockAuditSessionOpened, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordAttendedExecutionAudit(openEvent); err != nil {
		t.Fatal(err)
	}

	drifted, err := newAttendedMockAuditEvent(AttendedMockAuditSessionClosed, session, now.Add(11*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	drifted.ControllerID += "-drift"
	if err := guard.RecordAttendedExecutionAudit(drifted); err == nil {
		t.Fatal("audit identity drift must be rejected")
	}

	regressed, err := newAttendedMockAuditEvent(AttendedMockAuditSessionClosed, session, now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordAttendedExecutionAudit(regressed); err == nil {
		t.Fatal("audit timestamp regression must be rejected")
	}
	if len(downstream.snapshot()) != 1 {
		t.Fatal("invalid close events must not reach downstream audit sink")
	}
}

func TestAttendedAuditIntegrityDuplicateEventIsIdempotent(t *testing.T) {
	now := time.Now().UTC()
	runtime, session := auditIntegrityFixture(t, now)
	defer runtime.Close()

	downstream := &recordingAttendedAuditSink{}
	guard, err := NewAttendedExecutionAuditIntegrityGuard(downstream)
	if err != nil {
		t.Fatal(err)
	}
	event, err := newAttendedMockAuditEvent(AttendedMockAuditSessionOpened, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordAttendedExecutionAudit(event); err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordAttendedExecutionAudit(event); err != nil {
		t.Fatalf("exact duplicate event must be idempotent: %v", err)
	}
	if len(downstream.snapshot()) != 1 {
		t.Fatal("idempotent duplicate must not be written downstream twice")
	}

	collision := event
	collision.ControllerID += "-different"
	if err := guard.RecordAttendedExecutionAudit(collision); err == nil {
		t.Fatal("same EventID with different metadata must be rejected")
	}
}

func TestAttendedAuditIntegrityConcurrentDuplicateOpenIsSerialized(t *testing.T) {
	now := time.Now().UTC()
	runtime, session := auditIntegrityFixture(t, now)
	defer runtime.Close()

	downstream := &recordingAttendedAuditSink{}
	guard, err := NewAttendedExecutionAuditIntegrityGuard(downstream)
	if err != nil {
		t.Fatal(err)
	}
	event, err := newAttendedMockAuditEvent(AttendedMockAuditSessionOpened, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	var failures int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := guard.RecordAttendedExecutionAudit(event); err != nil {
				atomic.AddInt32(&failures, 1)
			}
		}()
	}
	wg.Wait()
	if failures != 0 {
		t.Fatalf("concurrent exact duplicates should all be idempotent, failures=%d", failures)
	}
	if len(downstream.snapshot()) != 1 {
		t.Fatal("concurrent duplicate open must produce exactly one downstream event")
	}
	events, sessions, openSessions := guard.SnapshotState()
	if events != 1 || sessions != 1 || openSessions != 1 {
		t.Fatalf("unexpected audit guard state: events=%d sessions=%d open=%d", events, sessions, openSessions)
	}
}

func TestMockObservationRetriesTransientCloseAuditWithStableEventID(t *testing.T) {
	now := time.Now().UTC()
	audit := &transientAttendedAuditSink{failCloseRemaining: 1}
	_, dispatcher, sink, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.Close(now.Add(10500 * time.Millisecond)); err == nil {
		t.Fatal("first transient close audit failure must be surfaced")
	}
	if err := session.Validate(runtime, grant, review, adapter, now.Add(10500*time.Millisecond)); err == nil {
		t.Fatal("session must already be closed after first teardown attempt")
	}
	if err := observation.Validate(now.Add(10500 * time.Millisecond)); err == nil {
		t.Fatal("observation must remain fail-closed while close audit is pending")
	}
	if err := observation.Close(now.Add(11 * time.Second)); err != nil {
		t.Fatalf("second close should retry and acknowledge audit: %v", err)
	}
	if err := observation.Close(now.Add(11500 * time.Millisecond)); err != nil {
		t.Fatalf("close after audit acknowledgement must be idempotent: %v", err)
	}

	events, attempts := audit.snapshot()
	if len(events) != 2 {
		t.Fatalf("expected one open and one successful close event, got %d", len(events))
	}
	if len(attempts) != 2 || attempts[0] == "" || attempts[0] != attempts[1] {
		t.Fatal("close audit retry must reuse the exact same stable EventID")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil || sink.batches != 0 {
		t.Fatal("teardown retry path must never activate or reach control sink")
	}
}

func TestMockObservationConcurrentCloseIsIdempotentAndFailClosed(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{}
	_, dispatcher, sink, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	var failures int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := observation.Close(now.Add(11 * time.Second)); err != nil {
				atomic.AddInt32(&failures, 1)
			}
		}()
	}
	wg.Wait()
	if failures != 0 {
		t.Fatalf("concurrent close must be idempotent, failures=%d", failures)
	}
	if err := observation.Validate(now.Add(11 * time.Second)); err == nil {
		t.Fatal("closed observation must never become valid again")
	}
	if err := session.Validate(runtime, grant, review, adapter, now.Add(11*time.Second)); err == nil {
		t.Fatal("execution session must remain closed after concurrent teardown")
	}
	if len(audit.snapshot()) != 2 {
		t.Fatal("concurrent teardown must produce exactly one open and one close audit event")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil || sink.batches != 0 {
		t.Fatal("concurrent teardown must leave dispatcher and control sink inactive")
	}
}

func TestMockObservationEmergencyStopThenTeardownStillAuditsClose(t *testing.T) {
	now := time.Now().UTC()
	audit := &recordingAttendedAuditSink{}
	arbiter, dispatcher, sink, runtime, grant, review, adapter, session := mockObservationFixture(t, now, audit)
	defer runtime.Close()

	observation, err := OpenMockAttendedExecutionObservation(runtime, grant, review, adapter, session, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(10500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := observation.Validate(now.Add(10500 * time.Millisecond)); err == nil {
		t.Fatal("emergency stop must invalidate observation immediately")
	}
	if err := observation.Close(now.Add(11 * time.Second)); err != nil {
		t.Fatalf("teardown after emergency stop should still close and audit: %v", err)
	}
	if len(audit.snapshot()) != 2 {
		t.Fatal("emergency-stop teardown must preserve open/close audit lifecycle")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil || sink.batches != 0 {
		t.Fatal("emergency-stop teardown must never activate control path")
	}
}
