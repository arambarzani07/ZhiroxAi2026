package remoteagent

import "errors"

// NewTransportHardenedOrchestrator explicitly composes the existing attended
// screen-only orchestrator with the session-scoped signaling transport guard.
// The default constructor remains unchanged, so production behavior is not
// altered unless this candidate constructor is selected by a future rollout.
func NewTransportHardenedOrchestrator(cfg OrchestratorConfig, observer SignalTransportObserver) (*Orchestrator, error) {
	return NewTransportLifecycleHardenedOrchestrator(cfg, observer, nil)
}

// NewTransportLifecycleHardenedOrchestrator adds metadata-only lifecycle
// observability for heartbeat, transient connection degradation, recovery,
// expiry, and terminal teardown while preserving the screen-only data path.
func NewTransportLifecycleHardenedOrchestrator(cfg OrchestratorConfig, observer SignalTransportObserver, lifecycleObserver SignalSessionLifecycleObserver) (*Orchestrator, error) {
	if cfg.Signaling == nil {
		return nil, errors.New("remote orchestrator signaling is required")
	}
	wrapped, err := NewTransportHardenedRemoteSignalingWithLifecycle(cfg.Signaling, observer, lifecycleObserver)
	if err != nil {
		return nil, err
	}
	cfg.Signaling = wrapped
	return NewOrchestrator(cfg)
}
