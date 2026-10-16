package remoteagent

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

type mockE2EControlSink struct {
	mu      sync.Mutex
	batches [][]ControlEvent
	notify  chan struct{}
}

func newMockE2EControlSink() *mockE2EControlSink {
	return &mockE2EControlSink{notify: make(chan struct{}, 32)}
}

func (s *mockE2EControlSink) ApplyControlEvents(events []ControlEvent) error {
	s.mu.Lock()
	copyEvents := append([]ControlEvent(nil), events...)
	s.batches = append(s.batches, copyEvents)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
	return nil
}

func (s *mockE2EControlSink) snapshot() [][]ControlEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]ControlEvent, len(s.batches))
	for i := range s.batches {
		out[i] = append([]ControlEvent(nil), s.batches[i]...)
	}
	return out
}

type mockE2EAttendedGate struct {
	mu      sync.RWMutex
	allowed bool
}

func (g *mockE2EAttendedGate) ValidateControlMessage(time.Time) error {
	g.mu.RLock()
	allowed := g.allowed
	g.mu.RUnlock()
	if !allowed {
		return errors.New("mock attended approval is no longer valid")
	}
	return nil
}

func (g *mockE2EAttendedGate) setAllowed(allowed bool) {
	g.mu.Lock()
	g.allowed = allowed
	g.mu.Unlock()
}

func activateMockDispatcherForTest(dispatcher *ControlDispatcher) {
	dispatcher.mu.Lock()
	dispatcher.active = true
	dispatcher.lastSequence = 0
	dispatcher.mu.Unlock()
}

func waitMockSinkBatch(t *testing.T, sink *mockE2EControlSink, timeout time.Duration) {
	t.Helper()
	select {
	case <-sink.notify:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for metadata-only mock control sink")
	}
}

func waitDataChannelState(t *testing.T, channel *webrtc.DataChannel, state webrtc.DataChannelState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if channel.ReadyState() == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("data channel did not reach %s; current=%s", state.String(), channel.ReadyState().String())
}

func newMockWebRTCE2E(t *testing.T) (*AgentSession, *webrtc.PeerConnection, *webrtc.DataChannel, *ControlDispatcher, *mockE2EControlSink, *mockE2EAttendedGate) {
	t.Helper()

	sink := newMockE2EControlSink()
	dispatcher, err := NewControlDispatcher(sink)
	if err != nil {
		t.Fatal(err)
	}
	// Test-only activation of the abstract dispatcher. No platform/OS input sink
	// exists in this fixture; the sink only records validated protocol events.
	activateMockDispatcherForTest(dispatcher)
	gate := &mockE2EAttendedGate{allowed: true}

	agent, err := NewAgentSession(Config{
		Signal:      func(string, any) error { return nil },
		Control:     dispatcher,
		ControlGate: gate,
	})
	if err != nil {
		t.Fatal(err)
	}

	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		_ = agent.Close()
		t.Fatal(err)
	}
	if _, err := browser.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal(err)
	}

	channelCh := make(chan *webrtc.DataChannel, 1)
	browser.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() == ControlChannel {
			select {
			case channelCh <- channel:
			default:
			}
		}
	})

	offer, err := browser.CreateOffer(nil)
	if err != nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal(err)
	}
	browserGather := webrtc.GatheringCompletePromise(browser)
	if err := browser.SetLocalDescription(offer); err != nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal(err)
	}
	select {
	case <-browserGather:
	case <-time.After(5 * time.Second):
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal("browser ICE gathering timed out")
	}
	localOffer := browser.LocalDescription()
	if localOffer == nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal("browser local offer is missing")
	}

	if _, err := agent.AcceptOffer(localOffer.SDP); err != nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatalf("agent failed to accept browser offer: %v", err)
	}
	agentGather := webrtc.GatheringCompletePromise(agent.pc)
	select {
	case <-agentGather:
	case <-time.After(5 * time.Second):
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal("agent ICE gathering timed out")
	}
	localAnswer := agent.pc.LocalDescription()
	if localAnswer == nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal("agent local answer is missing")
	}
	if err := browser.SetRemoteDescription(*localAnswer); err != nil {
		_ = browser.Close()
		_ = agent.Close()
		t.Fatalf("browser failed to apply agent answer: %v", err)
	}

	var channel *webrtc.DataChannel
	select {
	case channel = <-channelCh:
	case <-time.After(8 * time.Second):
		_ = browser.Close()
		_ = agent.Close()
		t.Fatal("browser did not receive attended control data channel")
	}
	waitDataChannelState(t, channel, webrtc.DataChannelStateOpen, 5*time.Second)

	t.Cleanup(func() {
		_ = channel.Close()
		_ = browser.Close()
		_ = agent.Close()
	})
	return agent, browser, channel, dispatcher, sink, gate
}

func sendMockBrowserControl(t *testing.T, channel *webrtc.DataChannel, sequence uint64, kind string, fields string) {
	t.Helper()
	payload := fmt.Sprintf(`{"v":2,"type":"input","events":[{"sequence":%d,"kind":%q%s}]}`, sequence, kind, fields)
	if err := channel.SendText(payload); err != nil {
		t.Fatalf("mock browser failed to send attended input payload: %v", err)
	}
}

func TestAttendedWebRTCMockE2EBrowserProtocolReachesOnlyMockSink(t *testing.T) {
	_, _, channel, dispatcher, sink, _ := newMockWebRTCE2E(t)

	sendMockBrowserControl(t, channel, 1, "pointer_move", `,"x":0.25,"y":0.75`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	sendMockBrowserControl(t, channel, 2, "pointer_button", `,"button":"left","down":true`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	sendMockBrowserControl(t, channel, 3, "pointer_button", `,"button":"left","down":false`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	sendMockBrowserControl(t, channel, 4, "key", `,"key_code":65,"down":true`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	sendMockBrowserControl(t, channel, 5, "key", `,"key_code":65,"down":false`)
	waitMockSinkBatch(t, sink, 3*time.Second)

	batches := sink.snapshot()
	if len(batches) != 5 {
		t.Fatalf("expected five mock sink batches, got %d", len(batches))
	}
	if batches[0][0].Kind != "pointer_move" || batches[0][0].X != 0.25 || batches[0][0].Y != 0.75 {
		t.Fatalf("unexpected normalized pointer event: %#v", batches[0][0])
	}
	if batches[1][0].Button != "left" || !batches[1][0].Down || batches[2][0].Down {
		t.Fatal("pointer press/release semantics did not survive WebRTC protocol boundary")
	}
	if batches[3][0].KeyCode != 65 || !batches[3][0].Down || batches[4][0].Down {
		t.Fatal("keyboard press/release semantics did not survive WebRTC protocol boundary")
	}
	if dispatcher.LastSequence() != 5 {
		t.Fatalf("expected replay cursor 5, got %d", dispatcher.LastSequence())
	}
}

func TestAttendedWebRTCMockE2ELocalApprovalRevocationFailsClosed(t *testing.T) {
	_, _, channel, dispatcher, sink, gate := newMockWebRTCE2E(t)

	sendMockBrowserControl(t, channel, 1, "pointer_move", `,"x":0.5,"y":0.5`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	gate.setAllowed(false)

	if err := channel.SendText(`{"v":2,"type":"input","events":[{"sequence":2,"kind":"key","key_code":66,"down":true}]}`); err != nil {
		// A close racing this send is also a valid fail-closed outcome.
	}
	waitDataChannelState(t, channel, webrtc.DataChannelStateClosed, 5*time.Second)
	time.Sleep(100 * time.Millisecond)

	if got := len(sink.snapshot()); got != 1 {
		t.Fatalf("revoked local approval must block new mock sink batches, got %d", got)
	}
	if dispatcher.LastSequence() != 1 {
		t.Fatalf("revoked message must not advance replay cursor, got %d", dispatcher.LastSequence())
	}
}

func TestAttendedWebRTCMockE2EReplayClosesChannelWithoutSecondSinkWrite(t *testing.T) {
	_, _, channel, dispatcher, sink, _ := newMockWebRTCE2E(t)

	sendMockBrowserControl(t, channel, 1, "key", `,"key_code":67,"down":true`)
	waitMockSinkBatch(t, sink, 3*time.Second)
	if err := channel.SendText(`{"v":2,"type":"input","events":[{"sequence":1,"kind":"key","key_code":67,"down":false}]}`); err != nil {
		// A close racing this send is also valid.
	}
	waitDataChannelState(t, channel, webrtc.DataChannelStateClosed, 5*time.Second)
	time.Sleep(100 * time.Millisecond)

	if got := len(sink.snapshot()); got != 1 {
		t.Fatalf("replayed protocol message must not reach mock sink twice, got %d", got)
	}
	if dispatcher.LastSequence() != 1 {
		t.Fatalf("replay must not advance sequence beyond 1, got %d", dispatcher.LastSequence())
	}
}
