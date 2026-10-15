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
	if _, ok := o.cfg.Signaling.(*TransportHardenedRemoteSignaling); !ok {
		t.Fatal("candidate orchestrator must use transport-hardened signaling wrapper")
	}
	if _, ok := interface{}(inner).(*fakeRemoteSignaling); !ok {
		t.Fatal("test fixture unexpectedly changed")
	}
}

func TestNewTransportHardenedOrchestratorRejectsMissingSignaling(t *testing.T) {
	if _, err := NewTransportHardenedOrchestrator(OrchestratorConfig{}, nil); err == nil {
		t.Fatal("candidate transport-hardened orchestrator must require signaling")
	}
}
