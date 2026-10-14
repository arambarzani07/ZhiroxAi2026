//go:build windows

package remoteagent

import (
	"context"
	"errors"
	"fmt"
	"io"
)

type WindowsCaptureHelperConfig struct {
	EncoderDLLPath   string
	EncoderDLLSHA256 string
	FPS              int
	Bitrate          uint32
}

func (c WindowsCaptureHelperConfig) normalized() (WindowsCaptureHelperConfig, ScreenStreamConfig, error) {
	stream := ScreenStreamConfig{FPS: c.FPS}
	stream, err := stream.normalized()
	if err != nil {
		return WindowsCaptureHelperConfig{}, ScreenStreamConfig{}, err
	}
	if c.Bitrate == 0 {
		c.Bitrate = 4_000_000
	}
	if c.Bitrate < 250_000 || c.Bitrate > 20_000_000 {
		return WindowsCaptureHelperConfig{}, ScreenStreamConfig{}, errors.New("screen bitrate must be between 250 Kbps and 20 Mbps")
	}
	c.FPS = stream.FPS
	return c, stream, nil
}

// RunWindowsCaptureHelper is intentionally media-only. It has no signaling,
// relay credentials, controller token, input injection, clipboard or file APIs.
// It must run in the signed-in interactive user's desktop and emits only a
// bounded H.264 stream over the provided local pipe writer.
func RunWindowsCaptureHelper(ctx context.Context, out io.Writer, cfg WindowsCaptureHelperConfig) error {
	if ctx == nil || out == nil {
		return errors.New("interactive capture helper requires context and output pipe")
	}
	cfg, streamCfg, err := cfg.normalized()
	if err != nil {
		return err
	}
	capture, err := NewPrimaryDisplayBGRACapture()
	if err != nil {
		return fmt.Errorf("create interactive desktop capture: %w", err)
	}
	defer capture.Close()

	width, height := capture.Dimensions()
	encoder, err := NewMFH264Encoder(
		cfg.EncoderDLLPath,
		cfg.EncoderDLLSHA256,
		width,
		height,
		uint32(streamCfg.FPS),
		cfg.Bitrate,
	)
	if err != nil {
		return fmt.Errorf("create interactive H264 encoder: %w", err)
	}
	defer encoder.Close()

	if err := WriteCapturePipeHeader(out, CapturePipeHeader{
		Width: width,
		Height: height,
		FPS: uint32(streamCfg.FPS),
	}); err != nil {
		return fmt.Errorf("write interactive capture header: %w", err)
	}
	return RunScreenStream(ctx, capture, encoder, NewCapturePipeSink(out), streamCfg)
}
