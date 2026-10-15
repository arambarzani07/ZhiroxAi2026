package remoteagent

import (
	"context"
	"errors"
	"time"
)

type AttendedSessionOrchestratorConfig struct {
	Context         context.Context
	Arbiter         *TrustedSessionArbiter
	Permit          TrustedControlPermit
	Consent         RemoteInputConsent
	ControllerLabel string
	ApprovalID      string
	Dispatcher      *ControlDispatcher
	Session         Config
	Now             time.Time
}

// PrepareAttendedAgentSession composes the full attended, non-executing control
// path: trusted permit -> explicit local approval -> visible indicator/runtime ->
// WebRTC AgentSession binding. The abstract dispatcher remains deactivated.
// This function does not enable platform input and does not call Activate().
func PrepareAttendedAgentSession(cfg AttendedSessionOrchestratorConfig) (*AttendedAgentSession, error) {
	if cfg.Context == nil {
		return nil, errors.New("attended session context is required")
	}
	if cfg.Dispatcher == nil {
		return nil, errors.New("control dispatcher is required")
	}
	now := trustedArbiterNow(cfg.Now)

	approval, err := RequestAttendedInputAuthorization(
		cfg.Arbiter,
		cfg.Permit,
		cfg.Consent,
		cfg.ControllerLabel,
		cfg.ApprovalID,
		now,
	)
	if err != nil {
		cfg.Dispatcher.Deactivate()
		return nil, err
	}

	runtime, err := PrepareAttendedControlRuntime(cfg.Context, AttendedControlRuntimeConfig{
		Arbiter:         cfg.Arbiter,
		Permit:          cfg.Permit,
		ApprovalResult:  approval,
		Consent:         cfg.Consent,
		ControllerLabel: cfg.ControllerLabel,
		Dispatcher:      cfg.Dispatcher,
		Now:             now,
	})
	if err != nil {
		cfg.Dispatcher.Deactivate()
		return nil, err
	}

	bound, err := NewAttendedAgentSession(AttendedAgentSessionConfig{
		Runtime: runtime,
		Session: cfg.Session,
		Now:     now,
	})
	if err != nil {
		// NewAttendedAgentSession closes the prepared runtime on construction
		// failure. Deactivate again here to keep this top-level boundary explicit.
		cfg.Dispatcher.Deactivate()
		return nil, err
	}
	return bound, nil
}
