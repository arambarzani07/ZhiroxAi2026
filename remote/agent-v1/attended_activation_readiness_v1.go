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
	AttendedActivationReadinessTTL = 15 * time.Second
	attendedReadinessIDBytes       = 16
)

// AttendedActivationReadiness is a short-lived, non-executing proof that the
// attended runtime is still live and that the same trusted permit plus local
// approval remain valid immediately before any separately reviewed activation
// phase. This proof never activates the dispatcher and never executes OS input.
type AttendedActivationReadiness struct {
	ReadinessID       string
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

func mintAttendedReadinessID() (string, error) {
	var raw [attendedReadinessIDBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("failed to mint attended activation readiness id")
	}
	return hex.EncodeToString(raw[:]), nil
}

func validateAttendedReadinessID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) != attendedReadinessIDBytes*2 {
		return errors.New("attended activation readiness id is invalid")
	}
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != attendedReadinessIDBytes {
		return errors.New("attended activation readiness id is invalid")
	}
	return nil
}

// PrepareAttendedActivationReadiness re-validates the runtime-owned attended
// gate and visible-runtime liveness. The abstract dispatcher remains inactive.
func PrepareAttendedActivationReadiness(runtime *AttendedControlRuntime, now time.Time) (AttendedActivationReadiness, error) {
	if runtime == nil {
		return AttendedActivationReadiness{}, errors.New("attended control runtime is required")
	}
	if !runtime.isActive() {
		return AttendedActivationReadiness{}, errors.New("attended control runtime is not active")
	}
	if runtime.dispatcher == nil {
		return AttendedActivationReadiness{}, errors.New("attended runtime control dispatcher is required")
	}
	if runtime.gate == nil {
		return AttendedActivationReadiness{}, errors.New("attended runtime control gate is required")
	}

	now = trustedArbiterNow(now)
	if err := runtime.gate.ValidateControlMessage(now); err != nil {
		runtime.dispatcher.Deactivate()
		return AttendedActivationReadiness{}, err
	}
	auth := runtime.gate.Authorization
	if !auth.Mouse && !auth.Keyboard {
		return AttendedActivationReadiness{}, errors.New("attended authorization contains no input capability")
	}

	expiresAt := now.Add(AttendedActivationReadinessTTL)
	if auth.ExpiresAt.Before(expiresAt) {
		expiresAt = auth.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return AttendedActivationReadiness{}, errors.New("attended activation readiness would already be expired")
	}
	readinessID, err := mintAttendedReadinessID()
	if err != nil {
		return AttendedActivationReadiness{}, err
	}

	return AttendedActivationReadiness{
		ReadinessID:       readinessID,
		ApprovalID:        strings.TrimSpace(auth.ApprovalID),
		ControllerID:      auth.ControllerID,
		AgentID:           auth.AgentID,
		SessionID:         auth.SessionID,
		Fingerprint:       strings.ToLower(strings.TrimSpace(auth.Fingerprint)),
		Mouse:             auth.Mouse,
		Keyboard:          auth.Keyboard,
		ArbiterGeneration: auth.ArbiterGeneration,
		IssuedAt:          now,
		ExpiresAt:         expiresAt,
	}, nil
}

// Validate re-checks runtime liveness and the runtime-owned attended gate so a
// stale readiness proof fails closed after indicator exit, revocation,
// emergency stop, capability drift, or approval expiry.
func (r AttendedActivationReadiness) Validate(runtime *AttendedControlRuntime, now time.Time) error {
	if runtime == nil {
		return errors.New("attended control runtime is required")
	}
	if !runtime.isActive() {
		return errors.New("attended control runtime is not active")
	}
	if runtime.gate == nil || runtime.dispatcher == nil {
		return errors.New("attended runtime is incomplete")
	}
	if err := validateAttendedReadinessID(r.ReadinessID); err != nil {
		return err
	}
	now = trustedArbiterNow(now)
	if r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.IssuedAt) || !r.ExpiresAt.After(now) {
		return errors.New("attended activation readiness is expired or invalid")
	}
	if r.ExpiresAt.Sub(r.IssuedAt) > AttendedActivationReadinessTTL+time.Second {
		return errors.New("attended activation readiness TTL exceeds policy")
	}
	if err := runtime.gate.ValidateControlMessage(now); err != nil {
		runtime.dispatcher.Deactivate()
		return err
	}
	auth := runtime.gate.Authorization
	if r.ApprovalID != strings.TrimSpace(auth.ApprovalID) || r.ControllerID != auth.ControllerID || r.AgentID != auth.AgentID || r.SessionID != auth.SessionID || r.ArbiterGeneration != auth.ArbiterGeneration {
		return errors.New("attended activation readiness identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(r.Fingerprint)) != strings.ToLower(strings.TrimSpace(auth.Fingerprint)) {
		return errors.New("attended activation readiness fingerprint mismatch")
	}
	if r.Mouse != auth.Mouse || r.Keyboard != auth.Keyboard || (!r.Mouse && !r.Keyboard) {
		return errors.New("attended activation readiness capability mismatch")
	}
	return nil
}

// AttendedActivationGrant is an immutable, non-executing record that one fresh
// readiness proof was consumed exactly once. It is the last policy object a
// separately reviewed execution implementation may inspect in a future phase.
// Creating or validating this grant never activates a dispatcher or injects OS input.
type AttendedActivationGrant struct {
	ReadinessID       string
	ApprovalID        string
	ControllerID      string
	AgentID           string
	SessionID         string
	Fingerprint       string
	Mouse             bool
	Keyboard          bool
	ArbiterGeneration uint64
	IssuedAt          time.Time
	ConsumedAt        time.Time
	ExpiresAt         time.Time
}

// AttendedActivationConsumer provides process-local one-time consumption of
// readiness proofs. Concurrent replay attempts are serialized and fail closed.
type AttendedActivationConsumer struct {
	mu       sync.Mutex
	consumed map[string]time.Time
}

func NewAttendedActivationConsumer() *AttendedActivationConsumer {
	return &AttendedActivationConsumer{consumed: make(map[string]time.Time)}
}

func (c *AttendedActivationConsumer) Consume(runtime *AttendedControlRuntime, readiness AttendedActivationReadiness, now time.Time) (AttendedActivationGrant, error) {
	if c == nil {
		return AttendedActivationGrant{}, errors.New("attended activation consumer is required")
	}
	now = trustedArbiterNow(now)
	if err := readiness.Validate(runtime, now); err != nil {
		return AttendedActivationGrant{}, err
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
	if _, exists := c.consumed[readiness.ReadinessID]; exists {
		return AttendedActivationGrant{}, errors.New("attended activation readiness has already been consumed")
	}
	c.consumed[readiness.ReadinessID] = readiness.ExpiresAt

	return AttendedActivationGrant{
		ReadinessID:       readiness.ReadinessID,
		ApprovalID:        readiness.ApprovalID,
		ControllerID:      readiness.ControllerID,
		AgentID:           readiness.AgentID,
		SessionID:         readiness.SessionID,
		Fingerprint:       readiness.Fingerprint,
		Mouse:             readiness.Mouse,
		Keyboard:          readiness.Keyboard,
		ArbiterGeneration: readiness.ArbiterGeneration,
		IssuedAt:          readiness.IssuedAt,
		ConsumedAt:        now,
		ExpiresAt:         readiness.ExpiresAt,
	}, nil
}

// Validate re-checks the complete live attended runtime after one-time
// consumption. A grant cannot outlive its readiness proof or authorization.
func (g AttendedActivationGrant) Validate(runtime *AttendedControlRuntime, now time.Time) error {
	if runtime == nil {
		return errors.New("attended control runtime is required")
	}
	if !runtime.isActive() {
		return errors.New("attended control runtime is not active")
	}
	if runtime.gate == nil || runtime.dispatcher == nil {
		return errors.New("attended runtime is incomplete")
	}
	if err := validateAttendedReadinessID(g.ReadinessID); err != nil {
		return err
	}
	now = trustedArbiterNow(now)
	if g.IssuedAt.IsZero() || g.ConsumedAt.IsZero() || g.ExpiresAt.IsZero() || !g.ExpiresAt.After(g.IssuedAt) || !g.ExpiresAt.After(g.ConsumedAt) || !g.ExpiresAt.After(now) {
		return errors.New("attended activation grant is expired or invalid")
	}
	if g.ConsumedAt.Before(g.IssuedAt.Add(-time.Second)) {
		return errors.New("attended activation grant consumption time is invalid")
	}
	if err := runtime.gate.ValidateControlMessage(now); err != nil {
		runtime.dispatcher.Deactivate()
		return err
	}
	auth := runtime.gate.Authorization
	if g.ApprovalID != strings.TrimSpace(auth.ApprovalID) || g.ControllerID != auth.ControllerID || g.AgentID != auth.AgentID || g.SessionID != auth.SessionID || g.ArbiterGeneration != auth.ArbiterGeneration {
		return errors.New("attended activation grant identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(g.Fingerprint)) != strings.ToLower(strings.TrimSpace(auth.Fingerprint)) {
		return errors.New("attended activation grant fingerprint mismatch")
	}
	if g.Mouse != auth.Mouse || g.Keyboard != auth.Keyboard || (!g.Mouse && !g.Keyboard) {
		return errors.New("attended activation grant capability mismatch")
	}
	return nil
}
