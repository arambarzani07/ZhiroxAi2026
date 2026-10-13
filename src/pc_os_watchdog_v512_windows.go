//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	pcOSZeroTouchWatchdogArg = "--zhirox-zero-touch-watchdog"
	pcOSZeroTouchMutexName   = `Global\ZhiroxZeroTouchWatchdog-v512`

	pcOSZeroTouchDetachedProcess    = 0x00000008
	pcOSZeroTouchNewProcessGroup    = 0x00000200
	pcOSZeroTouchServiceQueryStatus = 0x0004
	pcOSZeroTouchServiceStart       = 0x0010
	pcOSZeroTouchServiceStop        = 0x0020

	pcOSZeroTouchServiceStopped     = 1
	pcOSZeroTouchServiceStartPending = 2
	pcOSZeroTouchServiceStopPending  = 3
	pcOSZeroTouchServiceRunning      = 4
	pcOSZeroTouchServiceControlStop  = 1

	pcOSZeroTouchErrAlreadyExists        = 183
	pcOSZeroTouchErrServiceAlreadyRunning = 1056
	pcOSZeroTouchErrServiceNotActive      = 1062
)

type pcOSZeroTouchServiceStatus struct {
	ServiceType             uint32
	CurrentState            uint32
	ControlsAccepted        uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
	CheckPoint              uint32
	WaitHint                uint32
}

type pcOSZeroTouchStateResponse struct {
	OK                  bool   `json:"ok"`
	AgentID             string `json:"agent_id"`
	Version             string `json:"version"`
	LastSeenAt          string `json:"last_seen_at"`
	HeartbeatAgeSeconds *int64 `json:"heartbeat_age_seconds"`
	LastHealthScore     *int   `json:"last_health_score"`
	ServerTime          string `json:"server_time"`
}

var (
	pcOSZeroTouchKernel32      = syscall.NewLazyDLL("kernel32.dll")
	pcOSZeroTouchCreateMutexW  = pcOSZeroTouchKernel32.NewProc("CreateMutexW")
	pcOSZeroTouchCloseHandle   = pcOSZeroTouchKernel32.NewProc("CloseHandle")
	pcOSZeroTouchControlService = pcOSAdvapi32.NewProc("ControlService")
	pcOSZeroTouchQueryServiceStatus = pcOSAdvapi32.NewProc("QueryServiceStatus")
	pcOSZeroTouchStartServiceW = pcOSAdvapi32.NewProc("StartServiceW")
)

func pcOSZeroTouchWatchdogRequested(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), pcOSZeroTouchWatchdogArg) {
			return true
		}
	}
	return false
}

func pcOSZeroTouchStatusPath() string {
	return filepath.Join(pcOSActiveRoot(), "zero-touch-watchdog-status.json")
}

func pcOSZeroTouchAcquireMutex() (func(), bool, error) {
	name, err := syscall.UTF16PtrFromString(pcOSZeroTouchMutexName)
	if err != nil {
		return nil, false, err
	}
	h, _, callErr := pcOSZeroTouchCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return nil, false, pcOSWinAPIError("CreateMutexW", callErr)
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == pcOSZeroTouchErrAlreadyExists {
		pcOSZeroTouchCloseHandle.Call(h)
		return nil, false, nil
	}
	return func() { pcOSZeroTouchCloseHandle.Call(h) }, true, nil
}

func pcOSZeroTouchWatchdogEndpoint(creds pcOSRelayCredentials) (string, error) {
	u, err := url.Parse(strings.TrimSpace(creds.Relay))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("invalid relay origin for watchdog state endpoint")
	}
	u.Path = "/functions/v1/zhirox-pc-watchdog-state-v512/api/agent/" + url.PathEscape(creds.AgentID) + "/state"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func pcOSZeroTouchFetchState() (time.Duration, bool, bool, pcOSZeroTouchStateResponse, error) {
	creds, err := pcOSLoadRelayCredentials()
	if err != nil {
		return -1, false, false, pcOSZeroTouchStateResponse{}, err
	}
	endpoint, err := pcOSZeroTouchWatchdogEndpoint(creds)
	if err != nil {
		return -1, false, false, pcOSZeroTouchStateResponse{}, err
	}

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return -1, false, false, pcOSZeroTouchStateResponse{}, err
	}
	req.Header.Set("X-Agent-ID", creds.AgentID)
	req.Header.Set("Authorization", "Bearer "+creds.Secret)
	req.Header.Set("User-Agent", "Zhirox-ZeroTouch-Watchdog/5.1.2")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return -1, false, false, pcOSZeroTouchStateResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return -1, true, false, pcOSZeroTouchStateResponse{}, fmt.Errorf("watchdog authentication returned HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return -1, true, true, pcOSZeroTouchStateResponse{}, fmt.Errorf("watchdog state endpoint returned HTTP %d", resp.StatusCode)
	}

	var state pcOSZeroTouchStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return -1, true, true, state, err
	}
	if !state.OK || state.HeartbeatAgeSeconds == nil || *state.HeartbeatAgeSeconds < 0 {
		return -1, true, true, state, errors.New("watchdog state response does not contain a valid heartbeat age")
	}
	return time.Duration(*state.HeartbeatAgeSeconds) * time.Second, true, true, state, nil
}

func pcOSZeroTouchQueryCoreState(svc uintptr) (uint32, error) {
	var status pcOSZeroTouchServiceStatus
	r1, _, callErr := pcOSZeroTouchQueryServiceStatus.Call(svc, uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return 0, pcOSWinAPIError("QueryServiceStatus", callErr)
	}
	return status.CurrentState, nil
}

func pcOSZeroTouchWaitCoreState(svc uintptr, wanted uint32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := pcOSZeroTouchQueryCoreState(svc)
		if err != nil {
			return err
		}
		if state == wanted {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for Zhirox Core service state %d", wanted)
}

func pcOSZeroTouchRestartCoreService() error {
	access := uintptr(pcOSZeroTouchServiceQueryStatus | pcOSZeroTouchServiceStart | pcOSZeroTouchServiceStop)
	svc, cleanup, err := pcOSOpenCoreService(access)
	if err != nil {
		return err
	}
	defer cleanup()

	state, err := pcOSZeroTouchQueryCoreState(svc)
	if err != nil {
		return err
	}
	if state != pcOSZeroTouchServiceStopped {
		var status pcOSZeroTouchServiceStatus
		r1, _, callErr := pcOSZeroTouchControlService.Call(
			svc,
			pcOSZeroTouchServiceControlStop,
			uintptr(unsafe.Pointer(&status)),
		)
		if r1 == 0 {
			if errno, ok := callErr.(syscall.Errno); !ok || errno != pcOSZeroTouchErrServiceNotActive {
				return pcOSWinAPIError("ControlService(stop ZhiroxAICore)", callErr)
			}
		}
		if err := pcOSZeroTouchWaitCoreState(svc, pcOSZeroTouchServiceStopped, 25*time.Second); err != nil {
			return err
		}
	}

	r1, _, callErr := pcOSZeroTouchStartServiceW.Call(svc, 0, 0)
	if r1 == 0 {
		if errno, ok := callErr.(syscall.Errno); !ok || errno != pcOSZeroTouchErrServiceAlreadyRunning {
			return pcOSWinAPIError("StartServiceW(ZhiroxAICore)", callErr)
		}
	}
	return pcOSZeroTouchWaitCoreState(svc, pcOSZeroTouchServiceRunning, 25*time.Second)
}

// pcOSZeroTouchEnsureSCMRecovery installs only fixed, bounded recovery actions
// for the Zhirox Core service: restart after 10s, 30s, 60s, then NONE. It never
// configures a Windows reboot or an external recovery command.
func pcOSZeroTouchEnsureSCMRecovery() error {
	svc, cleanup, err := pcOSOpenCoreService(pcOSServiceQueryConfig | pcOSServiceChangeConfig)
	if err != nil {
		return err
	}
	defer cleanup()

	actions := []pcOSSCAction{
		{Type: pcOSSCActionRestart, Delay: 10_000},
		{Type: pcOSSCActionRestart, Delay: 30_000},
		{Type: pcOSSCActionRestart, Delay: 60_000},
		{Type: pcOSSCActionNone, Delay: 0},
	}
	failure := pcOSServiceFailureActions{
		ResetPeriod:  24 * 60 * 60,
		RebootMsg:    nil,
		Command:      nil,
		ActionsCount: uint32(len(actions)),
		Actions:      &actions[0],
	}
	r1, _, callErr := pcOSChangeServiceConfig2W.Call(
		svc,
		pcOSServiceConfigFailure,
		uintptr(unsafe.Pointer(&failure)),
	)
	if r1 == 0 {
		return pcOSWinAPIError("ChangeServiceConfig2W(zero-touch failure actions)", callErr)
	}

	flag := pcOSServiceFailureActionsFlag{FailureActionsOnNonCrashFailures: 1}
	r1, _, callErr = pcOSChangeServiceConfig2W.Call(
		svc,
		pcOSServiceConfigFailFlag,
		uintptr(unsafe.Pointer(&flag)),
	)
	if r1 == 0 {
		return pcOSWinAPIError("ChangeServiceConfig2W(zero-touch failure flag)", callErr)
	}
	return nil
}

func pcOSZeroTouchStartCompanion() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, pcOSZeroTouchWatchdogArg)
	pcOSApplyZeroUI(cmd)
	cmd.SysProcAttr.CreationFlags |= pcOSZeroTouchDetachedProcess | pcOSZeroTouchNewProcessGroup
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func pcOSZeroTouchCompanionStatusFresh() bool {
	info, err := os.Stat(pcOSZeroTouchStatusPath())
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) >= 0 && time.Since(info.ModTime()) < 75*time.Second
}

func pcOSZeroTouchWatchdogEnsureLoop() {
	time.Sleep(10 * time.Second)
	_ = pcOSZeroTouchEnsureSCMRecovery()

	for {
		if !pcOSZeroTouchCompanionStatusFresh() {
			_ = pcOSZeroTouchStartCompanion()
		}
		time.Sleep(30 * time.Second)
	}
}

func pcOSZeroTouchWriteStatus(status map[string]interface{}) {
	status["module"] = "zero_touch_recovery"
	status["zero_ui"] = true
	status["target_service"] = pcOSCoreServiceName
	status["windows_reboot_allowed"] = false
	status["external_recovery_command_allowed"] = false
	status["business_apps_mutated"] = false
	status["updated_utc"] = time.Now().UTC().Format(time.RFC3339)
	_ = pcOSWriteAtomicJSON(pcOSZeroTouchStatusPath(), status)
}

func pcOSRunZeroTouchWatchdog() {
	release, acquired, err := pcOSZeroTouchAcquireMutex()
	if err != nil {
		pcOSZeroTouchWriteStatus(map[string]interface{}{"ok": false, "reason": "mutex_error", "error": err.Error()})
		return
	}
	if !acquired {
		return
	}
	defer release()

	consecutiveStale := 0
	var lastRestartAttempt time.Time
	restartAttempts := make([]time.Time, 0, pcOSZeroTouchMaxRestartsPerWindow)

	for {
		now := time.Now()
		cutoff := now.Add(-pcOSZeroTouchRestartWindow)
		kept := restartAttempts[:0]
		for _, ts := range restartAttempts {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		restartAttempts = kept

		heartbeatAge, reachable, authenticated, state, fetchErr := pcOSZeroTouchFetchState()
		sinceLastRestart := pcOSZeroTouchRestartCooldown
		if !lastRestartAttempt.IsZero() {
			sinceLastRestart = now.Sub(lastRestartAttempt)
		}
		decision := pcOSZeroTouchEvaluate(
			reachable,
			authenticated,
			heartbeatAge,
			consecutiveStale,
			sinceLastRestart,
			len(restartAttempts),
		)
		consecutiveStale = decision.ConsecutiveStale

		status := map[string]interface{}{
			"ok":                    fetchErr == nil,
			"reason":                decision.Reason,
			"endpoint_reachable":    reachable,
			"authenticated":         authenticated,
			"consecutive_stale":     consecutiveStale,
			"restart_attempts_hour": len(restartAttempts),
			"agent_version":         state.Version,
			"server_time":           state.ServerTime,
		}
		if heartbeatAge >= 0 {
			status["heartbeat_age_seconds"] = int64(heartbeatAge / time.Second)
		}
		if fetchErr != nil {
			status["error"] = fetchErr.Error()
		}

		if decision.Restart {
			lastRestartAttempt = now
			restartAttempts = append(restartAttempts, now)
			consecutiveStale = 0
			status["restart_attempted"] = true
			if err := pcOSZeroTouchRestartCoreService(); err != nil {
				status["restart_ok"] = false
				status["restart_error"] = err.Error()
				status["reason"] = "restart_failed"
			} else {
				status["restart_ok"] = true
				status["reason"] = "restart_completed"
			}
		}

		pcOSZeroTouchWriteStatus(status)
		time.Sleep(pcOSZeroTouchCheckInterval)
	}
}

func pcOSZeroTouchStatus() (interface{}, error) {
	out := map[string]interface{}{
		"module":              "zero_touch_recovery",
		"enabled":             true,
		"zero_ui":             true,
		"heartbeat_stale_after_seconds": int64(pcOSZeroTouchHeartbeatStaleAfter / time.Second),
		"confirmations_required":        pcOSZeroTouchConfirmations,
		"restart_cooldown_seconds":      int64(pcOSZeroTouchRestartCooldown / time.Second),
		"max_restarts_per_hour":         pcOSZeroTouchMaxRestartsPerWindow,
		"windows_reboot_allowed":        false,
		"external_command_allowed":      false,
	}
	if raw, err := os.ReadFile(pcOSZeroTouchStatusPath()); err == nil {
		var state interface{}
		if json.Unmarshal(raw, &state) == nil {
			out["watchdog"] = state
		}
	}
	if recovery, err := pcOSCoreRecoveryStatus(); err == nil {
		out["scm_recovery"] = recovery
	} else {
		out["scm_recovery_error"] = err.Error()
	}
	return out, nil
}
