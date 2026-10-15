package remoteagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type transportRemoteSignalingFake struct {
	mu       sync.Mutex
	pending  []PendingSession
	signals  []Signal
	approved []string
	rejected []string
	active   []string
	posted   []string
	pollErr  error
	postErr  error
}

func (f *transportRemoteSignalingFake) Pending(context.Context) ([]PendingSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PendingSession(nil), f.pending...), nil
}

func (f *transportRemoteSignalingFake) Approve(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approved = append(f.approved, sessionID)
	return nil
}

func (f *transportRemoteSignalingFake) Reject(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rejected = append(f.rejected, sessionID)
	return nil
}

func (f *transportRemoteSignalingFake) MarkActive(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = append(f.active, sessionID)
	return nil
}

func (f *transportRemoteSignalingFake) PostSignal(_ context.Context, sessionID, kind string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.postErr != nil {
		return f.postErr
	}
	f.posted = append(f.posted, sessionID+":"+kind)
	return nil
}

func (f *transportRemoteSignalingFake) PollSignals(_ context.Context, _ string, after int64) ([]Signal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pollErr != nil {
		return nil, f.pollErr
	}
	out := make([]Signal, 0, len(f.signals))
	for _, sig := range f.signals {
		if sig.ID > after {
			out = append(out, sig)
		}
	}
	return out, nil
}

func validTransportPending(id string, now time.Time) PendingSession {
	return PendingSession{
		ID:        id,
		State:     "pending",
		ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339Nano),
		RequestedCapabilities: map[string]any{
			"screen_stream": true,
			"mouse":         false,
			"keyboard":      false,
			"clipboard":     false,
			"file_transfer": false,
			"audio":         false,
		},
	}
}

func TestTransportHardenedRemoteSignalingBindsPendingSessionAndValidatesTraffic(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440020"
	inner := &transportRemoteSignalingFake{
		pending: []PendingSession{validTransportPending(id, now)},
		signals: []Signal{
			{ID: 1, Sender: "controller", Kind: "offer", Payload: rawJSON(map[string]any{"type": "offer", "sdp": "v=0"}), CreatedAt: now.Format(time.RFC3339Nano)},
			{ID: 2, Sender: "agent", Kind: "ice_candidate", Payload: rawJSON(map[string]any{"candidate": "candidate:1"}), CreatedAt: now.Format(time.RFC3339Nano)},
		},
	}
	observer := &recordingSignalTransportObserver{}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, observer)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pending, err := wrapped.Pending(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending failed: len=%d err=%v", len(pending), err)
	}
	if err := wrapped.Approve(ctx, id); err != nil {
		t.Fatalf("approve should use bound transport session: %v", err)
	}
	if err := wrapped.MarkActive(ctx, id); err != nil {
		t.Fatalf("mark active should use bound transport session: %v", err)
	}
	if err := wrapped.PostSignal(ctx, id, "answer", map[string]any{"type": "answer", "sdp": "v=0"}); err != nil {
		t.Fatalf("approved outbound answer should pass: %v", err)
	}
	signals, err := wrapped.PollSignals(ctx, id, 0)
	if err != nil {
		t.Fatalf("valid inbound batch should pass: %v", err)
	}
	if len(signals) != 2 {
		t.Fatalf("validated wrapper must preserve original batch for cursor semantics, got %d", len(signals))
	}
	snapshot, err := wrapped.Snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.InboundAccepted != 1 || snapshot.InboundIgnored != 1 || snapshot.OutboundAccepted != 1 || snapshot.LastSignalID != 2 {
		t.Fatalf("unexpected wrapped transport snapshot: %#v", snapshot)
	}
	if len(observer.snapshot()) != 3 {
		t.Fatal("expected metadata-only observations for outbound and inbound rows")
	}
}

func TestTransportHardenedRemoteSignalingFailsClosedWithoutPendingBinding(t *testing.T) {
	wrapped, err := NewTransportHardenedRemoteSignaling(&transportRemoteSignalingFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := "550e8400-e29b-41d4-a716-446655440021"
	if err := wrapped.Approve(context.Background(), id); err == nil {
		t.Fatal("approve without validated pending binding must fail closed")
	}
	if err := wrapped.PostSignal(context.Background(), id, "hangup", map[string]any{"reason": "done"}); err == nil {
		t.Fatal("post without validated pending binding must fail closed")
	}
	if _, err := wrapped.PollSignals(context.Background(), id, 0); err == nil {
		t.Fatal("poll without validated pending binding must fail closed")
	}
}

func TestTransportHardenedRemoteSignalingRejectsInvalidInboundBeforeOrchestrator(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440022"
	inner := &transportRemoteSignalingFake{
		pending: []PendingSession{validTransportPending(id, now)},
		signals: []Signal{{
			ID: 1, Sender: "controller", Kind: "control",
			Payload: rawJSON(map[string]any{"unsafe": true}),
		}},
	}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Pending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.PollSignals(context.Background(), id, 0); err == nil {
		t.Fatal("unsupported inbound kind must not reach orchestrator")
	}
}

func TestTransportHardenedRemoteSignalingRejectForgetsSessionBinding(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440023"
	inner := &transportRemoteSignalingFake{pending: []PendingSession{validTransportPending(id, now)}}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Pending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.Reject(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Snapshot(id); err == nil {
		t.Fatal("rejected session binding must be forgotten")
	}
}

func TestTransportHardenedRemoteSignalingOutboundHangupForgetsOnlyAfterDelivery(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440025"
	inner := &transportRemoteSignalingFake{pending: []PendingSession{validTransportPending(id, now)}}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Pending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.PostSignal(context.Background(), id, "hangup", map[string]any{"reason": "done"}); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Snapshot(id); err == nil {
		t.Fatal("successfully delivered terminal hangup must retire guard")
	}
}

func TestTransportHardenedRemoteSignalingInboundHangupForgetsAfterAtomicValidation(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440026"
	inner := &transportRemoteSignalingFake{
		pending: []PendingSession{validTransportPending(id, now)},
		signals: []Signal{{
			ID:        1,
			Sender:    "controller",
			Kind:      "hangup",
			Payload:   rawJSON(map[string]any{"reason": "controller_done"}),
			CreatedAt: now.Format(time.RFC3339Nano),
		}},
	}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Pending(context.Background()); err != nil {
		t.Fatal(err)
	}
	signals, err := wrapped.PollSignals(context.Background(), id, 0)
	if err != nil || len(signals) != 1 {
		t.Fatalf("validated controller hangup should be returned once: len=%d err=%v", len(signals), err)
	}
	if _, err := wrapped.Snapshot(id); err == nil {
		t.Fatal("validated inbound terminal hangup must retire guard")
	}
}

func TestTransportHardenedRemoteSignalingPropagatesUnderlyingTransportFailure(t *testing.T) {
	now := time.Now().UTC()
	id := "550e8400-e29b-41d4-a716-446655440024"
	inner := &transportRemoteSignalingFake{
		pending: []PendingSession{validTransportPending(id, now)},
		postErr: errors.New("network unavailable"),
		pollErr: errors.New("poll unavailable"),
	}
	wrapped, err := NewTransportHardenedRemoteSignaling(inner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Pending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.PostSignal(context.Background(), id, "hangup", map[string]any{"reason": "done"}); err == nil {
		t.Fatal("underlying post failure must be propagated")
	}
	if _, err := wrapped.Snapshot(id); err != nil {
		t.Fatalf("failed terminal delivery must preserve guard for safe retry: %v", err)
	}
	if _, err := wrapped.PollSignals(context.Background(), id, 0); err == nil {
		t.Fatal("underlying poll failure must be propagated")
	}
}
