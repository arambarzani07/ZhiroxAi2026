//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

type bootstrap struct {
	SignalURL   string `json:"signal_url"`
	AgentID     string `json:"agent_id"`
	AgentSecret string `json:"agent_secret"`
}

func fail(code int) {
	os.Exit(code)
}

func main() {
	if len(os.Args) != 1 {
		fail(10)
	}
	if os.Getenv("ZHIROX_REMOTE_BOOTSTRAP_STDIN") != "1" {
		fail(11)
	}
	for _, forbidden := range []string{
		"ZHIROX_AGENT_SECRET",
		"ZHIROX_REMOTE_ICE_SERVERS_JSON",
		"ZHIROX_REMOTE_TURN_USERNAME",
		"ZHIROX_REMOTE_TURN_CREDENTIAL",
	} {
		if strings.TrimSpace(os.Getenv(forbidden)) != "" {
			fail(12)
		}
	}
	for _, required := range []string{
		"ZHIROX_REMOTE_CONSENT_PATH",
		"ZHIROX_REMOTE_CONSENT_SHA256",
		"ZHIROX_REMOTE_ENCODER_PATH",
		"ZHIROX_REMOTE_ENCODER_SHA256",
		"ZHIROX_REMOTE_FPS",
		"ZHIROX_REMOTE_BITRATE",
	} {
		if strings.TrimSpace(os.Getenv(required)) == "" {
			fail(13)
		}
	}

	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 64*1024+1))
	if err != nil || len(raw) == 0 || len(raw) > 64*1024 {
		fail(14)
	}
	if bytes.Contains([]byte(strings.Join(os.Environ(), "\n")), []byte("probe-secret-over-pipe-only")) {
		fail(15)
	}
	var cfg bootstrap
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		// The real bootstrap contains additional bounded fields; the probe only
		// needs to verify the credential-bearing core. Decode generically below.
		var generic map[string]any
		if json.Unmarshal(raw, &generic) != nil {
			fail(16)
		}
		cfg.SignalURL, _ = generic["signal_url"].(string)
		cfg.AgentID, _ = generic["agent_id"].(string)
		cfg.AgentSecret, _ = generic["agent_secret"].(string)
	}
	if cfg.SignalURL != "https://example.supabase.co/functions/v1/zhirox-pc-remote-signaling-v1" {
		fail(17)
	}
	if cfg.AgentID != "Server-probe" || cfg.AgentSecret != "probe-secret-over-pipe-only" {
		fail(18)
	}
	os.Exit(0)
}
