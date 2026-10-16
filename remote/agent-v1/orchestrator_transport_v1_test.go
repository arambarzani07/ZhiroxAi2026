package remoteagent

import "testing"

func TestNewTransportHardenedOrchestratorWrapsSignalingWithoutChangingDefaultPath(t *testing.T) {
	inner := &fakeRemoteSignaling{}
	observer := &recordingSignalTransportObserver{}
	o, err := NewTransportHardenedOrchestrator(OrchestratorConfig{
		Signaling: inner,
		Consent:   &fakeRemoteConsent{approved: true},
		NewSession: func(SignalSink, StatusSink) (*AgentSession, error) {
			return &AgentSession{}, nil
		},
		NewPipeline: func(*AgentSession) (ScreenPipeline, error) {
			return nil, nil
		},
	}, observer)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ok := o.cfg.Signaling.(*TransportHardenedRemoteSignaling)
	if !ok {
		t.Fatal("candidate orchestrator must use transport-hardened signaling wrapper")
	}
	if wrapped.lifecycleObserver != nil {
		t.Fatal("legacy transport-hardened constructor must not invent a lifecycle observer")
	}
	if _, ok := interface{}(inner).(*fakeRemoteSignaling); !ok {
		t.Fatal("test fixture unexpectedly changed")
	}
}

func TestNewTransportLifecycleHardenedOrchestratorBindsLifecycleObserver(t *testing.T) {
	inner := &fakeRemoteSignaling{}
	transportObserver := &recordingSignalTransportObserver{}
	lifecycleObserver := &recordingSignalLifecycleObserver{}
	o, err := NewTransportLifecycleHardenedOrchestrator(OrchestratorConfig{
		Signaling: inner,
		Consent:   &fakeRemoteConsent{approved: true},
		NewSession: func(SignalSink, StatusSink) (*AgentSession, error) {
			return &AgentSession{}, nil
		},
		NewPipeline: func(*AgentSession) (ScreenPipeline, error) {
			return nil, nil
		},
	}, transportObserver, lifecycleObserver)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, ok := o.cfg.Signaling.(*TransportHardenedRemoteSignaling)
	if !ok {
		t.Fatal("lifecycle candidate orchestrator must use transport-hardened signaling wrapper")
	}
	if wrapped.lifecycleObserver != lifecycleObserver {
		t.Fatal("lifecycle observer must be bound to the hardened signaling wrapper")
	}
}

func TestNewTransportHardenedOrchestratorRejectsMissingSignaling(t *testing.T) {
	if _, err := NewTransportHardenedOrchestrator(OrchestratorConfig{}, nil); err == nil {
		t.Fatal("candidate transport-hardened orchestrator must require signaling")
	}
	if _, err := NewTransportLifecycleHardenedOrchestrator(OrchestratorConfig{}, nil, nil); err == nil {
		t.Fatal("lifecycle transport-hardened orchestrator must require signaling")
	}
}
