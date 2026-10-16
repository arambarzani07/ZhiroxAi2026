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
	inner             RemoteSignaling
	observer          SignalTransportObserver
	lifecycleObserver SignalSessionLifecycleObserver
	mu                sync.Mutex
	guards            map[string]*SignalTransportGuard
	lifecycles        map[string]*SignalSessionLifecycleGuard
}

func NewTransportHardenedRemoteSignaling(inner RemoteSignaling, observer SignalTransportObserver) (*TransportHardenedRemoteSignaling, error) {
	return NewTransportHardenedRemoteSignalingWithLifecycle(inner, observer, nil)
}

func NewTransportHardenedRemoteSignalingWithLifecycle(inner RemoteSignaling, observer SignalTransportObserver, lifecycleObserver SignalSessionLifecycleObserver) (*TransportHardenedRemoteSignaling, error) {
	if inner == nil {
		return nil, errors.New("remote signaling transport is required")
	}
	return &TransportHardenedRemoteSignaling{
		inner:             inner,
		observer:          observer,
		lifecycleObserver: lifecycleObserver,
		guards:            make(map[string]*SignalTransportGuard),
		lifecycles:        make(map[string]*SignalSessionLifecycleGuard),
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
	guard, err := NewSignalTransportGuard(request.ID, expiresAt, t.observer, now)
	if err != nil {
		return
	}
	lifecycle, err := NewSignalSessionLifecycleGuard(request.ID, expiresAt, t.lifecycleObserver, now)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.guards[request.ID]; exists {
		return
	}
	t.guards[request.ID] = guard
	t.lifecycles[request.ID] = lifecycle
}

func (t *TransportHardenedRemoteSignaling) boundSessionFor(sessionID string, now time.Time) (*SignalTransportGuard, *SignalSessionLifecycleGuard, error) {
	if t == nil || t.inner == nil {
		return nil, nil, errors.New("transport-hardened signaling is unavailable")
	}
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return nil, nil, err
	}
	t.mu.Lock()
	guard := t.guards[sessionID]
	lifecycle := t.lifecycles[sessionID]
	t.mu.Unlock()
	if guard == nil || lifecycle == nil {
		return nil, nil, errors.New("signaling session is not bound to a validated pending request")
	}
	snapshot := lifecycle.Evaluate(now)
	if lifecycleClosedState(snapshot.State) {
		t.Forget(sessionID)
		return nil, nil, errors.New("signaling session lifecycle is closed")
	}
	return guard, lifecycle, nil
}

func (t *TransportHardenedRemoteSignaling) guardFor(sessionID string) (*SignalTransportGuard, error) {
	guard, _, err := t.boundSessionFor(sessionID, time.Now().UTC())
	return guard, err
}

func (t *TransportHardenedRemoteSignaling) recordLifecycleSuccess(sessionID string, lifecycle *SignalSessionLifecycleGuard, now time.Time, reason string) error {
	if lifecycle == nil {
		t.Forget(sessionID)
		return errors.New("signal lifecycle guard is unavailable")
	}
	if err := lifecycle.RecordSuccess(now, reason); err != nil {
		t.Forget(sessionID)
		return err
	}
	return nil
}

func (t *TransportHardenedRemoteSignaling) recordLifecycleFailure(sessionID string, lifecycle *SignalSessionLifecycleGuard, now time.Time, reason string) {
	if lifecycle == nil {
		t.Forget(sessionID)
		return
	}
	snapshot := lifecycle.RecordFailure(now, reason)
	if lifecycleClosedState(snapshot.State) {
		t.Forget(sessionID)
	}
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
	_, lifecycle, err := t.boundSessionFor(sessionID, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := t.inner.Approve(ctx, sessionID); err != nil {
		t.recordLifecycleFailure(sessionID, lifecycle, time.Now().UTC(), "approve_failed")
		return err
	}
	return t.recordLifecycleSuccess(sessionID, lifecycle, time.Now().UTC(), "approve_ok")
}

func (t *TransportHardenedRemoteSignaling) Reject(ctx context.Context, sessionID string) error {
	if t == nil || t.inner == nil {
		return errors.New("transport-hardened signaling is unavailable")
	}
	err := t.inner.Reject(ctx, sessionID)
	if err == nil {
		t.mu.Lock()
		lifecycle := t.lifecycles[sessionID]
		t.mu.Unlock()
		if lifecycle != nil {
			lifecycle.Terminate(time.Now().UTC(), "rejected")
		}
		t.Forget(sessionID)
	}
	return err
}

func (t *TransportHardenedRemoteSignaling) MarkActive(ctx context.Context, sessionID string) error {
	_, lifecycle, err := t.boundSessionFor(sessionID, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := t.inner.MarkActive(ctx, sessionID); err != nil {
		t.recordLifecycleFailure(sessionID, lifecycle, time.Now().UTC(), "mark_active_failed")
		return err
	}
	return t.recordLifecycleSuccess(sessionID, lifecycle, time.Now().UTC(), "mark_active_ok")
}

func (t *TransportHardenedRemoteSignaling) PostSignal(ctx context.Context, sessionID, kind string, payload any) error {
	now := time.Now().UTC()
	guard, lifecycle, err := t.boundSessionFor(sessionID, now)
	if err != nil {
		return err
	}
	if err := guard.ValidateOutbound(kind, payload, now); err != nil {
		return err
	}
	if err := t.inner.PostSignal(ctx, sessionID, kind, payload); err != nil {
		t.recordLifecycleFailure(sessionID, lifecycle, time.Now().UTC(), "post_failed")
		return err
	}
	if err := t.recordLifecycleSuccess(sessionID, lifecycle, time.Now().UTC(), "post_ok"); err != nil {
		return err
	}
	// A successfully delivered terminal hangup retires the session-scoped state.
	// Failures deliberately preserve the session so the caller can retry safely.
	if normalizeSignalKind(kind) == "hangup" {
		lifecycle.Terminate(time.Now().UTC(), "agent_hangup")
		t.Forget(sessionID)
	}
	return nil
}

func (t *TransportHardenedRemoteSignaling) PollSignals(ctx context.Context, sessionID string, after int64) ([]Signal, error) {
	now := time.Now().UTC()
	guard, lifecycle, err := t.boundSessionFor(sessionID, now)
	if err != nil {
		return nil, err
	}
	signals, err := t.inner.PollSignals(ctx, sessionID, after)
	if err != nil {
		t.recordLifecycleFailure(sessionID, lifecycle, time.Now().UTC(), "poll_failed")
		return nil, err
	}
	if err := t.recordLifecycleSuccess(sessionID, lifecycle, time.Now().UTC(), "poll_ok"); err != nil {
		return nil, err
	}
	// Validate the complete batch before exposing any row to the orchestrator.
	// Return the original validated batch so the existing orchestrator can advance
	// its cursor across harmless agent-origin echo rows before ignoring them.
	if _, _, err := guard.FilterInboundBatch(signals, after, time.Now().UTC()); err != nil {
		return nil, err
	}
	for _, sig := range signals {
		if strings.EqualFold(strings.TrimSpace(sig.Sender), "controller") && normalizeSignalKind(sig.Kind) == "hangup" {
			lifecycle.Terminate(time.Now().UTC(), "controller_hangup")
			t.Forget(sessionID)
			break
		}
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

func (t *TransportHardenedRemoteSignaling) LifecycleSnapshot(sessionID string) (SignalSessionLifecycleSnapshot, error) {
	if t == nil || t.inner == nil {
		return SignalSessionLifecycleSnapshot{}, errors.New("transport-hardened signaling is unavailable")
	}
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return SignalSessionLifecycleSnapshot{}, err
	}
	t.mu.Lock()
	lifecycle := t.lifecycles[sessionID]
	t.mu.Unlock()
	if lifecycle == nil {
		return SignalSessionLifecycleSnapshot{}, errors.New("signaling session lifecycle is not bound")
	}
	return lifecycle.Evaluate(time.Now().UTC()), nil
}

func (t *TransportHardenedRemoteSignaling) Forget(sessionID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	delete(t.guards, sessionID)
	delete(t.lifecycles, sessionID)
	t.mu.Unlock()
}

var _ RemoteSignaling = (*TransportHardenedRemoteSignaling)(nil)
