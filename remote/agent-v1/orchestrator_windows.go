//go:build windows

package remoteagent

import (
	"errors"
	"time"

	"github.com/pion/webrtc/v4"
)

type WindowsOrchestratorConfig struct {
	Signaling RemoteSignaling

	ConsentHelperPath   string
	ConsentHelperSHA256 string
	EncoderDLLPath      string
	EncoderDLLSHA256    string

	ICEServers         []webrtc.ICEServer
	ICETransportPolicy webrtc.ICETransportPolicy
	FPS                int
	Bitrate            uint32
	PendingInterval    time.Duration
	SignalInterval     time.Duration
	MaxSession         time.Duration
}

func NewWindowsOrchestrator(cfg WindowsOrchestratorConfig) (*Orchestrator, error) {
	if cfg.Signaling == nil {
		return nil, errors.New("Windows remote orchestrator requires signaling")
	}
	consent, err := NewConsentLauncher(cfg.ConsentHelperPath, cfg.ConsentHelperSHA256)
	if err != nil {
		return nil, err
	}
	if cfg.FPS == 0 {
		cfg.FPS = DefaultScreenFPS
	}
	if cfg.Bitrate == 0 {
		cfg.Bitrate = 4_000_000
	}

	return NewOrchestrator(OrchestratorConfig{
		Signaling: cfg.Signaling,
		Consent:   consent,
		NewSession: func(signal SignalSink, status StatusSink) (*AgentSession, error) {
			return NewAgentSession(Config{
				ICEServers:         cfg.ICEServers,
				ICETransportPolicy: cfg.ICETransportPolicy,
				Signal:             signal,
				Status:             status,
			})
		},
		NewPipeline: func(session *AgentSession) (ScreenPipeline, error) {
			// The service-side agent owns auth/signaling/WebRTC only. Desktop
			// capture and Media Foundation encoding run in a separately launched,
			// media-only process on the signed-in user's interactive desktop.
			return NewInteractiveWindowsScreenPipeline(
				session,
				cfg.EncoderDLLPath,
				cfg.EncoderDLLSHA256,
				cfg.FPS,
				cfg.Bitrate,
			)
		},
		PendingInterval: cfg.PendingInterval,
		SignalInterval:  cfg.SignalInterval,
		MaxSession:      cfg.MaxSession,
	})
}
