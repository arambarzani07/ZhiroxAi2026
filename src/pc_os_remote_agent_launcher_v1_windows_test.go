//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func pcOSRemoteTestArtifact(t *testing.T, dir, name, body string) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	return path, hex.EncodeToString(sum[:])
}

func pcOSRemoteTestSpec(t *testing.T) pcOSRemoteAgentLaunchSpec {
	t.Helper()
	dir := t.TempDir()
	agent, agentSHA := pcOSRemoteTestArtifact(t, dir, "ZhiroxRemoteScreenAgent.exe", "agent")
	consent, consentSHA := pcOSRemoteTestArtifact(t, dir, "ZhiroxRemoteConsent.exe", "consent")
	encoder, encoderSHA := pcOSRemoteTestArtifact(t, dir, "ZhiroxMFH264Encoder.dll", "encoder")
	return pcOSRemoteAgentLaunchSpec{
		SignalURL:     "https://example.supabase.co/functions/v1/zhirox-pc-remote-signaling-v1",
		AgentPath:     agent,
		AgentSHA256:   agentSHA,
		ConsentPath:   consent,
		ConsentSHA256: consentSHA,
		EncoderPath:   encoder,
		EncoderSHA256: encoderSHA,
	}
}

func TestRemoteAgentLaunchSpecFailClosed(t *testing.T) {
	spec := pcOSRemoteTestSpec(t)
	normalized, err := pcOSRemoteNormalizeLaunchSpec(spec)
	if err != nil {
		t.Fatalf("valid launch spec rejected: %v", err)
	}
	if normalized.FPS != pcOSRemoteBridgeDefaultFPS || normalized.Bitrate != pcOSRemoteBridgeDefaultBitrate {
		t.Fatal("default media bounds were not applied")
	}
	if err := pcOSRemoteVerifyLaunchArtifacts(normalized); err != nil {
		t.Fatalf("valid pinned artifacts rejected: %v", err)
	}

	bad := spec
	bad.SignalURL = "http://example.invalid/functions/v1/zhirox-pc-remote-signaling-v1"
	if _, err := pcOSRemoteNormalizeLaunchSpec(bad); err == nil {
		t.Fatal("non-HTTPS signaling URL was accepted")
	}
	bad = spec
	bad.RelayOnly = true
	bad.ICEServers = []pcOSRemoteBridgeICEServer{{URLs: []string{"stun:stun.example.invalid:3478"}}}
	if _, err := pcOSRemoteNormalizeLaunchSpec(bad); err == nil {
		t.Fatal("relay-only launch without TURN was accepted")
	}
}

func TestRemoteBootstrapSecretOnlyInPipePayload(t *testing.T) {
	spec, err := pcOSRemoteNormalizeLaunchSpec(pcOSRemoteTestSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	secret := "ci-secret-must-not-enter-environment"
	payload, err := pcOSRemoteBuildBootstrapPayload(spec, pcOSRelayCredentials{
		AgentID: "Server-ci",
		Secret:  secret,
		Relay:   "https://relay.example.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pcOSRemoteZeroBytes(payload)
	var decoded pcOSRemoteBridgeBootstrap
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AgentSecret != secret || decoded.AgentID != "Server-ci" {
		t.Fatal("secure bootstrap payload lost agent credentials")
	}

	env, err := pcOSRemoteBuildMinimalEnvironment(spec)
	if err != nil {
		t.Fatal(err)
	}
	runes := utf16.Decode(env)
	envText := string(runes)
	if strings.Contains(envText, secret) || strings.Contains(envText, "ZHIROX_AGENT_SECRET") || strings.Contains(envText, "TURN_CREDENTIAL") {
		t.Fatal("secret-bearing data leaked into child environment")
	}
	for _, required := range []string{
		"ZHIROX_REMOTE_BOOTSTRAP_STDIN=1",
		"ZHIROX_REMOTE_CONSENT_PATH=",
		"ZHIROX_REMOTE_ENCODER_PATH=",
	} {
		if !strings.Contains(envText, required) {
			t.Fatalf("required non-secret environment entry missing: %s", required)
		}
	}
}

func TestRemoteArtifactPinChangeFailsClosed(t *testing.T) {
	spec, err := pcOSRemoteNormalizeLaunchSpec(pcOSRemoteTestSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.EncoderPath, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pcOSRemoteVerifyLaunchArtifacts(spec); err == nil {
		t.Fatal("tampered encoder artifact was accepted")
	}
}
