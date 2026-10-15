package remoteagent

import (
	"errors"
	"strings"
	"time"
)

const LocalInputApprovalTTL = 2 * time.Minute

// LocalInputApproval is a short-lived, session-bound proof that the interactive
// user on the target PC explicitly approved the requested input capabilities.
// This type is policy state only; it does not show UI, activate a dispatcher,
// or execute OS input.
type LocalInputApproval struct {
	ApprovalID       string
	ControllerID     string
	AgentID          string
	SessionID        string
	Fingerprint      string
	Mouse            bool
	Keyboard         bool
	InteractiveUser  bool
	ApprovedAt       time.Time
	ExpiresAt        time.Time
}

// AttendedControlAuthorization is the final policy decision immediately before
// an implementation may consider enabling an abstract input dispatcher. It
// requires both a live trusted-controller permit and explicit local approval.
type AttendedControlAuthorization struct {
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

func validateLocalInputApproval(approval LocalInputApproval, permit TrustedControlPermit, now time.Time) error {
	if strings.TrimSpace(approval.ApprovalID) == "" || len(strings.TrimSpace(approval.ApprovalID)) > 200 {
		return errors.New("local input approval id is invalid")
	}
	if !approval.InteractiveUser {
		return errors.New("explicit interactive local approval is required")
	}
	if approval.ControllerID != permit.ControllerID || approval.AgentID != permit.AgentID || approval.SessionID != permit.SessionID {
		return errors.New("local input approval identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(approval.Fingerprint)) != strings.ToLower(strings.TrimSpace(permit.Fingerprint)) {
		return errors.New("local input approval fingerprint mismatch")
	}
	if approval.ApprovedAt.IsZero() || approval.ExpiresAt.IsZero() || !approval.ExpiresAt.After(approval.ApprovedAt) {
		return errors.New("local input approval timestamps are invalid")
	}
	if approval.ExpiresAt.Sub(approval.ApprovedAt) > LocalInputApprovalTTL+5*time.Second {
		return errors.New("local input approval TTL exceeds policy")
	}
	if now.Before(approval.ApprovedAt.Add(-5*time.Second)) || !approval.ExpiresAt.After(now) {
		return errors.New("local input approval is expired or not yet valid")
	}
	if approval.Mouse && !permit.Mouse {
		return errors.New("local mouse approval exceeds trusted permit")
	}
	if approval.Keyboard && !permit.Keyboard {
		return errors.New("local keyboard approval exceeds trusted permit")
	}
	if permit.Mouse && !approval.Mouse {
		return errors.New("trusted mouse capability lacks matching local approval")
	}
	if permit.Keyboard && !approval.Keyboard {
		return errors.New("trusted keyboard capability lacks matching local approval")
	}
	if !approval.Mouse && !approval.Keyboard {
		return errors.New("local input approval contains no input capability")
	}
	return nil
}

// AuthorizeAttendedControl combines the continuously validated trusted permit
// with a fresh explicit local approval. The shorter expiry wins. It deliberately
// performs no ControlDispatcher.Activate call and no platform-specific input.
func AuthorizeAttendedControl(
	arbiter *TrustedSessionArbiter,
	permit TrustedControlPermit,
	approval LocalInputApproval,
	now time.Time,
) (AttendedControlAuthorization, error) {
	if arbiter == nil {
		return AttendedControlAuthorization{}, errors.New("trusted session arbiter is required")
	}
	now = trustedArbiterNow(now)
	if err := permit.Validate(arbiter, now); err != nil {
		return AttendedControlAuthorization{}, err
	}
	if err := validateLocalInputApproval(approval, permit, now); err != nil {
		return AttendedControlAuthorization{}, err
	}
	expiresAt := permit.ExpiresAt.UTC()
	if approval.ExpiresAt.Before(expiresAt) {
		expiresAt = approval.ExpiresAt.UTC()
	}
	return AttendedControlAuthorization{
		ApprovalID:        strings.TrimSpace(approval.ApprovalID),
		ControllerID:      permit.ControllerID,
		AgentID:           permit.AgentID,
		SessionID:         permit.SessionID,
		Fingerprint:       strings.ToLower(strings.TrimSpace(permit.Fingerprint)),
		Mouse:             permit.Mouse,
		Keyboard:          permit.Keyboard,
		ArbiterGeneration: permit.ArbiterGeneration,
		IssuedAt:          now,
		ExpiresAt:         expiresAt,
	}, nil
}

// Validate re-checks the trusted permit and the local approval on every input
// enable/use boundary so revocation, expiry, emergency stop or approval expiry
// fails closed.
func (a AttendedControlAuthorization) Validate(
	arbiter *TrustedSessionArbiter,
	permit TrustedControlPermit,
	approval LocalInputApproval,
	now time.Time,
) error {
	now = trustedArbiterNow(now)
	if strings.TrimSpace(a.ApprovalID) == "" || a.ControllerID == "" || a.AgentID == "" || a.SessionID == "" || a.Fingerprint == "" {
		return errors.New("attended control authorization identity is incomplete")
	}
	if a.IssuedAt.IsZero() || a.ExpiresAt.IsZero() || !a.ExpiresAt.After(a.IssuedAt) || !a.ExpiresAt.After(now) {
		return errors.New("attended control authorization is expired or invalid")
	}
	if a.ControllerID != permit.ControllerID || a.AgentID != permit.AgentID || a.SessionID != permit.SessionID || a.ArbiterGeneration != permit.ArbiterGeneration {
		return errors.New("attended control authorization no longer matches trusted permit")
	}
	if a.Mouse != permit.Mouse || a.Keyboard != permit.Keyboard {
		return errors.New("attended control authorization capability mismatch")
	}
	if strings.ToLower(strings.TrimSpace(a.Fingerprint)) != strings.ToLower(strings.TrimSpace(permit.Fingerprint)) {
		return errors.New("attended control authorization fingerprint mismatch")
	}
	if err := permit.Validate(arbiter, now); err != nil {
		return err
	}
	if err := validateLocalInputApproval(approval, permit, now); err != nil {
		return err
	}
	if strings.TrimSpace(approval.ApprovalID) != a.ApprovalID {
		return errors.New("attended control authorization approval id mismatch")
	}
	return nil
}
