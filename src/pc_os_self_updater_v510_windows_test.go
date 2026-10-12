//go:build windows && zhiroxtest

package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
    "time"
)

func TestMain(m *testing.M) {
    mode := strings.TrimSpace(os.Getenv("ZHIROX_UPDATER_FIXTURE_MODE"))
    if mode != "" {
        switch mode {
        case "hold":
            ms, _ := strconv.Atoi(os.Getenv("ZHIROX_UPDATER_FIXTURE_HOLD_MS"))
            if ms <= 0 { ms = 750 }
            time.Sleep(time.Duration(ms) * time.Millisecond)
            os.Exit(0)
        case "exit":
            os.Exit(17)
        case "healthy":
            marker := os.Getenv("ZHIROX_UPDATER_FIXTURE_MARKER")
            version := os.Getenv("ZHIROX_UPDATER_FIXTURE_VERSION")
            stop := os.Getenv("ZHIROX_UPDATER_FIXTURE_STOP")
            time.Sleep(500 * time.Millisecond)
            if marker != "" {
                raw, _ := json.Marshal(map[string]interface{}{
                    "version": version,
                    "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
                })
                _ = os.WriteFile(marker, raw, 0600)
            }
            until := time.Now().Add(110 * time.Second)
            for time.Now().Before(until) {
                if stop != "" {
                    if _, err := os.Stat(stop); err == nil { os.Exit(0) }
                }
                time.Sleep(100 * time.Millisecond)
            }
            os.Exit(0)
        default:
            os.Exit(23)
        }
    }
    os.Exit(m.Run())
}

func copyFixture(t *testing.T, dst string, overlay string) string {
    t.Helper()
    src, err := os.Executable()
    if err != nil { t.Fatal(err) }
    in, err := os.Open(src)
    if err != nil { t.Fatal(err) }
    defer in.Close()
    out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
    if err != nil { t.Fatal(err) }
    if _, err := io.Copy(out, in); err != nil { _ = out.Close(); t.Fatal(err) }
    if overlay != "" {
        if _, err := out.Write([]byte(overlay)); err != nil { _ = out.Close(); t.Fatal(err) }
    }
    if err := out.Close(); err != nil { t.Fatal(err) }
    h, err := pcOSUpdaterHashFile(dst)
    if err != nil { t.Fatal(err) }
    return h
}

func readUpdaterState(t *testing.T, root string) map[string]interface{} {
    t.Helper()
    raw, err := os.ReadFile(filepath.Join(root, "update-install-state.json"))
    if err != nil { t.Fatal(err) }
    var state map[string]interface{}
    if err := json.Unmarshal(raw, &state); err != nil { t.Fatal(err) }
    return state
}

func startOldFixture(t *testing.T, current string) *exec.Cmd {
    t.Helper()
    cmd := pcOSUpdaterHiddenCommand(current)
    cmd.Env = append(os.Environ(),
        "ZHIROX_UPDATER_FIXTURE_MODE=hold",
        "ZHIROX_UPDATER_FIXTURE_HOLD_MS=900",
    )
    if err := cmd.Start(); err != nil { t.Fatal(err) }
    t.Cleanup(func() {
        if cmd.Process != nil { _ = cmd.Process.Kill() }
        _ = cmd.Wait()
    })
    time.Sleep(150 * time.Millisecond)
    return cmd
}

func withUpdaterArgs(t *testing.T, pid int, current, next, version, hash, marker string) {
    t.Helper()
    oldArgs := os.Args
    os.Args = []string{
        oldArgs[0],
        "--pid", strconv.Itoa(pid),
        "--current", current,
        "--new", next,
        "--version", version,
        "--sha256", hash,
        "--health-marker", marker,
    }
    t.Cleanup(func() { os.Args = oldArgs })
}

func TestPCOSUpdaterFailureRollsBack(t *testing.T) {
    base := t.TempDir()
    root := filepath.Join(base, "ZhiroxAI")
    if err := os.MkdirAll(root, 0700); err != nil { t.Fatal(err) }
    current := filepath.Join(root, "ZhiroxAISupervisor.exe")
    next := filepath.Join(root, "ZhiroxAISupervisor.exe.new")
    marker := filepath.Join(root, "update-health.json")

    oldHash := copyFixture(t, current, "")
    newHash := copyFixture(t, next, "ZHIROX-NEW-OVERLAY-ROLLBACK-TEST")
    old := startOldFixture(t, current)

    t.Setenv("ZHIROX_UPDATER_FIXTURE_MODE", "exit")
    withUpdaterArgs(t, old.Process.Pid, current, next, "5.1.0-test-fail", newHash, marker)

    err := pcOSRunUpdaterMode()
    if err == nil { t.Fatal("expected new Supervisor failure to trigger rollback") }

    restoredHash, hashErr := pcOSUpdaterHashFile(current)
    if hashErr != nil { t.Fatal(hashErr) }
    if restoredHash != oldHash {
        t.Fatalf("rollback did not restore old Supervisor: got %s want %s", restoredHash, oldHash)
    }

    failed, globErr := filepath.Glob(filepath.Join(root, "ZhiroxAISupervisor.exe.failed-*"))
    if globErr != nil { t.Fatal(globErr) }
    if len(failed) != 1 { t.Fatalf("expected one quarantined failed binary, got %d", len(failed)) }
    failedHash, hashErr := pcOSUpdaterHashFile(failed[0])
    if hashErr != nil { t.Fatal(hashErr) }
    if failedHash != newHash {
        t.Fatalf("failed binary hash changed: got %s want %s", failedHash, newHash)
    }

    state := readUpdaterState(t, root)
    if state["status"] != "rolled_back" {
        t.Fatalf("expected rolled_back state, got %#v", state["status"])
    }
}

func TestPCOSUpdaterSuccessHealthGate(t *testing.T) {
    if testing.Short() { t.Skip("full updater health gate test") }
    base := t.TempDir()
    root := filepath.Join(base, "ZhiroxAI")
    if err := os.MkdirAll(root, 0700); err != nil { t.Fatal(err) }
    current := filepath.Join(root, "ZhiroxAISupervisor.exe")
    next := filepath.Join(root, "ZhiroxAISupervisor.exe.new")
    marker := filepath.Join(root, "update-health.json")
    stop := filepath.Join(root, "fixture-stop")

    oldHash := copyFixture(t, current, "")
    newHash := copyFixture(t, next, "ZHIROX-NEW-OVERLAY-SUCCESS-TEST")
    old := startOldFixture(t, current)

    t.Setenv("ZHIROX_UPDATER_FIXTURE_MODE", "healthy")
    t.Setenv("ZHIROX_UPDATER_FIXTURE_MARKER", marker)
    t.Setenv("ZHIROX_UPDATER_FIXTURE_VERSION", "5.1.0-test-ok")
    t.Setenv("ZHIROX_UPDATER_FIXTURE_STOP", stop)
    withUpdaterArgs(t, old.Process.Pid, current, next, "5.1.0-test-ok", newHash, marker)

    started := time.Now()
    if err := pcOSRunUpdaterMode(); err != nil { t.Fatal(err) }
    if time.Since(started) < 89*time.Second {
        t.Fatal("health gate returned before the fixed 90-second validation window")
    }

    currentHash, err := pcOSUpdaterHashFile(current)
    if err != nil { t.Fatal(err) }
    if currentHash != newHash {
        t.Fatalf("promoted Supervisor hash mismatch: got %s want %s", currentHash, newHash)
    }

    backups, err := filepath.Glob(filepath.Join(root, "ZhiroxAISupervisor.exe.pre-vrollback-*.bak"))
    if err != nil { t.Fatal(err) }
    if len(backups) != 1 { t.Fatalf("expected one rollback backup, got %d", len(backups)) }
    backupHash, err := pcOSUpdaterHashFile(backups[0])
    if err != nil { t.Fatal(err) }
    if backupHash != oldHash {
        t.Fatalf("rollback backup hash mismatch: got %s want %s", backupHash, oldHash)
    }

    state := readUpdaterState(t, root)
    if state["status"] != "installed_health_gate_passed" {
        t.Fatalf("expected installed_health_gate_passed state, got %#v", state["status"])
    }

    _ = os.WriteFile(stop, []byte("stop"), 0600)
    time.Sleep(1500 * time.Millisecond)
}

func TestPCOSUpdaterRejectsHashMismatchBeforeMutation(t *testing.T) {
    base := t.TempDir()
    root := filepath.Join(base, "ZhiroxAI")
    if err := os.MkdirAll(root, 0700); err != nil { t.Fatal(err) }
    current := filepath.Join(root, "ZhiroxAISupervisor.exe")
    next := filepath.Join(root, "ZhiroxAISupervisor.exe.new")
    marker := filepath.Join(root, "update-health.json")

    oldHash := copyFixture(t, current, "")
    _ = copyFixture(t, next, "ZHIROX-BAD-HASH-TEST")
    withUpdaterArgs(t, os.Getpid(), current, next, "5.1.0-test-hash", strings.Repeat("0", 64), marker)

    if err := pcOSRunUpdaterMode(); err == nil {
        t.Fatal("expected hash mismatch rejection")
    }
    currentHash, err := pcOSUpdaterHashFile(current)
    if err != nil { t.Fatal(err) }
    if currentHash != oldHash {
        t.Fatal("current Supervisor changed despite pre-mutation hash rejection")
    }
    if _, err := os.Stat(next); err != nil {
        t.Fatal("staged file should remain untouched after pre-mutation rejection")
    }
}

func TestPCOSUpdaterHashHelperMatchesStandardLibrary(t *testing.T) {
    path := filepath.Join(t.TempDir(), "sample.bin")
    data := []byte("zhirox-updater-hash-test")
    if err := os.WriteFile(path, data, 0600); err != nil { t.Fatal(err) }
    got, err := pcOSUpdaterHashFile(path)
    if err != nil { t.Fatal(err) }
    sum := sha256.Sum256(data)
    want := hex.EncodeToString(sum[:])
    if got != want { t.Fatalf("got %s want %s", got, want) }
}
