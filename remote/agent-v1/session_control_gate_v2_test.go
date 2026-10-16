package remoteagent

import (
	"errors"
	"testing"
	"time"
)

type sessionGateTestSink struct{}

func (sessionGateTestSink) ApplyControlEvents(events []ControlEvent) error { return nil }

type sessionGateTestAuthorizer struct {
	err error
}

func (g sessionGateTestAuthorizer) ValidateControlMessage(now time.Time) error { return g.err }

func TestAgentSessionRejectsDispatcherWithoutAttendedGate(t *testing.T) {
	dispatcher, err := NewControlDispatcher(sessionGateTestSink{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewAgentSession(Config{
		Signal:  func(string, any) error { return nil },
		Control: dispatcher,
	}); err == nil {
		t.Fatal("expected control dispatcher without attended gate to fail closed")
	}
}

func TestAgentSessionAcceptsAbstractDispatcherWithAttendedGate(t *testing.T) {
	dispatcher, err := NewControlDispatcher(sessionGateTestSink{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewAgentSession(Config{
		Signal:      func(string, any) error { return nil },
		Control:     dispatcher,
		ControlGate: sessionGateTestAuthorizer{},
	})
	if err != nil {
		t.Fatalf("expected abstract dispatcher with attended gate to construct: %v", err)
	}
	defer s.Close()
	if s.controlDispatch != dispatcher || s.controlGate == nil {
		t.Fatal("expected session to retain dispatcher and attended gate")
	}
	if err := dispatcher.Dispatch([]byte(`{"protocol":2,"sequence":1,"events":[]}`)); err == nil {
		t.Fatal("dispatcher must remain inactive; session construction must not activate it")
	}
}

func TestSessionGateTestAuthorizerCanFailClosed(t *testing.T) {
	want := errors.New("approval expired")
	gate := sessionGateTestAuthorizer{err: want}
	if err := gate.ValidateControlMessage(time.Now().UTC()); !errors.Is(err, want) {
		t.Fatalf("expected gate error %v, got %v", want, err)
	}
}
