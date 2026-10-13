package remoteagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

const (
	DefaultPendingPollInterval = 2 * time.Second
	DefaultSignalPollInterval  = 250 * time.Millisecond
	MaxRemoteSessionDuration   = 30 * time.Minute
)

type RemoteSignaling interface {
	Pending(ctx context.Context) ([]PendingSession, error)
	Approve(ctx context.Context, sessionID string) error
	Reject(ctx context.Context, sessionID string) error
	MarkActive(ctx context.Context, sessionID string) error
	PostSignal(ctx context.Context, sessionID, kind string, payload any) error
	PollSignals(ctx context.Context, sessionID string, after int64) ([]Signal, error)
}

type RemoteConsent interface {
	Approve(controllerLabel, controllerFingerprint string) (bool, error)
	StartIndicator(controllerLabel string) (<-chan error, error)
}

type ScreenPipeline interface {
	Run(ctx context.Context) error
	Close() error
}

type SessionFactory func(signal SignalSink, status StatusSink) (*AgentSession, error)
type PipelineFactory func(session *AgentSession) (ScreenPipeline, error)

type OrchestratorConfig struct {
	Signaling       RemoteSignaling
	Consent         RemoteConsent
	NewSession      SessionFactory
	NewPipeline     PipelineFactory
	PendingInterval time.Duration
	SignalInterval  time.Duration
	MaxSession      time.Duration
}

type Orchestrator struct {
	cfg OrchestratorConfig
}

func NewOrchestrator(cfg OrchestratorConfig) (*Orchestrator, error) {
	if cfg.Signaling == nil || cfg.Consent == nil || cfg.NewSession == nil || cfg.NewPipeline == nil {
		return nil, errors.New("remote orchestrator requires signaling, consent, session and pipeline dependencies")
	}
	if cfg.PendingInterval == 0 {
		cfg.PendingInterval = DefaultPendingPollInterval
	}
	if cfg.SignalInterval == 0 {
		cfg.SignalInterval = DefaultSignalPollInterval
	}
	if cfg.MaxSession == 0 {
		cfg.MaxSession = MaxRemoteSessionDuration
	}
	if cfg.PendingInterval < 250*time.Millisecond || cfg.PendingInterval > 30*time.Second {
		return nil, errors.New("pending poll interval outside approved bounds")
	}
	if cfg.SignalInterval < 100*time.Millisecond || cfg.SignalInterval > 5*time.Second {
		return nil, errors.New("signal poll interval outside approved bounds")
	}
	if cfg.MaxSession <= 0 || cfg.MaxSession > MaxRemoteSessionDuration {
		return nil, errors.New("maximum remote session duration cannot exceed 30 minutes")
	}
	return &Orchestrator{cfg: cfg}, nil
}

func screenOnlyCapabilities(caps map[string]any) bool {
	if len(caps) == 0 {
		return false
	}
	screen, ok := caps["screen_stream"].(bool)
	if !ok || !screen {
		return false
	}
	for _, key := range []string{"mouse", "keyboard", "clipboard", "file_transfer", "audio"} {
		if v, exists := caps[key]; exists {
			enabled, ok := v.(bool)
			if !ok || enabled {
				return false
			}
		}
	}
	return true
}

// Run polls for attended screen-only requests and services at most one session
// at a time. Every request requires a fresh local approval. Unsupported
// capabilities fail closed and are rejected before any WebRTC object exists.
func (o *Orchestrator) Run(ctx context.Context) error {
	if o == nil {
		return errors.New("remote orchestrator is nil")
	}
	ticker := time.NewTicker(o.cfg.PendingInterval)
	defer ticker.Stop()

	check := func() error {
		pending, err := o.cfg.Signaling.Pending(ctx)
		if err != nil {
			return err
		}
		for _, request := range pending {
			if err := pcOSRemoteValidateSessionID(request.ID); err != nil {
				continue
			}
			if !screenOnlyCapabilities(request.RequestedCapabilities) {
				_ = o.cfg.Signaling.Reject(ctx, request.ID)
				continue
			}
			return o.handleRequest(ctx, request)
		}
		return nil
	}

	if err := check(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := check(); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (o *Orchestrator) handleRequest(parent context.Context, request PendingSession) error {
	approved, err := o.cfg.Consent.Approve(request.ControllerLabel, request.ControllerFingerprint)
	if err != nil {
		_ = o.cfg.Signaling.Reject(parent, request.ID)
		return fmt.Errorf("local remote-session consent failed: %w", err)
	}
	if !approved {
		return o.cfg.Signaling.Reject(parent, request.ID)
	}
	if err := o.cfg.Signaling.Approve(parent, request.ID); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, o.cfg.MaxSession)
	defer cancel()

	connected := make(chan struct{}, 1)
	failed := make(chan error, 1)
	postSignal := func(kind string, payload any) error {
		return o.cfg.Signaling.PostSignal(ctx, request.ID, kind, payload)
	}
	status := func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected:
			select {
			case connected <- struct{}{}:
			default:
			}
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			select {
			case failed <- fmt.Errorf("WebRTC peer entered %s state", state.String()):
			default:
			}
		}
	}

	session, err := o.cfg.NewSession(postSignal, status)
	if err != nil {
		_ = o.cfg.Signaling.PostSignal(parent, request.ID, "hangup", map[string]any{"reason": "agent_session_create_failed"})
		return err
	}
	defer session.Close()

	var (
		cursor       int64
		offerApplied bool
		queuedICE    []webrtc.ICECandidateInit
		activeOnce   sync.Once
		pipeline     ScreenPipeline
		pipelineDone chan error
		indicatorDone <-chan error
	)

	startActive := func() error {
		var startErr error
		activeOnce.Do(func() {
			if err := o.cfg.Signaling.MarkActive(ctx, request.ID); err != nil {
				startErr = err
				return
			}
			indicatorDone, startErr = o.cfg.Consent.StartIndicator(request.ControllerLabel)
			if startErr != nil {
				return
			}
			pipeline, startErr = o.cfg.NewPipeline(session)
			if startErr != nil {
				return
			}
			pipelineDone = make(chan error, 1)
			go func() {
				pipelineDone <- pipeline.Run(ctx)
				close(pipelineDone)
			}()
		})
		return startErr
	}

	cleanup := func(reason string) {
		cancel()
		if pipeline != nil {
			_ = pipeline.Close()
		}
		_ = o.cfg.Signaling.PostSignal(parent, request.ID, "hangup", map[string]any{"reason": reason})
	}
	defer func() {
		if pipeline != nil {
			_ = pipeline.Close()
		}
	}()

	ticker := time.NewTicker(o.cfg.SignalInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			reason := "session_timeout"
			if parent.Err() != nil {
				reason = "agent_shutdown"
			}
			cleanup(reason)
			return nil
		case <-connected:
			if !offerApplied {
				cleanup("connected_without_offer")
				return errors.New("WebRTC connected before an approved offer was applied")
			}
			if err := startActive(); err != nil {
				cleanup("active_start_failed")
				return err
			}
		case err := <-failed:
			cleanup("webrtc_failed")
			return err
		case err, ok := <-indicatorDone:
			if ok {
				if err != nil {
					cleanup("indicator_failed")
					return err
				}
				cleanup("local_disconnect")
				return nil
			}
		case err, ok := <-pipelineDone:
			if ok && err != nil && !errors.Is(err, context.Canceled) {
				cleanup("screen_pipeline_failed")
				return err
			}
		case <-ticker.C:
			signals, err := o.cfg.Signaling.PollSignals(ctx, request.ID, cursor)
			if err != nil {
				cleanup("signal_poll_failed")
				return err
			}
			for _, sig := range signals {
				if sig.ID > cursor {
					cursor = sig.ID
				}
				if !strings.EqualFold(sig.Sender, "controller") {
					continue
				}
				switch strings.ToLower(sig.Kind) {
				case "offer":
					if offerApplied {
						cleanup("duplicate_offer")
						return errors.New("duplicate WebRTC offer refused")
					}
					var offer struct {
						Type string `json:"type"`
						SDP  string `json:"sdp"`
					}
					if err := json.Unmarshal(sig.Payload, &offer); err != nil || !strings.EqualFold(offer.Type, "offer") || strings.TrimSpace(offer.SDP) == "" {
						cleanup("invalid_offer")
						return errors.New("invalid controller WebRTC offer")
					}
					answer, err := session.AcceptOffer(offer.SDP)
					if err != nil {
						cleanup("offer_refused")
						return err
					}
					offerApplied = true
					for _, candidate := range queuedICE {
						if err := session.AddRemoteICE(candidate); err != nil {
							cleanup("queued_ice_refused")
							return err
						}
					}
					queuedICE = nil
					if err := o.cfg.Signaling.PostSignal(ctx, request.ID, "answer", map[string]any{"type": "answer", "sdp": answer.SDP}); err != nil {
						cleanup("answer_post_failed")
						return err
					}
				case "ice_candidate":
					var candidate webrtc.ICECandidateInit
					if err := json.Unmarshal(sig.Payload, &candidate); err != nil || strings.TrimSpace(candidate.Candidate) == "" {
						cleanup("invalid_ice")
						return errors.New("invalid controller ICE candidate")
					}
					if !offerApplied {
						if len(queuedICE) >= 256 {
							cleanup("ice_queue_limit")
							return errors.New("pre-offer ICE candidate queue limit exceeded")
						}
						queuedICE = append(queuedICE, candidate)
						continue
					}
					if err := session.AddRemoteICE(candidate); err != nil {
						cleanup("ice_refused")
						return err
					}
				case "hangup":
					cancel()
					if pipeline != nil {
						_ = pipeline.Close()
					}
					return nil
				case "answer":
					cleanup("unexpected_answer")
					return errors.New("unexpected controller answer refused; controller must be the WebRTC offerer")
				default:
					cleanup("unsupported_signal")
					return fmt.Errorf("unsupported controller signal kind %q", sig.Kind)
				}
			}
		}
	}
}
