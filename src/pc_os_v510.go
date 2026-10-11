package main

import (
    "crypto/ed25519"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net"
    "os"
    "path/filepath"
    "runtime"
    "sort"
    "strconv"
    "strings"
    "sync"
    "time"
)

const pcOSVersion = "5.1.0"

var (
    pcOSStartedAt = time.Now().UTC()
    pcOSStateMu   sync.Mutex
)

func pcOSActiveRoot() string {
    exe, err := os.Executable()
    if err != nil {
        return filepath.Clean(`C:\ProgramData\ZhiroxAI`)
    }
    abs, err := filepath.Abs(exe)
    if err != nil {
        return filepath.Clean(`C:\ProgramData\ZhiroxAI`)
    }
    return filepath.Clean(filepath.Dir(abs))
}

func pcOSManagedRoots() []string {
    roots := []string{
        pcOSActiveRoot(),
        filepath.Clean(`C:\ProgramData\ZhiroxAI`),
        filepath.Clean(`D:\DataBase`),
    }
    seen := map[string]bool{}
    out := make([]string, 0, len(roots))
    for _, root := range roots {
        k := strings.ToLower(filepath.Clean(root))
        if k == "" || seen[k] {
            continue
        }
        seen[k] = true
        out = append(out, filepath.Clean(root))
    }
    return out
}

func pcOSPathWithin(path, root string) bool {
    p := strings.ToLower(filepath.Clean(path))
    r := strings.TrimRight(strings.ToLower(filepath.Clean(root)), `\/`)
    return p == r || strings.HasPrefix(p, r+string(os.PathSeparator)) || strings.HasPrefix(p, r+`\`)
}

func pcOSReadOnlyPath(path string) (string, error) {
    if strings.TrimSpace(path) == "" {
        path = pcOSActiveRoot()
    }
    abs, err := filepath.Abs(path)
    if err != nil {
        return "", err
    }
    abs = filepath.Clean(abs)
    for _, root := range pcOSManagedRoots() {
        if pcOSPathWithin(abs, root) {
            return abs, nil
        }
    }
    return "", errors.New("pc_os read-only file manager is restricted to managed roots")
}

func pcOSHashFile(path string, maxBytes int64) map[string]interface{} {
    result := map[string]interface{}{"path": filepath.Clean(path)}
    f, err := os.Open(path)
    if err != nil {
        result["exists"] = false
        result["error"] = err.Error()
        return result
    }
    defer f.Close()

    info, err := f.Stat()
    if err != nil {
        result["error"] = err.Error()
        return result
    }
    result["exists"] = true
    result["size"] = info.Size()
    result["modified_utc"] = info.ModTime().UTC().Format(time.RFC3339)
    result["mode"] = info.Mode().String()
    if !info.Mode().IsRegular() {
        result["hash_skipped"] = "not_regular_file"
        return result
    }
    if maxBytes > 0 && info.Size() > maxBytes {
        result["hash_skipped"] = "size_limit"
        return result
    }

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        result["error"] = err.Error()
        return result
    }
    result["sha256"] = hex.EncodeToString(h.Sum(nil))
    return result
}

func pcOSModules() (interface{}, error) {
    return map[string]interface{}{
        "version": pcOSVersion,
        "mode": "enterprise-safe",
        "zero_ui": true,
        "arbitrary_shell": false,
        "critical_protection_lock": true,
        "modules": []string{
            "guardian",
            "smart_backup_vault",
            "business_continuity",
            "signed_update_manager",
            "disk_intelligence",
            "network_control_center",
            "security_sentinel",
            "remote_file_manager_read_only",
            "inventory_asset_manager",
            "local_ai_operator",
            "zero_ui_automation_engine",
            "disaster_recovery",
        },
    }, nil
}

func pcOSGuardian() (interface{}, error) {
    critical := []string{
        `D:\DataBase\Admin.exe`,
        `D:\DataBase\Cashier.exe`,
        filepath.Join(pcOSActiveRoot(), "ZhiroxAISupervisor.exe"),
    }
    files := make([]map[string]interface{}, 0, len(critical))
    allPresent := true
    for _, p := range critical {
        item := pcOSHashFile(p, 128*1024*1024)
        if exists, _ := item["exists"].(bool); !exists {
            allPresent = false
        }
        files = append(files, item)
    }
    return map[string]interface{}{
        "module": "guardian",
        "healthy": allPresent,
        "critical_assets": files,
        "mutation_policy": "deny_by_default",
        "protected_business_root": filepath.Clean(`D:\DataBase`),
        "auto_recovery": "start-only for critical services; stop/restart remains locked",
    }, nil
}

func pcOSBackupVault() (interface{}, error) {
    roots := pcOSManagedRoots()
    return map[string]interface{}{
        "module": "smart_backup_vault",
        "status": "ready_with_safety_gate",
        "managed_roots": roots,
        "agent_state_backup": map[string]interface{}{
            "source": pcOSActiveRoot(),
            "strategy": "versioned + checksum + atomic manifest",
        },
        "business_database": map[string]interface{}{
            "root": filepath.Clean(`D:\DataBase`),
            "strategy": "database-native backup required",
            "live_file_copy": false,
            "reason": "production database files must never be copied as a crash-consistency substitute",
        },
        "retention": map[string]interface{}{
            "daily": 7,
            "weekly": 4,
            "monthly": 3,
            "immutable_copy_recommended": true,
        },
        "destructive_restore": false,
    }, nil
}

func pcOSBusinessContinuity() (interface{}, error) {
    assets := []string{`D:\DataBase\Admin.exe`, `D:\DataBase\Cashier.exe`}
    checks := make([]map[string]interface{}, 0, len(assets))
    for _, p := range assets {
        info := pcOSHashFile(p, 0)
        delete(info, "sha256")
        checks = append(checks, info)
    }
    return map[string]interface{}{
        "module": "business_continuity",
        "critical_assets": checks,
        "policy": map[string]interface{}{
            "critical_process_stop": false,
            "critical_process_restart": false,
            "critical_service_start_for_recovery": true,
            "critical_service_disable": false,
            "database_mutation": false,
            "hikvision_mutation": false,
        },
        "recovery_order": []string{
            "network reachability",
            "SQL service availability",
            "business executable presence/integrity",
            "application availability",
            "remote relay heartbeat",
        },
    }, nil
}

type pcOSUpdateManifest struct {
    Version   string `json:"version"`
    File      string `json:"file"`
    SHA256    string `json:"sha256"`
    Size      int64  `json:"size"`
    Signature string `json:"signature"`
}

func pcOSLoadPublicKey() (ed25519.PublicKey, error) {
    p := filepath.Join(pcOSActiveRoot(), "update-public-key.hex")
    raw, err := os.ReadFile(p)
    if err != nil {
        return nil, fmt.Errorf("update public key unavailable: %w", err)
    }
    b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
    if err != nil || len(b) != ed25519.PublicKeySize {
        return nil, errors.New("invalid Ed25519 update public key")
    }
    return ed25519.PublicKey(b), nil
}

func pcOSVerifyUpdateWithKey(manifest pcOSUpdateManifest, artifact string, pub ed25519.PublicKey) (map[string]interface{}, error) {
    if manifest.Version == "" || manifest.File == "" || manifest.SHA256 == "" || manifest.Signature == "" {
        return nil, errors.New("signed update manifest is incomplete")
    }
    f, err := os.Open(artifact)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    info, err := f.Stat()
    if err != nil {
        return nil, err
    }
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return nil, err
    }
    gotHash := hex.EncodeToString(h.Sum(nil))
    if !strings.EqualFold(gotHash, manifest.SHA256) {
        return nil, errors.New("update SHA-256 mismatch")
    }
    if manifest.Size > 0 && info.Size() != manifest.Size {
        return nil, errors.New("update size mismatch")
    }
    sig, err := base64.StdEncoding.DecodeString(manifest.Signature)
    if err != nil {
        return nil, errors.New("invalid update signature encoding")
    }
    signed := []byte(manifest.Version + "|" + strings.ToLower(manifest.SHA256) + "|" + strconv.FormatInt(info.Size(), 10))
    if !ed25519.Verify(pub, signed, sig) {
        return nil, errors.New("Ed25519 update signature verification failed")
    }
    return map[string]interface{}{
        "verified": true,
        "version": manifest.Version,
        "file": artifact,
        "sha256": gotHash,
        "size": info.Size(),
        "signature": "ed25519_verified",
        "rollback_required_on_health_failure": true,
    }, nil
}

func pcOSVerifyUpdate(manifestPath, artifactPath string) (interface{}, error) {
    manifestPath, err := pcOSReadOnlyPath(manifestPath)
    if err != nil {
        return nil, err
    }
    artifactPath, err = pcOSReadOnlyPath(artifactPath)
    if err != nil {
        return nil, err
    }
    raw, err := os.ReadFile(manifestPath)
    if err != nil {
        return nil, err
    }
    var manifest pcOSUpdateManifest
    if err := json.Unmarshal(raw, &manifest); err != nil {
        return nil, err
    }
    pub, err := pcOSLoadPublicKey()
    if err != nil {
        return nil, err
    }
    return pcOSVerifyUpdateWithKey(manifest, artifactPath, pub)
}

func pcOSUpdateChain() (interface{}, error) {
    root := pcOSActiveRoot()
    _, keyErr := pcOSLoadPublicKey()
    healthPath := filepath.Join(root, "update-health.json")
    health := map[string]interface{}{"path": healthPath}
    if raw, err := os.ReadFile(healthPath); err == nil {
        var v interface{}
        if json.Unmarshal(raw, &v) == nil {
            health["data"] = v
        } else {
            health["present"] = true
        }
    } else {
        health["present"] = false
    }
    return map[string]interface{}{
        "module": "signed_update_manager",
        "sha256_required": true,
        "ed25519_signature_required": true,
        "public_key_ready": keyErr == nil,
        "fail_closed_without_key_or_signature": true,
        "staging_required": true,
        "post_update_health_gate": "3 successful heartbeats",
        "rollback_required": true,
        "old_version_cleanup_after_health_gate": true,
        "health": health,
    }, nil
}

type pcOSFileEntry struct {
    Path       string `json:"path"`
    Size       int64  `json:"size"`
    Modified  string `json:"modified_utc"`
    Directory bool   `json:"directory"`
}

func pcOSBoundedScan(root string, maxEntries, maxDepth int) ([]pcOSFileEntry, error) {
    root = filepath.Clean(root)
    baseDepth := strings.Count(root, string(os.PathSeparator))
    entries := make([]pcOSFileEntry, 0, 128)
    err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return nil
        }
        if len(entries) >= maxEntries {
            if d.IsDir() && path != root {
                return filepath.SkipDir
            }
            return nil
        }
        depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - baseDepth
        if d.IsDir() && depth > maxDepth {
            return filepath.SkipDir
        }
        if d.Type()&os.ModeSymlink != 0 {
            if d.IsDir() {
                return filepath.SkipDir
            }
            return nil
        }
        if path == root {
            return nil
        }
        info, ierr := d.Info()
        if ierr != nil {
            return nil
        }
        entries = append(entries, pcOSFileEntry{
            Path: filepath.Clean(path),
            Size: info.Size(),
            Modified: info.ModTime().UTC().Format(time.RFC3339),
            Directory: info.IsDir(),
        })
        return nil
    })
    return entries, err
}

func pcOSDiskIntelligence() (interface{}, error) {
    targets := []string{pcOSActiveRoot(), filepath.Clean(`D:\DataBase`)}
    out := map[string]interface{}{
        "module": "disk_intelligence",
        "delete_policy": "never delete unknown files automatically",
        "warning_free_percent": 15,
        "critical_free_percent": 10,
    }
    scans := []map[string]interface{}{}
    for _, root := range targets {
        scan := map[string]interface{}{"root": root}
        entries, err := pcOSBoundedScan(root, 2000, 2)
        if err != nil {
            scan["error"] = err.Error()
            scans = append(scans, scan)
            continue
        }
        files := make([]pcOSFileEntry, 0, len(entries))
        var bytes int64
        for _, e := range entries {
            if !e.Directory {
                files = append(files, e)
                bytes += e.Size
            }
        }
        sort.Slice(files, func(i, j int) bool { return files[i].Size > files[j].Size })
        if len(files) > 20 {
            files = files[:20]
        }
        scan["entries_scanned"] = len(entries)
        scan["file_bytes_scanned"] = bytes
        scan["largest_files"] = files
        scans = append(scans, scan)
    }
    out["scans"] = scans
    return out, nil
}

func pcOSNetwork() (interface{}, error) {
    ifaces, err := net.Interfaces()
    if err != nil {
        return nil, err
    }
    adapters := []map[string]interface{}{}
    for _, i := range ifaces {
        addrs, _ := i.Addrs()
        xs := make([]string, 0, len(addrs))
        for _, a := range addrs {
            xs = append(xs, a.String())
        }
        adapters = append(adapters, map[string]interface{}{
            "name": i.Name,
            "index": i.Index,
            "mtu": i.MTU,
            "flags": i.Flags.String(),
            "addresses": xs,
            "up": i.Flags&net.FlagUp != 0,
        })
    }
    relayHost := "vllwundrtrgdqmkqbduy.supabase.co:443"
    started := time.Now()
    conn, dialErr := net.DialTimeout("tcp", relayHost, 4*time.Second)
    latency := time.Since(started).Milliseconds()
    if conn != nil {
        _ = conn.Close()
    }
    return map[string]interface{}{
        "module": "network_control_center",
        "adapters": adapters,
        "relay_target": relayHost,
        "relay_tcp_reachable": dialErr == nil,
        "relay_connect_ms": latency,
        "relay_error": func() string { if dialErr != nil { return dialErr.Error() }; return "" }(),
        "automatic_reconnect": true,
        "shellless": true,
    }, nil
}

func pcOSSecurity() (interface{}, error) {
    targets := []string{
        filepath.Join(pcOSActiveRoot(), "ZhiroxAISupervisor.exe"),
        `D:\DataBase\Admin.exe`,
        `D:\DataBase\Cashier.exe`,
    }
    hashes := make([]map[string]interface{}, 0, len(targets))
    for _, p := range targets {
        hashes = append(hashes, pcOSHashFile(p, 128*1024*1024))
    }
    return map[string]interface{}{
        "module": "security_sentinel",
        "integrity": hashes,
        "arbitrary_shell": false,
        "protected_business_root": true,
        "tamper_policy": "report + refuse mutation",
        "usb_policy": "audit-only unless separately approved",
        "firewall_policy": "observe by default",
        "defender_policy": "observe/update/scan only through structured actions",
    }, nil
}

func pcOSFileManager(path string) (interface{}, error) {
    target, err := pcOSReadOnlyPath(path)
    if err != nil {
        return nil, err
    }
    info, err := os.Stat(target)
    if err != nil {
        return nil, err
    }
    if !info.IsDir() {
        return pcOSHashFile(target, 128*1024*1024), nil
    }
    entries, err := os.ReadDir(target)
    if err != nil {
        return nil, err
    }
    out := make([]map[string]interface{}, 0, len(entries))
    limit := len(entries)
    if limit > 500 {
        limit = 500
    }
    for _, e := range entries[:limit] {
        item := map[string]interface{}{
            "name": e.Name(),
            "directory": e.IsDir(),
        }
        if i, ierr := e.Info(); ierr == nil {
            item["size"] = i.Size()
            item["modified_utc"] = i.ModTime().UTC().Format(time.RFC3339)
            item["mode"] = i.Mode().String()
        }
        out = append(out, item)
    }
    return map[string]interface{}{
        "module": "remote_file_manager",
        "path": target,
        "read_only": true,
        "entries": out,
        "truncated": len(entries) > limit,
    }, nil
}

func pcOSInventory() (interface{}, error) {
    hostname, _ := os.Hostname()
    ifaces, _ := net.Interfaces()
    network := make([]map[string]interface{}, 0, len(ifaces))
    for _, i := range ifaces {
        network = append(network, map[string]interface{}{
            "name": i.Name,
            "mac": i.HardwareAddr.String(),
            "mtu": i.MTU,
            "flags": i.Flags.String(),
        })
    }
    return map[string]interface{}{
        "module": "inventory_asset_manager",
        "hostname": hostname,
        "os": runtime.GOOS,
        "arch": runtime.GOARCH,
        "logical_cpu": runtime.NumCPU(),
        "go_runtime": runtime.Version(),
        "agent_root": pcOSActiveRoot(),
        "network": network,
        "inventory_policy": "read_only",
    }, nil
}

func pcOSZeroUI() (interface{}, error) {
    return map[string]interface{}{
        "module": "zero_ui_automation_engine",
        "zero_ui": true,
        "windows_gui_binary": true,
        "shell_execution": false,
        "cmd_window": false,
        "powershell_window": false,
        "popup_required": false,
        "structured_actions_only": true,
        "critical_mutations_locked": true,
    }, nil
}

func pcOSDisasterRecovery() (interface{}, error) {
    return map[string]interface{}{
        "module": "disaster_recovery",
        "mode": "plan_and_verify",
        "automatic_destructive_restore": false,
        "recovery_sequence": []string{
            "verify hardware/storage health",
            "restore Windows/agent prerequisites if needed",
            "restore Zhirox Supervisor from signed known-good artifact",
            "verify network and relay heartbeat",
            "restore SQL using database-native verified backup",
            "verify business apps without modifying binaries",
            "run read-only health audit",
            "return to service only after health gate",
        },
        "requirements": []string{
            "signed known-good Supervisor artifact",
            "offline recovery media",
            "verified database-native backup",
            "configuration export",
            "documented rollback point",
        },
    }, nil
}

func pcOSLocalAI() (interface{}, error) {
    findings := []map[string]interface{}{}
    guardRaw, _ := pcOSGuardian()
    if guard, ok := guardRaw.(map[string]interface{}); ok {
        if healthy, _ := guard["healthy"].(bool); !healthy {
            findings = append(findings, map[string]interface{}{
                "severity": "critical",
                "code": "critical_asset_missing",
                "recommendation": "keep system read-only and investigate the missing business/agent asset before any repair",
            })
        }
    }
    netRaw, netErr := pcOSNetwork()
    if netErr != nil {
        findings = append(findings, map[string]interface{}{
            "severity": "warning",
            "code": "network_probe_failed",
            "detail": netErr.Error(),
        })
    } else if n, ok := netRaw.(map[string]interface{}); ok {
        if reachable, _ := n["relay_tcp_reachable"].(bool); !reachable {
            findings = append(findings, map[string]interface{}{
                "severity": "warning",
                "code": "relay_unreachable",
                "recommendation": "classify internet/DNS/relay path before changing the PC",
            })
        }
    }
    if len(findings) == 0 {
        findings = append(findings, map[string]interface{}{
            "severity": "info",
            "code": "no_local_guardian_fault",
            "recommendation": "continue telemetry and read-only monitoring",
        })
    }
    return map[string]interface{}{
        "module": "local_ai_operator",
        "engine": "deterministic safety rules",
        "autonomous_destructive_actions": false,
        "findings": findings,
    }, nil
}

func pcOSOverview() (interface{}, error) {
    modules, _ := pcOSModules()
    inventory, _ := pcOSInventory()
    guardian, _ := pcOSGuardian()
    network, _ := pcOSNetwork()
    update, _ := pcOSUpdateChain()
    return map[string]interface{}{
        "pc_os_version": pcOSVersion,
        "generated_utc": time.Now().UTC().Format(time.RFC3339),
        "uptime_seconds": int64(time.Since(pcOSStartedAt).Seconds()),
        "modules": modules,
        "inventory": inventory,
        "guardian": guardian,
        "network": network,
        "update_chain": update,
        "safe_mode": true,
        "destructive_action_executed": false,
    }, nil
}

func pcOSBackgroundSnapshot() map[string]interface{} {
    hostname, _ := os.Hostname()
    ifaces, _ := net.Interfaces()
    up := []string{}
    for _, i := range ifaces {
        if i.Flags&net.FlagUp != 0 && i.Flags&net.FlagLoopback == 0 {
            up = append(up, i.Name)
        }
    }
    return map[string]interface{}{
        "version": pcOSVersion,
        "timestamp_utc": time.Now().UTC().Format(time.RFC3339),
        "hostname": hostname,
        "network_up": up,
        "zero_ui": true,
        "critical_lock": true,
    }
}

func pcOSWriteAtomicJSON(path string, value interface{}) error {
    raw, err := json.MarshalIndent(value, "", "  ")
    if err != nil {
        return err
    }
    pcOSStateMu.Lock()
    defer pcOSStateMu.Unlock()
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, raw, 0600); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}

func pcOSBackgroundLoop() {
    time.Sleep(30 * time.Second)
    ticker := time.NewTicker(2 * time.Minute)
    defer ticker.Stop()
    for {
        statePath := filepath.Join(pcOSActiveRoot(), "pc-os-health.json")
        if err := pcOSWriteAtomicJSON(statePath, pcOSBackgroundSnapshot()); err != nil {
            // No UI and no recovery mutation here. Existing agent logging may surface this.
        }
        <-ticker.C
    }
}
