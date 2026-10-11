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
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

const (
    pcOSBackupDefaultRoot = `D:\ZhiroxBackups`
    pcOSBackupMaxOutput   = 1024 * 1024
)

var pcOSDBNameRE = regexp.MustCompile(`^[A-Za-z0-9_ .()\-]{1,128}$`)

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
    if p, err := exec.LookPath("sqlcmd.exe"); err == nil {
        return filepath.Clean(p), nil
    }
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
    return "", errors.New("sqlcmd.exe not found")
}

func pcOSValidateDatabaseName(name string) (string, error) {
    name = strings.TrimSpace(name)
    if !pcOSDBNameRE.MatchString(name) {
        return "", errors.New("database name contains unsupported characters")
    }
    return name, nil
}

func pcOSQuoteSQLIdentifier(name string) string {
    return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func pcOSQuoteSQLString(value string) string {
    return "N'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func pcOSBackupDestination(raw, database string) (string, error) {
    if strings.TrimSpace(raw) == "" {
        stamp := time.Now().UTC().Format("20060102T150405Z")
        raw = filepath.Join(pcOSBackupDefaultRoot, database, database+"-"+stamp+".bak")
    }
    abs, err := filepath.Abs(raw)
    if err != nil {
        return "", err
    }
    abs = filepath.Clean(abs)
    if !filepath.IsAbs(abs) {
        return "", errors.New("backup destination must be absolute")
    }
    if pcOSPathWithin(abs, filepath.Clean(`D:\DataBase`)) {
        return "", errors.New("backup destination cannot be inside protected production root D:\\DataBase")
    }
    if strings.ToLower(filepath.Ext(abs)) != ".bak" {
        return "", errors.New("SQL backup destination must use .bak extension")
    }
    return abs, nil
}

func pcOSRunSQLCmd(ctx context.Context, sqlcmd, server, query string) (string, error) {
    if strings.TrimSpace(server) == "" {
        server = "localhost"
    }
    // No shell: sqlcmd.exe is launched directly and hidden by the existing Zero-UI helper.
    cmd := hiddenCommandContext(ctx, sqlcmd,
        "-S", server,
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
    err := cmd.Run()
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
        "protected_database_root":    filepath.Clean(`D:\DataBase`),
        "execute_requires_approval":  true,
        "verify_only_supported":      true,
        "hash_catalog_supported":     true,
    }
    if sqlErr != nil {
        result["sqlcmd_ready"] = false
        result["sqlcmd_error"] = sqlErr.Error()
        return result, nil
    }
    result["sqlcmd_ready"] = true
    result["sqlcmd_path"] = sqlcmd

    server := sarg(args, "server")
    if strings.TrimSpace(server) == "" {
        server = "localhost"
    }
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
    rawPath := sarg(args, "path")
    if strings.TrimSpace(rawPath) == "" {
        return nil, errors.New("path is required")
    }
    abs, err := filepath.Abs(rawPath)
    if err != nil {
        return nil, err
    }
    abs = filepath.Clean(abs)
    if strings.ToLower(filepath.Ext(abs)) != ".bak" {
        return nil, errors.New("verify path must be a .bak file")
    }
    if pcOSPathWithin(abs, filepath.Clean(`D:\DataBase`)) {
        return nil, errors.New("protected production root is not a backup verification target")
    }
    info, err := os.Stat(abs)
    if err != nil {
        return nil, err
    }
    if !info.Mode().IsRegular() {
        return nil, errors.New("backup target is not a regular file")
    }

    sqlcmd, err := pcOSFindSQLCmd()
    if err != nil {
        return nil, err
    }
    server := sarg(args, "server")
    if strings.TrimSpace(server) == "" {
        server = "localhost"
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
    }, nil
}

func pcOSSQLBackupExecute(args map[string]interface{}) (interface{}, error) {
    database, err := pcOSValidateDatabaseName(sarg(args, "database"))
    if err != nil {
        return nil, err
    }
    confirm := strings.TrimSpace(sarg(args, "confirm"))
    expectedConfirm := "BACKUP " + database
    if confirm != expectedConfirm {
        return nil, fmt.Errorf("explicit confirmation required: %s", expectedConfirm)
    }
    destination, err := pcOSBackupDestination(sarg(args, "destination"), database)
    if err != nil {
        return nil, err
    }

    sqlcmd, err := pcOSFindSQLCmd()
    if err != nil {
        return nil, err
    }
    server := sarg(args, "server")
    if strings.TrimSpace(server) == "" {
        server = "localhost"
    }

    if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
        return nil, fmt.Errorf("create backup directory: %w", err)
    }
    if _, err := os.Stat(destination); err == nil {
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
        "timeout_minutes":   timeoutMinutes,
    }, nil
}
