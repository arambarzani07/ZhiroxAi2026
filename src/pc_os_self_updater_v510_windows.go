//go:build windows

package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "time"
)

const pcOSNoWindowCreationFlag = 0x08000000

func pcOSUpdaterHiddenCommand(exe string, args ...string) *exec.Cmd {
    cmd := exec.Command(exe, args...)
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: pcOSNoWindowCreationFlag}
    return cmd
}

func pcOSUpdaterArg(name string) string {
    for i := 1; i+1 < len(os.Args); i++ {
        if os.Args[i] == name {
            return strings.TrimSpace(os.Args[i+1])
        }
    }
    return ""
}

func pcOSUpdaterWriteState(root, status string, extra map[string]interface{}) {
    state := map[string]interface{}{
        "status": status,
        "timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano),
        "zero_ui": true,
    }
    for k, v := range extra {
        state[k] = v
    }
    raw, _ := json.MarshalIndent(state, "", "  ")
    _ = os.WriteFile(filepath.Join(root, "update-install-state.json"), raw, 0600)
}

func pcOSUpdaterValidatePaths(current, next, marker string) (string, error) {
    currentAbs, err := filepath.Abs(current)
    if err != nil { return "", err }
    nextAbs, err := filepath.Abs(next)
    if err != nil { return "", err }
    markerAbs, err := filepath.Abs(marker)
    if err != nil { return "", err }

    currentAbs = filepath.Clean(currentAbs)
    nextAbs = filepath.Clean(nextAbs)
    markerAbs = filepath.Clean(markerAbs)
    root := filepath.Clean(filepath.Dir(currentAbs))

    if !strings.EqualFold(filepath.Base(root), "ZhiroxAI") {
        return "", errors.New("updater refused: active root must be named ZhiroxAI")
    }
    if !strings.EqualFold(filepath.Base(currentAbs), "ZhiroxAISupervisor.exe") {
        return "", errors.New("updater refused: current executable name is not ZhiroxAISupervisor.exe")
    }
    if !strings.EqualFold(filepath.Base(nextAbs), "ZhiroxAISupervisor.exe.new") || !strings.EqualFold(filepath.Dir(nextAbs), root) {
        return "", errors.New("updater refused: staged executable must be ZhiroxAISupervisor.exe.new in the active root")
    }
    expectedMarker := filepath.Join(root, "update-health.json")
    if !strings.EqualFold(markerAbs, expectedMarker) {
        return "", errors.New("updater refused: health marker path is outside the fixed update contract")
    }
    return root, nil
}

func pcOSUpdaterHashFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return hex.EncodeToString(h.Sum(nil)), nil
}

func pcOSUpdaterWaitRename(oldPath, newPath string, deadline time.Duration) error {
    until := time.Now().Add(deadline)
    var last error
    for time.Now().Before(until) {
        if err := os.Rename(oldPath, newPath); err == nil {
            return nil
        } else {
            last = err
        }
        time.Sleep(500 * time.Millisecond)
    }
    if last == nil { last = errors.New("rename deadline exceeded") }
    return last
}

func pcOSUpdaterMarkerHealthy(marker, targetVersion string, installedAfter time.Time) bool {
    info, err := os.Stat(marker)
    if err != nil || !info.ModTime().After(installedAfter) {
        return false
    }
    raw, err := os.ReadFile(marker)
    if err != nil { return false }
    var doc map[string]interface{}
    if json.Unmarshal(raw, &doc) != nil { return false }
    return strings.EqualFold(strings.TrimSpace(fmt.Sprint(doc["version"])), strings.TrimSpace(targetVersion))
}

func pcOSUpdaterRollback(root, current, backup string, launched *exec.Cmd, targetVersion string, cause error) error {
    if launched != nil && launched.Process != nil {
        // Wait() is owned by the monitor goroutine. Kill only here; the rename
        // retry loop waits for Windows to release the executable handle.
        _ = launched.Process.Kill()
        time.Sleep(750 * time.Millisecond)
    }

    failed := filepath.Join(root, "ZhiroxAISupervisor.exe.failed-"+time.Now().UTC().Format("20060102T150405Z"))
    if _, err := os.Stat(current); err == nil {
        if err := pcOSUpdaterWaitRename(current, failed, 20*time.Second); err != nil {
            pcOSUpdaterWriteState(root, "rollback_failed", map[string]interface{}{
                "target_version": targetVersion,
                "cause": cause.Error(),
                "failed_binary_move_error": err.Error(),
                "backup": backup,
            })
            return err
        }
    }
    if err := pcOSUpdaterWaitRename(backup, current, 20*time.Second); err != nil {
        pcOSUpdaterWriteState(root, "rollback_failed", map[string]interface{}{
            "target_version": targetVersion,
            "cause": cause.Error(),
            "restore_error": err.Error(),
            "failed_binary": failed,
            "backup": backup,
        })
        return err
    }

    restored := pcOSUpdaterHiddenCommand(current)
    restored.Dir = root
    startErr := restored.Start()
    if startErr == nil && restored.Process != nil {
        _ = restored.Process.Release()
    }
    extra := map[string]interface{}{
        "target_version": targetVersion,
        "cause": cause.Error(),
        "failed_binary": failed,
        "backup_restored": current,
    }
    if startErr != nil { extra["restart_error"] = startErr.Error() }
    pcOSUpdaterWriteState(root, "rolled_back", extra)
    return startErr
}

func pcOSRunUpdaterMode() error {
    current := pcOSUpdaterArg("--current")
    next := pcOSUpdaterArg("--new")
    targetVersion := pcOSUpdaterArg("--version")
    expectedHash := strings.ToLower(pcOSUpdaterArg("--sha256"))
    marker := pcOSUpdaterArg("--health-marker")
    pidText := pcOSUpdaterArg("--pid")

    if current == "" || next == "" || targetVersion == "" || expectedHash == "" || marker == "" || pidText == "" {
        return errors.New("updater refused: required arguments are missing")
    }
    if sourcePID, err := strconv.Atoi(pidText); err != nil || sourcePID <= 0 {
        return errors.New("updater refused: invalid source pid")
    }
    if len(expectedHash) != 64 {
        return errors.New("updater refused: expected SHA-256 must contain 64 hex characters")
    }
    if _, err := hex.DecodeString(expectedHash); err != nil {
        return errors.New("updater refused: expected SHA-256 is not hexadecimal")
    }

    root, err := pcOSUpdaterValidatePaths(current, next, marker)
    if err != nil { return err }
    actualHash, err := pcOSUpdaterHashFile(next)
    if err != nil { return fmt.Errorf("hash staged update: %w", err) }
    if !strings.EqualFold(actualHash, expectedHash) {
        return errors.New("updater refused: staged update SHA-256 mismatch")
    }

    backup := filepath.Join(root, "ZhiroxAISupervisor.exe.pre-vrollback-"+time.Now().UTC().Format("20060102T150405Z")+".bak")
    if err := pcOSUpdaterWaitRename(current, backup, 60*time.Second); err != nil {
        return fmt.Errorf("wait for old supervisor / create rollback copy: %w", err)
    }
    if err := os.Rename(next, current); err != nil {
        _ = os.Rename(backup, current)
        return fmt.Errorf("promote verified update: %w", err)
    }

    installedAt := time.Now()
    child := pcOSUpdaterHiddenCommand(current)
    child.Dir = root
    if err := child.Start(); err != nil {
        _ = pcOSUpdaterRollback(root, current, backup, nil, targetVersion, err)
        return fmt.Errorf("start new supervisor: %w", err)
    }

    exited := make(chan error, 1)
    go func() { exited <- child.Wait() }()
    healthSeen := false
    deadline := time.NewTimer(90 * time.Second)
    ticker := time.NewTicker(2 * time.Second)
    defer deadline.Stop()
    defer ticker.Stop()

    for {
        select {
        case err := <-exited:
            if err == nil { err = errors.New("new supervisor exited during health gate") }
            _ = pcOSUpdaterRollback(root, current, backup, nil, targetVersion, err)
            return err
        case <-ticker.C:
            if pcOSUpdaterMarkerHealthy(marker, targetVersion, installedAt) {
                healthSeen = true
            }
        case <-deadline.C:
            if !healthSeen {
                err := errors.New("new supervisor did not produce a matching health marker within 90 seconds")
                _ = pcOSUpdaterRollback(root, current, backup, child, targetVersion, err)
                return err
            }
            pcOSUpdaterWriteState(root, "installed_health_gate_passed", map[string]interface{}{
                "target_version": targetVersion,
                "sha256": actualHash,
                "rollback_backup": backup,
                "health_marker": marker,
                "health_gate_seconds": 90,
            })
            // Keep the rollback backup. The Supervisor's existing three-heartbeat
            // cleanup gate may remove it only after cloud health is proven.
            return nil
        }
    }
}

func pcOSMaybeRunUpdaterMode() bool {
    exe, err := os.Executable()
    if err != nil || !strings.EqualFold(filepath.Base(exe), "ZhiroxUpdater.exe") {
        return false
    }
    if pcOSUpdaterArg("--current") == "" || pcOSUpdaterArg("--new") == "" {
        return false
    }
    root := filepath.Dir(exe)
    if err := pcOSRunUpdaterMode(); err != nil {
        pcOSUpdaterWriteState(root, "updater_failed", map[string]interface{}{"error": err.Error()})
    }
    return true
}

func pcOSEnsureSelfUpdater() error {
    current, err := os.Executable()
    if err != nil { return err }
    current, err = filepath.Abs(current)
    if err != nil { return err }
    current = filepath.Clean(current)
    root := filepath.Dir(current)
    if !strings.EqualFold(filepath.Base(root), "ZhiroxAI") || !strings.EqualFold(filepath.Base(current), "ZhiroxAISupervisor.exe") {
        return errors.New("self-updater provisioning refused unexpected active path")
    }

    sourceHash, err := pcOSUpdaterHashFile(current)
    if err != nil { return err }
    updater := filepath.Join(root, "ZhiroxUpdater.exe")
    if existingHash, err := pcOSUpdaterHashFile(updater); err == nil && strings.EqualFold(existingHash, sourceHash) {
        return nil
    }

    stage := filepath.Join(root, "ZhiroxUpdater.exe.new")
    in, err := os.Open(current)
    if err != nil { return err }
    defer in.Close()
    out, err := os.OpenFile(stage, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
    if err != nil { return err }
    if _, err := io.Copy(out, in); err != nil { _ = out.Close(); return err }
    if err := out.Close(); err != nil { return err }
    stageHash, err := pcOSUpdaterHashFile(stage)
    if err != nil || !strings.EqualFold(stageHash, sourceHash) {
        return errors.New("self-updater staged copy integrity mismatch")
    }

    previous := ""
    if _, err := os.Stat(updater); err == nil {
        previous = filepath.Join(root, "ZhiroxUpdater.exe.pre-refresh-"+time.Now().UTC().Format("20060102T150405Z")+".bak")
        if err := os.Rename(updater, previous); err != nil { return err }
    }
    if err := os.Rename(stage, updater); err != nil {
        if previous != "" { _ = os.Rename(previous, updater) }
        return err
    }
    pcOSUpdaterWriteState(root, "companion_updater_refreshed", map[string]interface{}{
        "sha256": sourceHash,
        "updater": updater,
        "previous_updater_backup": previous,
        "automatic_delete": false,
    })
    return nil
}

func pcOSCompanionUpdaterLoop() {
    // Do not replace the known-good legacy updater until this Supervisor has
    // proved cloud health with three consecutive heartbeats.
    time.Sleep(150 * time.Second)
    for {
        healthy := true
        for i := 0; i < 3; i++ {
            if err := sendHeartbeat(); err != nil {
                healthy = false
                break
            }
            if i < 2 { time.Sleep(15 * time.Second) }
        }
        if !healthy {
            time.Sleep(5 * time.Minute)
            continue
        }
        if err := pcOSEnsureSelfUpdater(); err != nil {
            pcOSUpdaterWriteState(pcOSActiveRoot(), "companion_updater_refresh_failed", map[string]interface{}{"error": err.Error()})
            time.Sleep(5 * time.Minute)
            continue
        }
        return
    }
}
