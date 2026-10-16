package remoteagent

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type TrustedSignalingIntent struct {
	SessionID   string
	ControllerID string
	Fingerprint string
	Requested   TrustedSessionCapabilityRequest
	ExpiresAt   time.Time
}

type TrustedSignalingBinding struct {
	ControllerID string
	SessionID    string
	Fingerprint  string
	Screen       bool
	Mouse        bool
	Keyboard     bool
	ExpiresAt    time.Time
}

func ParseTrustedSignalingIntent(p PendingSession, now time.Time) (TrustedSignalingIntent, error) {
	if err := pcOSRemoteValidateSessionID(p.ID); err != nil {
		return TrustedSignalingIntent{}, err
	}
	if strings.TrimSpace(p.State) != "pending" {
		return TrustedSignalingIntent{}, errors.New("trusted signaling session must be pending")
	}
	controllerID := strings.TrimSpace(p.ControllerID)
	if controllerID == "" || len(controllerID) > 160 {
		return TrustedSignalingIntent{}, errors.New("trusted signaling controller id is invalid")
	}
	fingerprint := strings.ToLower(strings.TrimSpace(p.ControllerFingerprint))
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != 32 {
		return TrustedSignalingIntent{}, errors.New("trusted signaling controller fingerprint is invalid")
	}
	caps, err := ParseRemoteCapabilitiesV2(p.RequestedCapabilities)
	if err != nil {
		return TrustedSignalingIntent{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(p.ExpiresAt))
	if err != nil {
		return TrustedSignalingIntent{}, errors.New("trusted signaling expiry is invalid")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	expiresAt = expiresAt.UTC()
	if !expiresAt.After(now) {
		return TrustedSignalingIntent{}, errors.New("trusted signaling session is expired")
	}
	return TrustedSignalingIntent{
		SessionID:    p.ID,
		ControllerID: controllerID,
		Fingerprint:  fingerprint,
		Requested: TrustedSessionCapabilityRequest{
			Screen:   caps.ScreenStream,
			Mouse:    caps.Mouse,
			Keyboard: caps.Keyboard,
		},
		ExpiresAt: expiresAt,
	}, nil
}

// BindTrustedGrantToSignaling requires the server-reported pending session and
// the cryptographically authorized controller grant to match exactly. This is
// a control-plane binding only; it does not activate or execute OS input.
func BindTrustedGrantToSignaling(grant TrustedSessionGrant, intent TrustedSignalingIntent, now time.Time) (TrustedSignalingBinding, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if !grant.ExpiresAt.After(now) || !intent.ExpiresAt.After(now) {
		return TrustedSignalingBinding{}, errors.New("trusted signaling grant or session is expired")
	}
	if grant.ControllerID != intent.ControllerID || grant.SessionID != intent.SessionID {
		return TrustedSignalingBinding{}, errors.New("trusted signaling identity binding mismatch")
	}
	if strings.ToLower(strings.TrimSpace(grant.Fingerprint)) != intent.Fingerprint {
		return TrustedSignalingBinding{}, errors.New("trusted signaling fingerprint mismatch")
	}
	if grant.Screen != intent.Requested.Screen || grant.Mouse != intent.Requested.Mouse || grant.Keyboard != intent.Requested.Keyboard {
		return TrustedSignalingBinding{}, errors.New("trusted signaling capability mismatch")
	}
	expiresAt := grant.ExpiresAt.UTC()
	if intent.ExpiresAt.Before(expiresAt) {
		expiresAt = intent.ExpiresAt.UTC()
	}
	return TrustedSignalingBinding{
		ControllerID: grant.ControllerID,
		SessionID:    grant.SessionID,
		Fingerprint:  intent.Fingerprint,
		Screen:       grant.Screen,
		Mouse:        grant.Mouse,
		Keyboard:     grant.Keyboard,
		ExpiresAt:    expiresAt,
	}, nil
}
