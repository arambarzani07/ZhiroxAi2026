package remoteagent

import (
	"errors"
	"strings"
	"time"
)

type TrustedSessionCapabilityRequest struct {
	Screen   bool
	Mouse    bool
	Keyboard bool
}

type TrustedSessionGrant struct {
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

// GrantTrustedSession converts a verified, challenge-bound controller
// authorization into the exact capability set requested for that session.
// It cannot elevate permissions and performs no OS input execution.
func GrantTrustedSession(auth TrustedAuthorization, requested TrustedSessionCapabilityRequest, now time.Time) (TrustedSessionGrant, error) {
	if strings.TrimSpace(auth.ControllerID) == "" || strings.TrimSpace(auth.AgentID) == "" || strings.TrimSpace(auth.SessionID) == "" {
		return TrustedSessionGrant{}, errors.New("trusted authorization is not bound to a controller, agent and session")
	}
	if auth.Fingerprint == "" || auth.IssuedAt.IsZero() || auth.ExpiresAt.IsZero() || !auth.ExpiresAt.After(auth.IssuedAt) {
		return TrustedSessionGrant{}, errors.New("trusted authorization metadata is invalid")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if now.Before(auth.IssuedAt.Add(-5*time.Second)) || !now.Before(auth.ExpiresAt) {
		return TrustedSessionGrant{}, errors.New("trusted authorization expired or not yet valid")
	}
	if auth.ExpiresAt.Sub(auth.IssuedAt) > TrustedSessionTTL+5*time.Second {
		return TrustedSessionGrant{}, errors.New("trusted authorization TTL exceeds local policy")
	}
	if !requested.Screen {
		return TrustedSessionGrant{}, errors.New("screen capability is required")
	}
	if requested.Screen && !auth.Screen {
		return TrustedSessionGrant{}, errors.New("screen capability exceeds trusted-controller permissions")
	}
	if requested.Mouse && !auth.Mouse {
		return TrustedSessionGrant{}, errors.New("mouse capability exceeds trusted-controller permissions")
	}
	if requested.Keyboard && !auth.Keyboard {
		return TrustedSessionGrant{}, errors.New("keyboard capability exceeds trusted-controller permissions")
	}
	return TrustedSessionGrant{
		ControllerID: auth.ControllerID,
		AgentID:      auth.AgentID,
		SessionID:    auth.SessionID,
		Fingerprint:  auth.Fingerprint,
		Screen:       requested.Screen,
		Mouse:        requested.Mouse,
		Keyboard:     requested.Keyboard,
		IssuedAt:     now,
		ExpiresAt:    auth.ExpiresAt,
	}, nil
}
