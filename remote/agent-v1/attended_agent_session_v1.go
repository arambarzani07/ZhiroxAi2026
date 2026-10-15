package remoteagent

import (
	"errors"
	"time"
)

// AttendedAgentSession binds a prepared attended-control runtime to the WebRTC
// AgentSession transport. It deliberately does not activate the dispatcher;
// Windows input execution remains unavailable until a separately reviewed
// activation/executor phase opts in while the same authorization stays valid.
type AttendedAgentSession struct {
	Session *AgentSession
	Runtime *AttendedControlRuntime
}

type AttendedAgentSessionConfig struct {
	Runtime *AttendedControlRuntime
	Session Config
	Now     time.Time
}

// NewAttendedAgentSession is the fail-closed integration boundary between the
// attended authorization runtime and the WebRTC session. The runtime owns both
// the dispatcher and per-message gate so callers cannot substitute a different
// control path after local approval.
func NewAttendedAgentSession(cfg AttendedAgentSessionConfig) (*AttendedAgentSession, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("attended control runtime is required")
	}
	if cfg.Runtime.dispatcher == nil {
		return nil, errors.New("attended runtime control dispatcher is required")
	}
	gate := cfg.Runtime.ControlGate()
	if gate == nil {
		return nil, errors.New("attended runtime control gate is required")
	}
	if err := gate.ValidateControlMessage(trustedArbiterNow(cfg.Now)); err != nil {
		return nil, err
	}
	if cfg.Session.Control != nil && cfg.Session.Control != cfg.Runtime.dispatcher {
		return nil, errors.New("session control dispatcher must match attended runtime dispatcher")
	}
	if cfg.Session.ControlGate != nil {
		return nil, errors.New("session control gate is owned by attended runtime")
	}

	cfg.Session.Control = cfg.Runtime.dispatcher
	cfg.Session.ControlGate = gate
	session, err := NewAgentSession(cfg.Session)
	if err != nil {
		_ = cfg.Runtime.Close()
		return nil, err
	}
	return &AttendedAgentSession{Session: session, Runtime: cfg.Runtime}, nil
}

// Close tears down both the WebRTC transport and attended runtime. Both paths
// deactivate the dispatcher, so teardown remains fail closed even if a future
// reviewed phase activates input later.
func (s *AttendedAgentSession) Close() error {
	if s == nil {
		return nil
	}
	var firstErr error
	if s.Session != nil {
		if err := s.Session.Close(); err != nil {
			firstErr = err
		}
	}
	if s.Runtime != nil {
		if err := s.Runtime.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
