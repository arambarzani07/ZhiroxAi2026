package remoteagent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// TransportHardenedRemoteSignaling wraps the existing signaling transport with
// per-session validation and metadata-only observability. It does not mutate
// WebRTC state and has no control/input execution capability.
type TransportHardenedRemoteSignaling struct {
	inner    RemoteSignaling
	observer SignalTransportObserver
	mu       sync.Mutex
	guards   map[string]*SignalTransportGuard
}

func NewTransportHardenedRemoteSignaling(inner RemoteSignaling, observer SignalTransportObserver) (*TransportHardenedRemoteSignaling, error) {
	if inner == nil {
		return nil, errors.New("remote signaling transport is required")
	}
	return &TransportHardenedRemoteSignaling{
		inner:    inner,
		observer: observer,
		guards:   make(map[string]*SignalTransportGuard),
	}, nil
}

func (t *TransportHardenedRemoteSignaling) rememberPending(request PendingSession, now time.Time) {
	if t == nil {
		return
	}
	if err := pcOSRemoteValidateSessionID(request.ID); err != nil {
		return
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(request.ExpiresAt))
	if err != nil || !expiresAt.UTC().After(now.UTC()) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.guards[request.ID]; exists {
		return
	}
	guard, err := NewSignalTransportGuard(request.ID, expiresAt, t.observer, now)
	if err != nil {
		return
	}
	t.guards[request.ID] = guard
}

func (t *TransportHardenedRemoteSignaling) guardFor(sessionID string) (*SignalTransportGuard, error) {
	if t == nil || t.inner == nil {
		return nil, errors.New("transport-hardened signaling is unavailable")
	}
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	guard := t.guards[sessionID]
	if guard == nil {
		return nil, errors.New("signaling session is not bound to a validated pending request")
	}
	return guard, nil
}

func (t *TransportHardenedRemoteSignaling) Pending(ctx context.Context) ([]PendingSession, error) {
	if t == nil || t.inner == nil {
		return nil, errors.New("transport-hardened signaling is unavailable")
	}
	pending, err := t.inner.Pending(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, request := range pending {
		t.rememberPending(request, now)
	}
	return pending, nil
}

func (t *TransportHardenedRemoteSignaling) Approve(ctx context.Context, sessionID string) error {
	if _, err := t.guardFor(sessionID); err != nil {
		return err
	}
	return t.inner.Approve(ctx, sessionID)
}

func (t *TransportHardenedRemoteSignaling) Reject(ctx context.Context, sessionID string) error {
	if t == nil || t.inner == nil {
		return errors.New("transport-hardened signaling is unavailable")
	}
	err := t.inner.Reject(ctx, sessionID)
	if err == nil {
		t.mu.Lock()
		delete(t.guards, sessionID)
		t.mu.Unlock()
	}
	return err
}

func (t *TransportHardenedRemoteSignaling) MarkActive(ctx context.Context, sessionID string) error {
	if _, err := t.guardFor(sessionID); err != nil {
		return err
	}
	return t.inner.MarkActive(ctx, sessionID)
}

func (t *TransportHardenedRemoteSignaling) PostSignal(ctx context.Context, sessionID, kind string, payload any) error {
	guard, err := t.guardFor(sessionID)
	if err != nil {
		return err
	}
	if err := guard.ValidateOutbound(kind, payload, time.Now().UTC()); err != nil {
		return err
	}
	return t.inner.PostSignal(ctx, sessionID, kind, payload)
}

func (t *TransportHardenedRemoteSignaling) PollSignals(ctx context.Context, sessionID string, after int64) ([]Signal, error) {
	guard, err := t.guardFor(sessionID)
	if err != nil {
		return nil, err
	}
	signals, err := t.inner.PollSignals(ctx, sessionID, after)
	if err != nil {
		return nil, err
	}
	// Validate the complete batch before exposing any row to the orchestrator.
	// Return the original validated batch so the existing orchestrator can advance
	// its cursor across harmless agent-origin echo rows before ignoring them.
	if _, _, err := guard.FilterInboundBatch(signals, after, time.Now().UTC()); err != nil {
		return nil, err
	}
	return signals, nil
}

func (t *TransportHardenedRemoteSignaling) Snapshot(sessionID string) (SignalTransportSnapshot, error) {
	guard, err := t.guardFor(sessionID)
	if err != nil {
		return SignalTransportSnapshot{}, err
	}
	return guard.Snapshot(), nil
}

func (t *TransportHardenedRemoteSignaling) Forget(sessionID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	delete(t.guards, sessionID)
	t.mu.Unlock()
}

var _ RemoteSignaling = (*TransportHardenedRemoteSignaling)(nil)
