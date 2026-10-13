from pathlib import Path

main = Path('buildsrc/main.go')
pc_os = Path('buildsrc/pc_os_v510.go')
watch = Path('buildsrc/pc_os_watchdog_v512_windows.go')

main_text = main.read_text()
pc_text = pc_os.read_text()
w = watch.read_text()

# v5.1.3 keeps the v5.1.2 server-observed heartbeat policy but adds a
# privilege-resilient process-level fallback. If Windows service START/STOP
# access is unavailable, the detached watchdog can terminate only the exact
# Supervisor PID recorded in update-health.json after verifying its executable
# path, wait for the service wrapper to relaunch it, and finally launch the
# same fixed Supervisor binary itself if the wrapper does not recover it.
# No shell, reboot, business-app mutation, or arbitrary process target exists.

if 'const pcOSVersion = "5.1.2"' not in pc_text:
    raise SystemExit('expected v5.1.2 pcOSVersion not found')
pc_text = pc_text.replace('const pcOSVersion = "5.1.2"', 'const pcOSVersion = "5.1.3"', 1)

if '5.1.2' not in main_text:
    raise SystemExit('expected v5.1.2 runtime version not found')
main_text = main_text.replace('5.1.2', '5.1.3')

# New Windows API surface is limited to the exact peer Supervisor process.
insert_before = '\nfunc pcOSZeroTouchWatchdogRequested(args []string) bool {'
if insert_before not in w:
    raise SystemExit('watchdog function insertion point not found')
helper = r'''

const (
	pcOSZeroTouchProcessTerminate               = 0x0001
	pcOSZeroTouchProcessQueryLimitedInformation = 0x1000
)

var (
	pcOSZeroTouchOpenProcess                = pcOSZeroTouchKernel32.NewProc("OpenProcess")
	pcOSZeroTouchTerminateProcess           = pcOSZeroTouchKernel32.NewProc("TerminateProcess")
	pcOSZeroTouchQueryFullProcessImageNameW = pcOSZeroTouchKernel32.NewProc("QueryFullProcessImageNameW")
)

type pcOSZeroTouchHealthMarker struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func pcOSZeroTouchOpenVerifiedPeer() (uintptr, int, string, error) {
	raw, err := os.ReadFile(filepath.Join(pcOSActiveRoot(), "update-health.json"))
	if err != nil {
		return 0, 0, "", fmt.Errorf("read update-health marker: %w", err)
	}
	var marker pcOSZeroTouchHealthMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		return 0, 0, "", fmt.Errorf("parse update-health marker: %w", err)
	}
	if marker.PID <= 0 || marker.PID == os.Getpid() {
		return 0, 0, "", errors.New("update-health marker does not identify a safe peer Supervisor PID")
	}

	access := uintptr(pcOSZeroTouchProcessTerminate | pcOSZeroTouchProcessQueryLimitedInformation)
	h, _, callErr := pcOSZeroTouchOpenProcess.Call(access, 0, uintptr(marker.PID))
	if h == 0 {
		return 0, marker.PID, marker.Version, pcOSWinAPIError("OpenProcess(verified Supervisor peer)", callErr)
	}

	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	r1, _, callErr := pcOSZeroTouchQueryFullProcessImageNameW.Call(
		h,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 || size == 0 || int(size) > len(buf) {
		pcOSZeroTouchCloseHandle.Call(h)
		return 0, marker.PID, marker.Version, pcOSWinAPIError("QueryFullProcessImageNameW", callErr)
	}
	peerPath := filepath.Clean(syscall.UTF16ToString(buf[:size]))
	exe, err := os.Executable()
	if err != nil {
		pcOSZeroTouchCloseHandle.Call(h)
		return 0, marker.PID, marker.Version, err
	}
	expected, err := filepath.Abs(exe)
	if err != nil {
		pcOSZeroTouchCloseHandle.Call(h)
		return 0, marker.PID, marker.Version, err
	}
	if !strings.EqualFold(peerPath, filepath.Clean(expected)) {
		pcOSZeroTouchCloseHandle.Call(h)
		return 0, marker.PID, marker.Version, fmt.Errorf("peer PID image mismatch: %s", peerPath)
	}
	return h, marker.PID, marker.Version, nil
}

func pcOSZeroTouchProcessAccessProbe() map[string]interface{} {
	out := map[string]interface{}{
		"available": false,
		"verified_exact_supervisor": false,
		"destructive_action_executed": false,
	}
	h, pid, version, err := pcOSZeroTouchOpenVerifiedPeer()
	out["pid"] = pid
	out["version"] = version
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	pcOSZeroTouchCloseHandle.Call(h)
	out["available"] = true
	out["verified_exact_supervisor"] = true
	return out
}

func pcOSZeroTouchServiceAccessProbe() map[string]interface{} {
	out := map[string]interface{}{"available": false, "destructive_action_executed": false}
	access := uintptr(pcOSZeroTouchServiceQueryStatus | pcOSZeroTouchServiceStart | pcOSZeroTouchServiceStop)
	svc, cleanup, err := pcOSOpenCoreService(access)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	cleanup()
	out["available"] = true
	return out
}

func pcOSZeroTouchTerminateVerifiedPeer() (int, string, error) {
	h, pid, version, err := pcOSZeroTouchOpenVerifiedPeer()
	if err != nil {
		return pid, version, err
	}
	defer pcOSZeroTouchCloseHandle.Call(h)
	r1, _, callErr := pcOSZeroTouchTerminateProcess.Call(h, 0)
	if r1 == 0 {
		return pid, version, pcOSWinAPIError("TerminateProcess(verified Supervisor peer)", callErr)
	}
	return pid, version, nil
}

func pcOSZeroTouchHeartbeatRecovered(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		age, reachable, authenticated, _, err := pcOSZeroTouchFetchState()
		if err == nil && reachable && authenticated && age >= 0 && age < 45*time.Second {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func pcOSZeroTouchLaunchNormalSupervisor() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	pcOSApplyZeroUI(cmd)
	cmd.SysProcAttr.CreationFlags |= pcOSZeroTouchDetachedProcess | pcOSZeroTouchNewProcessGroup
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func pcOSZeroTouchRecover() (string, error) {
	serviceErr := pcOSZeroTouchRestartCoreService()
	if serviceErr == nil && pcOSZeroTouchHeartbeatRecovered(45*time.Second) {
		return "service_restart", nil
	}

	pid, _, processErr := pcOSZeroTouchTerminateVerifiedPeer()
	if processErr != nil {
		if serviceErr == nil {
			return "", fmt.Errorf("service restarted but heartbeat did not recover; process fallback PID %d failed: %w", pid, processErr)
		}
		return "", fmt.Errorf("service restart failed: %v; process fallback PID %d failed: %w", serviceErr, pid, processErr)
	}

	// Give the existing privileged service wrapper the first chance to relaunch
	// its terminated child. This avoids duplicate Supervisors.
	if pcOSZeroTouchHeartbeatRecovered(35 * time.Second) {
		return "verified_process_terminate_service_relaunch", nil
	}

	if err := pcOSZeroTouchLaunchNormalSupervisor(); err != nil {
		return "", fmt.Errorf("verified peer terminated but direct relaunch failed: %w", err)
	}
	if !pcOSZeroTouchHeartbeatRecovered(45 * time.Second) {
		return "", errors.New("verified peer terminated and Supervisor relaunched, but heartbeat did not recover")
	}
	return "verified_process_terminate_direct_relaunch", nil
}
'''.replace('\\t', '\t')
w = w.replace(insert_before, helper + insert_before, 1)

old = '''\t\tif decision.Restart {
\t\t\tlastRestartAttempt = now
\t\t\trestartAttempts = append(restartAttempts, now)
\t\t\tconsecutiveStale = 0
\t\t\tstatus["restart_attempted"] = true
\t\t\tif err := pcOSZeroTouchRestartCoreService(); err != nil {
\t\t\t\tstatus["restart_ok"] = false
\t\t\t\tstatus["restart_error"] = err.Error()
\t\t\t\tstatus["reason"] = "restart_failed"
\t\t\t} else {
\t\t\t\tstatus["restart_ok"] = true
\t\t\t\tstatus["reason"] = "restart_completed"
\t\t\t}
\t\t}'''
new = '''\t\tif decision.Restart {
\t\t\tlastRestartAttempt = now
\t\t\trestartAttempts = append(restartAttempts, now)
\t\t\tconsecutiveStale = 0
\t\t\tstatus["restart_attempted"] = true
\t\t\tmethod, recoverErr := pcOSZeroTouchRecover()
\t\t\tif recoverErr != nil {
\t\t\t\tstatus["restart_ok"] = false
\t\t\t\tstatus["restart_error"] = recoverErr.Error()
\t\t\t\tstatus["reason"] = "restart_failed"
\t\t\t} else {
\t\t\t\tstatus["restart_ok"] = true
\t\t\t\tstatus["restart_method"] = method
\t\t\t\tstatus["reason"] = "restart_completed"
\t\t\t}
\t\t}'''
if old not in w:
    raise SystemExit('watchdog recovery block not found')
w = w.replace(old, new, 1)

needle = '''\tif recovery, err := pcOSCoreRecoveryStatus(); err == nil {
\t\tout["scm_recovery"] = recovery
\t} else {
\t\tout["scm_recovery_error"] = err.Error()
\t}
\treturn out, nil
}'''
replacement = '''\tout["service_restart_access_probe"] = pcOSZeroTouchServiceAccessProbe()
\tout["process_fallback_access_probe"] = pcOSZeroTouchProcessAccessProbe()
\tif recovery, err := pcOSCoreRecoveryStatus(); err == nil {
\t\tout["scm_recovery"] = recovery
\t} else {
\t\tout["scm_recovery_error"] = err.Error()
\t}
\treturn out, nil
}'''
if needle not in w:
    raise SystemExit('status probe insertion point not found')
w = w.replace(needle, replacement, 1)

w = w.replace('ZhiroxZeroTouchWatchdog-v512', 'ZhiroxZeroTouchWatchdog-v513')
w = w.replace('Zhirox-ZeroTouch-Watchdog/5.1.2', 'Zhirox-ZeroTouch-Watchdog/5.1.3')

for invariant in (
    'pcOSZeroTouchOpenVerifiedPeer',
    'QueryFullProcessImageNameW',
    'TerminateProcess(verified Supervisor peer)',
    'verified_process_terminate_service_relaunch',
    'verified_process_terminate_direct_relaunch',
    'process_fallback_access_probe',
    'destructive_action_executed',
):
    if invariant not in w:
        raise SystemExit(f'missing v5.1.3 fallback invariant: {invariant}')

main.write_text(main_text)
pc_os.write_text(pc_text)
watch.write_text(w)
