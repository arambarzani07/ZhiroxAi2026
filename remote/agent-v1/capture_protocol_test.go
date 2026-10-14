package remoteagent

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCapturePipeHeaderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := CapturePipeHeader{Width: 1920, Height: 1080, FPS: 15}
	if err := WriteCapturePipeHeader(&buf, want); err != nil {
		t.Fatalf("write header: %v", err)
	}
	got, err := ReadCapturePipeHeader(&buf)
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	if got != want {
		t.Fatalf("header mismatch: got %+v want %+v", got, want)
	}
}

func TestCapturePipeRejectsBadMagicAndGeometry(t *testing.T) {
	bad := make([]byte, capturePipeHeaderBytes)
	copy(bad[:8], []byte("BADMAGIC"))
	binary.BigEndian.PutUint32(bad[8:12], 1920)
	binary.BigEndian.PutUint32(bad[12:16], 1080)
	binary.BigEndian.PutUint32(bad[16:20], 15)
	if _, err := ReadCapturePipeHeader(bytes.NewReader(bad)); err == nil {
		t.Fatal("bad capture-pipe magic must be rejected")
	}

	var buf bytes.Buffer
	if err := WriteCapturePipeHeader(&buf, CapturePipeHeader{Width: 0, Height: 1080, FPS: 15}); err == nil {
		t.Fatal("zero width must be rejected")
	}
	if err := WriteCapturePipeHeader(&buf, CapturePipeHeader{Width: 1920, Height: 1080, FPS: MaxScreenFPS + 1}); err == nil {
		t.Fatal("oversized FPS must be rejected")
	}
}

func TestH264AccessUnitFrameRoundTrip(t *testing.T) {
	payload := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84}
	var buf bytes.Buffer
	if err := WriteH264AccessUnitFrame(&buf, payload); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	got, err := ReadH264AccessUnitFrame(&buf)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("frame mismatch")
	}
}

func TestH264AccessUnitFrameRejectsOversize(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxH264AccessUnitBytes+1)
	if _, err := ReadH264AccessUnitFrame(bytes.NewReader(header[:])); err == nil {
		t.Fatal("oversized access unit must be rejected before allocation")
	}
	if err := WriteH264AccessUnitFrame(&bytes.Buffer{}, make([]byte, MaxH264AccessUnitBytes+1)); err == nil {
		t.Fatal("oversized access unit must not be written")
	}
}
