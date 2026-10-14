//go:build windows

package remoteagent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	remoteCreateNoWindow       = 0x08000000
	remoteWaitObject0          = 0x00000000
	remoteWaitTimeout          = 0x00000102
	remoteInfinite             = 0xFFFFFFFF
	remoteStillActive          = 259
	remoteConsentApproveYes    = 0
	remoteConsentApproveNo     = 10
	remoteConsentDisconnect    = 20
	remoteConsentAgentStop     = 21
	remoteConsentApprovalTTLMS = 2 * 60 * 1000
	remoteConsentKillWaitMS    = 5000
	remoteMaxConsentArgBytes   = 512
	remoteConsentHelperBase    = "ZhiroxRemoteConsent.exe"
)

var (
	remoteKernel32                     = syscall.NewLazyDLL("kernel32.dll")
	remoteWTSAPI32                     = syscall.NewLazyDLL("wtsapi32.dll")
	remoteAdvapi32                     = syscall.NewLazyDLL("advapi32.dll")
	remoteWTSGetActiveConsoleSessionID = remoteKernel32.NewProc("WTSGetActiveConsoleSessionId")
	remoteCloseHandle                  = remoteKernel32.NewProc("CloseHandle")
	remoteWaitForSingleObject          = remoteKernel32.NewProc("WaitForSingleObject")
	remoteGetExitCodeProcess           = remoteKernel32.NewProc("GetExitCodeProcess")
	remoteTerminateProcess             = remoteKernel32.NewProc("TerminateProcess")
	remoteWTSQueryUserToken            = remoteWTSAPI32.NewProc("WTSQueryUserToken")
	remoteCreateProcessAsUserW         = remoteAdvapi32.NewProc("CreateProcessAsUserW")
)

type remoteStartupInfo struct {
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

type remoteProcessInformation struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessID uint32
	ThreadID  uint32
}

type ConsentLauncher struct {
	helperPath string
	helperSHA  [sha256.Size]byte
}

type interactiveProcess struct {
	mu      sync.Mutex
	process syscall.Handle
}

type windowsSessionIndicator struct {
	proc *interactiveProcess
	done chan error
}

func NewConsentLauncher(helperPath, expectedSHA256 string) (*ConsentLauncher, error) {
	if !filepath.IsAbs(helperPath) {
		return nil, errors.New("consent helper path must be absolute")
	}
	clean := filepath.Clean(helperPath)
	if !strings.EqualFold(filepath.Base(clean), remoteConsentHelperBase) {
		return nil, fmt.Errorf("consent helper basename must be %s", remoteConsentHelperBase)
	}
	if len(expectedSHA256) != 64 {
		return nil, errors.New("consent helper SHA-256 pin is required")
	}
	want, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(want) != sha256.Size {
		return nil, errors.New("consent helper SHA-256 pin is invalid")
	}
	var pin [sha256.Size]byte
	copy(pin[:], want)
	launcher := &ConsentLauncher{helperPath: clean, helperSHA: pin}
	if err := launcher.verifyPinnedHelper(); err != nil {
		return nil, err
	}
	return launcher, nil
}

func (l *ConsentLauncher) verifyPinnedHelper() error {
	if l == nil || l.helperPath == "" {
		return errors.New("consent launcher is not initialized")
	}
	b, err := os.ReadFile(l.helperPath)
	if err != nil {
		return fmt.Errorf("read consent helper: %w", err)
	}
	got := sha256.Sum256(b)
	if !equalBytes(got[:], l.helperSHA[:]) {
		return errors.New("consent helper SHA-256 mismatch")
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func encodeConsentArg(value string) (string, error) {
	if len(value) > remoteMaxConsentArgBytes {
		return "", errors.New("consent display argument exceeds approved size")
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value)), nil
}

func (p *interactiveProcess) wait(waitMS uint32) (uint32, bool, error) {
	if p == nil {
		return 0, false, errors.New("interactive process is nil")
	}
	p.mu.Lock()
	h := p.process
	p.mu.Unlock()
	if h == 0 {
		return 0, false, errors.New("interactive process handle is closed")
	}
	waitRet, _, waitErr := remoteWaitForSingleObject.Call(uintptr(h), uintptr(waitMS))
	if waitRet == remoteWaitTimeout {
		return 0, true, nil
	}
	if waitRet != remoteWaitObject0 {
		return 0, false, fmt.Errorf("WaitForSingleObject failed: %v", waitErr)
	}
	var exitCode uint32 = remoteStillActive
	ret, _, callErr := remoteGetExitCodeProcess.Call(uintptr(h), uintptr(unsafe.Pointer(&exitCode)))
	if ret == 0 {
		return 0, false, fmt.Errorf("GetExitCodeProcess failed: %v", callErr)
	}
	if exitCode == remoteStillActive {
		return 0, false, errors.New("interactive process remained active after wait completion")
	}
	return exitCode, false, nil
}

func (p *interactiveProcess) stop(exitCode uint32) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process == 0 {
		return nil
	}
	var current uint32 = remoteStillActive
	ret, _, callErr := remoteGetExitCodeProcess.Call(uintptr(p.process), uintptr(unsafe.Pointer(&current)))
	if ret == 0 {
		return fmt.Errorf("GetExitCodeProcess before stop failed: %v", callErr)
	}
	if current != remoteStillActive {
		return nil
	}
	ret, _, callErr = remoteTerminateProcess.Call(uintptr(p.process), uintptr(exitCode))
	if ret == 0 {
		return fmt.Errorf("TerminateProcess failed: %v", callErr)
	}
	return nil
}

func (p *interactiveProcess) close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process != 0 {
		remoteCloseHandle.Call(uintptr(p.process))
		p.process = 0
	}
}

func (i *windowsSessionIndicator) Done() <-chan error {
	if i == nil {
		return nil
	}
	return i.done
}

func (i *windowsSessionIndicator) Stop() error {
	if i == nil || i.proc == nil {
		return nil
	}
	return i.proc.stop(remoteConsentAgentStop)
}

func (l *ConsentLauncher) Approve(controllerLabel, controllerFingerprint string) (bool, error) {
	label, err := encodeConsentArg(controllerLabel)
	if err != nil {
		return false, err
	}
	fingerprint, err := encodeConsentArg(controllerFingerprint)
	if err != nil {
		return false, err
	}
	proc, err := l.startInteractive([]string{
		"--mode=approve",
		"--controller-label-b64=" + label,
		"--controller-fingerprint-b64=" + fingerprint,
	})
	if err != nil {
		return false, err
	}
	defer proc.close()
	exitCode, timedOut, err := proc.wait(remoteConsentApprovalTTLMS)
	if err != nil {
		return false, err
	}
	if timedOut {
		_ = proc.stop(124)
		_, _, _ = proc.wait(remoteConsentKillWaitMS)
		return false, errors.New("consent helper timed out after the 2-minute approval window")
	}
	switch exitCode {
	case remoteConsentApproveYes:
		return true, nil
	case remoteConsentApproveNo:
		return false, nil
	default:
		return false, fmt.Errorf("consent helper approval exited with code %d", exitCode)
	}
}

// StartIndicator launches the visible active-session indicator in the signed-in
// user's desktop. Stop terminates the indicator when the remote session ends so
// the target PC never keeps showing a stale "active" state.
func (l *ConsentLauncher) StartIndicator(controllerLabel string) (SessionIndicator, error) {
	label, err := encodeConsentArg(controllerLabel)
	if err != nil {
		return nil, err
	}
	proc, err := l.startInteractive([]string{
		"--mode=indicator",
		"--controller-label-b64=" + label,
	})
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
		case remoteConsentDisconnect, remoteConsentAgentStop:
			indicator.done <- nil
		default:
			indicator.done <- fmt.Errorf("consent helper indicator exited with code %d", exitCode)
		}
	}()
	return indicator, nil
}

func quoteWindowsArgument(arg string) string {
	// All orchestrator-generated arguments use a restricted base64url/value
	// alphabet and fixed prefixes. Reject characters that could change parsing.
	if strings.ContainsAny(arg, "\x00\r\n\t\"") {
		return ""
	}
	return `"` + arg + `"`
}

func (l *ConsentLauncher) startInteractive(args []string) (*interactiveProcess, error) {
	if err := l.verifyPinnedHelper(); err != nil {
		return nil, err
	}
	activeRaw, _, _ := remoteWTSGetActiveConsoleSessionID.Call()
	sessionID := uint32(activeRaw)
	if sessionID == 0xFFFFFFFF {
		return nil, errors.New("no active interactive console session")
	}
	var token syscall.Handle
	ret, _, callErr := remoteWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&token)))
	if ret == 0 || token == 0 {
		return nil, fmt.Errorf("WTSQueryUserToken failed for interactive session %d: %v", sessionID, callErr)
	}
	defer remoteCloseHandle.Call(uintptr(token))

	appPtr, err := syscall.UTF16PtrFromString(l.helperPath)
	if err != nil {
		return nil, err
	}
	parts := []string{quoteWindowsArgument(remoteConsentHelperBase)}
	for _, arg := range args {
		q := quoteWindowsArgument(arg)
		if q == "" {
			return nil, errors.New("consent helper argument contains forbidden characters")
		}
		parts = append(parts, q)
	}
	cmdPtr, err := syscall.UTF16PtrFromString(strings.Join(parts, " "))
	if err != nil {
		return nil, err
	}
	desktopPtr, _ := syscall.UTF16PtrFromString(`winsta0\default`)
	cwdPtr, _ := syscall.UTF16PtrFromString(filepath.Dir(l.helperPath))
	si := remoteStartupInfo{Cb: uint32(unsafe.Sizeof(remoteStartupInfo{})), Desktop: desktopPtr}
	var pi remoteProcessInformation
	ret, _, callErr = remoteCreateProcessAsUserW.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(cmdPtr)),
		0,
		0,
		0,
		remoteCreateNoWindow,
		0,
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CreateProcessAsUserW failed: %v", callErr)
	}
	if pi.Thread != 0 {
		remoteCloseHandle.Call(uintptr(pi.Thread))
	}
	return &interactiveProcess{process: pi.Process}, nil
}
