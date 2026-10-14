package remoteagent

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	CapturePipeProtocolVersion = 1
	MaxH264AccessUnitBytes     = 4 * 1024 * 1024
	capturePipeMagic           = "ZHXH2641"
	capturePipeHeaderBytes     = 20
)

type CapturePipeHeader struct {
	Width  uint32
	Height uint32
	FPS    uint32
}

func (h CapturePipeHeader) validate() error {
	if h.Width == 0 || h.Height == 0 || h.Width > 16384 || h.Height > 16384 {
		return fmt.Errorf("capture pipe geometry %dx%d is outside approved bounds", h.Width, h.Height)
	}
	if h.FPS < 1 || h.FPS > MaxScreenFPS {
		return fmt.Errorf("capture pipe FPS %d is outside approved bounds", h.FPS)
	}
	return nil
}

func WriteCapturePipeHeader(w io.Writer, h CapturePipeHeader) error {
	if w == nil {
		return errors.New("capture pipe writer is required")
	}
	if err := h.validate(); err != nil {
		return err
	}
	buf := make([]byte, capturePipeHeaderBytes)
	copy(buf[:8], []byte(capturePipeMagic))
	binary.BigEndian.PutUint32(buf[8:12], h.Width)
	binary.BigEndian.PutUint32(buf[12:16], h.Height)
	binary.BigEndian.PutUint32(buf[16:20], h.FPS)
	_, err := w.Write(buf)
	return err
}

func ReadCapturePipeHeader(r io.Reader) (CapturePipeHeader, error) {
	if r == nil {
		return CapturePipeHeader{}, errors.New("capture pipe reader is required")
	}
	buf := make([]byte, capturePipeHeaderBytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return CapturePipeHeader{}, fmt.Errorf("read capture pipe header: %w", err)
	}
	if string(buf[:8]) != capturePipeMagic {
		return CapturePipeHeader{}, errors.New("capture pipe magic/version mismatch")
	}
	h := CapturePipeHeader{
		Width: binary.BigEndian.Uint32(buf[8:12]),
		Height: binary.BigEndian.Uint32(buf[12:16]),
		FPS: binary.BigEndian.Uint32(buf[16:20]),
	}
	if err := h.validate(); err != nil {
		return CapturePipeHeader{}, err
	}
	return h, nil
}

func WriteH264AccessUnitFrame(w io.Writer, encoded []byte) error {
	if w == nil {
		return errors.New("capture pipe writer is required")
	}
	if len(encoded) == 0 {
		return nil
	}
	if len(encoded) > MaxH264AccessUnitBytes {
		return fmt.Errorf("H264 access unit exceeds %d-byte safety limit", MaxH264AccessUnitBytes)
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(encoded)))
	if _, err := w.Write(size[:]); err != nil {
		return err
	}
	_, err := w.Write(encoded)
	return err
}

func ReadH264AccessUnitFrame(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("capture pipe reader is required")
	}
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MaxH264AccessUnitBytes {
		return nil, fmt.Errorf("capture pipe H264 access unit length %d is invalid", n)
	}
	buf := make([]byte, int(n))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read capture pipe H264 access unit: %w", err)
	}
	return buf, nil
}

type capturePipeSink struct {
	w io.Writer
}

func NewCapturePipeSink(w io.Writer) H264FrameSink {
	return &capturePipeSink{w: w}
}

func (s *capturePipeSink) WriteH264AccessUnit(encoded []byte, _ time.Duration) error {
	if s == nil || s.w == nil {
		return errors.New("capture pipe sink is not initialized")
	}
	return WriteH264AccessUnitFrame(s.w, encoded)
}
