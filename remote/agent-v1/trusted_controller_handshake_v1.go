package remoteagent

import (
	"crypto/ed25519"
	"errors"
	"time"
)

// TrustedHandshakeStart is the immutable control-plane state produced before a
// controller proves possession of its private key. It is bound to one pending
// signaling session and does not activate or execute OS input.
type TrustedHandshakeStart struct {
	Intent    TrustedSignalingIntent
	Challenge TrustedChallenge
}

// TrustedHandshakeResult is the final, cryptographically verified session
// binding. Producing this value still performs no OS input execution.
type TrustedHandshakeResult struct {
	Authorization TrustedAuthorization
	Grant         TrustedSessionGrant
	Binding       TrustedSignalingBinding
}

// StartTrustedControllerHandshake converts a pending signaling session into a
// short-lived challenge bound to the exact controller, agent and session.
func StartTrustedControllerHandshake(pending PendingSession, agentID string, now time.Time) (TrustedHandshakeStart, error) {
	intent, err := ParseTrustedSignalingIntent(pending, now)
	if err != nil {
		return TrustedHandshakeStart{}, err
	}
	challenge, err := NewTrustedChallenge(intent.ControllerID, agentID, intent.SessionID, now)
	if err != nil {
		return TrustedHandshakeStart{}, err
	}
	if challenge.ExpiresAt.After(intent.ExpiresAt) {
		challenge.ExpiresAt = intent.ExpiresAt.UTC()
		if !challenge.ExpiresAt.After(challenge.IssuedAt) {
			return TrustedHandshakeStart{}, errors.New("trusted signaling session expires before challenge can be issued")
		}
	}
	return TrustedHandshakeStart{Intent: intent, Challenge: challenge}, nil
}

// CompleteTrustedControllerHandshake verifies the signed challenge exactly
// once, grants only the capabilities requested by the bound signaling session,
// and requires controller/session/fingerprint/capability identity to match.
// This is authorization only and deliberately does not activate an input sink.
func CompleteTrustedControllerHandshake(
	registry TrustedControllerRegistry,
	replayGuard TrustedChallengeReplayGuard,
	start TrustedHandshakeStart,
	signature []byte,
	now time.Time,
) (TrustedHandshakeResult, error) {
	if start.Challenge.ControllerID != start.Intent.ControllerID || start.Challenge.SessionID != start.Intent.SessionID {
		return TrustedHandshakeResult{}, errors.New("trusted handshake challenge does not match signaling intent")
	}
	if start.Challenge.AgentID == "" {
		return TrustedHandshakeResult{}, errors.New("trusted handshake agent id is missing")
	}
	if len(signature) != ed25519.SignatureSize {
		return TrustedHandshakeResult{}, errors.New("trusted handshake signature is invalid")
	}
	auth, err := VerifyTrustedControllerOnce(registry, replayGuard, start.Challenge, signature, now)
	if err != nil {
		return TrustedHandshakeResult{}, err
	}
	grant, err := GrantTrustedSession(auth, start.Intent.Requested, now)
	if err != nil {
		return TrustedHandshakeResult{}, err
	}
	binding, err := BindTrustedGrantToSignaling(grant, start.Intent, now)
	if err != nil {
		return TrustedHandshakeResult{}, err
	}
	return TrustedHandshakeResult{Authorization: auth, Grant: grant, Binding: binding}, nil
}
