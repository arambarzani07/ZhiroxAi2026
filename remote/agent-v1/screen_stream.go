package remoteagent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	DefaultScreenFPS              = 15
	DefaultScreenKeyframeInterval = 2 * time.Second
	MaxScreenFPS                  = 30
)

type BGRAFrame struct {
	Pixels     []byte
	Width      uint32
	Height     uint32
	Stride     uint32
	CapturedAt time.Time
}

type BGRAFrameSource interface {
	Dimensions() (width, height uint32)
	CaptureBGRA() (BGRAFrame, error)
}

type H264FrameEncoder interface {
	EncodeBGRA(frame []byte, stride uint32, timestamp100ns int64, forceKeyframe bool) ([]byte, error)
}

type H264FrameSink interface {
	WriteH264AccessUnit(encoded []byte, duration time.Duration) error
}

type ScreenStreamConfig struct {
	FPS              int
	KeyframeInterval time.Duration
}

func (c ScreenStreamConfig) normalized() (ScreenStreamConfig, error) {
	if c.FPS == 0 {
		c.FPS = DefaultScreenFPS
	}
	if c.KeyframeInterval == 0 {
		c.KeyframeInterval = DefaultScreenKeyframeInterval
	}
	if c.FPS < 1 || c.FPS > MaxScreenFPS {
		return ScreenStreamConfig{}, fmt.Errorf("screen FPS must be between 1 and %d", MaxScreenFPS)
	}
	if c.KeyframeInterval < time.Second || c.KeyframeInterval > 10*time.Second {
		return ScreenStreamConfig{}, errors.New("screen keyframe interval must be between 1s and 10s")
	}
	return c, nil
}

func validateBGRAFrame(frame BGRAFrame, expectedWidth, expectedHeight uint32) error {
	if frame.Width == 0 || frame.Height == 0 || frame.Width != expectedWidth || frame.Height != expectedHeight {
		return fmt.Errorf("captured frame geometry changed from %dx%d to %dx%d", expectedWidth, expectedHeight, frame.Width, frame.Height)
	}
	if frame.Stride < frame.Width*4 {
		return errors.New("captured BGRA stride is smaller than width*4")
	}
	need := uint64(frame.Stride) * uint64(frame.Height)
	if need == 0 || need > uint64(len(frame.Pixels)) {
		return errors.New("captured BGRA buffer is smaller than stride*height")
	}
	return nil
}

// RunScreenStream performs a bounded, single-threaded capture -> encode -> WebRTC
// loop. The source buffer may be reused by the capture implementation, so each
// frame is fully encoded before the next capture occurs.
func RunScreenStream(ctx context.Context, source BGRAFrameSource, encoder H264FrameEncoder, sink H264FrameSink, cfg ScreenStreamConfig) error {
	if ctx == nil || source == nil || encoder == nil || sink == nil {
		return errors.New("screen stream requires context, source, encoder and sink")
	}
	cfg, err := cfg.normalized()
	if err != nil {
		return err
	}
	width, height := source.Dimensions()
	if width == 0 || height == 0 || width > 16384 || height > 16384 {
		return fmt.Errorf("screen source geometry %dx%d is outside approved bounds", width, height)
	}

	frameDuration := time.Second / time.Duration(cfg.FPS)
	keyframeEvery := int(cfg.KeyframeInterval / frameDuration)
	if keyframeEvery < 1 {
		keyframeEvery = 1
	}
	timestampStep := int64(10_000_000 / cfg.FPS)
	if timestampStep <= 0 {
		return errors.New("invalid encoder timestamp step")
	}

	encodeOne := func(index int64) error {
		frame, err := source.CaptureBGRA()
		if err != nil {
			return fmt.Errorf("capture desktop frame: %w", err)
		}
		if err := validateBGRAFrame(frame, width, height); err != nil {
			return err
		}
		forceKeyframe := index == 0 || index%int64(keyframeEvery) == 0
		encoded, err := encoder.EncodeBGRA(frame.Pixels, frame.Stride, index*timestampStep, forceKeyframe)
		if err != nil {
			return fmt.Errorf("encode desktop frame: %w", err)
		}
		if len(encoded) == 0 {
			return nil
		}
		if err := sink.WriteH264AccessUnit(encoded, frameDuration); err != nil {
			return fmt.Errorf("publish desktop frame: %w", err)
		}
		return nil
	}

	var index int64
	if err := encodeOne(index); err != nil {
		return err
	}
	index++

	ticker := time.NewTicker(frameDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := encodeOne(index); err != nil {
				return err
			}
			index++
		}
	}
}
