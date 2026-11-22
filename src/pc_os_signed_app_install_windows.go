//go:build windows

// This is a narrowly scoped signed *fresh-install* executor. It does NOT
// grant generic process launch, arbitrary script, arbitrary URL, or upgrades
// to running POS software. The existing Electron updater handles upgrades.
package main

import (
    "context"
    "crypto/sha256"
    "crypto/subtle"
    "encoding/hex"
    "errors"
    "io"
    "os"
    "os/exec"
    "os/user"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

var signedAppInstallMutex sync.Mutex
var signedAppInstallRecord = struct {
    Running bool
    State, AppID, Version, Started, Finished, ErrorCode string
}{State: "idle"}

func pcOSSignedAppInstallStatus() (interface{}, error) {
    signedAppInstallMutex.Lock()
    defer signedAppInstallMutex.Unlock()
    return map[string]interface{}{
        "action": "signed_app_install",
        "state": signedAppInstallRecord.State,
        "app_id": signedAppInstallRecord.AppID,
        "version": signedAppInstallRecord.Version,
        "started": signedAppInstallRecord.Started,
        "finished": signedAppInstallRecord.Finished,
        "error_code": signedAppInstallRecord.ErrorCode,
        "arbitrary_shell": false,
        "scope": "allowlisted_fresh_install_only",
    }, nil
}

func pcOSSignedAppInstall(args map[string]interface{}) (interface{}, error) {
    spec, _, err := validateSignedAppInstallArgs(args)
    if err != nil { return nil, err }
    // The connector's service-role RPC must have consumed a separate exact,
    // one-shot authenticated owner approval *before* enqueueing this command.
    signedAppInstallMutex.Lock()
    if signedAppInstallRecord.Running {
        signedAppInstallMutex.Unlock()
        return nil, errors.New("signed_install_already_running")
    }
    signedAppInstallRecord.Running = true
    signedAppInstallRecord.State = "accepted"
    signedAppInstallRecord.AppID = spec.AppID
    signedAppInstallRecord.Version = spec.Version
    signedAppInstallRecord.Started = time.Now().UTC().Format(time.RFC3339)
    signedAppInstallRecord.Finished = ""
    signedAppInstallRecord.ErrorCode = ""
    signedAppInstallMutex.Unlock()

    go func() {
        code := pcOSRunAllowlistedFreshInstall(spec)
        signedAppInstallMutex.Lock()
        defer signedAppInstallMutex.Unlock()
        signedAppInstallRecord.Running = false
        signedAppInstallRecord.Finished = time.Now().UTC().Format(time.RFC3339)
        signedAppInstallRecord.ErrorCode = code
        if code == "" {
            // Windows exit 0 alone cannot establish app functionality.
            signedAppInstallRecord.State = "installer_exited_zero_needs_app_verification"
        } else {
            signedAppInstallRecord.State = "blocked_or_failed"
        }
    }()
    return map[string]interface{}{
        "action": "signed_app_install",
        "state": "accepted",
        "app_id": spec.AppID,
        "version": spec.Version,
        "status_action": "signed_app_install_status",
        "physical_install_verified": false,
    }, nil
}

func pcOSRunAllowlistedFreshInstall(spec SignedAppInstallSpec) string {
    currentUser, err := user.Current()
    if err != nil { return "interactive_identity_unavailable" }
    identity := strings.ToLower(currentUser.Username)
    // The deployed Relay runs as NT AUTHORITY\SYSTEM. Electron's NSIS
    // package is per-user; a silent SYSTEM install would target the wrong
    // profile and must never be attempted. A future reviewed attended
    // user-session broker is required to enable this on the store server.
    if strings.Contains(identity, "system") ||
        strings.EqualFold(os.Getenv("USERNAME"), "SYSTEM") ||
        strings.EqualFold(os.Getenv("SESSIONNAME"), "Services") ||
        os.Getenv("LOCALAPPDATA") == "" {
        return "interactive_user_broker_required"
    }

    fixedDir := filepath.Clean("C:/ProgramData/ZhiroxAI/ApprovedInstallers")
    fi, err := os.Lstat(fixedDir)
    if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
        return "approved_staging_directory_missing"
    }
    stagedPath := filepath.Join(fixedDir, spec.Filename)
    fi, err = os.Lstat(stagedPath)
    if err != nil || !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 || fi.Size() != spec.Size {
        return "approved_artifact_missing_or_wrong_size"
    }

    // Untrusted writable staging files cannot be executed directly.
    // Copy into a private temporary directory, hash that same copy,
    // validate Authenticode, and execute ONLY the private verified copy.
    privateDir, err := os.MkdirTemp("", "zhirox-verified-signed-app-")
    if err != nil { return "private_staging_failed" }
    defer os.RemoveAll(privateDir)
    privatePath := filepath.Join(privateDir, spec.Filename)
    in, err := os.Open(stagedPath)
    if err != nil { return "staged_file_unreadable" }
    out, err := os.OpenFile(privatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if err != nil { in.Close(); return "private_copy_failed" }
    n, copyErr := io.Copy(out, io.LimitReader(in, spec.Size+1))
    closeErr := out.Close()
    in.Close()
    if copyErr != nil || closeErr != nil || n != spec.Size {
        return "private_copy_integrity_failed"
    }
    verifyFile, err := os.Open(privatePath)
    if err != nil { return "private_copy_missing" }
    h := sha256.New()
    _, err = io.Copy(h, verifyFile)
    verifyFile.Close()
    if err != nil { return "sha256_read_failed" }
    expected, err := hex.DecodeString(spec.SHA256)
    if err != nil || subtle.ConstantTimeCompare(h.Sum(nil), expected) != 1 {
        return "sha256_mismatch"
    }

    // Windows Authenticode validation uses a fixed local OS binary, not
    // caller-supplied PowerShell or an untrusted signing command.
    // The exact SHA256 pin also prevents same-subject rogue certificates.
    ps := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
    if os.Getenv("SystemRoot") == "" { return "windows_system_root_missing" }
    safePath := strings.ReplaceAll(privatePath, "'", "''")
    safeSubject := strings.ReplaceAll(spec.PublisherSubject, "'", "''")
    script := "$ErrorActionPreference='Stop'; " +
        "$s=Get-AuthenticodeSignature -LiteralPath '" + safePath + "'; " +
        "if ($s.Status -ne 'Valid' -or !$s.SignerCertificate -or $s.SignerCertificate.Subject -cne '" + safeSubject + "') { exit 41 }; " +
        "if ($s.SignerCertificate.NotBefore -gt (Get-Date) -or $s.SignerCertificate.NotAfter -le (Get-Date)) { exit 42 }; " +
        "$eku=@($s.SignerCertificate.Extensions | Where-Object { $_.Oid.Value -eq '2.5.29.37' }); " +
        "if ($eku.Count -ne 1) { exit 43 }; " +
        "$ids=@(([System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]$eku[0]).EnhancedKeyUsages | ForEach-Object { $_.Value }); " +
        "if ($ids -notcontains '1.3.6.1.5.5.7.3.3') { exit 44 }; exit 0"
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    cmd := exec.CommandContext(ctx, ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
    pcOSApplyZeroUI(cmd)
    verifyErr := cmd.Run()
    cancel()
    if verifyErr != nil { return "windows_authenticode_identity_or_trust_failed" }

    // A running POS must not be interrupted. Existing installations and
    // their data require Electron's own verified-backup update path.
    userInstall := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Cashierynwe POS")
    if _, err := os.Stat(userInstall); err == nil {
        return "existing_install_use_in_app_updater"
    }
    // Presence of any named cashier process is a hard block; this is NOT
    // a substitute for active-sale checks before a future upgrade feature.
    tasklist := filepath.Join(os.Getenv("SystemRoot"), "System32", "tasklist.exe")
    pCtx, pCancel := context.WithTimeout(context.Background(), 15*time.Second)
    proc := exec.CommandContext(pCtx, tasklist, "/FI", "IMAGENAME eq Cashierynwe POS.exe", "/FO", "CSV", "/NH")
    pcOSApplyZeroUI(proc)
    b, processErr := proc.Output()
    pCancel()
    if processErr != nil { return "running_app_check_unavailable" }
    if strings.Contains(strings.ToLower(string(b)), "cashierynwe pos.exe") { return "cashier_process_active" }

    installCtx, installCancel := context.WithTimeout(context.Background(), 4*time.Minute)
    defer installCancel()
    installer := exec.CommandContext(installCtx, privatePath, "/S")
    pcOSApplyZeroUI(installer)
    // NSIS runs only in the locally authenticated interactive user session,
    // never as the SYSTEM service or on an arbitrary supplied executable.
    if err := installer.Run(); err != nil { return "allowlisted_installer_failed" }
    return ""
}
