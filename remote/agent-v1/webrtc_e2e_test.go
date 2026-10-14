package remoteagent

import (
	"os"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestDirectICEScreenTrackDelivery(t *testing.T) {
	runScreenTrackICEGate(t, nil, webrtc.ICETransportPolicyAll)
}

func TestTURNRelayScreenTrackDelivery(t *testing.T) {
	url := os.Getenv("ZHIROX_TURN_URL")
	username := os.Getenv("ZHIROX_TURN_USERNAME")
	credential := os.Getenv("ZHIROX_TURN_CREDENTIAL")
	if url == "" || username == "" || credential == "" {
		t.Skip("credentialed TURN gate is not configured")
	}
	runScreenTrackICEGate(t, []webrtc.ICEServer{{
		URLs:       []string{url},
		Username:   username,
		Credential: credential,
	}}, webrtc.ICETransportPolicyRelay)
}

func runScreenTrackICEGate(t *testing.T, servers []webrtc.ICEServer, policy webrtc.ICETransportPolicy) {
	t.Helper()

	agentConnected := make(chan struct{}, 1)
	agent, err := NewAgentSession(Config{
		ICEServers: servers,
		ICETransportPolicy: policy,
		Signal: func(string, any) error { return nil },
		Status: func(state webrtc.PeerConnectionState) {
			if state == webrtc.PeerConnectionStateConnected {
				select { case agentConnected <- struct{}{}: default: }
			}
		},
	})
	if err != nil {
		t.Fatalf("create agent peer: %v", err)
	}
	defer agent.Close()

	controller, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: servers,
		ICETransportPolicy: policy,
	})
	if err != nil {
		t.Fatalf("create controller peer: %v", err)
	}
	defer controller.Close()

	if _, err := controller.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		t.Fatalf("add video transceiver: %v", err)
	}

	rtpReceived := make(chan struct{}, 1)
	controller.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		go func() {
			if _, _, err := track.ReadRTP(); err == nil {
				select { case rtpReceived <- struct{}{}: default: }
			}
		}()
	})

	offer, err := controller.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	controllerGathering := webrtc.GatheringCompletePromise(controller)
	if err := controller.SetLocalDescription(offer); err != nil {
		t.Fatalf("set controller local description: %v", err)
	}
	select {
	case <-controllerGathering:
	case <-time.After(10 * time.Second):
		t.Fatal("controller ICE gathering timed out")
	}
	localOffer := controller.LocalDescription()
	if localOffer == nil {
		t.Fatal("controller local offer missing")
	}

	if _, err := agent.AcceptOffer(localOffer.SDP); err != nil {
		t.Fatalf("agent accept offer: %v", err)
	}
	agentGathering := webrtc.GatheringCompletePromise(agent.pc)
	select {
	case <-agentGathering:
	case <-time.After(10 * time.Second):
		t.Fatal("agent ICE gathering timed out")
	}
	answer := agent.pc.LocalDescription()
	if answer == nil {
		t.Fatal("agent local answer missing")
	}
	if err := controller.SetRemoteDescription(*answer); err != nil {
		t.Fatalf("set controller remote answer: %v", err)
	}

	select {
	case <-agentConnected:
	case <-time.After(12 * time.Second):
		t.Fatalf("ICE peer connection did not reach connected state; policy=%s", policy.String())
	}

	// Minimal Annex-B access unit with SPS, PPS and IDR NAL units. The e2e gate
	// validates encrypted WebRTC packet delivery; decoding is a separate browser gate.
	sample := []byte{
		0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xe0, 0x1f, 0x95, 0xa8, 0x14, 0x01, 0x6e, 0x9b, 0x80,
		0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x06, 0xe2,
		0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x21, 0xa0,
	}
	if err := agent.WriteH264AccessUnit(sample, time.Second/15); err != nil {
		t.Fatalf("write H264 sample: %v", err)
	}

	select {
	case <-rtpReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("controller did not receive H264 RTP over established ICE path")
	}
}
