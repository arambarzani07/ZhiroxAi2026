//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	pcOSSMCXScreen                  = 0
	pcOSSMCYScreen                  = 1
	pcOSSRCCopy                     = 0x00CC0020
	pcOSDIBRGBColors                = 0
	pcOSBI_RGB                      = 0
	pcOSProcessQueryLimitedInfo     = 0x1000
	pcOSHandleFlagInherit           = 0x00000001
	pcOSCreateNoWindow              = 0x08000000
	pcOSWaitObject0                 = 0x00000000
	pcOSWaitTimeout                 = 0x00000102
	pcOSScreenHelperTimeoutMS       = 20000
	pcOSScreenHelperMaxResponseByte = 1400000
)

var (
	pcOSUser32                        = syscall.NewLazyDLL("user32.dll")
	pcOSGDI32                         = syscall.NewLazyDLL("gdi32.dll")
	pcOSKernel32                      = syscall.NewLazyDLL("kernel32.dll")
	pcOSWTSAPI32                      = syscall.NewLazyDLL("wtsapi32.dll")
	pcOSGetForegroundWindow           = pcOSUser32.NewProc("GetForegroundWindow")
	pcOSGetWindowTextLengthW          = pcOSUser32.NewProc("GetWindowTextLengthW")
	pcOSGetWindowTextW                = pcOSUser32.NewProc("GetWindowTextW")
	pcOSGetWindowThreadProcessId      = pcOSUser32.NewProc("GetWindowThreadProcessId")
	pcOSGetSystemMetrics              = pcOSUser32.NewProc("GetSystemMetrics")
	pcOSGetDC                         = pcOSUser32.NewProc("GetDC")
	pcOSReleaseDC                     = pcOSUser32.NewProc("ReleaseDC")
	pcOSCreateCompatibleDC            = pcOSGDI32.NewProc("CreateCompatibleDC")
	pcOSDeleteDC                      = pcOSGDI32.NewProc("DeleteDC")
	pcOSCreateCompatibleBitmap        = pcOSGDI32.NewProc("CreateCompatibleBitmap")
	pcOSSelectObject                  = pcOSGDI32.NewProc("SelectObject")
	pcOSDeleteObject                  = pcOSGDI32.NewProc("DeleteObject")
	pcOSBitBlt                        = pcOSGDI32.NewProc("BitBlt")
	pcOSGetDIBits                     = pcOSGDI32.NewProc("GetDIBits")
	pcOSOpenProcess                   = pcOSKernel32.NewProc("OpenProcess")
	pcOSCloseHandle                   = pcOSKernel32.NewProc("CloseHandle")
	pcOSQueryFullProcessImageNameW    = pcOSKernel32.NewProc("QueryFullProcessImageNameW")
	pcOSProcessIdToSessionId          = pcOSKernel32.NewProc("ProcessIdToSessionId")
	pcOSWTSGetActiveConsoleSessionId  = pcOSKernel32.NewProc("WTSGetActiveConsoleSessionId")
	pcOSCreatePipe                    = pcOSKernel32.NewProc("CreatePipe")
	pcOSSetHandleInformation          = pcOSKernel32.NewProc("SetHandleInformation")
	pcOSWaitForSingleObject           = pcOSKernel32.NewProc("WaitForSingleObject")
	pcOSWTSQueryUserToken             = pcOSWTSAPI32.NewProc("WTSQueryUserToken")
	pcOSCreateProcessAsUserW          = syscall.NewLazyDLL("advapi32.dll").NewProc("CreateProcessAsUserW")
)

type pcOSBitmapInfoHeader struct {
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

type pcOSBitmapInfo struct {
	Header pcOSBitmapInfoHeader
	Colors [1]uint32
}

type pcOSSecurityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      int32
}

type pcOSStartupInfo struct {
	Cb            uint32
	Reserved      *uint16
	Desktop       *uint16
	Title         *uint16
	X             uint32
	Y             uint32
	XSize         uint32
	YSize         uint32
	XCountChars   uint32
	YCountChars   uint32
	FillAttribute uint32
	Flags         uint32
	ShowWindow    uint16
	Reserved2Size uint16
	Reserved2     *byte
	StdInput      syscall.Handle
	StdOutput     syscall.Handle
	StdError      syscall.Handle
}

type pcOSProcessInformation struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessID uint32
	ThreadID  uint32
}

type pcOSScreenHelperEnvelope struct {
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func pcOSScreenObservationHelperRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--zhirox-screen-observation-helper" {
			return true
		}
	}
	return false
}

func pcOSRunScreenObservationHelper(args []string) int {
	mode := ""
	pipeHandle := uintptr(0)
	cfg := pcOSScreenCaptureConfig{MaxWidth: pcOSScreenDefaultMaxWidth, Quality: pcOSScreenDefaultQuality}
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--mode="):
			mode = strings.TrimPrefix(arg, "--mode=")
		case strings.HasPrefix(arg, "--pipe-handle="):
			n, _ := strconv.ParseUint(strings.TrimPrefix(arg, "--pipe-handle="), 10, 64)
			pipeHandle = uintptr(n)
		case strings.HasPrefix(arg, "--max-width="):
			n, _ := strconv.Atoi(strings.TrimPrefix(arg, "--max-width="))
			cfg.MaxWidth = n
		case strings.HasPrefix(arg, "--quality="):
			n, _ := strconv.Atoi(strings.TrimPrefix(arg, "--quality="))
			cfg.Quality = n
		}
	}
	if pipeHandle == 0 {
		return 2
	}
	file := os.NewFile(pipeHandle, "zhirox-screen-observation-pipe")
	if file == nil {
		return 2
	}
	defer file.Close()

	validatedMode, err := pcOSScreenObservationMode(mode)
	if err != nil {
		_ = json.NewEncoder(file).Encode(pcOSScreenHelperEnvelope{OK: false, Error: err.Error()})
		return 3
	}

	var result interface{}
	switch validatedMode {
	case "active_window":
		result, err = pcOSActiveWindowLocal()
	case "screenshot":
		if cfg.MaxWidth < pcOSScreenMinMaxWidth || cfg.MaxWidth > pcOSScreenMaxMaxWidth || cfg.Quality < pcOSScreenMinQuality || cfg.Quality > pcOSScreenMaxQuality {
			err = errors.New("screen helper capture configuration outside approved bounds")
		} else {
			result, err = pcOSCaptureScreenshotLocal(cfg)
		}
	}
	if err != nil {
		_ = json.NewEncoder(file).Encode(pcOSScreenHelperEnvelope{OK: false, Error: err.Error()})
		return 4
	}
	_ = json.NewEncoder(file).Encode(pcOSScreenHelperEnvelope{OK: true, Result: result})
	return 0
}

func pcOSActiveWindow() (interface{}, error) {
	return pcOSRunScreenObservation("active_window", pcOSScreenCaptureConfig{})
}

func pcOSCaptureScreenshot(args map[string]interface{}) (interface{}, error) {
	cfg, err := pcOSScreenCaptureConfigFromArgs(args)
	if err != nil {
		return nil, err
	}
	return pcOSRunScreenObservation("screenshot", cfg)
}

func pcOSRunScreenObservation(mode string, cfg pcOSScreenCaptureConfig) (interface{}, error) {
	mode, err := pcOSScreenObservationMode(mode)
	if err != nil {
		return nil, err
	}
	same, activeSession, err := pcOSCurrentProcessInActiveConsoleSession()
	if err != nil {
		return nil, err
	}
	if same {
		if mode == "active_window" {
			return pcOSActiveWindowLocal()
		}
		return pcOSCaptureScreenshotLocal(cfg)
	}
	return pcOSRunObservationInSession(mode, cfg, activeSession)
}

func pcOSCurrentProcessInActiveConsoleSession() (bool, uint32, error) {
	activeRaw, _, _ := pcOSWTSGetActiveConsoleSessionId.Call()
	active := uint32(activeRaw)
	if active == 0xFFFFFFFF {
		return false, active, errors.New("no active interactive console session")
	}
	var current uint32
	ret, _, callErr := pcOSProcessIdToSessionId.Call(uintptr(os.Getpid()), uintptr(unsafe.Pointer(&current)))
	if ret == 0 {
		return false, active, fmt.Errorf("ProcessIdToSessionId failed: %v", callErr)
	}
	return current == active, active, nil
}

func pcOSRunObservationInSession(mode string, cfg pcOSScreenCaptureConfig, sessionID uint32) (interface{}, error) {
	var token syscall.Handle
	ret, _, callErr := pcOSWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if ret == 0 || token == 0 {
		return nil, fmt.Errorf("WTSQueryUserToken failed for interactive session %d: %v", sessionID, callErr)
	}
	defer pcOSCloseHandle.Call(uintptr(token))

	var readPipe, writePipe syscall.Handle
	sa := pcOSSecurityAttributes{Length: uint32(unsafe.Sizeof(pcOSSecurityAttributes{})), InheritHandle: 1}
	ret, _, callErr = pcOSCreatePipe.Call(uintptr(unsafe.Pointer(&readPipe)), uintptr(unsafe.Pointer(&writePipe)), uintptr(unsafe.Pointer(&sa)), 0)
	if ret == 0 {
		return nil, fmt.Errorf("CreatePipe failed: %v", callErr)
	}
	readFile := os.NewFile(uintptr(readPipe), "zhirox-screen-observation-read")
	if readFile == nil {
		pcOSCloseHandle.Call(uintptr(readPipe))
		pcOSCloseHandle.Call(uintptr(writePipe))
		return nil, errors.New("failed to wrap screen observation read pipe")
	}
	defer readFile.Close()
	defer pcOSCloseHandle.Call(uintptr(writePipe))
	pcOSSetHandleInformation.Call(uintptr(readPipe), pcOSHandleFlagInherit, 0)

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}
	appPtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return nil, err
	}
	desktopPtr, _ := syscall.UTF16PtrFromString(`winsta0\default`)
	cmd := fmt.Sprintf(`"ZhiroxAISupervisor.exe" --zhirox-screen-observation-helper --mode=%s --pipe-handle=%d --max-width=%d --quality=%d`, mode, uintptr(writePipe), cfg.MaxWidth, cfg.Quality)
	cmdPtr, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return nil, err
	}
	cwdPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	si := pcOSStartupInfo{Cb: uint32(unsafe.Sizeof(pcOSStartupInfo{})), Desktop: desktopPtr}
	var pi pcOSProcessInformation
	ret, _, callErr = pcOSCreateProcessAsUserW.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0, 0, 1,
		pcOSCreateNoWindow,
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CreateProcessAsUserW failed: %v", callErr)
	}
	defer pcOSCloseHandle.Call(uintptr(pi.Process))
	defer pcOSCloseHandle.Call(uintptr(pi.Thread))

	pcOSCloseHandle.Call(uintptr(writePipe))
	writePipe = 0

	readCh := make(chan struct {
		b   []byte
		err error
	}, 1)
	go func() {
		b, e := io.ReadAll(io.LimitReader(readFile, pcOSScreenHelperMaxResponseByte+1))
		readCh <- struct {
			b   []byte
			err error
		}{b: b, err: e}
	}()

	waitRet, _, waitErr := pcOSWaitForSingleObject.Call(uintptr(pi.Process), pcOSScreenHelperTimeoutMS)
	if waitRet == pcOSWaitTimeout {
		return nil, errors.New("interactive screen observation helper timed out")
	}
	if waitRet != pcOSWaitObject0 {
		return nil, fmt.Errorf("WaitForSingleObject failed: %v", waitErr)
	}

	readResult := <-readCh
	if readResult.err != nil {
		return nil, readResult.err
	}
	if len(readResult.b) > pcOSScreenHelperMaxResponseByte {
		return nil, errors.New("interactive screen observation response exceeded safety limit")
	}
	var envelope pcOSScreenHelperEnvelope
	if err := json.Unmarshal(readResult.b, &envelope); err != nil {
		return nil, fmt.Errorf("invalid interactive screen observation response: %w", err)
	}
	if !envelope.OK {
		return nil, errors.New(envelope.Error)
	}
	return envelope.Result, nil
}

func pcOSActiveWindowLocal() (interface{}, error) {
	hwnd, _, _ := pcOSGetForegroundWindow.Call()
	if hwnd == 0 {
		return nil, errors.New("no foreground window is available in the interactive session")
	}
	length, _, _ := pcOSGetWindowTextLengthW.Call(hwnd)
	if length > 4096 {
		length = 4096
	}
	buf := make([]uint16, int(length)+2)
	if len(buf) > 1 {
		pcOSGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	}
	title := syscall.UTF16ToString(buf)
	var pid uint32
	pcOSGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	path := pcOSProcessImagePath(pid)
	return map[string]interface{}{
		"module":          "screen_observability_read_only",
		"captured_at_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"window_title":    title,
		"process_id":      pid,
		"process_name":    filepath.Base(path),
		"executable_path": path,
		"read_only":       true,
		"input_control":   false,
	}, nil
}

func pcOSProcessImagePath(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, _, _ := pcOSOpenProcess.Call(pcOSProcessQueryLimitedInfo, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer pcOSCloseHandle.Call(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	ret, _, _ := pcOSQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ret == 0 || size == 0 || int(size) > len(buf) {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

func pcOSCaptureScreenshotLocal(cfg pcOSScreenCaptureConfig) (interface{}, error) {
	img, err := pcOSCapturePrimaryDisplayRGBA()
	if err != nil {
		return nil, err
	}
	attempts := []pcOSScreenCaptureConfig{
		cfg,
		{MaxWidth: minInt(cfg.MaxWidth, 1024), Quality: minInt(cfg.Quality, 35)},
		{MaxWidth: 800, Quality: 25},
	}
	var encoded []byte
	var final image.Image
	var used pcOSScreenCaptureConfig
	for _, attempt := range attempts {
		candidate := pcOSScaleImageToMaxWidth(img, attempt.MaxWidth)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, candidate, &jpeg.Options{Quality: attempt.Quality}); err != nil {
			return nil, err
		}
		if buf.Len() <= pcOSScreenMaxJPEGBytes {
			encoded = append([]byte(nil), buf.Bytes()...)
			final = candidate
			used = attempt
			break
		}
	}
	if len(encoded) == 0 || final == nil {
		return nil, errors.New("screenshot could not be reduced below the approved payload limit")
	}
	sum := sha256.Sum256(encoded)
	bounds := final.Bounds()
	return map[string]interface{}{
		"module":             "screen_observability_read_only",
		"captured_at_utc":    time.Now().UTC().Format(time.RFC3339Nano),
		"capture_scope":      "primary_display",
		"mime_type":          "image/jpeg",
		"width":              bounds.Dx(),
		"height":             bounds.Dy(),
		"quality":            used.Quality,
		"bytes":              len(encoded),
		"sha256":             hex.EncodeToString(sum[:]),
		"image_base64":       base64.StdEncoding.EncodeToString(encoded),
		"on_demand_only":     true,
		"continuous_capture": false,
		"input_control":      false,
		"transient_only":     true,
	}, nil
}

func pcOSCapturePrimaryDisplayRGBA() (*image.RGBA, error) {
	wRaw, _, _ := pcOSGetSystemMetrics.Call(pcOSSMCXScreen)
	hRaw, _, _ := pcOSGetSystemMetrics.Call(pcOSSMCYScreen)
	w, h := int(int32(wRaw)), int(int32(hRaw))
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, fmt.Errorf("invalid primary display geometry %dx%d", w, h)
	}
	screenDC, _, callErr := pcOSGetDC.Call(0)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC failed: %v", callErr)
	}
	defer pcOSReleaseDC.Call(0, screenDC)
	memDC, _, callErr := pcOSCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC failed: %v", callErr)
	}
	defer pcOSDeleteDC.Call(memDC)
	bitmap, _, callErr := pcOSCreateCompatibleBitmap.Call(screenDC, uintptr(w), uintptr(h))
	if bitmap == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap failed: %v", callErr)
	}
	defer pcOSDeleteObject.Call(bitmap)
	old, _, _ := pcOSSelectObject.Call(memDC, bitmap)
	if old == 0 {
		return nil, errors.New("SelectObject failed")
	}
	defer pcOSSelectObject.Call(memDC, old)
	ret, _, callErr := pcOSBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, 0, 0, pcOSSRCCopy)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt failed: %v", callErr)
	}
	pixels := make([]byte, w*h*4)
	bmi := pcOSBitmapInfo{Header: pcOSBitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(pcOSBitmapInfoHeader{})),
		Width:       int32(w),
		Height:      -int32(h),
		Planes:      1,
		BitCount:    32,
		Compression: pcOSBI_RGB,
		SizeImage:   uint32(len(pixels)),
	}}
	ret, _, callErr = pcOSGetDIBits.Call(memDC, bitmap, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bmi)), pcOSDIBRGBColors)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed: %v", callErr)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, j := 0, 0; i+3 < len(pixels) && j+3 < len(img.Pix); i, j = i+4, j+4 {
		img.Pix[j+0] = pixels[i+2]
		img.Pix[j+1] = pixels[i+1]
		img.Pix[j+2] = pixels[i+0]
		img.Pix[j+3] = 0xFF
	}
	return img, nil
}

func pcOSScaleImageToMaxWidth(src *image.RGBA, maxWidth int) image.Image {
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if maxWidth <= 0 || sw <= maxWidth {
		return src
	}
	dw := maxWidth
	dh := maxInt(1, int(float64(sh)*float64(dw)/float64(sw)))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := y * sh / dh
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
