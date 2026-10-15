package remoteagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

type memoryTrustedRegistry struct {
	record TrustedControllerRecord
	ok     bool
}

func (m memoryTrustedRegistry) LookupTrustedController(id string) (TrustedControllerRecord, bool, error) {
	if !m.ok || m.record.ID != id {
		return TrustedControllerRecord{}, false, nil
	}
	return m.record, true, nil
}

func TestTrustedControllerChallengeVerification(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	challenge, err := NewTrustedChallenge("controller-1", "Server-c24b5c0f", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := challenge.canonical()
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, canonical)
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID: "controller-1", Label: "Owner browser", PublicKey: pub,
		AllowedScreen: true, AllowedMouse: true, AllowedKeyboard: true,
	}}
	auth, err := VerifyTrustedController(registry, challenge, sig, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Screen || !auth.Mouse || !auth.Keyboard {
		t.Fatalf("expected approved trusted capabilities: %+v", auth)
	}
	if auth.Fingerprint == "" || !auth.ExpiresAt.After(now) {
		t.Fatalf("expected fingerprint and bounded expiry: %+v", auth)
	}
}

func TestTrustedControllerRejectsRevoked(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	challenge, err := NewTrustedChallenge("controller-1", "agent-1", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := challenge.canonical()
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID: "controller-1", PublicKey: pub, Revoked: true, AllowedScreen: true,
	}}
	if _, err := VerifyTrustedController(registry, challenge, ed25519.Sign(priv, canonical), now.Add(time.Second)); err == nil {
		t.Fatal("expected revoked controller to be rejected")
	}
}

func TestTrustedControllerRejectsExpiredChallenge(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	challenge, err := NewTrustedChallenge("controller-1", "agent-1", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := challenge.canonical()
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID: "controller-1", PublicKey: pub, AllowedScreen: true,
	}}
	if _, err := VerifyTrustedController(registry, challenge, ed25519.Sign(priv, canonical), challenge.ExpiresAt); err == nil {
		t.Fatal("expected expired challenge to be rejected")
	}
}

func TestTrustedControllerRejectsTamperedSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	challenge, err := NewTrustedChallenge("controller-1", "agent-1", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := challenge.canonical()
	sig := ed25519.Sign(priv, canonical)
	sig[0] ^= 0xff
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID: "controller-1", PublicKey: pub, AllowedScreen: true,
	}}
	if _, err := VerifyTrustedController(registry, challenge, sig, now.Add(time.Second)); err == nil {
		t.Fatal("expected tampered signature to be rejected")
	}
}
