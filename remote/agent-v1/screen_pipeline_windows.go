//go:build windows

package remoteagent

import (
	"context"
	"errors"
	"fmt"
)

type WindowsScreenPipeline struct {
	capture *PrimaryDisplayBGRACapture
	encoder *MFH264Encoder
	session *AgentSession
	config  ScreenStreamConfig
}

func NewWindowsScreenPipeline(session *AgentSession, dllPath, expectedSHA256 string, fps int, bitrate uint32) (*WindowsScreenPipeline, error) {
	if session == nil {
		return nil, errors.New("agent session is required")
	}
	cfg := ScreenStreamConfig{FPS: fps}
	cfg, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	if bitrate < 250_000 || bitrate > 20_000_000 {
		return nil, errors.New("screen bitrate must be between 250 Kbps and 20 Mbps")
	}

	capture, err := NewPrimaryDisplayBGRACapture()
	if err != nil {
		return nil, fmt.Errorf("create primary display capture: %w", err)
	}
	width, height := capture.Dimensions()
	encoder, err := NewMFH264Encoder(dllPath, expectedSHA256, width, height, uint32(cfg.FPS), bitrate)
	if err != nil {
		capture.Close()
		return nil, fmt.Errorf("create Media Foundation encoder: %w", err)
	}
	return &WindowsScreenPipeline{
		capture: capture,
		encoder: encoder,
		session: session,
		config: cfg,
	}, nil
}

func (p *WindowsScreenPipeline) Run(ctx context.Context) error {
	if p == nil || p.capture == nil || p.encoder == nil || p.session == nil {
		return errors.New("Windows screen pipeline is not initialized")
	}
	return RunScreenStream(ctx, p.capture, p.encoder, p.session, p.config)
}

func (p *WindowsScreenPipeline) Close() error {
	if p == nil {
		return nil
	}
	var first error
	if p.encoder != nil {
		if err := p.encoder.Close(); err != nil && first == nil {
			first = err
		}
		p.encoder = nil
	}
	if p.capture != nil {
		if err := p.capture.Close(); err != nil && first == nil {
			first = err
		}
		p.capture = nil
	}
	return first
}
