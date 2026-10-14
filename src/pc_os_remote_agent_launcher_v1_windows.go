//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	pcOSRemoteBridgeMaxBootstrapBytes           = 64 * 1024
	pcOSRemoteBridgeDefaultFPS                  = 15
	pcOSRemoteBridgeMinFPS                      = 5
	pcOSRemoteBridgeMaxFPS                      = 30
	pcOSRemoteBridgeDefaultBitrate       uint32 = 4_000_000
	pcOSRemoteBridgeMinBitrate           uint32 = 250_000
	pcOSRemoteBridgeMaxBitrate           uint32 = 20_000_000
	pcOSRemoteBridgeHandleFlagInherit            = 0x00000001
	pcOSRemoteBridgeStartFUseStdHandles           = 0x00000100
	pcOSRemoteBridgeExtendedStartupInfo           = 0x00080000
	pcOSRemoteBridgeCreateUnicodeEnv              = 0x00000400
	pcOSRemoteBridgeCreateNoWindow                = 0x08000000
	pcOSRemoteBridgeProcHandleList                = 0x00020002
	pcOSRemoteBridgeWaitObject0                   = 0x00000000
	pcOSRemoteBridgeWaitTimeout                   = 0x00000102
	pcOSRemoteBridgeInfinite                      = 0xffffffff
	pcOSRemoteBridgeTerminateExit          uint32 = 31
)

var (
	pcOSRemoteBridgeKernel32                    = syscall.NewLazyDLL("kernel32.dll")
	pcOSRemoteBridgeCreatePipe                  = pcOSRemoteBridgeKernel32.NewProc("CreatePipe")
	pcOSRemoteBridgeSetHandleInformation        = pcOSRemoteBridgeKernel32.NewProc("SetHandleInformation")
	pcOSRemoteBridgeInitializeAttributeList     = pcOSRemoteBridgeKernel32.NewProc("InitializeProcThreadAttributeList")
	pcOSRemoteBridgeUpdateAttribute             = pcOSRemoteBridgeKernel32.NewProc("UpdateProcThreadAttribute")
	pcOSRemoteBridgeDeleteAttributeList         = pcOSRemoteBridgeKernel32.NewProc("DeleteProcThreadAttributeList")
	pcOSRemoteBridgeCreateProcessW              = pcOSRemoteBridgeKernel32.NewProc("CreateProcessW")
	pcOSRemoteBridgeWaitForSingleObject         = pcOSRemoteBridgeKernel32.NewProc("WaitForSingleObject")
	pcOSRemoteBridgeGetExitCodeProcess          = pcOSRemoteBridgeKernel32.NewProc("GetExitCodeProcess")
	pcOSRemoteBridgeTerminateProcess            = pcOSRemoteBridgeKernel32.NewProc("TerminateProcess")
	pcOSRemoteBridgeCloseHandle                 = pcOSRemoteBridgeKernel32.NewProc("CloseHandle")
)

type pcOSRemoteBridgeSecurityAttributes struct {
	Length             uint32
	SecurityDescriptor uintptr
	InheritHandle      int32
}

type pcOSRemoteBridgeStartupInfo struct {
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

type pcOSRemoteBridgeStartupInfoEx struct {
	StartupInfo   pcOSRemoteBridgeStartupInfo
	AttributeList uintptr
}

type pcOSRemoteBridgeProcessInfo struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessID uint32
	ThreadID  uint32
}

type pcOSRemoteBridgeICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type pcOSRemoteBridgeBootstrap struct {
	SignalURL   string                      `json:"signal_url"`
	AgentID     string                      `json:"agent_id"`
	AgentSecret string                      `json:"agent_secret"`
	ICEServers  []pcOSRemoteBridgeICEServer `json:"ice_servers,omitempty"`
	RelayOnly   bool                        `json:"relay_only,omitempty"`
}

type pcOSRemoteAgentLaunchSpec struct {
	SignalURL        string
	AgentPath        string
	AgentSHA256      string
	ConsentPath      string
	ConsentSHA256    string
	EncoderPath      string
	EncoderSHA256    string
	ICEServers       []pcOSRemoteBridgeICEServer
	RelayOnly        bool
	FPS              int
	Bitrate          uint32
}

type pcOSRemoteAgentProcess struct {
	handle syscall.Handle
	pid    uint32
}

func pcOSLaunchRemoteScreenAgent(spec pcOSRemoteAgentLaunchSpec) (*pcOSRemoteAgentProcess, error) {
	creds, err := pcOSLoadRelayCredentials()
	if err != nil {
		return nil, errors.New("remote screen agent credentials are unavailable")
	}
	return pcOSStartRemoteScreenAgent(spec, creds)
}

func pcOSStartRemoteScreenAgent(spec pcOSRemoteAgentLaunchSpec, creds pcOSRelayCredentials) (*pcOSRemoteAgentProcess, error) {
	normalized, err := pcOSRemoteNormalizeLaunchSpec(spec)
	if err != nil {
		return nil, err
	}
	payload, err := pcOSRemoteBuildBootstrapPayload(normalized, creds)
	if err != nil {
		return nil, err
	}
	defer pcOSRemoteZeroBytes(payload)

	if err := pcOSRemoteVerifyLaunchArtifacts(normalized); err != nil {
		return nil, err
	}
	environment, err := pcOSRemoteBuildMinimalEnvironment(normalized)
	if err != nil {
		return nil, err
	}
	if len(environment) == 0 {
		return nil, errors.New("remote screen agent environment is empty")
	}

	var readPipe, writePipe syscall.Handle
	sa := pcOSRemoteBridgeSecurityAttributes{
		Length:        uint32(unsafe.Sizeof(pcOSRemoteBridgeSecurityAttributes{})),
		InheritHandle: 1,
	}
	ret, _, callErr := pcOSRemoteBridgeCreatePipe.Call(
		uintptr(unsafe.Pointer(&readPipe)),
		uintptr(unsafe.Pointer(&writePipe)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if ret == 0 || readPipe == 0 || writePipe == 0 {
		return nil, fmt.Errorf("create secure bootstrap pipe: %v", callErr)
	}
	cleanupRead, cleanupWrite := true, true
	defer func() {
		if cleanupRead {
			pcOSRemoteBridgeCloseHandle.Call(uintptr(readPipe))
		}
		if cleanupWrite {
			pcOSRemoteBridgeCloseHandle.Call(uintptr(writePipe))
		}
	}()
	ret, _, callErr = pcOSRemoteBridgeSetHandleInformation.Call(
		uintptr(writePipe), pcOSRemoteBridgeHandleFlagInherit, 0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("protect secure bootstrap writer: %v", callErr)
	}

	nulFile, err := os.OpenFile("NUL", os.O_WRONLY, 0)
	if err != nil {
		return nil, errors.New("open remote agent output sink")
	}
	defer nulFile.Close()
	nulHandle := syscall.Handle(nulFile.Fd())
	ret, _, callErr = pcOSRemoteBridgeSetHandleInformation.Call(
		uintptr(nulHandle), pcOSRemoteBridgeHandleFlagInherit, pcOSRemoteBridgeHandleFlagInherit,
	)
	if ret == 0 {
		return nil, fmt.Errorf("prepare remote agent output sink: %v", callErr)
	}
	defer pcOSRemoteBridgeSetHandleInformation.Call(uintptr(nulHandle), pcOSRemoteBridgeHandleFlagInherit, 0)

	var attrSize uintptr
	pcOSRemoteBridgeInitializeAttributeList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 || attrSize > 1024*1024 {
		return nil, errors.New("remote agent process attribute-list size is invalid")
	}
	attrBuf := make([]byte, int(attrSize))
	attrPtr := uintptr(unsafe.Pointer(&attrBuf[0]))
	ret, _, callErr = pcOSRemoteBridgeInitializeAttributeList.Call(
		attrPtr, 1, 0, uintptr(unsafe.Pointer(&attrSize)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("initialize remote agent handle allowlist: %v", callErr)
	}
	defer pcOSRemoteBridgeDeleteAttributeList.Call(attrPtr)

	// CreateProcess requires standard output/error handles when STARTF_USESTDHANDLES
	// is used. The explicit handle list therefore contains exactly the secret-bearing
	// bootstrap reader plus a single NUL sink handle. No ambient service handle is
	// inherited by the child.
	handles := []syscall.Handle{readPipe, nulHandle}
	ret, _, callErr = pcOSRemoteBridgeUpdateAttribute.Call(
		attrPtr,
		0,
		pcOSRemoteBridgeProcHandleList,
		uintptr(unsafe.Pointer(&handles[0])),
		uintptr(len(handles))*unsafe.Sizeof(handles[0]),
		0,
		0,
	)
	if ret == 0 {
		return nil, fmt.Errorf("set remote agent inherited-handle allowlist: %v", callErr)
	}

	appPtr, err := syscall.UTF16PtrFromString(normalized.AgentPath)
	if err != nil {
		return nil, errors.New("remote screen agent path contains invalid characters")
	}
	cmdLine := `"` + filepath.Base(normalized.AgentPath) + `"`
	cmd16, err := syscall.UTF16FromString(cmdLine)
	if err != nil {
		return nil, errors.New("remote screen agent command line is invalid")
	}
	cwdPtr, err := syscall.UTF16PtrFromString(filepath.Dir(normalized.AgentPath))
	if err != nil {
		return nil, errors.New("remote screen agent working directory is invalid")
	}

	si := pcOSRemoteBridgeStartupInfoEx{
		StartupInfo: pcOSRemoteBridgeStartupInfo{
			Flags:     pcOSRemoteBridgeStartFUseStdHandles,
			StdInput:  readPipe,
			StdOutput: nulHandle,
			StdError:  nulHandle,
		},
		AttributeList: attrPtr,
	}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	var pi pcOSRemoteBridgeProcessInfo

	// Verify the exact binaries again immediately before process creation. This
	// narrows the path-swap window and fails closed if any candidate component
	// changed between validation and launch.
	if err := pcOSRemoteVerifyLaunchArtifacts(normalized); err != nil {
		return nil, err
	}
	ret, _, callErr = pcOSRemoteBridgeCreateProcessW.Call(
		uintptr(unsafe.Pointer(appPtr)),
		uintptr(unsafe.Pointer(&cmd16[0])),
		0,
		0,
		1,
		pcOSRemoteBridgeCreateNoWindow|pcOSRemoteBridgeExtendedStartupInfo|pcOSRemoteBridgeCreateUnicodeEnv,
		uintptr(unsafe.Pointer(&environment[0])),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if ret == 0 || pi.Process == 0 {
		return nil, fmt.Errorf("start remote screen agent: %v", callErr)
	}
	if pi.Thread != 0 {
		pcOSRemoteBridgeCloseHandle.Call(uintptr(pi.Thread))
	}

	// The parent must close its read end before writing; only the child owns the
	// allowed inherited read handle from this point forward.
	pcOSRemoteBridgeCloseHandle.Call(uintptr(readPipe))
	cleanupRead = false
	writer := os.NewFile(uintptr(writePipe), "zhirox-remote-bootstrap-write")
	if writer == nil {
		pcOSRemoteBridgeTerminateProcess.Call(uintptr(pi.Process), uintptr(pcOSRemoteBridgeTerminateExit))
		pcOSRemoteBridgeCloseHandle.Call(uintptr(pi.Process))
		return nil, errors.New("wrap secure bootstrap writer")
	}
	cleanupWrite = false
	written, writeErr := writer.Write(payload)
	closeErr := writer.Close()
	if writeErr != nil || closeErr != nil || written != len(payload) {
		pcOSRemoteBridgeTerminateProcess.Call(uintptr(pi.Process), uintptr(pcOSRemoteBridgeTerminateExit))
		pcOSRemoteBridgeCloseHandle.Call(uintptr(pi.Process))
		return nil, errors.New("deliver secure bootstrap payload")
	}

	return &pcOSRemoteAgentProcess{handle: pi.Process, pid: pi.ProcessID}, nil
}

func pcOSRemoteNormalizeLaunchSpec(spec pcOSRemoteAgentLaunchSpec) (pcOSRemoteAgentLaunchSpec, error) {
	u, err := url.Parse(strings.TrimSpace(spec.SignalURL))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return spec, errors.New("remote signaling URL must be an absolute HTTPS URL")
	}
	if !strings.Contains(strings.TrimRight(u.Path, "/"), "/functions/v1/zhirox-pc-remote-signaling-v1") {
		return spec, errors.New("remote signaling URL does not target the approved signaling function")
	}
	spec.SignalURL = strings.TrimRight(u.String(), "/")

	var errPath error
	if spec.AgentPath, errPath = pcOSRemoteNormalizePinnedPath(spec.AgentPath, "ZhiroxRemoteScreenAgent.exe"); errPath != nil {
		return spec, errPath
	}
	if spec.ConsentPath, errPath = pcOSRemoteNormalizePinnedPath(spec.ConsentPath, "ZhiroxRemoteConsent.exe"); errPath != nil {
		return spec, errPath
	}
	if spec.EncoderPath, errPath = pcOSRemoteNormalizePinnedPath(spec.EncoderPath, "ZhiroxMFH264Encoder.dll"); errPath != nil {
		return spec, errPath
	}
	for _, pin := range []*string{&spec.AgentSHA256, &spec.ConsentSHA256, &spec.EncoderSHA256} {
		*pin = strings.ToLower(strings.TrimSpace(*pin))
		decoded, err := hex.DecodeString(*pin)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != *pin {
			return spec, errors.New("remote component SHA-256 pin is invalid")
		}
	}
	if spec.FPS == 0 {
		spec.FPS = pcOSRemoteBridgeDefaultFPS
	}
	if spec.FPS < pcOSRemoteBridgeMinFPS || spec.FPS > pcOSRemoteBridgeMaxFPS {
		return spec, fmt.Errorf("remote FPS must be between %d and %d", pcOSRemoteBridgeMinFPS, pcOSRemoteBridgeMaxFPS)
	}
	if spec.Bitrate == 0 {
		spec.Bitrate = pcOSRemoteBridgeDefaultBitrate
	}
	if spec.Bitrate < pcOSRemoteBridgeMinBitrate || spec.Bitrate > pcOSRemoteBridgeMaxBitrate {
		return spec, errors.New("remote bitrate is outside the approved range")
	}
	if len(spec.ICEServers) > 8 {
		return spec, errors.New("remote launch allows at most 8 ICE servers")
	}
	turnPresent := false
	for i := range spec.ICEServers {
		server := &spec.ICEServers[i]
		if len(server.URLs) == 0 || len(server.URLs) > 8 || len(server.Username) > 1024 || len(server.Credential) > 4096 {
			return spec, errors.New("remote ICE server configuration is outside safety bounds")
		}
		for j := range server.URLs {
			raw := strings.TrimSpace(server.URLs[j])
			lower := strings.ToLower(raw)
			if !(strings.HasPrefix(lower, "stun:") || strings.HasPrefix(lower, "stuns:") || strings.HasPrefix(lower, "turn:") || strings.HasPrefix(lower, "turns:")) {
				return spec, errors.New("remote ICE URL scheme is not allowed")
			}
			if strings.HasPrefix(lower, "turn:") || strings.HasPrefix(lower, "turns:") {
				turnPresent = true
			}
			server.URLs[j] = raw
		}
	}
	if spec.RelayOnly && !turnPresent {
		return spec, errors.New("relay-only remote launch requires a TURN server")
	}
	return spec, nil
}

func pcOSRemoteNormalizePinnedPath(raw, requiredBase string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("%s path is required", requiredBase)
	}
	abs, err := filepath.Abs(path)
	if err != nil || !filepath.IsAbs(abs) {
		return "", fmt.Errorf("%s path must be absolute", requiredBase)
	}
	if !strings.EqualFold(filepath.Base(abs), requiredBase) {
		return "", fmt.Errorf("remote component basename must be %s", requiredBase)
	}
	return filepath.Clean(abs), nil
}

func pcOSRemoteBuildBootstrapPayload(spec pcOSRemoteAgentLaunchSpec, creds pcOSRelayCredentials) ([]byte, error) {
	agentID := strings.TrimSpace(creds.AgentID)
	secret := strings.TrimSpace(creds.Secret)
	if agentID == "" || len(agentID) > 160 || secret == "" || len(secret) > 4096 {
		return nil, errors.New("remote relay credentials are invalid")
	}
	payload, err := json.Marshal(pcOSRemoteBridgeBootstrap{
		SignalURL:   spec.SignalURL,
		AgentID:     agentID,
		AgentSecret: secret,
		ICEServers:  append([]pcOSRemoteBridgeICEServer(nil), spec.ICEServers...),
		RelayOnly:   spec.RelayOnly,
	})
	if err != nil {
		return nil, errors.New("encode secure remote bootstrap")
	}
	if len(payload) == 0 || len(payload) > pcOSRemoteBridgeMaxBootstrapBytes {
		pcOSRemoteZeroBytes(payload)
		return nil, errors.New("secure remote bootstrap payload exceeds safety bounds")
	}
	return payload, nil
}

func pcOSRemoteVerifyLaunchArtifacts(spec pcOSRemoteAgentLaunchSpec) error {
	checks := []struct {
		path string
		pin  string
		name string
	}{
		{spec.AgentPath, spec.AgentSHA256, "remote screen agent"},
		{spec.ConsentPath, spec.ConsentSHA256, "remote consent helper"},
		{spec.EncoderPath, spec.EncoderSHA256, "remote H264 encoder"},
	}
	for _, check := range checks {
		f, err := os.Open(check.path)
		if err != nil {
			return fmt.Errorf("open %s: %w", check.name, err)
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			return fmt.Errorf("hash %s", check.name)
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), check.pin) {
			return fmt.Errorf("%s SHA-256 mismatch", check.name)
		}
	}
	return nil
}

func pcOSRemoteBuildMinimalEnvironment(spec pcOSRemoteAgentLaunchSpec) ([]uint16, error) {
	values := map[string]string{}
	for _, key := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			values[key] = value
		}
	}
	if values["SystemRoot"] == "" && values["WINDIR"] == "" {
		return nil, errors.New("Windows root environment is unavailable")
	}
	values["ZHIROX_REMOTE_BOOTSTRAP_STDIN"] = "1"
	values["ZHIROX_REMOTE_CONSENT_PATH"] = spec.ConsentPath
	values["ZHIROX_REMOTE_CONSENT_SHA256"] = spec.ConsentSHA256
	values["ZHIROX_REMOTE_ENCODER_PATH"] = spec.EncoderPath
	values["ZHIROX_REMOTE_ENCODER_SHA256"] = spec.EncoderSHA256
	values["ZHIROX_REMOTE_FPS"] = strconv.Itoa(spec.FPS)
	values["ZHIROX_REMOTE_BITRATE"] = strconv.FormatUint(uint64(spec.Bitrate), 10)

	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("remote agent environment contains forbidden characters")
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return strings.ToUpper(keys[i]) < strings.ToUpper(keys[j]) })
	var block strings.Builder
	for _, key := range keys {
		block.WriteString(key)
		block.WriteByte('=')
		block.WriteString(values[key])
		block.WriteByte(0)
	}
	block.WriteByte(0)
	return utf16.Encode([]rune(block.String())), nil
}

func pcOSRemoteZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func (p *pcOSRemoteAgentProcess) Wait(timeout time.Duration) (uint32, error) {
	if p == nil || p.handle == 0 {
		return 0, errors.New("remote agent process is not running")
	}
	waitMS := uintptr(pcOSRemoteBridgeInfinite)
	if timeout >= 0 {
		if timeout > time.Duration(^uint32(0)-1)*time.Millisecond {
			return 0, errors.New("remote agent wait timeout is too large")
		}
		waitMS = uintptr(uint32(timeout / time.Millisecond))
	}
	ret, _, callErr := pcOSRemoteBridgeWaitForSingleObject.Call(uintptr(p.handle), waitMS)
	switch ret {
	case pcOSRemoteBridgeWaitObject0:
		var exitCode uint32
		ok, _, exitErr := pcOSRemoteBridgeGetExitCodeProcess.Call(uintptr(p.handle), uintptr(unsafe.Pointer(&exitCode)))
		if ok == 0 {
			return 0, fmt.Errorf("read remote agent exit code: %v", exitErr)
		}
		return exitCode, nil
	case pcOSRemoteBridgeWaitTimeout:
		return 0, errors.New("remote agent process wait timed out")
	default:
		return 0, fmt.Errorf("wait for remote agent process: %v", callErr)
	}
}

func (p *pcOSRemoteAgentProcess) Stop() error {
	if p == nil || p.handle == 0 {
		return nil
	}
	ret, _, callErr := pcOSRemoteBridgeTerminateProcess.Call(uintptr(p.handle), uintptr(pcOSRemoteBridgeTerminateExit))
	if ret == 0 {
		return fmt.Errorf("stop remote agent process: %v", callErr)
	}
	return nil
}

func (p *pcOSRemoteAgentProcess) Close() {
	if p == nil || p.handle == 0 {
		return
	}
	pcOSRemoteBridgeCloseHandle.Call(uintptr(p.handle))
	p.handle = 0
	p.pid = 0
}
