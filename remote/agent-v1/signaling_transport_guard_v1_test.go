package remoteagent

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type recordingSignalTransportObserver struct {
	mu           sync.Mutex
	observations []SignalTransportObservation
}

func (r *recordingSignalTransportObserver) RecordSignalTransportObservation(observation SignalTransportObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.observations = append(r.observations, observation)
}

func (r *recordingSignalTransportObserver) snapshot() []SignalTransportObservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]SignalTransportObservation(nil), r.observations...)
}

func newTransportGuardForTest(t *testing.T, now time.Time, observer SignalTransportObserver) *SignalTransportGuard {
	t.Helper()
	guard, err := NewSignalTransportGuard("550e8400-e29b-41d4-a716-446655440010", now.Add(10*time.Minute), observer, now)
	if err != nil {
		t.Fatal(err)
	}
	return guard
}

func rawJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestSignalTransportGuardAcceptsOrderedControllerSignalsAndIgnoresAgentEcho(t *testing.T) {
	now := time.Now().UTC()
	observer := &recordingSignalTransportObserver{}
	guard := newTransportGuardForTest(t, now, observer)
	signals := []Signal{
		{ID: 1, Sender: "controller", Kind: "offer", Payload: rawJSON(map[string]any{"type": "offer", "sdp": "v=0"}), CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: 2, Sender: "agent", Kind: "ice_candidate", Payload: rawJSON(map[string]any{"candidate": "candidate:1"}), CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: 3, Sender: "controller", Kind: "hangup", Payload: rawJSON(map[string]any{"reason": "done"}), CreatedAt: now.Format(time.RFC3339Nano)},
	}
	accepted, cursor, err := guard.FilterInboundBatch(signals, 0, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cursor != 3 {
		t.Fatalf("expected cursor 3, got %d", cursor)
	}
	if len(accepted) != 2 || accepted[0].ID != 1 || accepted[1].ID != 3 {
		t.Fatalf("unexpected accepted controller signals: %#v", accepted)
	}
	snapshot := guard.Snapshot()
	if snapshot.InboundAccepted != 2 || snapshot.InboundIgnored != 1 || snapshot.InboundRejected != 0 || snapshot.LastSignalID != 3 {
		t.Fatalf("unexpected transport snapshot: %#v", snapshot)
	}
	if len(observer.snapshot()) != 3 {
		t.Fatal("every validated transport row should produce metadata-only observability")
	}
}

func TestSignalTransportGuardRejectsReplayAndOutOfOrderBatch(t *testing.T) {
	now := time.Now().UTC()
	guard := newTransportGuardForTest(t, now, nil)
	valid := []Signal{{ID: 4, Sender: "controller", Kind: "hangup", Payload: rawJSON(map[string]any{"reason": "done"})}}
	if _, cursor, err := guard.FilterInboundBatch(valid, 3, now); err != nil || cursor != 4 {
		t.Fatalf("first signal should validate: cursor=%d err=%v", cursor, err)
	}
	if _, _, err := guard.FilterInboundBatch(valid, 3, now.Add(time.Second)); err == nil {
		t.Fatal("replayed signal must fail closed")
	}

	guard2 := newTransportGuardForTest(t, now, nil)
	outOfOrder := []Signal{
		{ID: 8, Sender: "controller", Kind: "hangup", Payload: rawJSON(map[string]any{"reason": "a"})},
		{ID: 7, Sender: "controller", Kind: "hangup", Payload: rawJSON(map[string]any{"reason": "b"})},
	}
	if _, _, err := guard2.FilterInboundBatch(outOfOrder, 6, now); err == nil {
		t.Fatal("out-of-order signal batch must fail closed")
	}
}

func TestSignalTransportGuardRejectsUnknownSenderUnsupportedKindAndMalformedPayload(t *testing.T) {
	now := time.Now().UTC()
	cases := []Signal{
		{ID: 1, Sender: "relay", Kind: "offer", Payload: rawJSON(map[string]any{"type": "offer"})},
		{ID: 1, Sender: "controller", Kind: "control", Payload: rawJSON(map[string]any{"x": 1})},
		{ID: 1, Sender: "controller", Kind: "offer", Payload: json.RawMessage(`{"broken"`)},
	}
	for i, sig := range cases {
		guard := newTransportGuardForTest(t, now, nil)
		if _, _, err := guard.FilterInboundBatch([]Signal{sig}, 0, now); err == nil {
			t.Fatalf("case %d must fail closed", i)
		}
	}
}

func TestSignalTransportGuardRejectsFutureTimestampAndExpiredSession(t *testing.T) {
	now := time.Now().UTC()
	guard := newTransportGuardForTest(t, now, nil)
	future := Signal{
		ID: 1, Sender: "controller", Kind: "hangup",
		Payload:   rawJSON(map[string]any{"reason": "done"}),
		CreatedAt: now.Add(maxSignalFutureClockSkew + time.Second).Format(time.RFC3339Nano),
	}
	if _, _, err := guard.FilterInboundBatch([]Signal{future}, 0, now); err == nil {
		t.Fatal("far-future signal timestamp must be rejected")
	}

	expiring, err := NewSignalTransportGuard("550e8400-e29b-41d4-a716-446655440011", now.Add(time.Second), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := expiring.ValidateOutbound("hangup", map[string]any{"reason": "done"}, now.Add(2*time.Second)); err == nil {
		t.Fatal("outbound signal after bound session expiry must fail closed")
	}
}

func TestSignalTransportGuardOutboundAllowlistAndCounters(t *testing.T) {
	now := time.Now().UTC()
	guard := newTransportGuardForTest(t, now, nil)
	if err := guard.ValidateOutbound("answer", map[string]any{"type": "answer", "sdp": "v=0"}, now); err != nil {
		t.Fatalf("answer should be allowed: %v", err)
	}
	if err := guard.ValidateOutbound("offer", map[string]any{"type": "offer", "sdp": "v=0"}, now); err == nil {
		t.Fatal("agent must not emit offer in the controller-offerer contract")
	}
	snapshot := guard.Snapshot()
	if snapshot.OutboundAccepted != 1 || snapshot.OutboundRejected != 1 {
		t.Fatalf("unexpected outbound counters: %#v", snapshot)
	}
}

func TestSignalTransportGuardRejectsOversizedBatchWithoutAdvancingCursor(t *testing.T) {
	now := time.Now().UTC()
	guard := newTransportGuardForTest(t, now, nil)
	signals := make([]Signal, maxInboundSignalBatch+1)
	for i := range signals {
		signals[i] = Signal{
			ID: int64(i + 1), Sender: "controller", Kind: "hangup",
			Payload: rawJSON(map[string]any{"reason": "done"}),
		}
	}
	if _, cursor, err := guard.FilterInboundBatch(signals, 0, now); err == nil || cursor != 0 {
		t.Fatalf("oversized batch must fail closed without cursor advance: cursor=%d err=%v", cursor, err)
	}
	if guard.Snapshot().LastSignalID != 0 {
		t.Fatal("rejected oversized batch must not commit a signal cursor")
	}
}
