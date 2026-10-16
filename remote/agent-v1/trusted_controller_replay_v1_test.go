package remoteagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

type memoryReplayGuard struct {
	consumed map[string]time.Time
	fail     bool
}

func (m *memoryReplayGuard) ConsumeTrustedChallenge(key string, expiresAt time.Time) (bool, error) {
	if m.fail {
		return false, errors.New("replay store unavailable")
	}
	if m.consumed == nil {
		m.consumed = make(map[string]time.Time)
	}
	if _, exists := m.consumed[key]; exists {
		return false, nil
	}
	m.consumed[key] = expiresAt
	return true, nil
}

func signedTrustedChallenge(t *testing.T, now time.Time) (TrustedChallenge, []byte, memoryTrustedRegistry) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := NewTrustedChallenge("controller-1", "agent-1", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := challenge.canonical()
	if err != nil {
		t.Fatal(err)
	}
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID: "controller-1", PublicKey: pub, AllowedScreen: true, AllowedMouse: true,
	}}
	return challenge, ed25519.Sign(priv, canonical), registry
}

func TestVerifyTrustedControllerOnceRejectsReplay(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)
	challenge, signature, registry := signedTrustedChallenge(t, now)
	guard := &memoryReplayGuard{}

	if _, err := VerifyTrustedControllerOnce(registry, guard, challenge, signature, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyTrustedControllerOnce(registry, guard, challenge, signature, now.Add(2*time.Second)); err == nil {
		t.Fatal("expected replayed trusted challenge to be rejected")
	}
}

func TestVerifyTrustedControllerOnceDoesNotConsumeInvalidSignature(t *testing.T) {
	now := time.Now().UTC()
	challenge, signature, registry := signedTrustedChallenge(t, now)
	guard := &memoryReplayGuard{}

	bad := append([]byte(nil), signature...)
	bad[0] ^= 0xff
	if _, err := VerifyTrustedControllerOnce(registry, guard, challenge, bad, now.Add(time.Second)); err == nil {
		t.Fatal("expected invalid signature to fail")
	}
	if len(guard.consumed) != 0 {
		t.Fatal("invalid signature must not consume challenge")
	}
	if _, err := VerifyTrustedControllerOnce(registry, guard, challenge, signature, now.Add(2*time.Second)); err != nil {
		t.Fatalf("valid signature should still be accepted after invalid attempt: %v", err)
	}
}

func TestVerifyTrustedControllerOnceFailsClosedWhenReplayStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	challenge, signature, registry := signedTrustedChallenge(t, now)
	guard := &memoryReplayGuard{fail: true}
	if _, err := VerifyTrustedControllerOnce(registry, guard, challenge, signature, now.Add(time.Second)); err == nil {
		t.Fatal("expected replay store failure to fail closed")
	}
}

func TestTrustedChallengeReplayKeyBindsSession(t *testing.T) {
	now := time.Now().UTC()
	challenge, _, _ := signedTrustedChallenge(t, now)
	key1, err := TrustedChallengeReplayKey(challenge)
	if err != nil {
		t.Fatal(err)
	}
	challenge.SessionID = "session-2"
	key2, err := TrustedChallengeReplayKey(challenge)
	if err != nil {
		t.Fatal(err)
	}
	if key1 == key2 {
		t.Fatal("replay key must be bound to session")
	}
}
