//go:build windows

package main

import (
	"context"
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

func rejectLegacySecretEnvironment() error {
	for _, name := range []string{
		"ZHIROX_AGENT_SECRET",
		"ZHIROX_REMOTE_ICE_SERVERS_JSON",
		"ZHIROX_REMOTE_TURN_USERNAME",
		"ZHIROX_REMOTE_TURN_CREDENTIAL",
	} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return errors.New("secret-bearing environment configuration is refused; secure bootstrap pipe is required")
		}
	}
	return nil
}

func run() error {
	if err := rejectLegacySecretEnvironment(); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ZHIROX_REMOTE_BOOTSTRAP_STDIN")), "1") &&
		!strings.EqualFold(strings.TrimSpace(os.Getenv("ZHIROX_REMOTE_BOOTSTRAP_STDIN")), "true") {
		return errors.New("secure bootstrap stdin mode is required")
	}
	bootstrap, err := readBootstrap(os.Stdin)
	if err != nil {
		return err
	}
	if bootstrap.normalizedMode() == "canary" {
		return runCanary(bootstrap)
	}

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
	iceServers := bootstrapWebRTCServers(bootstrap)

	policy := webrtc.ICETransportPolicyAll
	if bootstrap.RelayOnly {
		if len(iceServers) == 0 {
			return errors.New("relay-only mode requires configured TURN/ICE servers")
		}
		policy = webrtc.ICETransportPolicyRelay
	}

	signaling, err := remoteagent.NewSignalingClient(bootstrap.SignalURL, bootstrap.AgentID, bootstrap.AgentSecret, nil)
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
	var err error
	if captureHelperRequested(os.Args[1:]) {
		// Media-only helper mode is dispatched before any service-side bootstrap,
		// signaling credential or TURN secret is read.
		err = runCaptureHelper(os.Args[1:])
	} else {
		err = run()
	}
	if err != nil {
		// Do not print bootstrap payloads, environment values or credentials.
		// Error messages are deliberately configuration/stage-only.
		fmt.Fprintf(os.Stderr, "Zhirox %s stopped: %v\n", candidateVersion, err)
		os.Exit(1)
	}
}
