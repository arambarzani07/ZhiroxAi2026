package remoteagent

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	AttendedMockAuditSessionOpened = "mock_session_opened"
	AttendedMockAuditSessionClosed = "mock_session_closed"
	attendedMockAuditEventIDBytes  = 16
)

// AttendedExecutionAuditEvent is metadata-only. It intentionally contains no
// control payload, key data, pointer coordinates, clipboard data, or OS command.
type AttendedExecutionAuditEvent struct {
	EventID            string
	EventType          string
	ExecutionSessionID string
	RequestID          string
	ReviewID           string
	ContractVersion    string
	ExecutorID         string
	ControllerID       string
	AgentID            string
	SessionID          string
	Mouse              bool
	Keyboard           bool
	ArbiterGeneration  uint64
	OccurredAt         time.Time
}

// AttendedExecutionAuditSink records metadata-only lifecycle events.
type AttendedExecutionAuditSink interface {
	RecordAttendedExecutionAudit(event AttendedExecutionAuditEvent) error
}

// MockReviewedAttendedExecutor is a non-executing adapter used to exercise the
// reviewed executor contract and lifecycle/audit behavior. It has no method that
// can inject input, activate a dispatcher, or accept a control payload.
type MockReviewedAttendedExecutor struct {
	id       string
	mouse    bool
	keyboard bool
	audit    AttendedExecutionAuditSink
}

func NewMockReviewedAttendedExecutor(id string, mouse, keyboard bool, audit AttendedExecutionAuditSink) (*MockReviewedAttendedExecutor, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("mock reviewed attended executor id is required")
	}
	if !mouse && !keyboard {
		return nil, errors.New("mock reviewed attended executor requires mouse or keyboard capability")
	}
	if audit == nil {
		return nil, errors.New("attended execution audit sink is required")
	}
	return &MockReviewedAttendedExecutor{id: id, mouse: mouse, keyboard: keyboard, audit: audit}, nil
}

func (m *MockReviewedAttendedExecutor) ExecutorID() string {
	if m == nil {
		return ""
	}
	return m.id
}

func (m *MockReviewedAttendedExecutor) SupportsAttendedCapabilities(mouse, keyboard bool) bool {
	if m == nil || (!mouse && !keyboard) {
		return false
	}
	if mouse && !m.mouse {
		return false
	}
	if keyboard && !m.keyboard {
		return false
	}
	return true
}

// MockAttendedExecutionObservation wraps one already-consumed metadata session
// and only revalidates/audits its lifecycle. It exposes no execution operation.
type MockAttendedExecutionObservation struct {
	mu       sync.Mutex
	adapter  *MockReviewedAttendedExecutor
	runtime  *AttendedControlRuntime
	grant    AttendedActivationGrant
	review   AttendedExecutionReview
	session  *AttendedExecutionSession
	closed   bool
	closedAt time.Time
}

func newAttendedMockAuditEvent(eventType string, session *AttendedExecutionSession, now time.Time) (AttendedExecutionAuditEvent, error) {
	if session == nil {
		return AttendedExecutionAuditEvent{}, errors.New("attended execution session is required")
	}
	if eventType != AttendedMockAuditSessionOpened && eventType != AttendedMockAuditSessionClosed {
		return AttendedExecutionAuditEvent{}, errors.New("attended mock audit event type is invalid")
	}
	eventID, err := mintAttendedExecutionTokenID(attendedMockAuditEventIDBytes)
	if err != nil {
		return AttendedExecutionAuditEvent{}, err
	}
	now = trustedArbiterNow(now)
	return AttendedExecutionAuditEvent{
		EventID:            eventID,
		EventType:          eventType,
		ExecutionSessionID: session.ExecutionSessionID,
		RequestID:          session.RequestID,
		ReviewID:           session.ReviewID,
		ContractVersion:    session.ContractVersion,
		ExecutorID:         session.ExecutorID,
		ControllerID:       session.ControllerID,
		AgentID:            session.AgentID,
		SessionID:          session.SessionID,
		Mouse:              session.Mouse,
		Keyboard:           session.Keyboard,
		ArbiterGeneration:  session.ArbiterGeneration,
		OccurredAt:         now,
	}, nil
}

// OpenMockAttendedExecutionObservation validates the complete attended chain and
// records one metadata-only open event. Audit failure closes the session and
// fails closed.
func OpenMockAttendedExecutionObservation(runtime *AttendedControlRuntime, grant AttendedActivationGrant, review AttendedExecutionReview, adapter *MockReviewedAttendedExecutor, session *AttendedExecutionSession, now time.Time) (*MockAttendedExecutionObservation, error) {
	if adapter == nil {
		return nil, errors.New("mock reviewed attended executor is required")
	}
	if session == nil {
		return nil, errors.New("attended execution session is required")
	}
	if err := session.Validate(runtime, grant, review, adapter, now); err != nil {
		return nil, err
	}
	event, err := newAttendedMockAuditEvent(AttendedMockAuditSessionOpened, session, now)
	if err != nil {
		_ = session.Close(now)
		return nil, err
	}
	if err := adapter.audit.RecordAttendedExecutionAudit(event); err != nil {
		_ = session.Close(now)
		return nil, errors.New("attended execution audit open failed")
	}
	return &MockAttendedExecutionObservation{
		adapter: adapter,
		runtime: runtime,
		grant:   grant,
		review:  review,
		session: session,
	}, nil
}

// Validate re-checks executor identity, runtime/revocation state and the bounded
// execution-session lifecycle. It has no control side effects.
func (o *MockAttendedExecutionObservation) Validate(now time.Time) error {
	if o == nil {
		return errors.New("mock attended execution observation is required")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errors.New("mock attended execution observation is closed")
	}
	if o.adapter == nil || o.session == nil {
		return errors.New("mock attended execution observation is invalid")
	}
	return o.session.Validate(o.runtime, o.grant, o.review, o.adapter, now)
}

// Close terminates the metadata session before recording the close audit event.
// It is idempotent. If the audit sink fails, the session still remains closed.
func (o *MockAttendedExecutionObservation) Close(now time.Time) error {
	if o == nil {
		return errors.New("mock attended execution observation is required")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	o.closedAt = trustedArbiterNow(now)
	if o.session == nil {
		return errors.New("mock attended execution observation session is missing")
	}
	if err := o.session.Close(now); err != nil {
		return err
	}
	if o.adapter == nil || o.adapter.audit == nil {
		return errors.New("attended execution audit sink is unavailable")
	}
	event, err := newAttendedMockAuditEvent(AttendedMockAuditSessionClosed, o.session, now)
	if err != nil {
		return err
	}
	if err := o.adapter.audit.RecordAttendedExecutionAudit(event); err != nil {
		return errors.New("attended execution audit close failed")
	}
	return nil
}
