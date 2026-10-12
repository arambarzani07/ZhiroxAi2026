package main

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"
)

const pcOSBackupMaxOutput = 1024 * 1024

type pcOSCappedBuffer struct {
    buf bytes.Buffer
    max int
}

func (b *pcOSCappedBuffer) Write(p []byte) (int, error) {
    n := len(p)
    remaining := b.max - b.buf.Len()
    if remaining > 0 {
        if len(p) > remaining {
            _, _ = b.buf.Write(p[:remaining])
        } else {
            _, _ = b.buf.Write(p)
        }
    }
    return n, nil
}

func (b *pcOSCappedBuffer) String() string {
    return b.buf.String()
}

func pcOSFindSQLCmd() (string, error) {
    // Fail closed to Microsoft SQL Server installation roots. Do not trust an
    // arbitrary PATH-provided executable for a privileged database backup.
    roots := []string{
        `C:\Program Files\Microsoft SQL Server\Client SDK\ODBC`,
        `C:\Program Files\Microsoft SQL Server`,
        `C:\Program Files (x86)\Microsoft SQL Server`,
    }
    for _, root := range roots {
        var found string
        _ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
            if err != nil || d == nil {
                return nil
            }
            if d.Type()&os.ModeSymlink != 0 {
                if d.IsDir() {
                    return filepath.SkipDir
                }
                return nil
            }
            if !d.IsDir() && strings.EqualFold(d.Name(), "sqlcmd.exe") {
                found = filepath.Clean(path)
                return filepath.SkipAll
            }
            return nil
        })
        if found != "" {
            return found, nil
        }
    }
    return "", errors.New("trusted sqlcmd.exe not found under Microsoft SQL Server installation roots")
}

func pcOSResolveLocalSQLServer(raw string) (string, error) {
    hostname, _ := os.Hostname()
    return pcOSValidateLocalSQLServer(raw, hostname)
}

func pcOSBackupVaultRoot(create bool) (string, error) {
    root := filepath.Clean(pcOSBackupDefaultRoot)
    if create {
        if err := os.MkdirAll(root, 0700); err != nil {
            return "", fmt.Errorf("create backup vault root: %w", err)
        }
    }
    info, err := os.Lstat(root)
    if err != nil {
        return "", fmt.Errorf("backup vault root unavailable: %w", err)
    }
    if !info.IsDir() {
        return "", errors.New("backup vault root is not a directory")
    }
    if info.Mode()&os.ModeSymlink != 0 {
        return "", errors.New("backup vault root must not be a symlink or junction")
    }
    resolved, err := filepath.EvalSymlinks(root)
    if err != nil {
        return "", fmt.Errorf("resolve backup vault root: %w", err)
    }
    resolved = filepath.Clean(resolved)
    if !pcOSPathWithin(resolved, root) || !pcOSPathWithin(root, resolved) {
        return "", errors.New("backup vault root resolves outside its fixed path")
    }
    return root, nil
}

func pcOSValidateExistingVaultFile(path string) (string, error) {
    root, err := pcOSBackupVaultRoot(false)
    if err != nil {
        return "", err
    }
    info, err := os.Lstat(path)
    if err != nil {
        return "", err
    }
    if info.Mode()&os.ModeSymlink != 0 {
        return "", errors.New("backup file must not be a symlink or junction")
    }
    if !info.Mode().IsRegular() {
        return "", errors.New("backup target is not a regular file")
    }
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil {
        return "", fmt.Errorf("resolve backup file: %w", err)
    }
    resolved = filepath.Clean(resolved)
    if !pcOSPathWithin(resolved, root) {
        return "", errors.New("resolved backup file escaped the backup vault")
    }
    return resolved, nil
}

func pcOSRunSQLCmd(ctx context.Context, sqlcmd, server, query string) (string, error) {
    validatedServer, err := pcOSResolveLocalSQLServer(server)
    if err != nil {
        return "", err
    }
    // No shell: sqlcmd.exe is launched directly and hidden by the existing Zero-UI helper.
    cmd := hiddenCommandContext(ctx, sqlcmd,
        "-S", validatedServer,
        "-E",
        "-b",
        "-W",
        "-h", "-1",
        "-Q", query,
    )
    stdout := &pcOSCappedBuffer{max: pcOSBackupMaxOutput}
    stderr := &pcOSCappedBuffer{max: pcOSBackupMaxOutput}
    cmd.Stdout = stdout
    cmd.Stderr = stderr
    err = cmd.Run()
    out := strings.TrimSpace(stdout.String())
    serr := strings.TrimSpace(stderr.String())
    if err != nil {
        if serr != "" {
            return out, fmt.Errorf("sqlcmd failed: %s", serr)
        }
        return out, fmt.Errorf("sqlcmd failed: %w", err)
    }
    if serr != "" {
        out = strings.TrimSpace(out + "\n" + serr)
    }
    return out, nil
}

func pcOSSQLBackupReadiness(args map[string]interface{}) (interface{}, error) {
    sqlcmd, sqlErr := pcOSFindSQLCmd()
    result := map[string]interface{}{
        "module":                     "smart_backup_vault",
        "mode":                       "readiness",
        "read_only":                  true,
        "zero_ui":                    true,
        "database_native_required":   true,
        "raw_mdf_ldf_copy_supported": false,
        "default_backup_root":        pcOSBackupDefaultRoot,
        "write_scope":                 "D:\\ZhiroxBackups direct files only",
        "sql_target_scope":            "local host/instance only",
        "protected_database_root":    filepath.Clean(`D:\DataBase`),
        "execute_requires_approval":  true,
        "verify_only_supported":      true,
        "hash_catalog_supported":     true,
        "junction_escape_blocked":    true,
    }
    if _, vaultErr := pcOSBackupVaultRoot(false); vaultErr != nil {
        result["vault_root_ready"] = false
        result["vault_root_status"] = vaultErr.Error()
    } else {
        result["vault_root_ready"] = true
    }
    if sqlErr != nil {
        result["sqlcmd_ready"] = false
        result["sqlcmd_error"] = sqlErr.Error()
        return result, nil
    }
    result["sqlcmd_ready"] = true
    result["sqlcmd_path"] = sqlcmd

    server, err := pcOSResolveLocalSQLServer(sarg(args, "server"))
    if err != nil {
        result["server_policy_ok"] = false
        result["server_policy_error"] = err.Error()
        return result, nil
    }
    result["server_policy_ok"] = true
    result["server"] = server

    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    query := "SET NOCOUNT ON; SELECT name + '|' + state_desc + '|' + recovery_model_desc FROM sys.databases WHERE database_id > 4 ORDER BY name;"
    out, err := pcOSRunSQLCmd(ctx, sqlcmd, server, query)
    if err != nil {
        result["database_probe_ok"] = false
        result["database_probe_error"] = err.Error()
        return result, nil
    }
    result["database_probe_ok"] = true
    rows := []map[string]string{}
    for _, line := range strings.Split(out, "\n") {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        parts := strings.Split(line, "|")
        row := map[string]string{"raw": line}
        if len(parts) >= 1 {
            row["name"] = strings.TrimSpace(parts[0])
        }
        if len(parts) >= 2 {
            row["state"] = strings.TrimSpace(parts[1])
        }
        if len(parts) >= 3 {
            row["recovery_model"] = strings.TrimSpace(parts[2])
        }
        rows = append(rows, row)
    }
    result["databases"] = rows
    return result, nil
}

func pcOSSQLBackupVerify(args map[string]interface{}) (interface{}, error) {
    requested, err := pcOSValidateBackupPath(sarg(args, "path"))
    if err != nil {
        return nil, err
    }
    abs, err := pcOSValidateExistingVaultFile(requested)
    if err != nil {
        return nil, err
    }
    info, err := os.Stat(abs)
    if err != nil {
        return nil, err
    }

    sqlcmd, err := pcOSFindSQLCmd()
    if err != nil {
        return nil, err
    }
    server, err := pcOSResolveLocalSQLServer(sarg(args, "server"))
    if err != nil {
        return nil, err
    }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
    defer cancel()
    query := "RESTORE VERIFYONLY FROM DISK = " + pcOSQuoteSQLString(abs) + " WITH CHECKSUM;"
    output, err := pcOSRunSQLCmd(ctx, sqlcmd, server, query)
    if err != nil {
        return nil, err
    }

    h := sha256.New()
    f, err := os.Open(abs)
    if err != nil {
        return nil, err
    }
    _, copyErr := io.Copy(h, f)
    closeErr := f.Close()
    if copyErr != nil {
        return nil, copyErr
    }
    if closeErr != nil {
        return nil, closeErr
    }

    return map[string]interface{}{
        "module":        "smart_backup_vault",
        "verified":      true,
        "verify_method": "RESTORE VERIFYONLY WITH CHECKSUM",
        "path":          abs,
        "bytes":         info.Size(),
        "sha256":        hex.EncodeToString(h.Sum(nil)),
        "server":        server,
        "sql_output":    output,
        "read_only":     true,
        "zero_ui":       true,
        "vault_bound":   true,
        "symlink_safe":  true,
    }, nil
}

func pcOSSQLBackupExecute(args map[string]interface{}) (interface{}, error) {
    database, err := pcOSValidateDatabaseName(sarg(args, "database"))
    if err != nil {
        return nil, err
    }
    if err := pcOSValidateBackupConfirm(database, sarg(args, "confirm")); err != nil {
        return nil, err
    }
    destination, err := pcOSBackupDestination(sarg(args, "destination"), database)
    if err != nil {
        return nil, err
    }

    sqlcmd, err := pcOSFindSQLCmd()
    if err != nil {
        return nil, err
    }
    server, err := pcOSResolveLocalSQLServer(sarg(args, "server"))
    if err != nil {
        return nil, err
    }

    root, err := pcOSBackupVaultRoot(true)
    if err != nil {
        return nil, err
    }
    if !strings.EqualFold(filepath.Clean(filepath.Dir(destination)), filepath.Clean(root)) {
        return nil, errors.New("backup destination parent is outside the fixed backup vault root")
    }
    if _, err := os.Lstat(destination); err == nil {
        return nil, errors.New("refusing to overwrite an existing backup file")
    } else if !os.IsNotExist(err) {
        return nil, err
    }

    timeoutMinutes := iarg(args, "timeout_minutes", 120)
    if timeoutMinutes < 10 {
        timeoutMinutes = 10
    }
    if timeoutMinutes > 720 {
        timeoutMinutes = 720
    }
    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMinutes)*time.Minute)
    defer cancel()

    // Re-check the fixed vault root immediately before SQL Server receives the path.
    if _, err := pcOSBackupVaultRoot(false); err != nil {
        return nil, err
    }
    query := "BACKUP DATABASE " + pcOSQuoteSQLIdentifier(database) +
        " TO DISK = " + pcOSQuoteSQLString(destination) +
        " WITH COPY_ONLY, CHECKSUM, COMPRESSION, INIT, STATS = 10;"
    output, err := pcOSRunSQLCmd(ctx, sqlcmd, server, query)
    if err != nil {
        return nil, err
    }

    verifyRaw, err := pcOSSQLBackupVerify(map[string]interface{}{
        "path":   destination,
        "server": server,
    })
    if err != nil {
        return nil, fmt.Errorf("backup created but verification failed: %w", err)
    }
    verify, _ := verifyRaw.(map[string]interface{})

    return map[string]interface{}{
        "module":            "smart_backup_vault",
        "created":           true,
        "database":          database,
        "server":            server,
        "destination":       destination,
        "backup_type":       "SQL Server COPY_ONLY native full backup",
        "checksum":          true,
        "compression":       true,
        "sql_output":        output,
        "verification":      verify,
        "immutable":         false,
        "immutable_note":    "Hash-verified local backup; true WORM/immutable storage requires a separate immutable target.",
        "automatic_restore": false,
        "zero_ui":           true,
        "approval_bound":    true,
        "vault_bound":       true,
        "local_sql_only":    true,
        "symlink_safe":      true,
        "timeout_minutes":   timeoutMinutes,
    }, nil
}
