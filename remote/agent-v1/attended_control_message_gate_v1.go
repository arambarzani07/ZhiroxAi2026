package remoteagent

import (
	"errors"
	"time"
)

// ControlMessageGate is the per-message authorization boundary for the WebRTC
// control DataChannel. It is policy-only and must not execute platform input.
type ControlMessageGate interface {
	ValidateControlMessage(now time.Time) error
}

// AttendedControlMessageGate re-validates the trusted permit and explicit local
// approval before every control message reaches the abstract dispatcher.
// Revocation, expiry, emergency-stop, session replacement, or approval expiry
// therefore fail closed even after the DataChannel is already open.
type AttendedControlMessageGate struct {
	Arbiter       *TrustedSessionArbiter
	Permit        TrustedControlPermit
	Approval      LocalInputApproval
	Authorization AttendedControlAuthorization
}

func (g *AttendedControlMessageGate) ValidateControlMessage(now time.Time) error {
	if g == nil || g.Arbiter == nil {
		return errors.New("attended control message gate is unavailable")
	}
	return g.Authorization.Validate(
		g.Arbiter,
		g.Permit,
		g.Approval,
		trustedArbiterNow(now),
	)
}
