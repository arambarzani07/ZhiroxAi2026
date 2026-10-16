package remoteagent

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	SignalHeartbeatInterval   = 10 * time.Second
	SignalConnectionLossGrace = 30 * time.Second

	signalLifecycleHealthy  = "healthy"
	signalLifecycleDegraded = "degraded"
	signalLifecycleLost     = "lost"
	signalLifecycleExpired  = "expired"
	signalLifecycleTerminal = "terminal"
)

// SignalSessionLifecycleObservation is metadata-only lifecycle telemetry. It
// records transport health and teardown state without carrying signaling bodies.
type SignalSessionLifecycleObservation struct {
	SessionID           string
	State               string
	Reason              string
	ConsecutiveFailures uint32
	LastSuccessAt       time.Time
	ObservedAt          time.Time
}

type SignalSessionLifecycleObserver interface {
	RecordSignalSessionLifecycleObservation(observation SignalSessionLifecycleObservation)
}

type SignalSessionLifecycleSnapshot struct {
	SessionID           string
	State               string
	HeartbeatDue        bool
	ConsecutiveFailures uint32
	LastSuccessAt       time.Time
	LastAttemptAt       time.Time
	ExpiresAt           time.Time
	TerminalReason      string
	LastReason          string
}

// SignalSessionLifecycleGuard tracks bounded heartbeat/loss state for exactly
// one signaling session. Degraded sessions may recover after a successful
// transport operation; lost, expired, and terminal sessions require re-admission.
type SignalSessionLifecycleGuard struct {
	mu        sync.Mutex
	sessionID string
	expiresAt time.Time
	observer  SignalSessionLifecycleObserver
	snapshot  SignalSessionLifecycleSnapshot
}

func NewSignalSessionLifecycleGuard(sessionID string, expiresAt time.Time, observer SignalSessionLifecycleObserver, now time.Time) (*SignalSessionLifecycleGuard, error) {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	now = trustedArbiterNow(now)
	expiresAt = expiresAt.UTC()
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return nil, errors.New("signal lifecycle session expiry is invalid")
	}
	return &SignalSessionLifecycleGuard{
		sessionID: sessionID,
		expiresAt: expiresAt,
		observer:  observer,
		snapshot: SignalSessionLifecycleSnapshot{
			SessionID:     sessionID,
			State:         signalLifecycleHealthy,
			LastSuccessAt: now,
			LastAttemptAt: now,
			ExpiresAt:     expiresAt,
		},
	}, nil
}

func normalizeLifecycleReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 128 {
		reason = reason[:128]
	}
	return reason
}

func lifecycleClosedState(state string) bool {
	switch state {
	case signalLifecycleLost, signalLifecycleExpired, signalLifecycleTerminal:
		return true
	default:
		return false
	}
}

func (g *SignalSessionLifecycleGuard) emitLocked(now time.Time) {
	if g.observer == nil {
		return
	}
	g.observer.RecordSignalSessionLifecycleObservation(SignalSessionLifecycleObservation{
		SessionID:           g.sessionID,
		State:               g.snapshot.State,
		Reason:              g.snapshot.LastReason,
		ConsecutiveFailures: g.snapshot.ConsecutiveFailures,
		LastSuccessAt:       g.snapshot.LastSuccessAt,
		ObservedAt:          now.UTC(),
	})
}

func (g *SignalSessionLifecycleGuard) evaluateLocked(now time.Time) {
	if g.snapshot.State == signalLifecycleTerminal {
		g.snapshot.HeartbeatDue = false
		return
	}
	if !g.expiresAt.After(now) {
		if g.snapshot.State != signalLifecycleExpired {
			g.snapshot.State = signalLifecycleExpired
			g.snapshot.LastReason = "session_expired"
			g.snapshot.HeartbeatDue = false
			g.emitLocked(now)
		}
		return
	}
	if g.snapshot.State != signalLifecycleLost && now.Sub(g.snapshot.LastSuccessAt) >= SignalConnectionLossGrace {
		g.snapshot.State = signalLifecycleLost
		g.snapshot.LastReason = "connection_loss_grace_exceeded"
		g.snapshot.HeartbeatDue = false
		g.emitLocked(now)
		return
	}
	if !lifecycleClosedState(g.snapshot.State) {
		g.snapshot.HeartbeatDue = now.Sub(g.snapshot.LastAttemptAt) >= SignalHeartbeatInterval
	}
}

func (g *SignalSessionLifecycleGuard) Evaluate(now time.Time) SignalSessionLifecycleSnapshot {
	if g == nil {
		return SignalSessionLifecycleSnapshot{}
	}
	now = trustedArbiterNow(now)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.evaluateLocked(now)
	return g.snapshot
}

func (g *SignalSessionLifecycleGuard) RecordSuccess(now time.Time, reason string) error {
	if g == nil {
		return errors.New("signal lifecycle guard is required")
	}
	now = trustedArbiterNow(now)
	reason = normalizeLifecycleReason(reason)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.evaluateLocked(now)
	if lifecycleClosedState(g.snapshot.State) {
		return errors.New("closed signal lifecycle cannot recover without re-admission")
	}
	g.snapshot.State = signalLifecycleHealthy
	g.snapshot.HeartbeatDue = false
	g.snapshot.ConsecutiveFailures = 0
	g.snapshot.LastSuccessAt = now
	g.snapshot.LastAttemptAt = now
	g.snapshot.LastReason = reason
	g.emitLocked(now)
	return nil
}

func (g *SignalSessionLifecycleGuard) RecordFailure(now time.Time, reason string) SignalSessionLifecycleSnapshot {
	if g == nil {
		return SignalSessionLifecycleSnapshot{}
	}
	now = trustedArbiterNow(now)
	reason = normalizeLifecycleReason(reason)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.evaluateLocked(now)
	if lifecycleClosedState(g.snapshot.State) {
		return g.snapshot
	}
	g.snapshot.LastAttemptAt = now
	g.snapshot.ConsecutiveFailures++
	g.snapshot.LastReason = reason
	if now.Sub(g.snapshot.LastSuccessAt) >= SignalConnectionLossGrace {
		g.snapshot.State = signalLifecycleLost
		g.snapshot.HeartbeatDue = false
	} else {
		g.snapshot.State = signalLifecycleDegraded
		g.snapshot.HeartbeatDue = false
	}
	g.emitLocked(now)
	return g.snapshot
}

func (g *SignalSessionLifecycleGuard) Terminate(now time.Time, reason string) SignalSessionLifecycleSnapshot {
	if g == nil {
		return SignalSessionLifecycleSnapshot{}
	}
	now = trustedArbiterNow(now)
	reason = normalizeLifecycleReason(reason)
	if reason == "" {
		reason = "terminal"
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.snapshot.State == signalLifecycleTerminal {
		return g.snapshot
	}
	g.snapshot.State = signalLifecycleTerminal
	g.snapshot.HeartbeatDue = false
	g.snapshot.LastAttemptAt = now
	g.snapshot.TerminalReason = reason
	g.snapshot.LastReason = reason
	g.emitLocked(now)
	return g.snapshot
}
