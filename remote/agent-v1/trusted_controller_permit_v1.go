package remoteagent

import (
	"errors"
	"strings"
	"time"
)

// TrustedControlPermit is the final fail-closed control-plane gate immediately
// before any platform-specific input implementation. It proves that a live
// activation lease is still the single arbiter holder and remains revocation-
// and emergency-stop clean. It never activates a dispatcher or executes OS input.
type TrustedControlPermit struct {
	ControllerID     string
	AgentID          string
	SessionID        string
	Fingerprint      string
	Mouse            bool
	Keyboard         bool
	ArbiterGeneration uint64
	IssuedAt         time.Time
	ExpiresAt        time.Time
}

// NewTrustedControlPermit derives a bounded permit only from the arbiter's
// current active holder. Screen-only leases cannot produce an input permit.
func NewTrustedControlPermit(arbiter *TrustedSessionArbiter, active TrustedActiveSession, now time.Time) (TrustedControlPermit, error) {
	if arbiter == nil {
		return TrustedControlPermit{}, errors.New("trusted session arbiter is required")
	}
	now = trustedArbiterNow(now)
	lease := active.Lease
	if !lease.AllowsInput() {
		return TrustedControlPermit{}, errors.New("trusted activation lease does not allow input")
	}
	fingerprint := strings.ToLower(strings.TrimSpace(lease.Fingerprint))
	if err := arbiter.ValidateActive(lease.ControllerID, lease.AgentID, lease.SessionID, fingerprint, now); err != nil {
		return TrustedControlPermit{}, err
	}
	status := arbiter.Status(now)
	if status.EmergencyStop.Engaged || status.Active == nil {
		return TrustedControlPermit{}, errors.New("trusted controller session is not active")
	}
	if status.Active.Generation != active.Generation || status.Generation != active.Generation {
		return TrustedControlPermit{}, errors.New("trusted controller arbiter generation mismatch")
	}
	current := status.Active.Lease
	if current.ControllerID != lease.ControllerID || current.AgentID != lease.AgentID || current.SessionID != lease.SessionID || strings.ToLower(strings.TrimSpace(current.Fingerprint)) != fingerprint {
		return TrustedControlPermit{}, errors.New("trusted controller active holder changed")
	}
	if current.Mouse != lease.Mouse || current.Keyboard != lease.Keyboard || current.Screen != lease.Screen {
		return TrustedControlPermit{}, errors.New("trusted controller active capabilities changed")
	}
	if !current.ExpiresAt.After(now) {
		return TrustedControlPermit{}, errors.New("trusted controller active lease expired")
	}
	return TrustedControlPermit{
		ControllerID:      current.ControllerID,
		AgentID:           current.AgentID,
		SessionID:         current.SessionID,
		Fingerprint:       fingerprint,
		Mouse:             current.Mouse,
		Keyboard:          current.Keyboard,
		ArbiterGeneration: active.Generation,
		IssuedAt:          now,
		ExpiresAt:         current.ExpiresAt.UTC(),
	}, nil
}

// Validate re-checks the arbiter on every use boundary. Release, expiry,
// revocation, emergency-stop, or holder replacement invalidates the permit.
func (p TrustedControlPermit) Validate(arbiter *TrustedSessionArbiter, now time.Time) error {
	if arbiter == nil {
		return errors.New("trusted session arbiter is required")
	}
	now = trustedArbiterNow(now)
	if strings.TrimSpace(p.ControllerID) == "" || strings.TrimSpace(p.AgentID) == "" || strings.TrimSpace(p.SessionID) == "" || strings.TrimSpace(p.Fingerprint) == "" {
		return errors.New("trusted control permit identity is incomplete")
	}
	if p.IssuedAt.IsZero() || p.ExpiresAt.IsZero() || !p.ExpiresAt.After(p.IssuedAt) || !p.ExpiresAt.After(now) {
		return errors.New("trusted control permit is expired or invalid")
	}
	if !p.Mouse && !p.Keyboard {
		return errors.New("trusted control permit has no input capability")
	}
	if err := arbiter.ValidateActive(p.ControllerID, p.AgentID, p.SessionID, p.Fingerprint, now); err != nil {
		return err
	}
	status := arbiter.Status(now)
	if status.EmergencyStop.Engaged || status.Active == nil {
		return errors.New("trusted controller session is not active")
	}
	if status.Generation != p.ArbiterGeneration || status.Active.Generation != p.ArbiterGeneration {
		return errors.New("trusted control permit was invalidated by arbiter generation change")
	}
	lease := status.Active.Lease
	if lease.ControllerID != p.ControllerID || lease.AgentID != p.AgentID || lease.SessionID != p.SessionID || strings.ToLower(strings.TrimSpace(lease.Fingerprint)) != strings.ToLower(strings.TrimSpace(p.Fingerprint)) {
		return errors.New("trusted control permit holder mismatch")
	}
	if lease.Mouse != p.Mouse || lease.Keyboard != p.Keyboard || !lease.Screen {
		return errors.New("trusted control permit capability mismatch")
	}
	if !lease.ExpiresAt.Equal(p.ExpiresAt) {
		return errors.New("trusted control permit expiry mismatch")
	}
	return nil
}
