//go:build windows

package remoteagent

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	remoteSMCXScreen   = 0
	remoteSMCYScreen   = 1
	remoteSRCCopy      = 0x00CC0020
	remoteDIBRGBColors = 0
	remoteBI_RGB       = 0
)

var (
	remoteUser32                 = syscall.NewLazyDLL("user32.dll")
	remoteGDI32                  = syscall.NewLazyDLL("gdi32.dll")
	remoteGetSystemMetrics       = remoteUser32.NewProc("GetSystemMetrics")
	remoteGetDC                  = remoteUser32.NewProc("GetDC")
	remoteReleaseDC              = remoteUser32.NewProc("ReleaseDC")
	remoteCreateCompatibleDC     = remoteGDI32.NewProc("CreateCompatibleDC")
	remoteDeleteDC               = remoteGDI32.NewProc("DeleteDC")
	remoteCreateDIBSection       = remoteGDI32.NewProc("CreateDIBSection")
	remoteSelectObject           = remoteGDI32.NewProc("SelectObject")
	remoteDeleteObject           = remoteGDI32.NewProc("DeleteObject")
	remoteBitBlt                 = remoteGDI32.NewProc("BitBlt")
)

type remoteBitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type remoteBitmapInfo struct {
	Header remoteBitmapInfoHeader
	Colors [1]uint32
}

// PrimaryDisplayBGRACapture owns a top-down 32-bit DIB section. BitBlt writes
// directly into the DIB's BGRA/BGRX memory, avoiding JPEG/base64 conversion and
// avoiding a second GetDIBits copy on every streaming frame.
type PrimaryDisplayBGRACapture struct {
	mu       sync.Mutex
	width    uint32
	height   uint32
	stride   uint32
	screenDC uintptr
	memDC    uintptr
	bitmap   uintptr
	old      uintptr
	bits     uintptr
	closed   bool
}

func NewPrimaryDisplayBGRACapture() (*PrimaryDisplayBGRACapture, error) {
	wRaw, _, _ := remoteGetSystemMetrics.Call(remoteSMCXScreen)
	hRaw, _, _ := remoteGetSystemMetrics.Call(remoteSMCYScreen)
	w := int(int32(wRaw))
	h := int(int32(hRaw))
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, fmt.Errorf("invalid primary display geometry %dx%d", w, h)
	}

	screenDC, _, callErr := remoteGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed: %v", callErr)
	}
	cleanupScreen := true
	defer func() {
		if cleanupScreen {
			remoteReleaseDC.Call(0, screenDC)
		}
	}()

	memDC, _, callErr := remoteCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed: %v", callErr)
	}
	cleanupMem := true
	defer func() {
		if cleanupMem {
			remoteDeleteDC.Call(memDC)
		}
	}()

	stride := uint32(w * 4)
	bmi := remoteBitmapInfo{Header: remoteBitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(remoteBitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h),
		Planes:      1,
		BitCount:    32,
		Compression: remoteBI_RGB,
		SizeImage:   stride * uint32(h),
	}}
	var bits uintptr
	bitmap, _, callErr := remoteCreateDIBSection.Call(
		screenDC,
		uintptr(unsafe.Pointer(&bmi)),
		remoteDIBRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if bitmap == 0 || bits == 0 {
		return nil, fmt.Errorf("CreateDIBSection failed: %v", callErr)
	}
	cleanupBitmap := true
	defer func() {
		if cleanupBitmap {
			remoteDeleteObject.Call(bitmap)
		}
	}()

	old, _, _ := remoteSelectObject.Call(memDC, bitmap)
	if old == 0 {
		return nil, errors.New("SelectObject failed for desktop DIB section")
	}

	cleanupScreen = false
	cleanupMem = false
	cleanupBitmap = false
	return &PrimaryDisplayBGRACapture{
		width: uint32(w), height: uint32(h), stride: stride,
		screenDC: screenDC, memDC: memDC, bitmap: bitmap, old: old, bits: bits,
	}, nil
}

func (c *PrimaryDisplayBGRACapture) Dimensions() (uint32, uint32) {
	if c == nil {
		return 0, 0
	}
	return c.width, c.height
}

func (c *PrimaryDisplayBGRACapture) CaptureBGRA() (BGRAFrame, error) {
	if c == nil {
		return BGRAFrame{}, errors.New("desktop capture is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.screenDC == 0 || c.memDC == 0 || c.bits == 0 {
		return BGRAFrame{}, errors.New("desktop capture is closed")
	}
	ret, _, callErr := remoteBitBlt.Call(
		c.memDC,
		0,
		0,
		uintptr(c.width),
		uintptr(c.height),
		c.screenDC,
		0,
		0,
		remoteSRCCopy,
	)
	if ret == 0 {
		return BGRAFrame{}, fmt.Errorf("BitBlt failed: %v", callErr)
	}
	size := uint64(c.stride) * uint64(c.height)
	if size == 0 || size > 16384*16384*4 {
		return BGRAFrame{}, errors.New("desktop DIB size is outside approved bounds")
	}
	pixels := unsafe.Slice((*byte)(unsafe.Pointer(c.bits)), int(size))
	return BGRAFrame{
		Pixels: pixels,
		Width: c.width,
		Height: c.height,
		Stride: c.stride,
		CapturedAt: time.Now().UTC(),
	}, nil
}

func (c *PrimaryDisplayBGRACapture) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.memDC != 0 && c.old != 0 {
		remoteSelectObject.Call(c.memDC, c.old)
		c.old = 0
	}
	if c.bitmap != 0 {
		remoteDeleteObject.Call(c.bitmap)
		c.bitmap = 0
		c.bits = 0
	}
	if c.memDC != 0 {
		remoteDeleteDC.Call(c.memDC)
		c.memDC = 0
	}
	if c.screenDC != 0 {
		remoteReleaseDC.Call(0, c.screenDC)
		c.screenDC = 0
	}
	return nil
}
