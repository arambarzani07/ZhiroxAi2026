package remoteagent

import (
	"errors"
	"strings"
	"time"
)

const AttendedActivationReadinessTTL = 15 * time.Second

// AttendedActivationReadiness is a short-lived, non-executing proof that the
// attended runtime is still live and that the same trusted permit plus local
// approval remain valid immediately before any separately reviewed activation
// phase. This proof never activates the dispatcher and never executes OS input.
type AttendedActivationReadiness struct {
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

	return AttendedActivationReadiness{
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
