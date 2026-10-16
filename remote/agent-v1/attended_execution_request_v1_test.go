package remoteagent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func executionRequestFixture(t *testing.T, now time.Time) (*TrustedSessionArbiter, *ControlDispatcher, *fakeControlSink, *AttendedControlRuntime, AttendedActivationGrant, fakeReviewedAttendedExecutor, AttendedExecutionReview) {
	t.Helper()
	arbiter, dispatcher, sink, runtime, grant := executionReviewFixture(t, now)
	executor := fakeReviewedAttendedExecutor{id: "reviewed-windows-input-v1", mouse: true, keyboard: true}
	review, err := PrepareReviewedAttendedExecution(runtime, grant, executor, now.Add(7*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return arbiter, dispatcher, sink, runtime, grant, executor, review
}

func TestAttendedExecutionRequestLifecycleKeepsDispatcherInactive(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, sink, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if request.RequestID == "" || request.ReviewID != review.ReviewID || request.ExecutorID != executor.id {
		t.Fatal("execution request binding is incomplete")
	}

	consumer := NewAttendedExecutionRequestConsumer()
	session, err := consumer.Consume(runtime, grant, review, executor, request, now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if session.ExecutionSessionID == "" || session.RequestID != request.RequestID || session.ReviewID != review.ReviewID {
		t.Fatal("execution session binding is incomplete")
	}
	if err := session.Validate(runtime, grant, review, executor, now.Add(10*time.Second)); err != nil {
		t.Fatalf("fresh non-executing session should validate: %v", err)
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("execution request/session lifecycle must not activate dispatcher")
	}
	if sink.batches != 0 {
		t.Fatal("execution request/session lifecycle must not reach control sink")
	}
}

func TestAttendedExecutionRequestConcurrentReplayRejected(t *testing.T) {
	now := time.Now().UTC()
	_, _, _, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	consumer := NewAttendedExecutionRequestConsumer()

	var successes int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := consumer.Consume(runtime, grant, review, executor, request, now.Add(9*time.Second)); err == nil {
				atomic.AddInt32(&successes, 1)
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("expected exactly one successful request consumption, got %d", successes)
	}
}

func TestAttendedExecutionRequestRejectsExecutorSubstitution(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, _, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	substitute := fakeReviewedAttendedExecutor{id: "substitute-executor", mouse: true, keyboard: true}
	if err := request.Validate(runtime, grant, review, substitute, now.Add(9*time.Second)); err == nil {
		t.Fatal("executor substitution must invalidate execution request")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("executor substitution must leave dispatcher inactive")
	}
}

func TestAttendedExecutionSessionCloseFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	_, dispatcher, _, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewAttendedExecutionRequestConsumer().Consume(runtime, grant, review, executor, request, now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(now.Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := session.Validate(runtime, grant, review, executor, now.Add(10*time.Second)); err == nil {
		t.Fatal("closed execution session must fail validation")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("closed execution session must leave dispatcher inactive")
	}
}

func TestAttendedExecutionRequestExpires(t *testing.T) {
	now := time.Now().UTC()
	_, _, _, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Validate(runtime, grant, review, executor, request.ExpiresAt); err == nil {
		t.Fatal("execution request must be invalid at expiry")
	}
}

func TestAttendedExecutionSessionFailsAfterEmergencyStop(t *testing.T) {
	now := time.Now().UTC()
	arbiter, dispatcher, _, runtime, grant, executor, review := executionRequestFixture(t, now)
	defer runtime.Close()

	request, err := PrepareAttendedExecutionRequest(runtime, grant, review, executor, now.Add(8*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewAttendedExecutionRequestConsumer().Consume(runtime, grant, review, executor, request, now.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arbiter.EngageEmergencyStop("local emergency stop", now.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := session.Validate(runtime, grant, review, executor, now.Add(10*time.Second)); err == nil {
		t.Fatal("emergency stop must invalidate execution session")
	}
	if err := dispatcher.Dispatch(controlPayload(t, 1)); err == nil {
		t.Fatal("emergency stop must leave dispatcher inactive")
	}
}
