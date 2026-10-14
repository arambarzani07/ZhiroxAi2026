//go:build windows

package remoteagent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	remoteConsentPairingCancel          = 30
	remotePairHandleFlagInherit         = 0x00000001
	remotePairStartFUseStdHandles       = 0x00000100
	remotePairExtendedStartupInfo       = 0x00080000
	remotePairProcThreadAttributeHandles = 0x00020002
	remotePairTerminateExit      uint32 = 33
)

var (
	remotePairCreatePipe              = remoteKernel32.NewProc("CreatePipe")
	remotePairSetHandleInformation    = remoteKernel32.NewProc("SetHandleInformation")
	remotePairInitializeAttributeList = remoteKernel32.NewProc("InitializeProcThreadAttributeList")
	remotePairUpdateAttribute         = remoteKernel32.NewProc("UpdateProcThreadAttribute")
	remotePairDeleteAttributeList     = remoteKernel32.NewProc("DeleteProcThreadAttributeList")
)

type remotePairSecurityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      int32
}

type remotePairStartupInfoEx struct {
	StartupInfo   remoteStartupInfo
	AttributeList uintptr
}

// StartPairing displays the one-time canary pairing code on the attended target
// desktop. The code itself is delivered only over an inherited anonymous stdin
// pipe. It never appears in the child command line or environment.
func (l *ConsentLauncher) StartPairing(pairingCode string) (SessionIndicator, error) {
	if _, err := NormalizeCanaryPairingCode(pairingCode); err != nil {
		return nil, err
	}
	proc, err := l.startInteractivePairing(pairingCode)
	if err != nil {
		return nil, err
	}
	indicator := &windowsSessionIndicator{proc: proc, done: make(chan error, 1)}
	go func() {
		defer proc.close()
		defer close(indicator.done)
		exitCode, _, waitErr := proc.wait(remoteInfinite)
		if waitErr != nil {
			indicator.done <- waitErr
			return
		}
		switch exitCode {
		case remoteConsentPairingCancel, remoteConsentAgentStop:
			indicator.done <- nil
		default:
			indicator.done <- fmt.Errorf("consent helper pairing display exited with code %d", exitCode)
		}
	}()
	return indicator, nil
}

func zeroPairingBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func (l *ConsentLauncher) startInteractivePairing(pairingCode string) (*interactiveProcess, error) {
	if err := l.verifyPinnedHelper(); err != nil {
		return nil, err
	}
	if _, err := NormalizeCanaryPairingCode(pairingCode); err != nil {
		return nil, err
	}

	activeRaw, _, _ := remoteWTSGetActiveConsoleSessionID.Call()
	sessionID := uint32(activeRaw)
	if sessionID == 0xFFFFFFFF {
		return nil, errors.New("no active interactive console session for pairing")
	}
	var token syscall.Handle
	ret, _, callErr := remoteWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if ret == 0 || token == 0 {
		return nil, fmt.Errorf("WTSQueryUserToken failed for pairing session %d: %v", sessionID, callErr)
	}
	defer remoteCloseHandle.Call(uintptr(token))

	var readPipe, writePipe syscall.Handle
	sa := remotePairSecurityAttributes{
		Length:        uint32(unsafe.Sizeof(remotePairSecurityAttributes{})),
		InheritHandle: 1,
	}
	ret, _, callErr = remotePairCreatePipe.Call(
		uintptr(unsafe.Pointer(&readPipe)),
		uintptr(unsafe.Pointer(&writePipe)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if ret == 0 || readPipe == 0 || writePipe == 0 {
		return nil, fmt.Errorf("create pairing stdin pipe: %v", callErr)
	}
	readOpen, writeOpen := true, true
	defer func() {
		if readOpen {
			remoteCloseHandle.Call(uintptr(readPipe))
		}
		if writeOpen {
			remoteCloseHandle.Call(uintptr(writePipe))
		}
	}()
	ret, _, callErr = remotePairSetHandleInformation.Call(
		uintptr(writePipe), remotePairHandleFlagInherit, 0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("protect pairing stdin writer: %v", callErr)
	}

	nulFile, err := os.OpenFile("NUL", os.O_WRONLY, 0)
	if err != nil {
		return nil, errors.New("open pairing output sink")
	}
	defer nulFile.Close()
	nulHandle := syscall.Handle(nulFile.Fd())
	ret, _, callErr = remotePairSetHandleInformation.Call(
		uintptr(nulHandle), remotePairHandleFlagInherit, remotePairHandleFlagInherit,
	)
	if ret == 0 {
		return nil, fmt.Errorf("prepare pairing output sink: %v", callErr)
	}
	defer remotePairSetHandleInformation.Call(uintptr(nulHandle), remotePairHandleFlagInherit, 0)

	var attrSize uintptr
	remotePairInitializeAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 || attrSize > 1024*1024 {
		return nil, errors.New("pairing process attribute-list size is invalid")
	}
	attrBuf := make([]byte, int(attrSize))
	attrPtr := uintptr(unsafe.Pointer(&attrBuf[0]))
	ret, _, callErr = remotePairInitializeAttributeList.Call(
		attrPtr, 1, 0, uintptr(unsafe.Pointer(&attrSize)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("initialize pairing handle allowlist: %v", callErr)
	}
	defer remotePairDeleteAttributeList.Call(attrPtr)

	handles := []syscall.Handle{readPipe, nulHandle}
	ret, _, callErr = remotePairUpdateAttribute.Call(
		attrPtr,
		0,
		remotePairProcThreadAttributeHandles,
		uintptr(unsafe.Pointer(&handles[0])),
		uintptr(len(handles))*unsafe.Sizeof(handles[0]),
		0,
		0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("set pairing inherited-handle allowlist: %v", callErr)
	}

	appPtr, err := syscall.UTF16PtrFromString(l.helperPath)
	if err != nil {
		return nil, errors.New("consent helper path contains invalid characters")
	}
	parts := []string{
		quoteWindowsArgument(remoteConsentHelperBase),
		quoteWindowsArgument("--mode=pairing"),
		quoteWindowsArgument("--pairing-stdin=1"),
	}
	for _, part := range parts {
		if part == "" {
			return nil, errors.New("pairing helper command line is invalid")
		}
	}
	cmdPtr, err := syscall.UTF16PtrFromString(strings.Join(parts, " "))
	if err != nil {
		return nil, errors.New("pairing helper command line contains invalid characters")
	}
	desktopPtr, _ := syscall.UTF16PtrFromString(`winsta0\default`)
	cwdPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(l.helperPath))
	si := remotePairStartupInfoEx{
		StartupInfo: remoteStartupInfo{
			Desktop:   desktopPtr,
			Flags:     remotePairStartFUseStdHandles,
			StdInput:  readPipe,
			StdOutput: nulHandle,
			StdError:  nulHandle,
		},
		AttributeList: attrPtr,
	}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	var pi remoteProcessInformation

	if err := l.verifyPinnedHelper(); err != nil {
		return nil, err
	}
	ret, _, callErr = remoteCreateProcessAsUserW.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0,
		0,
		1,
		remoteCreateNoWindow|remotePairExtendedStartupInfo,
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 || pi.Process == 0 {
		return nil, fmt.Errorf("CreateProcessAsUserW pairing helper failed: %v", callErr)
	}
	if pi.Thread != 0 {
		remoteCloseHandle.Call(uintptr(pi.Thread))
	}

	remoteCloseHandle.Call(uintptr(readPipe))
	readOpen = false
	writer := os.NewFile(uintptr(writePipe), "zhirox-pairing-stdin-write")
	if writer == nil {
		remoteTerminateProcess.Call(uintptr(pi.Process), uintptr(remotePairTerminateExit))
		remoteCloseHandle.Call(uintptr(pi.Process))
		return nil, errors.New("wrap pairing stdin writer")
	}
	writeOpen = false
	payload := []byte(pairingCode + "\n")
	defer zeroPairingBytes(payload)
	written, writeErr := writer.Write(payload)
	closeErr := writer.Close()
	if writeErr != nil || closeErr != nil || written != len(payload) {
		remoteTerminateProcess.Call(uintptr(pi.Process), uintptr(remotePairTerminateExit))
		remoteCloseHandle.Call(uintptr(pi.Process))
		return nil, errors.New("deliver pairing code through secure stdin")
	}

	return &interactiveProcess{process: pi.Process}, nil
}
