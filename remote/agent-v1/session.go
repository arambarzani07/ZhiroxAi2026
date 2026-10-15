package remoteagent

import (
	"errors"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	ProtocolVersion = "1"
	ScreenTrackID   = "screen"
	ScreenStreamID  = "zhirox-remote"
	ControlChannel  = "zhirox-control"
	H264FMTP        = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
)

type SignalSink func(kind string, payload any) error

type StatusSink func(state webrtc.PeerConnectionState)

type Config struct {
	ICEServers         []webrtc.ICEServer
	ICETransportPolicy webrtc.ICETransportPolicy
	Signal             SignalSink
	Status             StatusSink
	Control            *ControlDispatcher
}

type AgentSession struct {
	mu              sync.RWMutex
	pc              *webrtc.PeerConnection
	screen          *webrtc.TrackLocalStaticSample
	control         *webrtc.DataChannel
	controlDispatch *ControlDispatcher
	closed          bool
}

func NewAgentSession(cfg Config) (*AgentSession, error) {
	if cfg.Signal == nil {
		return nil, errors.New("signal sink is required")
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers:         cfg.ICEServers,
		ICETransportPolicy: cfg.ICETransportPolicy,
	})
	if err != nil {
		return nil, err
	}

	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: H264FMTP,
		},
		ScreenTrackID,
		ScreenStreamID,
	)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	go drainRTCP(sender)

	control, err := pc.CreateDataChannel(ControlChannel, nil)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	// Phase 2 transport boundary: the DataChannel may feed only the abstract,
	// approval-gated dispatcher. No Windows input implementation is provided by
	// this layer. Missing/inactive dispatchers therefore remain fail-closed.
	control.OnMessage(func(message webrtc.DataChannelMessage) {
		if cfg.Control == nil {
			return
		}
		if err := cfg.Control.Dispatch(message.Data); err != nil {
			_ = control.Close()
		}
	})

	s := &AgentSession{pc: pc, screen: track, control: control, controlDispatch: cfg.Control}
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		_ = cfg.Signal("ice_candidate", candidate.ToJSON())
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if cfg.Status != nil {
			cfg.Status(state)
		}
	})
	return s, nil
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func (s *AgentSession) AcceptOffer(sdp string) (webrtc.SessionDescription, error) {
	if s == nil || s.pc == nil {
		return webrtc.SessionDescription{}, errors.New("remote session is not initialized")
	}
	if sdp == "" {
		return webrtc.SessionDescription{}, errors.New("offer SDP is required")
	}
	if err := s.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}); err != nil {
		return webrtc.SessionDescription{}, err
	}
	answer, err := s.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}
	if err := s.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, err
	}
	local := s.pc.LocalDescription()
	if local == nil {
		return webrtc.SessionDescription{}, errors.New("local answer was not created")
	}
	return *local, nil
}

func (s *AgentSession) AddRemoteICE(candidate webrtc.ICECandidateInit) error {
	if s == nil || s.pc == nil {
		return errors.New("remote session is not initialized")
	}
	return s.pc.AddICECandidate(candidate)
}

// WriteH264AccessUnit accepts one encoded H.264 access unit produced by the
// Windows Media Foundation encoder. The encoder side must provide regular IDR
// frames and Annex-B compatible sample boundaries suitable for WebRTC packetization.
func (s *AgentSession) WriteH264AccessUnit(encoded []byte, duration time.Duration) error {
	if s == nil || s.screen == nil {
		return errors.New("screen track is not initialized")
	}
	if len(encoded) == 0 {
		return errors.New("encoded screen frame is empty")
	}
	if duration <= 0 || duration > time.Second {
		return errors.New("screen frame duration is outside approved bounds")
	}
	s.mu.RLock()
	closed := s.closed
	s.mu.RUnlock()
	if closed {
		return errors.New("remote session is closed")
	}
	return s.screen.WriteSample(media.Sample{Data: encoded, Duration: duration})
}

func (s *AgentSession) ConnectionState() webrtc.PeerConnectionState {
	if s == nil || s.pc == nil {
		return webrtc.PeerConnectionStateClosed
	}
	return s.pc.ConnectionState()
}

func (s *AgentSession) Close() error {
	if s == nil || s.pc == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	dispatcher := s.controlDispatch
	s.mu.Unlock()
	if dispatcher != nil {
		dispatcher.Deactivate()
	}
	return s.pc.Close()
}
