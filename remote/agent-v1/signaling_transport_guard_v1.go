package remoteagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxInboundSignalBatch      = 256
	maxSignalFutureClockSkew   = 5 * time.Minute
	transportDirectionInbound  = "inbound"
	transportDirectionOutbound = "outbound"
	transportDispositionAccept = "accepted"
	transportDispositionIgnore = "ignored"
	transportDispositionReject = "rejected"
)

// SignalTransportObservation is metadata-only transport telemetry. It never
// contains SDP, ICE candidates, control payloads, key data, pointer coordinates,
// clipboard data, files, or arbitrary commands.
type SignalTransportObservation struct {
	SessionID    string
	Direction    string
	Disposition  string
	SignalID     int64
	Kind         string
	Sender       string
	PayloadBytes int
	Reason       string
	ObservedAt   time.Time
}

// SignalTransportObserver receives metadata-only transport observations.
// It deliberately has no return value so telemetry cannot create an execution
// path or bypass a fail-closed validation result.
type SignalTransportObserver interface {
	RecordSignalTransportObservation(observation SignalTransportObservation)
}

type SignalTransportSnapshot struct {
	SessionID        string
	InboundAccepted  uint64
	InboundIgnored   uint64
	InboundRejected  uint64
	OutboundAccepted uint64
	OutboundRejected uint64
	LastSignalID     int64
	LastObservedAt   time.Time
	LastRejectReason string
}

// SignalTransportGuard is scoped to exactly one signaling session. It validates
// cursor monotonicity, sender/kind boundaries, payload size/JSON shape, optional
// server timestamps, and session expiry before the orchestrator sees a signal.
// It performs no WebRTC mutation and has no control/input execution capability.
type SignalTransportGuard struct {
	mu        sync.Mutex
	sessionID string
	expiresAt time.Time
	observer  SignalTransportObserver
	snapshot  SignalTransportSnapshot
}

func NewSignalTransportGuard(sessionID string, expiresAt time.Time, observer SignalTransportObserver, now time.Time) (*SignalTransportGuard, error) {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	now = trustedArbiterNow(now)
	if expiresAt.IsZero() || !expiresAt.UTC().After(now) {
		return nil, errors.New("signal transport session expiry is invalid")
	}
	expiresAt = expiresAt.UTC()
	return &SignalTransportGuard{
		sessionID: sessionID,
		expiresAt: expiresAt,
		observer:  observer,
		snapshot:  SignalTransportSnapshot{SessionID: sessionID},
	}, nil
}

func normalizeSignalKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func allowedInboundSignalKind(kind string) bool {
	switch normalizeSignalKind(kind) {
	case "offer", "ice_candidate", "hangup", "answer":
		return true
	default:
		return false
	}
}

func allowedOutboundSignalKind(kind string) bool {
	switch normalizeSignalKind(kind) {
	case "answer", "ice_candidate", "hangup":
		return true
	default:
		return false
	}
}

func (g *SignalTransportGuard) emitLocked(observation SignalTransportObservation) {
	observation.SessionID = g.sessionID
	observation.ObservedAt = observation.ObservedAt.UTC()
	g.snapshot.LastObservedAt = observation.ObservedAt
	if observation.Disposition == transportDispositionReject {
		g.snapshot.LastRejectReason = observation.Reason
	}
	if g.observer != nil {
		g.observer.RecordSignalTransportObservation(observation)
	}
}

func (g *SignalTransportGuard) rejectLocked(direction string, signalID int64, kind, sender string, payloadBytes int, reason string, now time.Time) error {
	if direction == transportDirectionInbound {
		g.snapshot.InboundRejected++
	} else {
		g.snapshot.OutboundRejected++
	}
	g.emitLocked(SignalTransportObservation{
		Direction:    direction,
		Disposition:  transportDispositionReject,
		SignalID:     signalID,
		Kind:         normalizeSignalKind(kind),
		Sender:       strings.ToLower(strings.TrimSpace(sender)),
		PayloadBytes: payloadBytes,
		Reason:       reason,
		ObservedAt:   now,
	})
	return errors.New(reason)
}

// ValidateOutbound records only metadata and verifies that this agent emits only
// answer/ICE/hangup signaling within the bound session lifetime.
func (g *SignalTransportGuard) ValidateOutbound(kind string, payload any, now time.Time) error {
	if g == nil {
		return errors.New("signal transport guard is required")
	}
	now = trustedArbiterNow(now)
	payloadBytes := payloadSizeJSON(payload)
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.expiresAt.After(now) {
		return g.rejectLocked(transportDirectionOutbound, 0, kind, "agent", payloadBytes, "signal transport session is expired", now)
	}
	if !allowedOutboundSignalKind(kind) {
		return g.rejectLocked(transportDirectionOutbound, 0, kind, "agent", payloadBytes, "outbound signal kind is not allowed", now)
	}
	if err := pcOSRemoteValidateSignal(normalizeSignalKind(kind), payloadBytes); err != nil {
		return g.rejectLocked(transportDirectionOutbound, 0, kind, "agent", payloadBytes, "outbound signal payload violates safety limits", now)
	}
	g.snapshot.OutboundAccepted++
	g.emitLocked(SignalTransportObservation{
		Direction:    transportDirectionOutbound,
		Disposition:  transportDispositionAccept,
		Kind:         normalizeSignalKind(kind),
		Sender:       "agent",
		PayloadBytes: payloadBytes,
		ObservedAt:   now,
	})
	return nil
}

// FilterInboundBatch validates the full polled batch atomically before committing
// cursor/counter state. It returns only controller-origin rows while agent-origin
// echo rows are counted as ignored. The caller can use returned cursor to advance
// across all validated rows. Any malformed row rejects the complete batch.
func (g *SignalTransportGuard) FilterInboundBatch(signals []Signal, after int64, now time.Time) ([]Signal, int64, error) {
	if g == nil {
		return nil, after, errors.New("signal transport guard is required")
	}
	if after < 0 {
		return nil, after, errors.New("signal transport cursor cannot be negative")
	}
	now = trustedArbiterNow(now)
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.expiresAt.After(now) {
		return nil, after, g.rejectLocked(transportDirectionInbound, 0, "", "", 0, "signal transport session is expired", now)
	}
	if len(signals) > maxInboundSignalBatch {
		return nil, after, g.rejectLocked(transportDirectionInbound, 0, "", "", 0, "inbound signal batch exceeds approved limit", now)
	}
	if g.snapshot.LastSignalID > after {
		return nil, after, g.rejectLocked(transportDirectionInbound, after, "", "", 0, "signal transport cursor regressed", now)
	}

	cursor := after
	accepted := make([]Signal, 0, len(signals))
	observations := make([]SignalTransportObservation, 0, len(signals))
	var acceptedCount uint64
	var ignoredCount uint64

	for _, sig := range signals {
		payloadBytes := len(sig.Payload)
		kind := normalizeSignalKind(sig.Kind)
		sender := strings.ToLower(strings.TrimSpace(sig.Sender))
		if sig.ID <= cursor {
			return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, "inbound signal replay or ordering violation", now)
		}
		if payloadBytes == 0 || payloadBytes > pcOSRemoteMaxSignalBytes || !json.Valid(sig.Payload) {
			return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, "inbound signal payload is invalid", now)
		}
		if !allowedInboundSignalKind(kind) {
			return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, fmt.Sprintf("unsupported inbound signal kind %q", sig.Kind), now)
		}
		if sig.CreatedAt != "" {
			createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(sig.CreatedAt))
			if err != nil {
				return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, "inbound signal timestamp is invalid", now)
			}
			if createdAt.UTC().After(now.Add(maxSignalFutureClockSkew)) {
				return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, "inbound signal timestamp is too far in the future", now)
			}
		}
		if sender != "controller" && sender != "agent" {
			return nil, after, g.rejectLocked(transportDirectionInbound, sig.ID, kind, sender, payloadBytes, "inbound signal sender is not trusted", now)
		}

		cursor = sig.ID
		observation := SignalTransportObservation{
			Direction:    transportDirectionInbound,
			SignalID:     sig.ID,
			Kind:         kind,
			Sender:       sender,
			PayloadBytes: payloadBytes,
			ObservedAt:   now,
		}
		if sender == "agent" {
			ignoredCount++
			observation.Disposition = transportDispositionIgnore
			observation.Reason = "agent echo ignored"
		} else {
			acceptedCount++
			observation.Disposition = transportDispositionAccept
			accepted = append(accepted, sig)
		}
		observations = append(observations, observation)
	}

	g.snapshot.InboundAccepted += acceptedCount
	g.snapshot.InboundIgnored += ignoredCount
	if cursor > g.snapshot.LastSignalID {
		g.snapshot.LastSignalID = cursor
	}
	for _, observation := range observations {
		g.emitLocked(observation)
	}
	return accepted, cursor, nil
}

func (g *SignalTransportGuard) Snapshot() SignalTransportSnapshot {
	if g == nil {
		return SignalTransportSnapshot{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.snapshot
}
