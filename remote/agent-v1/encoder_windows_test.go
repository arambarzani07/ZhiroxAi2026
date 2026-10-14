//go:build windows

package remoteagent

import (
	"os"
	"testing"
)

func TestMFH264EncoderDLLBridge(t *testing.T) {
	path := os.Getenv("ZHIROX_MF_ENCODER_DLL")
	hash := os.Getenv("ZHIROX_MF_ENCODER_SHA256")
	if path == "" || hash == "" {
		t.Skip("Media Foundation encoder DLL probe environment is not configured")
	}

	const width = uint32(640)
	const height = uint32(480)
	const fps = uint32(15)
	enc, err := NewMFH264Encoder(path, hash, width, height, fps, 1500000)
	if err != nil {
		t.Fatalf("create encoder bridge: %v", err)
	}
	defer enc.Close()

	frame := make([]byte, int(width*height*4))
	for y := uint32(0); y < height; y++ {
		for x := uint32(0); x < width; x++ {
			p := int((y*width + x) * 4)
			frame[p+0] = byte((x + y) & 0xff)
			frame[p+1] = byte((2 * y) & 0xff)
			frame[p+2] = byte((2 * x) & 0xff)
			frame[p+3] = 0xff
		}
	}

	var total int
	for i := 0; i < 12; i++ {
		out, err := enc.EncodeBGRA(frame, width*4, int64(i)*(10000000/int64(fps)), i == 0)
		if err != nil {
			t.Fatalf("encode frame %d: %v", i, err)
		}
		total += len(out)
	}
	if total == 0 {
		t.Fatal("encoder bridge produced no H264 bytes")
	}
}

func TestMFH264EncoderRequiresSHA256Pin(t *testing.T) {
	if _, err := NewMFH264Encoder(`C:\\temp\\ZhiroxMFH264Encoder.dll`, "", 640, 480, 15, 1500000); err == nil {
		t.Fatal("encoder DLL must not load without SHA-256 pin")
	}
}
