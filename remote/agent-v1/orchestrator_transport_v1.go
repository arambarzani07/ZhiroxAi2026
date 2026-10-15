package remoteagent

import "errors"

// NewTransportHardenedOrchestrator explicitly composes the existing attended
// screen-only orchestrator with the session-scoped signaling transport guard.
// The default constructor remains unchanged, so production behavior is not
// altered unless this candidate constructor is selected by a future rollout.
func NewTransportHardenedOrchestrator(cfg OrchestratorConfig, observer SignalTransportObserver) (*Orchestrator, error) {
	if cfg.Signaling == nil {
		return nil, errors.New("remote orchestrator signaling is required")
	}
	wrapped, err := NewTransportHardenedRemoteSignaling(cfg.Signaling, observer)
	if err != nil {
		return nil, err
	}
	cfg.Signaling = wrapped
	return NewOrchestrator(cfg)
}
