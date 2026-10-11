package main

import (
    "errors"
    "path"
    "regexp"
    "strings"
    "time"
)

const pcOSBackupDefaultRoot = `D:\ZhiroxBackups`

var (
    pcOSDBNameRE      = regexp.MustCompile(`^[A-Za-z0-9_ .()\-]{1,128}$`)
    pcOSSQLInstanceRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func pcOSValidateDatabaseName(name string) (string, error) {
    name = strings.TrimSpace(name)
    if !pcOSDBNameRE.MatchString(name) {
        return "", errors.New("database name contains unsupported characters")
    }
    return name, nil
}

func pcOSQuoteSQLIdentifier(name string) string {
    return "[" + strings.ReplaceAll(name, "]", "]]" ) + "]"
}

func pcOSQuoteSQLString(value string) string {
    return "N'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func pcOSValidateBackupConfirm(database, confirm string) error {
    expected := "BACKUP " + database
    if strings.TrimSpace(confirm) != expected {
        return errors.New("explicit confirmation required: " + expected)
    }
    return nil
}

func pcOSCanonicalWindowsPath(raw string) (string, error) {
    s := strings.TrimSpace(strings.ReplaceAll(raw, "/", `\`))
    if s == "" {
        return "", errors.New("path is required")
    }
    if strings.HasPrefix(s, `\\`) {
        return "", errors.New("UNC/device paths are not allowed for local backup vault operations")
    }
    if len(s) < 3 || !((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) || s[1] != ':' || s[2] != '\\' {
        return "", errors.New("backup path must be an absolute Windows drive path")
    }
    if strings.Contains(s[2:], ":") {
        return "", errors.New("alternate data streams are not allowed in backup paths")
    }

    cleaned := path.Clean(strings.ReplaceAll(s, `\`, "/"))
    cleaned = strings.ReplaceAll(cleaned, "/", `\`)
    if len(cleaned) < 3 || cleaned[1] != ':' || cleaned[2] != '\\' {
        return "", errors.New("backup path normalization failed")
    }
    cleaned = strings.ToUpper(cleaned[:1]) + cleaned[1:]
    return cleaned, nil
}

func pcOSValidateBackupPath(raw string) (string, error) {
    candidate, err := pcOSCanonicalWindowsPath(raw)
    if err != nil {
        return "", err
    }
    root, err := pcOSCanonicalWindowsPath(pcOSBackupDefaultRoot)
    if err != nil {
        return "", err
    }
    lowerCandidate := strings.ToLower(candidate)
    lowerRoot := strings.ToLower(root)
    prefix := lowerRoot + `\`
    if !strings.HasPrefix(lowerCandidate, prefix) {
        return "", errors.New("backup path must stay inside D:\\ZhiroxBackups")
    }
    relative := candidate[len(root)+1:]
    if relative == "" || strings.Contains(relative, `\`) {
        return "", errors.New("backup files must be stored directly in D:\\ZhiroxBackups; nested paths are not allowed")
    }
    if !strings.HasSuffix(lowerCandidate, ".bak") {
        return "", errors.New("SQL backup path must use .bak extension")
    }
    return candidate, nil
}

func pcOSBackupDestination(raw, database string) (string, error) {
    database, err := pcOSValidateDatabaseName(database)
    if err != nil {
        return "", err
    }
    if strings.TrimSpace(raw) == "" {
        stamp := time.Now().UTC().Format("20060102T150405Z")
        raw = pcOSBackupDefaultRoot + `\` + database + "-" + stamp + ".bak"
    }
    return pcOSValidateBackupPath(raw)
}

func pcOSValidateLocalSQLServer(raw, hostname string) (string, error) {
    server := strings.TrimSpace(raw)
    if server == "" {
        server = "localhost"
    }
    if strings.ContainsAny(server, "/,:;") {
        return "", errors.New("SQL backup target must be a local host or local named instance")
    }

    parts := strings.Split(server, `\`)
    if len(parts) > 2 || strings.TrimSpace(parts[0]) == "" {
        return "", errors.New("invalid local SQL Server target")
    }
    host := strings.TrimSpace(parts[0])
    localHost := strings.TrimSpace(hostname)
    allowedHost := host == "." || strings.EqualFold(host, "(local)") || strings.EqualFold(host, "localhost") || host == "127.0.0.1"
    if localHost != "" && strings.EqualFold(host, localHost) {
        allowedHost = true
    }
    if !allowedHost {
        return "", errors.New("remote SQL Server targets are not allowed by backup policy")
    }
    if len(parts) == 2 {
        instance := strings.TrimSpace(parts[1])
        if !pcOSSQLInstanceRE.MatchString(instance) {
            return "", errors.New("SQL named instance contains unsupported characters")
        }
        return host + `\` + instance, nil
    }
    return host, nil
}
