package remoteagent

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeRemoteSignaling struct {
	mu       sync.Mutex
	pending  []PendingSession
	rejected []string
	approved []string
}

func (f *fakeRemoteSignaling) Pending(context.Context) ([]PendingSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]PendingSession(nil), f.pending...)
	f.pending = nil
	return out, nil
}
func (f *fakeRemoteSignaling) Approve(_ context.Context, id string) error {
	f.mu.Lock(); defer f.mu.Unlock(); f.approved = append(f.approved, id); return nil
}
func (f *fakeRemoteSignaling) Reject(_ context.Context, id string) error {
	f.mu.Lock(); defer f.mu.Unlock(); f.rejected = append(f.rejected, id); return nil
}
func (f *fakeRemoteSignaling) MarkActive(context.Context, string) error { return nil }
func (f *fakeRemoteSignaling) PostSignal(context.Context, string, string, any) error { return nil }
func (f *fakeRemoteSignaling) PollSignals(context.Context, string, int64) ([]Signal, error) { return nil, nil }

type fakeRemoteConsent struct {
	approved bool
	calls    int
}
func (f *fakeRemoteConsent) Approve(string, string) (bool, error) { f.calls++; return f.approved, nil }
func (f *fakeRemoteConsent) StartIndicator(string) (<-chan error, error) {
	ch := make(chan error)
	return ch, nil
}

func validScreenOnlyCapabilities() map[string]any {
	return map[string]any{
		"screen_stream": true,
		"mouse": false, "keyboard": false, "clipboard": false,
		"file_transfer": false, "audio": false,
	}
}

func testOrchestrator(t *testing.T, signaling RemoteSignaling, consent RemoteConsent) *Orchestrator {
	t.Helper()
	o, err := NewOrchestrator(OrchestratorConfig{
		Signaling: signaling,
		Consent: consent,
		NewSession: func(SignalSink, StatusSink) (*AgentSession, error) { return &AgentSession{}, nil },
		NewPipeline: func(*AgentSession) (ScreenPipeline, error) { return nil, nil },
		PendingInterval: 250 * time.Millisecond,
	})
	if err != nil { t.Fatalf("new orchestrator: %v", err) }
	return o
}

func TestScreenOnlyCapabilities(t *testing.T) {
	if !screenOnlyCapabilities(validScreenOnlyCapabilities()) {
		t.Fatal("screen-only capabilities should be accepted")
	}
	for _, key := range []string{"mouse", "keyboard", "clipboard", "file_transfer", "audio"} {
		caps := validScreenOnlyCapabilities()
		caps[key] = true
		if screenOnlyCapabilities(caps) {
			t.Fatalf("capability %s must fail closed", key)
		}
	}
	if screenOnlyCapabilities(map[string]any{"screen_stream": false}) {
		t.Fatal("screen_stream=false must be refused")
	}
	if screenOnlyCapabilities(map[string]any{"screen_stream": "true"}) {
		t.Fatal("non-boolean screen_stream must be refused")
	}
}

func TestUnsupportedCapabilitiesAreRejectedBeforeConsent(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440000"
	sig := &fakeRemoteSignaling{pending: []PendingSession{{
		ID: id,
		RequestedCapabilities: map[string]any{"screen_stream": true, "keyboard": true},
	}}}
	consent := &fakeRemoteConsent{approved: true}
	o := testOrchestrator(t, sig, consent)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := o.Run(ctx); err != nil { t.Fatalf("run: %v", err) }
	if consent.calls != 0 { t.Fatal("unsupported request must not reach local consent") }
	if len(sig.rejected) != 1 || sig.rejected[0] != id { t.Fatalf("request was not rejected: %#v", sig.rejected) }
	if len(sig.approved) != 0 { t.Fatal("unsupported request must never be approved") }
}

func TestLocalNoRejectsBeforeWebRTCSession(t *testing.T) {
	id := "550e8400-e29b-41d4-a716-446655440001"
	sig := &fakeRemoteSignaling{pending: []PendingSession{{
		ID: id,
		RequestedCapabilities: validScreenOnlyCapabilities(),
		ControllerLabel: "Support Desk",
	}}}
	consent := &fakeRemoteConsent{approved: false}
	o := testOrchestrator(t, sig, consent)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if err := o.Run(ctx); err != nil { t.Fatalf("run: %v", err) }
	if consent.calls != 1 { t.Fatalf("expected one consent prompt, got %d", consent.calls) }
	if len(sig.rejected) != 1 || sig.rejected[0] != id { t.Fatalf("local No was not rejected: %#v", sig.rejected) }
	if len(sig.approved) != 0 { t.Fatal("local No must never approve the session") }
}

func TestOrchestratorRejectsSessionDurationOverThirtyMinutes(t *testing.T) {
	_, err := NewOrchestrator(OrchestratorConfig{
		Signaling: &fakeRemoteSignaling{}, Consent: &fakeRemoteConsent{},
		NewSession: func(SignalSink, StatusSink) (*AgentSession, error) { return &AgentSession{}, nil },
		NewPipeline: func(*AgentSession) (ScreenPipeline, error) { return nil, nil },
		MaxSession: 31 * time.Minute,
	})
	if err == nil { t.Fatal("session duration above 30 minutes must be refused") }
}
