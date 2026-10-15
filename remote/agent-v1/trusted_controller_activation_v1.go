package remoteagent

import (
	"errors"
	"strings"
	"time"
)

// TrustedActivationLease is a short-lived authorization envelope derived only
// from a completed trusted-controller handshake. It is a control-plane object:
// it does not activate a ControlDispatcher and does not execute OS input.
type TrustedActivationLease struct {
	ControllerID string
	AgentID      string
	SessionID    string
	Fingerprint  string
	Screen       bool
	Mouse        bool
	Keyboard     bool
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

// NewTrustedActivationLease seals the final handshake identities, capabilities,
// and expiry into one fail-closed lease. The shortest upstream expiry wins.
func NewTrustedActivationLease(result TrustedHandshakeResult, now time.Time) (TrustedActivationLease, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	auth := result.Authorization
	grant := result.Grant
	binding := result.Binding

	if strings.TrimSpace(auth.ControllerID) == "" || strings.TrimSpace(auth.AgentID) == "" || strings.TrimSpace(auth.SessionID) == "" {
		return TrustedActivationLease{}, errors.New("trusted activation authorization identity is incomplete")
	}
	if auth.ControllerID != grant.ControllerID || auth.AgentID != grant.AgentID || auth.SessionID != grant.SessionID {
		return TrustedActivationLease{}, errors.New("trusted activation authorization-to-grant identity mismatch")
	}
	if grant.ControllerID != binding.ControllerID || grant.SessionID != binding.SessionID {
		return TrustedActivationLease{}, errors.New("trusted activation grant-to-signaling identity mismatch")
	}
	if auth.Fingerprint == "" || auth.Fingerprint != grant.Fingerprint || grant.Fingerprint != binding.Fingerprint {
		return TrustedActivationLease{}, errors.New("trusted activation fingerprint mismatch")
	}
	if !binding.Screen || grant.Screen != binding.Screen || grant.Mouse != binding.Mouse || grant.Keyboard != binding.Keyboard {
		return TrustedActivationLease{}, errors.New("trusted activation capability binding mismatch")
	}
	if grant.Screen && !auth.Screen || grant.Mouse && !auth.Mouse || grant.Keyboard && !auth.Keyboard {
		return TrustedActivationLease{}, errors.New("trusted activation capabilities exceed authorization")
	}
	if auth.IssuedAt.IsZero() || grant.IssuedAt.IsZero() || auth.ExpiresAt.IsZero() || grant.ExpiresAt.IsZero() || binding.ExpiresAt.IsZero() {
		return TrustedActivationLease{}, errors.New("trusted activation timestamps are incomplete")
	}
	if now.Before(auth.IssuedAt.Add(-5*time.Second)) || now.Before(grant.IssuedAt.Add(-5*time.Second)) {
		return TrustedActivationLease{}, errors.New("trusted activation is not yet valid")
	}

	expiresAt := auth.ExpiresAt.UTC()
	if grant.ExpiresAt.Before(expiresAt) {
		expiresAt = grant.ExpiresAt.UTC()
	}
	if binding.ExpiresAt.Before(expiresAt) {
		expiresAt = binding.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return TrustedActivationLease{}, errors.New("trusted activation lease is expired")
	}
	if expiresAt.Sub(now) > TrustedSessionTTL+5*time.Second {
		return TrustedActivationLease{}, errors.New("trusted activation lease exceeds local TTL policy")
	}

	return TrustedActivationLease{
		ControllerID: grant.ControllerID,
		AgentID:      grant.AgentID,
		SessionID:    grant.SessionID,
		Fingerprint:  grant.Fingerprint,
		Screen:       grant.Screen,
		Mouse:        grant.Mouse,
		Keyboard:     grant.Keyboard,
		IssuedAt:     now,
		ExpiresAt:    expiresAt,
	}, nil
}

// ValidateFor requires the caller to present the exact controller, agent and
// session identity bound into the lease. It performs no side effects.
func (l TrustedActivationLease) ValidateFor(controllerID, agentID, sessionID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if controllerID != l.ControllerID || agentID != l.AgentID || sessionID != l.SessionID {
		return errors.New("trusted activation lease identity mismatch")
	}
	if l.IssuedAt.IsZero() || l.ExpiresAt.IsZero() || !l.ExpiresAt.After(l.IssuedAt) || !l.ExpiresAt.After(now) {
		return errors.New("trusted activation lease is expired or invalid")
	}
	if !l.Screen {
		return errors.New("trusted activation lease lacks screen capability")
	}
	return nil
}

func (l TrustedActivationLease) AllowsInput() bool {
	return l.Mouse || l.Keyboard
}
