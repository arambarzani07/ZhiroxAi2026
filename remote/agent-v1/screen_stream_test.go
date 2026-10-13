package remoteagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeFrameSource struct {
	mu      sync.Mutex
	width   uint32
	height  uint32
	count   int
	badGeom bool
}

func (f *fakeFrameSource) Dimensions() (uint32, uint32) { return f.width, f.height }
func (f *fakeFrameSource) CaptureBGRA() (BGRAFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	w, h := f.width, f.height
	if f.badGeom && f.count > 1 {
		w++
	}
	return BGRAFrame{Pixels: make([]byte, int(w*h*4)), Width: w, Height: h, Stride: w * 4, CapturedAt: time.Now()}, nil
}

type fakeEncoder struct {
	mu        sync.Mutex
	calls     int
	keyframes int
}

func (e *fakeEncoder) EncodeBGRA(frame []byte, stride uint32, timestamp100ns int64, forceKeyframe bool) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if forceKeyframe {
		e.keyframes++
	}
	if len(frame) == 0 || stride == 0 || timestamp100ns < 0 {
		return nil, errors.New("invalid fake encoder input")
	}
	return []byte{0, 0, 0, 1, 0x65, byte(e.calls)}, nil
}

type fakeSink struct {
	mu    sync.Mutex
	calls int
}

func (s *fakeSink) WriteH264AccessUnit(encoded []byte, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(encoded) == 0 || duration <= 0 {
		return errors.New("invalid fake H264 sample")
	}
	s.calls++
	return nil
}

func TestRunScreenStreamPublishesFramesAndKeyframes(t *testing.T) {
	source := &fakeFrameSource{width: 64, height: 48}
	encoder := &fakeEncoder{}
	sink := &fakeSink{}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Millisecond)
	defer cancel()
	if err := RunScreenStream(ctx, source, encoder, sink, ScreenStreamConfig{FPS: 30, KeyframeInterval: time.Second}); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if encoder.calls < 2 || sink.calls < 2 {
		t.Fatalf("expected multiple frames, encoder=%d sink=%d", encoder.calls, sink.calls)
	}
	if encoder.keyframes < 1 {
		t.Fatal("first frame must be a forced keyframe")
	}
}

func TestRunScreenStreamRejectsGeometryChange(t *testing.T) {
	source := &fakeFrameSource{width: 64, height: 48, badGeom: true}
	encoder := &fakeEncoder{}
	sink := &fakeSink{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := RunScreenStream(ctx, source, encoder, sink, ScreenStreamConfig{FPS: 30})
	if err == nil {
		t.Fatal("geometry change must fail closed")
	}
}

func TestScreenStreamConfigBounds(t *testing.T) {
	if _, err := (ScreenStreamConfig{FPS: 31}).normalized(); err == nil {
		t.Fatal("FPS above the approved bound must fail")
	}
	if err := validateBGRAFrame(BGRAFrame{Pixels: make([]byte, 10), Width: 10, Height: 10, Stride: 40}, 10, 10); err == nil {
		t.Fatal("undersized BGRA buffer must fail")
	}
}
