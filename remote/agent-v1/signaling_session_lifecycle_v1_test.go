package remoteagent

import (
	"sync"
	"testing"
	"time"
)

type recordingSignalLifecycleObserver struct {
	mu           sync.Mutex
	observations []SignalSessionLifecycleObservation
}

func (r *recordingSignalLifecycleObserver) RecordSignalSessionLifecycleObservation(observation SignalSessionLifecycleObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, observation)
}

func (r *recordingSignalLifecycleObserver) snapshot() []SignalSessionLifecycleObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SignalSessionLifecycleObservation(nil), r.observations...)
}

func newSignalLifecycleGuardForTest(t *testing.T, now time.Time, observer SignalSessionLifecycleObserver) *SignalSessionLifecycleGuard {
	t.Helper()
	guard, err := NewSignalSessionLifecycleGuard(
		"550e8400-e29b-41d4-a716-446655440030",
		now.Add(10*time.Minute),
		observer,
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func TestSignalSessionLifecycleHeartbeatBecomesDueWithoutClosingSession(t *testing.T) {
	now := time.Now().UTC()
	guard := newSignalLifecycleGuardForTest(t, now, nil)

	before := guard.Evaluate(now.Add(SignalHeartbeatInterval - time.Millisecond))
	if before.HeartbeatDue {
		t.Fatal("heartbeat must not become due before the bounded interval")
	}
	after := guard.Evaluate(now.Add(SignalHeartbeatInterval))
	if !after.HeartbeatDue {
		t.Fatal("heartbeat must become due at the bounded interval")
	}
	if after.State != signalLifecycleHealthy {
		t.Fatalf("heartbeat scheduling must not close a healthy session: %q", after.State)
	}
}

func TestSignalSessionLifecycleDegradedConnectionRecoversOnSuccess(t *testing.T) {
	now := time.Now().UTC()
	observer := &recordingSignalLifecycleObserver{}
	guard := newSignalLifecycleGuardForTest(t, now, observer)

	degraded := guard.RecordFailure(now.Add(5*time.Second), "poll_failed")
	if degraded.State != signalLifecycleDegraded || degraded.ConsecutiveFailures != 1 {
		t.Fatalf("unexpected degraded snapshot: %#v", degraded)
	}
	if err := guard.RecordSuccess(now.Add(6*time.Second), "poll_ok"); err != nil {
		t.Fatalf("degraded session should recover before grace expiry: %v", err)
	}
	recovered := guard.Evaluate(now.Add(6 * time.Second))
	if recovered.State != signalLifecycleHealthy || recovered.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected recovered snapshot: %#v", recovered)
	}
	if len(observer.snapshot()) < 2 {
		t.Fatal("degrade and recovery transitions should be observable")
	}
}

func TestSignalSessionLifecycleLossFailsClosedAfterGrace(t *testing.T) {
	now := time.Now().UTC()
	guard := newSignalLifecycleGuardForTest(t, now, nil)

	lost := guard.RecordFailure(now.Add(SignalConnectionLossGrace), "poll_failed")
	if lost.State != signalLifecycleLost {
		t.Fatalf("expected lost state after grace window, got %q", lost.State)
	}
	if lost.HeartbeatDue {
		t.Fatal("lost session must not continue heartbeat scheduling")
	}
	if err := guard.RecordSuccess(now.Add(SignalConnectionLossGrace+time.Second), "poll_ok"); err == nil {
		t.Fatal("lost session must require re-admission instead of silent recovery")
	}
}

func TestSignalSessionLifecycleExpiryIsTerminalForAdmission(t *testing.T) {
	now := time.Now().UTC()
	guard := newSignalLifecycleGuardForTest(t, now, nil)

	expired := guard.Evaluate(now.Add(11 * time.Minute))
	if expired.State != signalLifecycleExpired {
		t.Fatalf("expected expired lifecycle, got %q", expired.State)
	}
	if err := guard.RecordSuccess(now.Add(11*time.Minute+time.Second), "late_success"); err == nil {
		t.Fatal("expired session must not be revived")
	}
}

func TestSignalSessionLifecycleTerminalTeardownIsIdempotent(t *testing.T) {
	now := time.Now().UTC()
	guard := newSignalLifecycleGuardForTest(t, now, nil)

	first := guard.Terminate(now.Add(time.Second), "controller_hangup")
	second := guard.Terminate(now.Add(2*time.Second), "different_reason")
	if first.State != signalLifecycleTerminal || second.State != signalLifecycleTerminal {
		t.Fatal("terminal teardown must stay terminal")
	}
	if second.TerminalReason != "controller_hangup" {
		t.Fatalf("terminal reason must be stable after first teardown: %q", second.TerminalReason)
	}
	if err := guard.RecordSuccess(now.Add(3*time.Second), "poll_ok"); err == nil {
		t.Fatal("terminal lifecycle must not recover")
	}
}

func TestSignalSessionLifecycleConcurrentFailuresRemainRaceSafe(t *testing.T) {
	now := time.Now().UTC()
	guard := newSignalLifecycleGuardForTest(t, now, nil)
	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			guard.RecordFailure(now.Add(time.Second), "poll_failed")
		}()
	}
	wg.Wait()

	snapshot := guard.Evaluate(now.Add(time.Second))
	if snapshot.State != signalLifecycleDegraded {
		t.Fatalf("concurrent failures before grace must remain degraded: %q", snapshot.State)
	}
	if snapshot.ConsecutiveFailures != workers {
		t.Fatalf("expected %d failures, got %d", workers, snapshot.ConsecutiveFailures)
	}
}
