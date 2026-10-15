package remoteagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TrustedControllerProtocolVersion = 1
	TrustedChallengeTTL               = 60 * time.Second
	TrustedSessionTTL                 = 15 * time.Minute
)

type TrustedControllerRecord struct {
	ID              string
	Label           string
	PublicKey       ed25519.PublicKey
	Revoked         bool
	AllowedScreen   bool
	AllowedMouse    bool
	AllowedKeyboard bool
}

type TrustedControllerRegistry interface {
	LookupTrustedController(id string) (TrustedControllerRecord, bool, error)
}

type TrustedChallenge struct {
	Version      int
	ControllerID string
	AgentID      string
	SessionID    string
	Nonce        string
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

type TrustedAuthorization struct {
	ControllerID string
	AgentID      string
	SessionID    string
	Label        string
	Fingerprint  string
	Screen       bool
	Mouse        bool
	Keyboard     bool
	IssuedAt     time.Time
	ExpiresAt    time.Time
}

func NewTrustedChallenge(controllerID, agentID, sessionID string, now time.Time) (TrustedChallenge, error) {
	controllerID = strings.TrimSpace(controllerID)
	agentID = strings.TrimSpace(agentID)
	sessionID = strings.TrimSpace(sessionID)
	if controllerID == "" || len(controllerID) > 160 {
		return TrustedChallenge{}, errors.New("trusted controller id is invalid")
	}
	if agentID == "" || len(agentID) > 160 {
		return TrustedChallenge{}, errors.New("trusted agent id is invalid")
	}
	if sessionID == "" || len(sessionID) > 160 {
		return TrustedChallenge{}, errors.New("trusted session id is invalid")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return TrustedChallenge{}, fmt.Errorf("generate trusted challenge nonce: %w", err)
	}
	return TrustedChallenge{
		Version:      TrustedControllerProtocolVersion,
		ControllerID: controllerID,
		AgentID:      agentID,
		SessionID:    sessionID,
		Nonce:        base64.RawURLEncoding.EncodeToString(buf),
		IssuedAt:     now.UTC(),
		ExpiresAt:    now.UTC().Add(TrustedChallengeTTL),
	}, nil
}

func (c TrustedChallenge) canonical() ([]byte, error) {
	if c.Version != TrustedControllerProtocolVersion {
		return nil, errors.New("unsupported trusted-controller protocol")
	}
	if c.ControllerID == "" || c.AgentID == "" || c.SessionID == "" || c.Nonce == "" {
		return nil, errors.New("trusted challenge is incomplete")
	}
	if c.IssuedAt.IsZero() || c.ExpiresAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) {
		return nil, errors.New("trusted challenge timestamps are invalid")
	}
	return []byte(fmt.Sprintf(
		"zhirox-trusted-controller-v%d\ncontroller=%s\nagent=%s\nsession=%s\nnonce=%s\nissued=%s\nexpires=%s\n",
		c.Version,
		c.ControllerID,
		c.AgentID,
		c.SessionID,
		c.Nonce,
		c.IssuedAt.UTC().Format(time.RFC3339Nano),
		c.ExpiresAt.UTC().Format(time.RFC3339Nano),
	)), nil
}

func TrustedControllerFingerprint(key ed25519.PublicKey) (string, error) {
	if len(key) != ed25519.PublicKeySize {
		return "", errors.New("trusted controller public key is invalid")
	}
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:]), nil
}

func VerifyTrustedController(
	registry TrustedControllerRegistry,
	challenge TrustedChallenge,
	signature []byte,
	now time.Time,
) (TrustedAuthorization, error) {
	if registry == nil {
		return TrustedAuthorization{}, errors.New("trusted controller registry is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(challenge.IssuedAt.Add(-5*time.Second)) || !now.Before(challenge.ExpiresAt) {
		return TrustedAuthorization{}, errors.New("trusted challenge expired or not yet valid")
	}
	if challenge.ExpiresAt.Sub(challenge.IssuedAt) > TrustedChallengeTTL+5*time.Second {
		return TrustedAuthorization{}, errors.New("trusted challenge TTL exceeds local policy")
	}
	record, ok, err := registry.LookupTrustedController(challenge.ControllerID)
	if err != nil {
		return TrustedAuthorization{}, err
	}
	if !ok || record.Revoked {
		return TrustedAuthorization{}, errors.New("trusted controller is unavailable")
	}
	if record.ID != challenge.ControllerID || len(record.PublicKey) != ed25519.PublicKeySize {
		return TrustedAuthorization{}, errors.New("trusted controller record is invalid")
	}
	if !record.AllowedScreen {
		return TrustedAuthorization{}, errors.New("trusted controller lacks screen permission")
	}
	canonical, err := challenge.canonical()
	if err != nil {
		return TrustedAuthorization{}, err
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(record.PublicKey, canonical, signature) {
		return TrustedAuthorization{}, errors.New("trusted controller signature verification failed")
	}
	fingerprint, err := TrustedControllerFingerprint(record.PublicKey)
	if err != nil {
		return TrustedAuthorization{}, err
	}
	now = now.UTC()
	return TrustedAuthorization{
		ControllerID: record.ID,
		AgentID:      challenge.AgentID,
		SessionID:    challenge.SessionID,
		Label:        strings.TrimSpace(record.Label),
		Fingerprint:  fingerprint,
		Screen:       record.AllowedScreen,
		Mouse:        record.AllowedMouse,
		Keyboard:     record.AllowedKeyboard,
		IssuedAt:     now,
		ExpiresAt:    now.Add(TrustedSessionTTL),
	}, nil
}
