package remoteagent

import (
	"errors"
	"strings"
	"sync"
)

// AttendedExecutionAuditIntegrityGuard serializes and validates the metadata-only
// attended execution audit lifecycle before forwarding it to the configured sink.
// It provides process-local duplicate suppression by EventID and enforces one
// open -> one close transition per execution session. It never carries control
// payloads and has no operating-system execution capability.
type AttendedExecutionAuditIntegrityGuard struct {
	mu         sync.Mutex
	downstream AttendedExecutionAuditSink
	seen       map[string]AttendedExecutionAuditEvent
	sessions   map[string]attendedExecutionAuditSessionState
}

type attendedExecutionAuditSessionState struct {
	opened AttendedExecutionAuditEvent
	closed bool
}

func NewAttendedExecutionAuditIntegrityGuard(downstream AttendedExecutionAuditSink) (*AttendedExecutionAuditIntegrityGuard, error) {
	if downstream == nil {
		return nil, errors.New("attended execution audit downstream is required")
	}
	return &AttendedExecutionAuditIntegrityGuard{
		downstream: downstream,
		seen:       make(map[string]AttendedExecutionAuditEvent),
		sessions:   make(map[string]attendedExecutionAuditSessionState),
	}, nil
}

func validateAttendedExecutionAuditEvent(event AttendedExecutionAuditEvent) error {
	if err := validateAttendedExecutionTokenID(event.EventID, attendedMockAuditEventIDBytes); err != nil {
		return errors.New("attended execution audit event id is invalid")
	}
	if err := validateAttendedExecutionTokenID(event.ExecutionSessionID, attendedExecutionSessionIDBytes); err != nil {
		return errors.New("attended execution audit session id is invalid")
	}
	if event.EventType != AttendedMockAuditSessionOpened && event.EventType != AttendedMockAuditSessionClosed {
		return errors.New("attended execution audit event type is invalid")
	}
	if strings.TrimSpace(event.RequestID) == "" || strings.TrimSpace(event.ReviewID) == "" || strings.TrimSpace(event.ContractVersion) == "" || strings.TrimSpace(event.ExecutorID) == "" || strings.TrimSpace(event.ControllerID) == "" || strings.TrimSpace(event.AgentID) == "" || strings.TrimSpace(event.SessionID) == "" {
		return errors.New("attended execution audit identity is incomplete")
	}
	if !event.Mouse && !event.Keyboard {
		return errors.New("attended execution audit capabilities are empty")
	}
	if event.OccurredAt.IsZero() {
		return errors.New("attended execution audit timestamp is required")
	}
	return nil
}

func attendedExecutionAuditBindingEqual(a, b AttendedExecutionAuditEvent) bool {
	return a.ExecutionSessionID == b.ExecutionSessionID &&
		a.RequestID == b.RequestID &&
		a.ReviewID == b.ReviewID &&
		a.ContractVersion == b.ContractVersion &&
		a.ExecutorID == b.ExecutorID &&
		a.ControllerID == b.ControllerID &&
		a.AgentID == b.AgentID &&
		a.SessionID == b.SessionID &&
		a.Mouse == b.Mouse &&
		a.Keyboard == b.Keyboard &&
		a.ArbiterGeneration == b.ArbiterGeneration
}

func attendedExecutionAuditEventEqual(a, b AttendedExecutionAuditEvent) bool {
	return a.EventID == b.EventID &&
		a.EventType == b.EventType &&
		attendedExecutionAuditBindingEqual(a, b) &&
		a.OccurredAt.Equal(b.OccurredAt)
}

// RecordAttendedExecutionAudit enforces audit ordering and immutable binding.
// The downstream write is attempted before committing local state, so a transient
// downstream failure can be retried with the same stable EventID. A downstream
// implementation should also deduplicate by EventID for durable idempotency.
func (g *AttendedExecutionAuditIntegrityGuard) RecordAttendedExecutionAudit(event AttendedExecutionAuditEvent) error {
	if g == nil {
		return errors.New("attended execution audit integrity guard is required")
	}
	if err := validateAttendedExecutionAuditEvent(event); err != nil {
		return err
	}
	event.OccurredAt = event.OccurredAt.UTC()

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.downstream == nil {
		return errors.New("attended execution audit downstream is unavailable")
	}
	if g.seen == nil {
		g.seen = make(map[string]AttendedExecutionAuditEvent)
	}
	if g.sessions == nil {
		g.sessions = make(map[string]attendedExecutionAuditSessionState)
	}

	if existing, ok := g.seen[event.EventID]; ok {
		if attendedExecutionAuditEventEqual(existing, event) {
			return nil
		}
		return errors.New("attended execution audit event id collision")
	}

	state, exists := g.sessions[event.ExecutionSessionID]
	switch event.EventType {
	case AttendedMockAuditSessionOpened:
		if exists {
			return errors.New("attended execution audit session has already opened")
		}
		if err := g.downstream.RecordAttendedExecutionAudit(event); err != nil {
			return err
		}
		g.seen[event.EventID] = event
		g.sessions[event.ExecutionSessionID] = attendedExecutionAuditSessionState{opened: event}
		return nil

	case AttendedMockAuditSessionClosed:
		if !exists {
			return errors.New("attended execution audit close has no matching open")
		}
		if state.closed {
			return errors.New("attended execution audit session has already closed")
		}
		if !attendedExecutionAuditBindingEqual(state.opened, event) {
			return errors.New("attended execution audit close binding mismatch")
		}
		if event.OccurredAt.Before(state.opened.OccurredAt) {
			return errors.New("attended execution audit timestamp regressed")
		}
		if err := g.downstream.RecordAttendedExecutionAudit(event); err != nil {
			return err
		}
		g.seen[event.EventID] = event
		state.closed = true
		g.sessions[event.ExecutionSessionID] = state
		return nil
	default:
		return errors.New("attended execution audit event type is invalid")
	}
}

// SnapshotState returns only lifecycle counters for tests/diagnostics. It does
// not expose control data because none is accepted by this audit layer.
func (g *AttendedExecutionAuditIntegrityGuard) SnapshotState() (events int, sessions int, openSessions int) {
	if g == nil {
		return 0, 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, state := range g.sessions {
		if !state.closed {
			openSessions++
		}
	}
	return len(g.seen), len(g.sessions), openSessions
}

var _ AttendedExecutionAuditSink = (*AttendedExecutionAuditIntegrityGuard)(nil)
