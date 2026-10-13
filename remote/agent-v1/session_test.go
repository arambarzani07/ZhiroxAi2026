package remoteagent

import (
	"testing"
	"time"
)

func TestAgentSessionRequiresSignalSink(t *testing.T) {
	if _, err := NewAgentSession(Config{}); err == nil {
		t.Fatal("session must require signaling sink")
	}
}

func TestAgentSessionCreatesScreenTransport(t *testing.T) {
	s, err := NewAgentSession(Config{Signal: func(string, any) error { return nil }})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer s.Close()
	if s.screen == nil || s.control == nil || s.pc == nil {
		t.Fatal("expected peer connection, screen track and control channel")
	}
	if err := s.WriteVP8Frame(nil, time.Second/15); err == nil {
		t.Fatal("empty frame must be rejected")
	}
	if err := s.WriteVP8Frame([]byte{0x00}, 0); err == nil {
		t.Fatal("zero duration must be rejected")
	}
}

func TestAgentSessionOfferValidationAndClose(t *testing.T) {
	s, err := NewAgentSession(Config{Signal: func(string, any) error { return nil }})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.AcceptOffer(""); err == nil {
		t.Fatal("empty offer must be rejected")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second close must be idempotent: %v", err)
	}
	if err := s.WriteVP8Frame([]byte{0x00}, time.Second/15); err == nil {
		t.Fatal("closed session must reject frames")
	}
}
