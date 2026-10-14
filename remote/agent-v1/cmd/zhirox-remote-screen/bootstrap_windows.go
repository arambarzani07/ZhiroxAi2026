//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pion/webrtc/v4"
)

const maxBootstrapBytes = 64 * 1024

type bootstrapICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type bootstrapConfig struct {
	Mode        string               `json:"mode,omitempty"`
	SignalURL   string               `json:"signal_url"`
	AgentID     string               `json:"agent_id,omitempty"`
	AgentSecret string               `json:"agent_secret,omitempty"`
	AgentLabel  string               `json:"agent_label,omitempty"`
	ICEServers  []bootstrapICEServer `json:"ice_servers,omitempty"`
	RelayOnly   bool                 `json:"relay_only,omitempty"`
}

func (c bootstrapConfig) normalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return "production"
	}
	return mode
}

func readBootstrap(r io.Reader) (bootstrapConfig, error) {
	if r == nil {
		return bootstrapConfig{}, errors.New("secure bootstrap pipe is required")
	}
	limited := io.LimitReader(r, maxBootstrapBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return bootstrapConfig{}, errors.New("secure bootstrap pipe could not be read")
	}
	if len(raw) == 0 {
		return bootstrapConfig{}, errors.New("secure bootstrap pipe was empty")
	}
	if len(raw) > maxBootstrapBytes {
		return bootstrapConfig{}, errors.New("secure bootstrap payload exceeds 64 KiB")
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg bootstrapConfig
	if err := dec.Decode(&cfg); err != nil {
		return bootstrapConfig{}, errors.New("secure bootstrap payload is invalid")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return bootstrapConfig{}, errors.New("secure bootstrap payload contains trailing data")
	}
	if err := validateBootstrap(cfg); err != nil {
		return bootstrapConfig{}, err
	}
	cfg.Mode = cfg.normalizedMode()
	return cfg, nil
}

func validateBootstrap(cfg bootstrapConfig) error {
	if strings.TrimSpace(cfg.SignalURL) == "" {
		return errors.New("bootstrap signaling URL is required")
	}
	switch cfg.normalizedMode() {
	case "production":
		if strings.TrimSpace(cfg.AgentID) == "" || len(cfg.AgentID) > 160 {
			return errors.New("bootstrap agent id is invalid")
		}
		if strings.TrimSpace(cfg.AgentSecret) == "" || len(cfg.AgentSecret) > 4096 {
			return errors.New("bootstrap agent secret is invalid")
		}
	case "canary":
		// Canary pairing is deliberately isolated from production identity.
		// Supplying production-style credentials in canary mode fails closed.
		if strings.TrimSpace(cfg.AgentID) != "" || strings.TrimSpace(cfg.AgentSecret) != "" {
			return errors.New("canary bootstrap refuses production agent credentials")
		}
		label := strings.TrimSpace(cfg.AgentLabel)
		if label == "" || len(label) > 120 {
			return errors.New("canary bootstrap agent label is invalid")
		}
	default:
		return errors.New("bootstrap mode is unsupported")
	}
	if len(cfg.ICEServers) > 8 {
		return errors.New("bootstrap allows at most 8 ICE servers")
	}
	for _, server := range cfg.ICEServers {
		if len(server.URLs) == 0 || len(server.URLs) > 8 {
			return errors.New("each ICE server must contain 1 to 8 URLs")
		}
		if len(server.Username) > 1024 || len(server.Credential) > 4096 {
			return errors.New("ICE credential fields exceed safety bounds")
		}
		for _, rawURL := range server.URLs {
			u := strings.ToLower(strings.TrimSpace(rawURL))
			if !(strings.HasPrefix(u, "stun:") || strings.HasPrefix(u, "stuns:") || strings.HasPrefix(u, "turn:") || strings.HasPrefix(u, "turns:")) {
				return fmt.Errorf("ICE URL scheme is not allowed")
			}
		}
	}
	if cfg.RelayOnly && len(cfg.ICEServers) == 0 {
		return errors.New("relay-only mode requires TURN/ICE servers")
	}
	return nil
}

func bootstrapWebRTCServers(cfg bootstrapConfig) []webrtc.ICEServer {
	out := make([]webrtc.ICEServer, 0, len(cfg.ICEServers))
	for _, server := range cfg.ICEServers {
		out = append(out, webrtc.ICEServer{
			URLs:       append([]string(nil), server.URLs...),
			Username:   server.Username,
			Credential: server.Credential,
		})
	}
	return out
}
