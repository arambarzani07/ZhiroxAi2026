//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	pcOSRemoteCanaryVersion   = "5.1.7"
	pcOSRemoteCanarySignalURL = "https://zhirox-remote-control-canary-aviglm.v2.appdeploy.ai"
	pcOSRemoteCanaryMaxRun    = 35 * time.Minute
)

var (
	pcOSRemoteCanaryMu        sync.Mutex
	pcOSRemoteCanaryProcess   *os.Process
	pcOSRemoteCanaryStartedAt time.Time
)

type pcOSRemoteCanaryBootstrap struct {
	Mode       string                      `json:"mode"`
	SignalURL  string                      `json:"signal_url"`
	AgentLabel string                      `json:"agent_label"`
	ICEServers []pcOSRemoteBridgeICEServer `json:"ice_servers,omitempty"`
	RelayOnly  bool                        `json:"relay_only,omitempty"`
}

type pcOSRemoteCanaryAsset struct {
	name string
	data []byte
	sha  string
}

func pcOSRemoteCanaryAssets() []pcOSRemoteCanaryAsset {
	return []pcOSRemoteCanaryAsset{
		{name: "ZhiroxRemoteScreenAgent.exe", data: pcOSRemoteCanaryAgentAsset, sha: pcOSRemoteCanaryAgentSHA256},
		{name: "ZhiroxRemoteConsent.exe", data: pcOSRemoteCanaryConsentAsset, sha: pcOSRemoteCanaryConsentSHA256},
		{name: "ZhiroxMFH264Encoder.dll", data: pcOSRemoteCanaryEncoderAsset, sha: pcOSRemoteCanaryEncoderSHA256},
	}
}

func pcOSRemoteCanaryValidateEmbeddedAssets() error {
	for _, asset := range pcOSRemoteCanaryAssets() {
		if len(asset.data) == 0 {
			return fmt.Errorf("embedded remote canary asset is empty: %s", asset.name)
		}
		pin := strings.ToLower(strings.TrimSpace(asset.sha))
		decoded, err := hex.DecodeString(pin)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != pin {
			return fmt.Errorf("embedded remote canary pin is invalid: %s", asset.name)
		}
		h := sha256.Sum256(asset.data)
		if hex.EncodeToString(h[:]) != pin {
			return fmt.Errorf("embedded remote canary hash mismatch: %s", asset.name)
		}
	}
	return nil
}

func pcOSRemoteCanaryRoot() string {
	return filepath.Join(pcOSActiveRoot(), "remote-canary", "v5.1.7")
}

func pcOSRemoteCanaryHashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func pcOSRemoteCanaryMaterializeAsset(root string, asset pcOSRemoteCanaryAsset) (string, error) {
	path := filepath.Join(root, asset.name)
	if got, err := pcOSRemoteCanaryHashFile(path); err == nil && strings.EqualFold(got, asset.sha) {
		return path, nil
	}

	tmp, err := os.CreateTemp(root, asset.name+".stage-*")
	if err != nil {
		return "", fmt.Errorf("stage remote canary asset %s", asset.name)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(asset.data); err != nil {
		return "", fmt.Errorf("write remote canary asset %s", asset.name)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("sync remote canary asset %s", asset.name)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close remote canary asset %s", asset.name)
	}
	got, err := pcOSRemoteCanaryHashFile(tmpPath)
	if err != nil || !strings.EqualFold(got, asset.sha) {
		return "", fmt.Errorf("staged remote canary asset failed SHA-256 verification: %s", asset.name)
	}
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("activate remote canary asset %s", asset.name)
	}
	cleanup = false
	got, err = pcOSRemoteCanaryHashFile(path)
	if err != nil || !strings.EqualFold(got, asset.sha) {
		return "", fmt.Errorf("active remote canary asset failed SHA-256 verification: %s", asset.name)
	}
	return path, nil
}

func pcOSRemoteCanaryPrepareAssets() (map[string]string, error) {
	if err := pcOSRemoteCanaryValidateEmbeddedAssets(); err != nil {
		return nil, err
	}
	root := pcOSRemoteCanaryRoot()
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, errors.New("create remote canary runtime directory")
	}
	paths := map[string]string{}
	for _, asset := range pcOSRemoteCanaryAssets() {
		path, err := pcOSRemoteCanaryMaterializeAsset(root, asset)
		if err != nil {
			return nil, err
		}
		paths[asset.name] = path
	}
	return paths, nil
}

func pcOSRemoteCanaryMinimalEnv(consentPath, encoderPath string) ([]string, error) {
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
	values["ZHIROX_REMOTE_CONSENT_PATH"] = consentPath
	values["ZHIROX_REMOTE_CONSENT_SHA256"] = pcOSRemoteCanaryConsentSHA256
	values["ZHIROX_REMOTE_ENCODER_PATH"] = encoderPath
	values["ZHIROX_REMOTE_ENCODER_SHA256"] = pcOSRemoteCanaryEncoderSHA256
	values["ZHIROX_REMOTE_FPS"] = "15"
	values["ZHIROX_REMOTE_BITRATE"] = "4000000"
	out := make([]string, 0, len(values))
	for key, value := range values {
		if strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsAny(value, "\x00\r\n") {
			return nil, errors.New("remote canary environment contains forbidden characters")
		}
		out = append(out, key+"="+value)
	}
	return out, nil
}

func pcOSRemoteCanaryIsRunningLocked() bool {
	if pcOSRemoteCanaryProcess == nil {
		return false
	}
	process, err := os.FindProcess(pcOSRemoteCanaryProcess.Pid)
	if err != nil || process == nil {
		pcOSRemoteCanaryProcess = nil
		return false
	}
	return true
}

func pcOSRemoteCanaryStart() (interface{}, error) {
	pcOSRemoteCanaryMu.Lock()
	defer pcOSRemoteCanaryMu.Unlock()
	if pcOSRemoteCanaryIsRunningLocked() {
		return nil, errors.New("remote canary is already running")
	}

	paths, err := pcOSRemoteCanaryPrepareAssets()
	if err != nil {
		return nil, err
	}
	agentPath := paths["ZhiroxRemoteScreenAgent.exe"]
	consentPath := paths["ZhiroxRemoteConsent.exe"]
	encoderPath := paths["ZhiroxMFH264Encoder.dll"]

	hostname, _ := os.Hostname()
	label := strings.TrimSpace(hostname)
	if label == "" {
		label = "Zhirox Canary Target"
	}
	if len(label) > 100 {
		label = label[:100]
	}
	bootstrap := pcOSRemoteCanaryBootstrap{
		Mode:       "canary",
		SignalURL:  pcOSRemoteCanarySignalURL,
		AgentLabel: label,
		ICEServers: []pcOSRemoteBridgeICEServer{{URLs: []string{"stun:stun.cloudflare.com:3478"}}},
		RelayOnly:  false,
	}
	payload, err := json.Marshal(bootstrap)
	if err != nil || len(payload) == 0 || len(payload) > pcOSRemoteBridgeMaxBootstrapBytes {
		return nil, errors.New("build remote canary bootstrap")
	}
	defer pcOSRemoteZeroBytes(payload)

	environment, err := pcOSRemoteCanaryMinimalEnv(consentPath, encoderPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(agentPath)
	cmd.Dir = filepath.Dir(agentPath)
	cmd.Env = environment
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return nil, errors.New("start attended remote canary")
	}
	pcOSRemoteCanaryProcess = cmd.Process
	pcOSRemoteCanaryStartedAt = time.Now().UTC()
	pid := cmd.Process.Pid
	startedAt := pcOSRemoteCanaryStartedAt

	go func(c *exec.Cmd, expectedPID int) {
		_ = c.Wait()
		pcOSRemoteCanaryMu.Lock()
		if pcOSRemoteCanaryProcess != nil && pcOSRemoteCanaryProcess.Pid == expectedPID {
			pcOSRemoteCanaryProcess = nil
			pcOSRemoteCanaryStartedAt = time.Time{}
		}
		pcOSRemoteCanaryMu.Unlock()
	}(cmd, pid)

	go func(expectedPID int) {
		timer := time.NewTimer(pcOSRemoteCanaryMaxRun)
		defer timer.Stop()
		<-timer.C
		pcOSRemoteCanaryMu.Lock()
		if pcOSRemoteCanaryProcess != nil && pcOSRemoteCanaryProcess.Pid == expectedPID {
			_ = pcOSRemoteCanaryProcess.Kill()
		}
		pcOSRemoteCanaryMu.Unlock()
	}(pid)

	return map[string]interface{}{
		"started": true,
		"pid": pid,
		"started_at": startedAt.Format(time.RFC3339),
		"mode": "canary",
		"screen_only": true,
		"attended_approval_required": true,
		"pairing_code_returned": false,
		"input_control": false,
		"unattended_access": false,
		"signal_origin": "pinned_appdeploy_canary",
		"max_runtime_minutes": 35,
	}, nil
}

func pcOSRemoteCanaryStatus() (interface{}, error) {
	pcOSRemoteCanaryMu.Lock()
	defer pcOSRemoteCanaryMu.Unlock()
	running := pcOSRemoteCanaryIsRunningLocked()
	result := map[string]interface{}{
		"version": pcOSRemoteCanaryVersion,
		"running": running,
		"screen_only": true,
		"attended_approval_required": true,
		"input_control": false,
		"unattended_access": false,
		"signal_origin": "pinned_appdeploy_canary",
	}
	if running && pcOSRemoteCanaryProcess != nil {
		result["pid"] = pcOSRemoteCanaryProcess.Pid
		result["started_at"] = pcOSRemoteCanaryStartedAt.Format(time.RFC3339)
	}
	return result, nil
}
