package remoteagent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	AttendedExecutionRequestTTL     = 3 * time.Second
	AttendedExecutionSessionTTL     = 4 * time.Second
	attendedExecutionRequestIDBytes = 16
	attendedExecutionSessionIDBytes = 16
)

// AttendedExecutionRequest is a short-lived, metadata-only request derived from
// one valid reviewed execution binding. It carries no mouse/keyboard payload and
// cannot activate a dispatcher or invoke operating-system input APIs.
type AttendedExecutionRequest struct {
	RequestID         string
	ReviewID          string
	ContractVersion   string
	ExecutorID        string
	GrantReadinessID  string
	ApprovalID        string
	ControllerID      string
	AgentID           string
	SessionID         string
	Fingerprint       string
	Mouse             bool
	Keyboard          bool
	ArbiterGeneration uint64
	IssuedAt          time.Time
	ExpiresAt         time.Time
}

func mintAttendedExecutionTokenID(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("attended execution token size is invalid")
	}
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", errors.New("failed to mint attended execution token")
	}
	return hex.EncodeToString(raw), nil
}

func validateAttendedExecutionTokenID(id string, size int) error {
	id = strings.TrimSpace(id)
	if len(id) != size*2 {
		return errors.New("attended execution token id is invalid")
	}
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != size {
		return errors.New("attended execution token id is invalid")
	}
	return nil
}

// PrepareAttendedExecutionRequest seals one still-valid reviewed execution
// binding into a very short-lived one-time request. No control payload is
// accepted at this layer.
func PrepareAttendedExecutionRequest(runtime *AttendedControlRuntime, grant AttendedActivationGrant, review AttendedExecutionReview, executor ReviewedAttendedExecutor, now time.Time) (AttendedExecutionRequest, error) {
	if err := review.Validate(runtime, grant, executor, now); err != nil {
		return AttendedExecutionRequest{}, err
	}
	now = trustedArbiterNow(now)

	expiresAt := now.Add(AttendedExecutionRequestTTL)
	if review.ExpiresAt.Before(expiresAt) {
		expiresAt = review.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return AttendedExecutionRequest{}, errors.New("attended execution request would already be expired")
	}

	requestID, err := mintAttendedExecutionTokenID(attendedExecutionRequestIDBytes)
	if err != nil {
		return AttendedExecutionRequest{}, err
	}

	return AttendedExecutionRequest{
		RequestID:         requestID,
		ReviewID:          review.ReviewID,
		ContractVersion:   review.ContractVersion,
		ExecutorID:        review.ExecutorID,
		GrantReadinessID:  review.GrantReadinessID,
		ApprovalID:        review.ApprovalID,
		ControllerID:      review.ControllerID,
		AgentID:           review.AgentID,
		SessionID:         review.SessionID,
		Fingerprint:       strings.ToLower(strings.TrimSpace(review.Fingerprint)),
		Mouse:             review.Mouse,
		Keyboard:          review.Keyboard,
		ArbiterGeneration: review.ArbiterGeneration,
		IssuedAt:          now,
		ExpiresAt:         expiresAt,
	}, nil
}

// Validate re-checks the complete reviewed binding. It has no execution side
// effects and rejects identity, capability, executor, or expiry drift.
func (r AttendedExecutionRequest) Validate(runtime *AttendedControlRuntime, grant AttendedActivationGrant, review AttendedExecutionReview, executor ReviewedAttendedExecutor, now time.Time) error {
	if err := validateAttendedExecutionTokenID(r.RequestID, attendedExecutionRequestIDBytes); err != nil {
		return err
	}
	if err := review.Validate(runtime, grant, executor, now); err != nil {
		return err
	}
	now = trustedArbiterNow(now)
	if r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.IssuedAt) || !r.ExpiresAt.After(now) {
		return errors.New("attended execution request is expired or invalid")
	}
	if r.ExpiresAt.Sub(r.IssuedAt) > AttendedExecutionRequestTTL+time.Second {
		return errors.New("attended execution request TTL exceeds policy")
	}
	if r.ReviewID != review.ReviewID || r.ContractVersion != review.ContractVersion || r.ExecutorID != review.ExecutorID || r.GrantReadinessID != review.GrantReadinessID || r.ApprovalID != review.ApprovalID || r.ControllerID != review.ControllerID || r.AgentID != review.AgentID || r.SessionID != review.SessionID || r.ArbiterGeneration != review.ArbiterGeneration {
		return errors.New("attended execution request identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(r.Fingerprint)) != strings.ToLower(strings.TrimSpace(review.Fingerprint)) {
		return errors.New("attended execution request fingerprint mismatch")
	}
	if r.Mouse != review.Mouse || r.Keyboard != review.Keyboard || (!r.Mouse && !r.Keyboard) {
		return errors.New("attended execution request capability mismatch")
	}
	return nil
}

// AttendedExecutionRequestConsumer provides process-local exactly-once request
// consumption. It only opens a non-executing lifecycle object; no OS input path
// exists here.
type AttendedExecutionRequestConsumer struct {
	mu       sync.Mutex
	consumed map[string]time.Time
}

func NewAttendedExecutionRequestConsumer() *AttendedExecutionRequestConsumer {
	return &AttendedExecutionRequestConsumer{consumed: make(map[string]time.Time)}
}

// AttendedExecutionSession is a bounded lifecycle record proving that one
// request was consumed exactly once while the reviewed attended chain remained
// valid. It intentionally contains no control payload and no execution method.
type AttendedExecutionSession struct {
	mu                sync.Mutex
	ExecutionSessionID string
	RequestID          string
	ReviewID           string
	ContractVersion    string
	ExecutorID         string
	GrantReadinessID   string
	ApprovalID         string
	ControllerID       string
	AgentID            string
	SessionID          string
	Fingerprint        string
	Mouse              bool
	Keyboard           bool
	ArbiterGeneration  uint64
	IssuedAt           time.Time
	ExpiresAt          time.Time
	closed             bool
	closedAt           time.Time
}

func (c *AttendedExecutionRequestConsumer) Consume(runtime *AttendedControlRuntime, grant AttendedActivationGrant, review AttendedExecutionReview, executor ReviewedAttendedExecutor, request AttendedExecutionRequest, now time.Time) (*AttendedExecutionSession, error) {
	if c == nil {
		return nil, errors.New("attended execution request consumer is required")
	}
	if err := request.Validate(runtime, grant, review, executor, now); err != nil {
		return nil, err
	}
	now = trustedArbiterNow(now)

	executionSessionID, err := mintAttendedExecutionTokenID(attendedExecutionSessionIDBytes)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.consumed == nil {
		c.consumed = make(map[string]time.Time)
	}
	for id, expiry := range c.consumed {
		if !expiry.After(now) {
			delete(c.consumed, id)
		}
	}
	if _, exists := c.consumed[request.RequestID]; exists {
		return nil, errors.New("attended execution request has already been consumed")
	}
	if err := request.Validate(runtime, grant, review, executor, now); err != nil {
		return nil, err
	}
	c.consumed[request.RequestID] = request.ExpiresAt

	expiresAt := now.Add(AttendedExecutionSessionTTL)
	if review.ExpiresAt.Before(expiresAt) {
		expiresAt = review.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return nil, errors.New("attended execution session would already be expired")
	}

	return &AttendedExecutionSession{
		ExecutionSessionID: executionSessionID,
		RequestID:          request.RequestID,
		ReviewID:           request.ReviewID,
		ContractVersion:    request.ContractVersion,
		ExecutorID:         request.ExecutorID,
		GrantReadinessID:   request.GrantReadinessID,
		ApprovalID:         request.ApprovalID,
		ControllerID:       request.ControllerID,
		AgentID:            request.AgentID,
		SessionID:          request.SessionID,
		Fingerprint:        request.Fingerprint,
		Mouse:              request.Mouse,
		Keyboard:           request.Keyboard,
		ArbiterGeneration:  request.ArbiterGeneration,
		IssuedAt:           now,
		ExpiresAt:          expiresAt,
	}, nil
}

// Validate continuously re-checks runtime/review validity and the immutable
// request binding. Closing the lifecycle object fails closed immediately.
func (s *AttendedExecutionSession) Validate(runtime *AttendedControlRuntime, grant AttendedActivationGrant, review AttendedExecutionReview, executor ReviewedAttendedExecutor, now time.Time) error {
	if s == nil {
		return errors.New("attended execution session is required")
	}
	if err := validateAttendedExecutionTokenID(s.ExecutionSessionID, attendedExecutionSessionIDBytes); err != nil {
		return err
	}
	if err := review.Validate(runtime, grant, executor, now); err != nil {
		return err
	}
	now = trustedArbiterNow(now)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("attended execution session is closed")
	}
	if s.IssuedAt.IsZero() || s.ExpiresAt.IsZero() || !s.ExpiresAt.After(s.IssuedAt) || !s.ExpiresAt.After(now) {
		return errors.New("attended execution session is expired or invalid")
	}
	if s.ExpiresAt.Sub(s.IssuedAt) > AttendedExecutionSessionTTL+time.Second {
		return errors.New("attended execution session TTL exceeds policy")
	}
	if s.ReviewID != review.ReviewID || s.ContractVersion != review.ContractVersion || s.ExecutorID != review.ExecutorID || s.GrantReadinessID != review.GrantReadinessID || s.ApprovalID != review.ApprovalID || s.ControllerID != review.ControllerID || s.AgentID != review.AgentID || s.SessionID != review.SessionID || s.ArbiterGeneration != review.ArbiterGeneration {
		return errors.New("attended execution session identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(s.Fingerprint)) != strings.ToLower(strings.TrimSpace(review.Fingerprint)) {
		return errors.New("attended execution session fingerprint mismatch")
	}
	if s.Mouse != review.Mouse || s.Keyboard != review.Keyboard || (!s.Mouse && !s.Keyboard) {
		return errors.New("attended execution session capability mismatch")
	}
	return nil
}

// Close terminates only this metadata lifecycle. It is idempotent and does not
// call a dispatcher or platform input API.
func (s *AttendedExecutionSession) Close(now time.Time) error {
	if s == nil {
		return errors.New("attended execution session is required")
	}
	now = trustedArbiterNow(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.closedAt = now
	return nil
}
