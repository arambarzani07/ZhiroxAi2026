//go:build windows

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadBootstrapAcceptsBoundedScreenOnlyCredentials(t *testing.T) {
	raw := `{
		"signal_url":"https://example.supabase.co/functions/v1/zhirox-pc-remote-signaling-v1",
		"agent_id":"Server-c24b5c0f",
		"agent_secret":"test-secret-never-log",
		"ice_servers":[{"urls":["turns:turn.example.test:5349"],"username":"ephemeral-user","credential":"ephemeral-credential"}],
		"relay_only":true
	}`
	cfg, err := readBootstrap(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("bootstrap rejected: %v", err)
	}
	if cfg.normalizedMode() != "production" || cfg.AgentID != "Server-c24b5c0f" || cfg.AgentSecret == "" || !cfg.RelayOnly {
		t.Fatalf("unexpected bootstrap config")
	}
	servers := bootstrapWebRTCServers(cfg)
	if len(servers) != 1 || len(servers[0].URLs) != 1 || servers[0].Username == "" {
		t.Fatalf("TURN configuration was not mapped")
	}
}

func TestReadBootstrapAcceptsCredentialFreeCanary(t *testing.T) {
	raw := `{
		"mode":"canary",
		"signal_url":"https://canary.example.test",
		"agent_label":"Server canary"
	}`
	cfg, err := readBootstrap(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("canary bootstrap rejected: %v", err)
	}
	if cfg.Mode != "canary" || cfg.AgentID != "" || cfg.AgentSecret != "" || cfg.AgentLabel != "Server canary" {
		t.Fatalf("unexpected canary bootstrap: %#v", cfg)
	}
}

func TestCanaryBootstrapRejectsProductionCredentials(t *testing.T) {
	for _, cfg := range []bootstrapConfig{
		{Mode: "canary", SignalURL: "https://canary.example.test", AgentLabel: "Server", AgentID: "agent"},
		{Mode: "canary", SignalURL: "https://canary.example.test", AgentLabel: "Server", AgentSecret: "secret"},
	} {
		if err := validateBootstrap(cfg); err == nil {
			t.Fatalf("canary bootstrap accepted production credential fields: %#v", cfg)
		}
	}
}

func TestReadBootstrapRejectsUnknownFields(t *testing.T) {
	raw := `{"signal_url":"https://example.test","agent_id":"agent","agent_secret":"secret","unexpected":true}`
	if _, err := readBootstrap(strings.NewReader(raw)); err == nil {
		t.Fatal("unknown bootstrap field must be rejected")
	}
}

func TestReadBootstrapRejectsOversizedPayload(t *testing.T) {
	raw := bytes.Repeat([]byte{'x'}, maxBootstrapBytes+1)
	if _, err := readBootstrap(bytes.NewReader(raw)); err == nil {
		t.Fatal("oversized bootstrap payload must be rejected")
	}
}

func TestBootstrapRejectsUnsupportedICEScheme(t *testing.T) {
	cfg := bootstrapConfig{
		SignalURL: "https://example.test",
		AgentID: "agent",
		AgentSecret: "secret",
		ICEServers: []bootstrapICEServer{{URLs: []string{"https://turn.example.test"}}},
	}
	if err := validateBootstrap(cfg); err == nil {
		t.Fatal("non STUN/TURN ICE URL must be rejected")
	}
}

func TestBootstrapRelayOnlyRequiresServer(t *testing.T) {
	cfg := bootstrapConfig{
		SignalURL: "https://example.test",
		AgentID: "agent",
		AgentSecret: "secret",
		RelayOnly: true,
	}
	if err := validateBootstrap(cfg); err == nil {
		t.Fatal("relay-only bootstrap without TURN/ICE server must be rejected")
	}
}

func TestBootstrapRejectsUnknownMode(t *testing.T) {
	cfg := bootstrapConfig{Mode: "hidden", SignalURL: "https://example.test", AgentID: "agent", AgentSecret: "secret"}
	if err := validateBootstrap(cfg); err == nil {
		t.Fatal("unsupported bootstrap mode must be rejected")
	}
}
