//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pion/webrtc/v4"
	remoteagent "zhirox/remoteagent"
)

const candidateVersion = "remote-screen-v1"

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return v, nil
}

func envUint32(name string, fallback uint32) (uint32, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", name)
	}
	return uint32(v), nil
}

func loadICEServers() ([]webrtc.ICEServer, error) {
	raw := strings.TrimSpace(os.Getenv("ZHIROX_REMOTE_ICE_SERVERS_JSON"))
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 32*1024 {
		return nil, errors.New("ZHIROX_REMOTE_ICE_SERVERS_JSON exceeds 32 KiB")
	}
	var servers []webrtc.ICEServer
	if err := json.Unmarshal([]byte(raw), &servers); err != nil {
		return nil, errors.New("ZHIROX_REMOTE_ICE_SERVERS_JSON is invalid JSON")
	}
	if len(servers) > 8 {
		return nil, errors.New("at most 8 ICE servers are allowed")
	}
	for _, server := range servers {
		if len(server.URLs) == 0 || len(server.URLs) > 8 {
			return nil, errors.New("each ICE server must contain 1 to 8 URLs")
		}
		for _, u := range server.URLs {
			lower := strings.ToLower(strings.TrimSpace(u))
			if !(strings.HasPrefix(lower, "stun:") || strings.HasPrefix(lower, "stuns:") || strings.HasPrefix(lower, "turn:") || strings.HasPrefix(lower, "turns:")) {
				return nil, errors.New("ICE URLs must use stun/stuns/turn/turns")
			}
		}
	}
	return servers, nil
}

func run() error {
	baseURL, err := requiredEnv("ZHIROX_REMOTE_SIGNAL_URL")
	if err != nil { return err }
	agentID, err := requiredEnv("ZHIROX_AGENT_ID")
	if err != nil { return err }
	agentSecret, err := requiredEnv("ZHIROX_AGENT_SECRET")
	if err != nil { return err }
	consentPath, err := requiredEnv("ZHIROX_REMOTE_CONSENT_PATH")
	if err != nil { return err }
	consentSHA, err := requiredEnv("ZHIROX_REMOTE_CONSENT_SHA256")
	if err != nil { return err }
	encoderPath, err := requiredEnv("ZHIROX_REMOTE_ENCODER_PATH")
	if err != nil { return err }
	encoderSHA, err := requiredEnv("ZHIROX_REMOTE_ENCODER_SHA256")
	if err != nil { return err }
	fps, err := envInt("ZHIROX_REMOTE_FPS", remoteagent.DefaultScreenFPS)
	if err != nil { return err }
	bitrate, err := envUint32("ZHIROX_REMOTE_BITRATE", 4_000_000)
	if err != nil { return err }
	iceServers, err := loadICEServers()
	if err != nil { return err }

	policy := webrtc.ICETransportPolicyAll
	if strings.EqualFold(strings.TrimSpace(os.Getenv("ZHIROX_REMOTE_RELAY_ONLY")), "1") || strings.EqualFold(strings.TrimSpace(os.Getenv("ZHIROX_REMOTE_RELAY_ONLY")), "true") {
		if len(iceServers) == 0 {
			return errors.New("relay-only mode requires configured TURN/ICE servers")
		}
		policy = webrtc.ICETransportPolicyRelay
	}

	signaling, err := remoteagent.NewSignalingClient(baseURL, agentID, agentSecret, nil)
	if err != nil { return err }
	orchestrator, err := remoteagent.NewWindowsOrchestrator(remoteagent.WindowsOrchestratorConfig{
		Signaling: signaling,
		ConsentHelperPath: consentPath,
		ConsentHelperSHA256: consentSHA,
		EncoderDLLPath: encoderPath,
		EncoderDLLSHA256: encoderSHA,
		ICEServers: iceServers,
		ICETransportPolicy: policy,
		FPS: fps,
		Bitrate: bitrate,
	})
	if err != nil { return err }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return orchestrator.Run(ctx)
}

func main() {
	if err := run(); err != nil {
		// Do not print environment values or credentials. Error messages are
		// deliberately configuration/stage-only.
		fmt.Fprintf(os.Stderr, "Zhirox %s stopped: %v\n", candidateVersion, err)
		os.Exit(1)
	}
}
