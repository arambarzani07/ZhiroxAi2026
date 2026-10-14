//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
	remoteagent "zhirox/remoteagent"
)

type prefetchedSignaling struct {
	remoteagent.RemoteSignaling
	mu    sync.Mutex
	first []remoteagent.PendingSession
}

func (p *prefetchedSignaling) Pending(ctx context.Context) ([]remoteagent.PendingSession, error) {
	p.mu.Lock()
	if len(p.first) > 0 {
		out := append([]remoteagent.PendingSession(nil), p.first...)
		p.first = nil
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()
	return p.RemoteSignaling.Pending(ctx)
}

func waitCanaryClaim(ctx context.Context, signaling *remoteagent.CanarySignalingClient, pairing remoteagent.SessionIndicator, expiresAt string) ([]remoteagent.PendingSession, error) {
	serverExpiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, errors.New("canary pairing expiry is invalid")
	}
	deadline := time.Now().Add(remoteagent.CanaryPairingTTL)
	if serverExpiry.Before(deadline) {
		deadline = serverExpiry
	}
	if !deadline.After(time.Now()) {
		return nil, errors.New("canary pairing already expired")
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	check := func() ([]remoteagent.PendingSession, error) {
		pending, err := signaling.Pending(ctx)
		if err != nil {
			return nil, err
		}
		if len(pending) > 1 {
			return nil, errors.New("canary pairing returned multiple controller claims")
		}
		return pending, nil
	}

	for {
		pending, err := check()
		if err != nil {
			return nil, err
		}
		if len(pending) == 1 {
			if err := pairing.Stop(); err != nil {
				return nil, errors.New("could not close pairing display after controller claim")
			}
			select {
			case helperErr, ok := <-pairing.Done():
				if ok && helperErr != nil {
					return nil, errors.New("pairing display failed while closing")
				}
			case <-time.After(5 * time.Second):
				return nil, errors.New("pairing display did not close after controller claim")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return pending, nil
		}

		select {
		case helperErr, ok := <-pairing.Done():
			if ok && helperErr != nil {
				return nil, errors.New("pairing display failed")
			}
			return nil, errors.New("pairing was cancelled locally")
		case <-ticker.C:
			continue
		case <-timer.C:
			_ = pairing.Stop()
			return nil, errors.New("canary pairing expired before controller claim")
		case <-ctx.Done():
			_ = pairing.Stop()
			return nil, ctx.Err()
		}
	}
}

func runCanary(bootstrap bootstrapConfig) error {
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
			return errors.New("relay-only canary mode requires configured TURN/ICE servers")
		}
		policy = webrtc.ICETransportPolicyRelay
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pairingCode, err := remoteagent.GenerateCanaryPairingCode()
	if err != nil {
		return errors.New("generate canary pairing code")
	}
	signaling, registration, err := remoteagent.RegisterCanarySignaling(ctx, bootstrap.SignalURL, pairingCode, bootstrap.AgentLabel, nil)
	if err != nil {
		return err
	}
	consent, err := remoteagent.NewConsentLauncher(consentPath, consentSHA)
	if err != nil {
		return err
	}
	pairingDisplay, err := consent.StartPairing(pairingCode)
	pairingCode = ""
	if err != nil {
		return err
	}

	pending, err := waitCanaryClaim(ctx, signaling, pairingDisplay, registration.ExpiresAt)
	if err != nil {
		_ = pairingDisplay.Stop()
		return err
	}
	wrapped := &prefetchedSignaling{RemoteSignaling: signaling, first: pending}
	orchestrator, err := remoteagent.NewWindowsOrchestrator(remoteagent.WindowsOrchestratorConfig{
		Signaling: wrapped,
		ConsentHelperPath: consentPath,
		ConsentHelperSHA256: consentSHA,
		EncoderDLLPath: encoderPath,
		EncoderDLLSHA256: encoderSHA,
		ICEServers: iceServers,
		ICETransportPolicy: policy,
		FPS: fps,
		Bitrate: bitrate,
	})
	if err != nil {
		return err
	}
	return orchestrator.Run(ctx)
}
