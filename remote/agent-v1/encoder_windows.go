//go:build windows

package remoteagent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const mfEncoderDLLName = "ZhiroxMFH264Encoder.dll"

type MFH264Encoder struct {
	dll      *syscall.DLL
	create   *syscall.Proc
	destroy  *syscall.Proc
	encode   *syscall.Proc
	handle   uintptr
	width    uint32
	height   uint32
	capacity int
}

func NewMFH264Encoder(dllPath, expectedSHA256 string, width, height, fps, bitrate uint32) (*MFH264Encoder, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dllPath))
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(abs) || !strings.EqualFold(filepath.Base(abs), mfEncoderDLLName) {
		return nil, errors.New("Media Foundation encoder must be loaded by absolute approved DLL path")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256) != 64 {
		return nil, errors.New("Media Foundation encoder SHA-256 pin is required")
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return nil, errors.New("Media Foundation encoder SHA-256 pin is invalid")
	}
	actual, err := fileSHA256(abs)
	if err != nil {
		return nil, err
	}
	if actual != expectedSHA256 {
		return nil, fmt.Errorf("Media Foundation encoder SHA-256 mismatch: expected %s, got %s", expectedSHA256, actual)
	}

	dll, err := syscall.LoadDLL(abs)
	if err != nil {
		return nil, fmt.Errorf("load Media Foundation encoder DLL: %w", err)
	}
	cleanup := func() { _ = dll.Release() }
	create, err := dll.FindProc("zhirox_encoder_create")
	if err != nil {
		cleanup()
		return nil, err
	}
	destroy, err := dll.FindProc("zhirox_encoder_destroy")
	if err != nil {
		cleanup()
		return nil, err
	}
	encode, err := dll.FindProc("zhirox_encoder_encode_bgra")
	if err != nil {
		cleanup()
		return nil, err
	}

	var native uintptr
	r1, _, _ := create.Call(
		uintptr(width),
		uintptr(height),
		uintptr(fps),
		uintptr(bitrate),
		uintptr(unsafe.Pointer(&native)),
	)
	if hresultFailed(r1) || native == 0 {
		cleanup()
		return nil, fmt.Errorf("create Media Foundation H264 encoder failed: HRESULT 0x%08x", uint32(r1))
	}

	capacity := int(uint64(width) * uint64(height) * 2)
	if capacity < 512*1024 {
		capacity = 512 * 1024
	}
	if capacity > 20*1024*1024 {
		destroy.Call(native)
		cleanup()
		return nil, errors.New("Media Foundation encoder output safety capacity exceeded")
	}
	return &MFH264Encoder{
		dll: dll, create: create, destroy: destroy, encode: encode,
		handle: native, width: width, height: height, capacity: capacity,
	}, nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func hresultFailed(v uintptr) bool {
	return int32(uint32(v)) < 0
}

func (e *MFH264Encoder) EncodeBGRA(frame []byte, stride uint32, timestamp100ns int64, forceKeyframe bool) ([]byte, error) {
	if e == nil || e.handle == 0 || e.encode == nil {
		return nil, errors.New("Media Foundation encoder is closed")
	}
	if stride < e.width*4 {
		return nil, errors.New("BGRA stride is smaller than the encoded width")
	}
	need := uint64(stride) * uint64(e.height)
	if need == 0 || need > uint64(len(frame)) {
		return nil, errors.New("BGRA frame buffer is smaller than stride*height")
	}
	output := make([]byte, e.capacity)
	var outSize uintptr
	force := uintptr(0)
	if forceKeyframe {
		force = 1
	}
	r1, _, _ := e.encode.Call(
		e.handle,
		uintptr(unsafe.Pointer(&frame[0])),
		uintptr(stride),
		uintptr(timestamp100ns),
		force,
		uintptr(unsafe.Pointer(&output[0])),
		uintptr(len(output)),
		uintptr(unsafe.Pointer(&outSize)),
	)
	if hresultFailed(r1) {
		return nil, fmt.Errorf("Media Foundation H264 encode failed: HRESULT 0x%08x (reported size %d)", uint32(r1), outSize)
	}
	if outSize == 0 {
		return nil, nil
	}
	if outSize > uintptr(len(output)) {
		return nil, errors.New("Media Foundation encoder reported output beyond approved buffer")
	}
	return output[:outSize], nil
}

func (e *MFH264Encoder) Close() error {
	if e == nil {
		return nil
	}
	if e.handle != 0 && e.destroy != nil {
		e.destroy.Call(e.handle)
		e.handle = 0
	}
	if e.dll != nil {
		err := e.dll.Release()
		e.dll = nil
		return err
	}
	return nil
}
