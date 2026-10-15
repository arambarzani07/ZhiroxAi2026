package remoteagent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// TrustedChallengeReplayGuard must atomically consume a challenge key exactly
// once. Implementations should retain consumed keys until the supplied expiry.
type TrustedChallengeReplayGuard interface {
	ConsumeTrustedChallenge(key string, expiresAt time.Time) (consumed bool, err error)
}

func TrustedChallengeReplayKey(challenge TrustedChallenge) (string, error) {
	canonical, err := challenge.canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyTrustedControllerOnce performs cryptographic verification first, then
// atomically consumes the challenge so the same signed challenge cannot create
// more than one authorization. It does not execute any OS input.
func VerifyTrustedControllerOnce(
	registry TrustedControllerRegistry,
	replayGuard TrustedChallengeReplayGuard,
	challenge TrustedChallenge,
	signature []byte,
	now time.Time,
) (TrustedAuthorization, error) {
	if replayGuard == nil {
		return TrustedAuthorization{}, errors.New("trusted challenge replay guard is required")
	}
	auth, err := VerifyTrustedController(registry, challenge, signature, now)
	if err != nil {
		return TrustedAuthorization{}, err
	}
	key, err := TrustedChallengeReplayKey(challenge)
	if err != nil {
		return TrustedAuthorization{}, err
	}
	consumed, err := replayGuard.ConsumeTrustedChallenge(key, challenge.ExpiresAt.UTC())
	if err != nil {
		return TrustedAuthorization{}, err
	}
	if !consumed {
		return TrustedAuthorization{}, errors.New("trusted challenge was already consumed")
	}
	return auth, nil
}
