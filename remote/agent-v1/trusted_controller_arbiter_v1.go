package remoteagent

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// TrustedControllerRevocationChecker is the fail-closed hook used by the local
// session arbiter before a lease is acquired or treated as still active.
type TrustedControllerRevocationChecker interface {
	IsTrustedControllerRevoked(controllerID, fingerprint string) (bool, error)
}

type TrustedActiveSession struct {
	Lease      TrustedActivationLease
	AcquiredAt time.Time
	Generation uint64
}

type TrustedEmergencyStopState struct {
	Engaged              bool
	Reason               string
	EngagedAt            time.Time
	ResetAt              *time.Time
	ResetAuthorizationID string
}

type TrustedArbiterStatus struct {
	Active        *TrustedActiveSession
	EmergencyStop TrustedEmergencyStopState
	Generation    uint64
}

// TrustedSessionArbiter enforces one controller session at a time and a local
// emergency-stop latch. It is control-plane policy only: it never activates a
// ControlDispatcher and never performs OS input.
type TrustedSessionArbiter struct {
	mu         sync.Mutex
	revocation TrustedControllerRevocationChecker
	active     *TrustedActiveSession
	emergency  TrustedEmergencyStopState
	generation uint64
}

func NewTrustedSessionArbiter(revocation TrustedControllerRevocationChecker) (*TrustedSessionArbiter, error) {
	if revocation == nil {
		return nil, errors.New("trusted controller revocation checker is required")
	}
	return &TrustedSessionArbiter{revocation: revocation}, nil
}

func trustedArbiterNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

// Acquire accepts only one non-expired, non-revoked activation lease at a time.
// Concurrent or duplicate acquisition attempts fail closed.
func (a *TrustedSessionArbiter) Acquire(lease TrustedActivationLease, now time.Time) (TrustedActiveSession, error) {
	if a == nil || a.revocation == nil {
		return TrustedActiveSession{}, errors.New("trusted session arbiter is unavailable")
	}
	now = trustedArbiterNow(now)
	if err := lease.ValidateFor(lease.ControllerID, lease.AgentID, lease.SessionID, now); err != nil {
		return TrustedActiveSession{}, err
	}
	fingerprint := strings.ToLower(strings.TrimSpace(lease.Fingerprint))
	if fingerprint == "" {
		return TrustedActiveSession{}, errors.New("trusted activation lease fingerprint is missing")
	}
	revoked, err := a.revocation.IsTrustedControllerRevoked(lease.ControllerID, fingerprint)
	if err != nil {
		return TrustedActiveSession{}, errors.New("trusted controller revocation state unavailable")
	}
	if revoked {
		return TrustedActiveSession{}, errors.New("trusted controller is revoked")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.emergency.Engaged {
		return TrustedActiveSession{}, errors.New("trusted session emergency stop is engaged")
	}
	if a.active != nil && !a.active.Lease.ExpiresAt.After(now) {
		a.active = nil
		a.generation++
	}
	if a.active != nil {
		return TrustedActiveSession{}, errors.New("another trusted controller session is already active")
	}
	a.generation++
	active := TrustedActiveSession{Lease: lease, AcquiredAt: now, Generation: a.generation}
	a.active = &active
	return active, nil
}

// ValidateActive requires exact holder identity/fingerprint, a live lease, no
// emergency-stop latch and a non-revoked controller.
func (a *TrustedSessionArbiter) ValidateActive(controllerID, agentID, sessionID, fingerprint string, now time.Time) error {
	if a == nil || a.revocation == nil {
		return errors.New("trusted session arbiter is unavailable")
	}
	now = trustedArbiterNow(now)
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.emergency.Engaged {
		return errors.New("trusted session emergency stop is engaged")
	}
	if a.active == nil {
		return errors.New("no trusted controller session is active")
	}
	if !a.active.Lease.ExpiresAt.After(now) {
		a.active = nil
		a.generation++
		return errors.New("trusted controller session lease expired")
	}
	lease := a.active.Lease
	if lease.ControllerID != controllerID || lease.AgentID != agentID || lease.SessionID != sessionID || strings.ToLower(strings.TrimSpace(lease.Fingerprint)) != fingerprint {
		return errors.New("trusted controller session holder mismatch")
	}
	revoked, err := a.revocation.IsTrustedControllerRevoked(controllerID, fingerprint)
	if err != nil {
		return errors.New("trusted controller revocation state unavailable")
	}
	if revoked {
		a.active = nil
		a.generation++
		return errors.New("trusted controller is revoked")
	}
	return nil
}

func (a *TrustedSessionArbiter) Release(controllerID, agentID, sessionID string) error {
	if a == nil {
		return errors.New("trusted session arbiter is unavailable")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		return errors.New("no trusted controller session is active")
	}
	lease := a.active.Lease
	if lease.ControllerID != controllerID || lease.AgentID != agentID || lease.SessionID != sessionID {
		return errors.New("trusted controller session holder mismatch")
	}
	a.active = nil
	a.generation++
	return nil
}

// InvalidateController is called after owner-authorized controller revocation to
// terminate any matching in-memory lease immediately. Future acquisition is
// still governed by the revocation checker.
func (a *TrustedSessionArbiter) InvalidateController(controllerID, fingerprint string, now time.Time) (bool, error) {
	if a == nil {
		return false, errors.New("trusted session arbiter is unavailable")
	}
	controllerID = strings.TrimSpace(controllerID)
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if controllerID == "" || fingerprint == "" {
		return false, errors.New("trusted controller invalidation identity is incomplete")
	}
	_ = trustedArbiterNow(now)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		return false, nil
	}
	lease := a.active.Lease
	if lease.ControllerID != controllerID || strings.ToLower(strings.TrimSpace(lease.Fingerprint)) != fingerprint {
		return false, nil
	}
	a.active = nil
	a.generation++
	return true, nil
}

// EngageEmergencyStop clears the active lease and latches the arbiter closed.
// Reset requires an explicit owner-authorization reference and never restores
// the prior session automatically.
func (a *TrustedSessionArbiter) EngageEmergencyStop(reason string, now time.Time) (TrustedEmergencyStopState, error) {
	if a == nil {
		return TrustedEmergencyStopState{}, errors.New("trusted session arbiter is unavailable")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 200 {
		return TrustedEmergencyStopState{}, errors.New("trusted emergency-stop reason is invalid")
	}
	now = trustedArbiterNow(now)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = nil
	a.generation++
	a.emergency = TrustedEmergencyStopState{Engaged: true, Reason: reason, EngagedAt: now}
	return a.emergency, nil
}

func (a *TrustedSessionArbiter) ResetEmergencyStop(ownerAuthorizationID string, now time.Time) (TrustedEmergencyStopState, error) {
	if a == nil {
		return TrustedEmergencyStopState{}, errors.New("trusted session arbiter is unavailable")
	}
	ownerAuthorizationID = strings.TrimSpace(ownerAuthorizationID)
	if ownerAuthorizationID == "" || len(ownerAuthorizationID) > 200 {
		return TrustedEmergencyStopState{}, errors.New("owner authorization is required to reset trusted emergency stop")
	}
	now = trustedArbiterNow(now)
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.emergency.Engaged {
		return TrustedEmergencyStopState{}, errors.New("trusted emergency stop is not engaged")
	}
	resetAt := now
	a.emergency.Engaged = false
	a.emergency.ResetAt = &resetAt
	a.emergency.ResetAuthorizationID = ownerAuthorizationID
	a.generation++
	return a.emergency, nil
}

func (a *TrustedSessionArbiter) Status(now time.Time) TrustedArbiterStatus {
	if a == nil {
		return TrustedArbiterStatus{}
	}
	now = trustedArbiterNow(now)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active != nil && !a.active.Lease.ExpiresAt.After(now) {
		a.active = nil
		a.generation++
	}
	status := TrustedArbiterStatus{EmergencyStop: a.emergency, Generation: a.generation}
	if a.active != nil {
		copyActive := *a.active
		status.Active = &copyActive
	}
	return status
}
