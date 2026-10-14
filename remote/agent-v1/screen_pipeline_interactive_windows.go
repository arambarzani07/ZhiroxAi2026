//go:build windows

package remoteagent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	remoteCaptureHelperFlag                = "--zhirox-capture-helper"
	remoteCaptureHelperBase                = "ZhiroxRemoteScreenAgent.exe"
	remoteHandleFlagInherit                = 0x00000001
	remoteExtendedStartupInfoPresent       = 0x00080000
	remoteCreateUnicodeEnvironment         = 0x00000400
	remoteProcThreadAttributeHandleList    = 0x00020002
	remoteCaptureHelperTerminateExit uint32 = 23
)

var (
	remoteCreatePipe                       = remoteKernel32.NewProc("CreatePipe")
	remoteSetHandleInformation             = remoteKernel32.NewProc("SetHandleInformation")
	remoteInitializeProcThreadAttributeList = remoteKernel32.NewProc("InitializeProcThreadAttributeList")
	remoteUpdateProcThreadAttribute         = remoteKernel32.NewProc("UpdateProcThreadAttribute")
	remoteDeleteProcThreadAttributeList     = remoteKernel32.NewProc("DeleteProcThreadAttributeList")
	remoteUserenv                           = syscall.NewLazyDLL("userenv.dll")
	remoteCreateEnvironmentBlock            = remoteUserenv.NewProc("CreateEnvironmentBlock")
	remoteDestroyEnvironmentBlock           = remoteUserenv.NewProc("DestroyEnvironmentBlock")
)

type remoteSecurityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      int32
}

type remoteStartupInfoEx struct {
	StartupInfo    remoteStartupInfo
	AttributeList  uintptr
}

type InteractiveWindowsScreenPipeline struct {
	session       *AgentSession
	helperPath    string
	helperSHA     [sha256.Size]byte
	encoderPath   string
	encoderSHA256 string
	fps           int
	bitrate       uint32

	mu       sync.Mutex
	proc     *interactiveProcess
	readFile *os.File
	closed   bool
}

func NewInteractiveWindowsScreenPipeline(session *AgentSession, encoderPath, encoderSHA256 string, fps int, bitrate uint32) (*InteractiveWindowsScreenPipeline, error) {
	if session == nil {
		return nil, errors.New("agent session is required")
	}
	cfg, _, err := (WindowsCaptureHelperConfig{EncoderDLLPath: encoderPath, EncoderDLLSHA256: encoderSHA256, FPS: fps, Bitrate: bitrate}).normalized()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve remote helper executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil || !filepath.IsAbs(exe) {
		return nil, errors.New("remote helper executable path is not absolute")
	}
	if !strings.EqualFold(filepath.Base(exe), remoteCaptureHelperBase) {
		return nil, fmt.Errorf("interactive capture helper basename must be %s", remoteCaptureHelperBase)
	}
	sum, err := fileSHA256(exe)
	if err != nil {
		return nil, fmt.Errorf("hash remote helper executable: %w", err)
	}
	decoded, err := hex.DecodeString(sum)
	if err != nil || len(decoded) != sha256.Size {
		return nil, errors.New("remote helper executable hash is invalid")
	}
	var pin [sha256.Size]byte
	copy(pin[:], decoded)
	return &InteractiveWindowsScreenPipeline{
		session: session,
		helperPath: exe,
		helperSHA: pin,
		encoderPath: cfg.EncoderDLLPath,
		encoderSHA256: cfg.EncoderDLLSHA256,
		fps: cfg.FPS,
		bitrate: cfg.Bitrate,
	}, nil
}

func (p *InteractiveWindowsScreenPipeline) verifyPinnedSelf() error {
	if p == nil || !filepath.IsAbs(p.helperPath) {
		return errors.New("interactive capture pipeline is not initialized")
	}
	data, err := os.ReadFile(p.helperPath)
	if err != nil {
		return fmt.Errorf("read interactive capture helper: %w", err)
	}
	got := sha256.Sum256(data)
	if !equalBytes(got[:], p.helperSHA[:]) {
		return errors.New("interactive capture helper SHA-256 changed after startup")
	}
	return nil
}

func (p *InteractiveWindowsScreenPipeline) launch() (*interactiveProcess, *os.File, error) {
	if err := p.verifyPinnedSelf(); err != nil {
		return nil, nil, err
	}
	activeRaw, _, _ := remoteWTSGetActiveConsoleSessionID.Call()
	sessionID := uint32(activeRaw)
	if sessionID == 0xFFFFFFFF {
		return nil, nil, errors.New("no active interactive console session for screen capture")
	}

	var token syscall.Handle
	ret, _, callErr := remoteWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if ret == 0 || token == 0 {
		return nil, nil, fmt.Errorf("WTSQueryUserToken failed for capture session %d: %v", sessionID, callErr)
	}
	defer remoteCloseHandle.Call(uintptr(token))

	var readPipe, writePipe syscall.Handle
	sa := remoteSecurityAttributes{Length: uint32(unsafe.Sizeof(remoteSecurityAttributes{})), InheritHandle: 1}
	ret, _, callErr = remoteCreatePipe.Call(
		uintptr(unsafe.Pointer(&readPipe)),
		uintptr(unsafe.Pointer(&writePipe)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if ret == 0 || readPipe == 0 || writePipe == 0 {
		return nil, nil, fmt.Errorf("CreatePipe for capture helper failed: %v", callErr)
	}
	cleanupRead, cleanupWrite := true, true
	defer func() {
		if cleanupRead { remoteCloseHandle.Call(uintptr(readPipe)) }
		if cleanupWrite { remoteCloseHandle.Call(uintptr(writePipe)) }
	}()
	ret, _, callErr = remoteSetHandleInformation.Call(uintptr(readPipe), remoteHandleFlagInherit, 0)
	if ret == 0 {
		return nil, nil, fmt.Errorf("SetHandleInformation on capture read pipe failed: %v", callErr)
	}

	var attrSize uintptr
	remoteInitializeProcThreadAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 || attrSize > 1024*1024 {
		return nil, nil, errors.New("capture helper attribute-list size is invalid")
	}
	attrBuf := make([]byte, int(attrSize))
	attrPtr := uintptr(unsafe.Pointer(&attrBuf[0]))
	ret, _, callErr = remoteInitializeProcThreadAttributeList.Call(attrPtr, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if ret == 0 {
		return nil, nil, fmt.Errorf("InitializeProcThreadAttributeList failed: %v", callErr)
	}
	defer remoteDeleteProcThreadAttributeList.Call(attrPtr)

	handleValue := writePipe
	ret, _, callErr = remoteUpdateProcThreadAttribute.Call(
		attrPtr,
		0,
		remoteProcThreadAttributeHandleList,
		uintptr(unsafe.Pointer(&handleValue)),
		unsafe.Sizeof(handleValue),
		0,
		0,
	)
	if ret == 0 {
		return nil, nil, fmt.Errorf("UpdateProcThreadAttribute(handle-list) failed: %v", callErr)
	}

	var environment uintptr
	ret, _, callErr = remoteCreateEnvironmentBlock.Call(uintptr(unsafe.Pointer(&environment)), uintptr(token), 0)
	if ret == 0 || environment == 0 {
		return nil, nil, fmt.Errorf("CreateEnvironmentBlock for capture helper failed: %v", callErr)
	}
	defer remoteDestroyEnvironmentBlock.Call(environment)

	appPtr, err := syscall.UTF16PtrFromString(p.helperPath)
	if err != nil { return nil, nil, err }
	desktopPtr, _ := syscall.UTF16PtrFromString(`winsta0\default`)
	cwdPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(p.helperPath))
	encodedEncoderPath := base64.RawURLEncoding.EncodeToString([]byte(p.encoderPath))
	args := []string{
		remoteCaptureHelperFlag,
		"--pipe-handle=" + strconv.FormatUint(uint64(uintptr(writePipe)), 10),
		"--encoder-path-b64=" + encodedEncoderPath,
		"--encoder-sha256=" + strings.ToLower(strings.TrimSpace(p.encoderSHA256)),
		"--fps=" + strconv.Itoa(p.fps),
		"--bitrate=" + strconv.FormatUint(uint64(p.bitrate), 10),
	}
	parts := []string{quoteWindowsArgument(remoteCaptureHelperBase)}
	for _, arg := range args {
		q := quoteWindowsArgument(arg)
		if q == "" {
			return nil, nil, errors.New("capture helper argument contains forbidden characters")
		}
		parts = append(parts, q)
	}
	cmd16, err := syscall.UTF16FromString(strings.Join(parts, " "))
	if err != nil { return nil, nil, err }

	si := remoteStartupInfoEx{
		StartupInfo: remoteStartupInfo{
			Desktop: desktopPtr,
		},
		AttributeList: attrPtr,
	}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	var pi remoteProcessInformation
	ret, _, callErr = remoteCreateProcessAsUserW.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(&cmd16[0])),
		0,
		0,
		1,
		remoteCreateNoWindow|remoteExtendedStartupInfoPresent|remoteCreateUnicodeEnvironment,
		environment,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 || pi.Process == 0 {
		return nil, nil, fmt.Errorf("CreateProcessAsUserW capture helper failed: %v", callErr)
	}
	if pi.Thread != 0 {
		remoteCloseHandle.Call(uintptr(pi.Thread))
	}

	// The child is now the only inheritor of the write handle. Closing the
	// parent's copy ensures EOF reaches the reader when the helper exits.
	remoteCloseHandle.Call(uintptr(writePipe))
	cleanupWrite = false
	readFile := os.NewFile(uintptr(readPipe), "zhirox-remote-h264-read")
	if readFile == nil {
		proc := &interactiveProcess{process: pi.Process}
		_ = proc.stop(remoteCaptureHelperTerminateExit)
		proc.close()
		return nil, nil, errors.New("failed to wrap capture helper read pipe")
	}
	cleanupRead = false
	return &interactiveProcess{process: pi.Process}, readFile, nil
}

func (p *InteractiveWindowsScreenPipeline) Run(ctx context.Context) error {
	if p == nil || p.session == nil || ctx == nil {
		return errors.New("interactive Windows screen pipeline is not initialized")
	}
	proc, reader, err := p.launch()
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		reader.Close()
		_ = proc.stop(remoteCaptureHelperTerminateExit)
		proc.close()
		return errors.New("interactive Windows screen pipeline was closed before launch completed")
	}
	p.proc = proc
	p.readFile = reader
	p.mu.Unlock()

	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Close()
		case <-watchDone:
		}
	}()
	defer close(watchDone)

	header, err := ReadCapturePipeHeader(reader)
	if err != nil {
		if ctx.Err() != nil { return nil }
		return err
	}
	if int(header.FPS) != p.fps {
		return fmt.Errorf("interactive capture helper FPS mismatch: got %d want %d", header.FPS, p.fps)
	}
	frameDuration := time.Second / time.Duration(header.FPS)
	for {
		encoded, err := ReadH264AccessUnitFrame(reader)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				if ctx.Err() != nil { return nil }
			}
			return fmt.Errorf("interactive capture helper stream ended: %w", err)
		}
		if err := p.session.WriteH264AccessUnit(encoded, frameDuration); err != nil {
			return fmt.Errorf("publish interactive H264 frame: %w", err)
		}
	}
}

func (p *InteractiveWindowsScreenPipeline) Close() error {
	if p == nil { return nil }
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	reader := p.readFile
	proc := p.proc
	p.readFile = nil
	p.proc = nil
	p.mu.Unlock()

	if reader != nil { _ = reader.Close() }
	if proc != nil {
		_ = proc.stop(remoteCaptureHelperTerminateExit)
		proc.close()
	}
	return nil
}
