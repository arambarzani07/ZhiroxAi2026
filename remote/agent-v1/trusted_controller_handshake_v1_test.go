package remoteagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func handshakeFixture(t *testing.T, now time.Time) (TrustedHandshakeStart, ed25519.PrivateKey, memoryTrustedRegistry, *memoryReplayGuard) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := TrustedControllerFingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingSession{
		ID:                    "session-123456789012345678901234",
		State:                 "pending",
		ExpiresAt:             now.Add(5 * time.Minute).Format(time.RFC3339Nano),
		ControllerID:          "controller-1",
		ControllerFingerprint: fingerprint,
		RequestedCapabilities: map[string]any{
			"screen_stream": true,
			"mouse":         true,
			"keyboard":      true,
			"clipboard":     false,
			"file_transfer": false,
			"audio":         false,
		},
	}
	start, err := StartTrustedControllerHandshake(pending, "agent-1", now)
	if err != nil {
		t.Fatal(err)
	}
	registry := memoryTrustedRegistry{ok: true, record: TrustedControllerRecord{
		ID:              "controller-1",
		Label:           "Owner browser",
		PublicKey:       pub,
		AllowedScreen:   true,
		AllowedMouse:    true,
		AllowedKeyboard: true,
	}}
	return start, priv, registry, &memoryReplayGuard{}
}

func signHandshake(t *testing.T, start TrustedHandshakeStart, priv ed25519.PrivateKey) []byte {
	t.Helper()
	canonical, err := start.Challenge.canonical()
	if err != nil {
		t.Fatal(err)
	}
	return ed25519.Sign(priv, canonical)
}

func TestTrustedHandshakeCompletesExactBoundSession(t *testing.T) {
	now := time.Date(2026, 10, 15, 13, 20, 0, 0, time.UTC)
	start, priv, registry, guard := handshakeFixture(t, now)
	result, err := CompleteTrustedControllerHandshake(registry, guard, start, signHandshake(t, start, priv), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Binding.ControllerID != start.Intent.ControllerID || result.Binding.SessionID != start.Intent.SessionID {
		t.Fatalf("unexpected trusted handshake binding: %+v", result.Binding)
	}
	if !result.Binding.Screen || !result.Binding.Mouse || !result.Binding.Keyboard {
		t.Fatalf("expected exact requested capabilities: %+v", result.Binding)
	}
}

func TestTrustedHandshakeRejectsReplay(t *testing.T) {
	now := time.Now().UTC()
	start, priv, registry, guard := handshakeFixture(t, now)
	sig := signHandshake(t, start, priv)
	if _, err := CompleteTrustedControllerHandshake(registry, guard, start, sig, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteTrustedControllerHandshake(registry, guard, start, sig, now.Add(2*time.Second)); err == nil {
		t.Fatal("expected completed handshake challenge replay to be rejected")
	}
}

func TestTrustedHandshakeRejectsIntentMutation(t *testing.T) {
	now := time.Now().UTC()
	start, priv, registry, guard := handshakeFixture(t, now)
	start.Intent.Requested.Keyboard = false
	if _, err := CompleteTrustedControllerHandshake(registry, guard, start, signHandshake(t, start, priv), now.Add(time.Second)); err == nil {
		t.Fatal("expected signaling intent mutation to be rejected")
	}
	if len(guard.consumed) != 0 {
		t.Fatal("mutated intent must fail before consuming challenge")
	}
}

func TestTrustedHandshakeRejectsFingerprintMismatch(t *testing.T) {
	now := time.Now().UTC()
	start, priv, registry, guard := handshakeFixture(t, now)
	start.Intent.Fingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := CompleteTrustedControllerHandshake(registry, guard, start, signHandshake(t, start, priv), now.Add(time.Second)); err == nil {
		t.Fatal("expected fingerprint mutation to be rejected")
	}
}

func TestTrustedHandshakeCapsChallengeAtPendingExpiry(t *testing.T) {
	now := time.Now().UTC()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := TrustedControllerFingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	pendingExpiry := now.Add(20 * time.Second)
	pending := PendingSession{
		ID:                    "session-123456789012345678901234",
		State:                 "pending",
		ExpiresAt:             pendingExpiry.Format(time.RFC3339Nano),
		ControllerID:          "controller-1",
		ControllerFingerprint: fingerprint,
		RequestedCapabilities: map[string]any{
			"screen_stream": true,
			"mouse": false,
			"keyboard": false,
			"clipboard": false,
			"file_transfer": false,
			"audio": false,
		},
	}
	start, err := StartTrustedControllerHandshake(pending, "agent-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Challenge.ExpiresAt.Equal(pendingExpiry) {
		t.Fatalf("expected challenge expiry to be capped by pending session: %s", start.Challenge.ExpiresAt)
	}
}
