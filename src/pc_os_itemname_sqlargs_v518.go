package main

import (
    "errors"
    "strings"
)

// pcOSItemNameSQLCmdArgs builds a fixed, read-only ItemName connection.
// Do not add -C (TrustServerCertificate), a plaintext fallback, SQL passwords,
// arbitrary database selection, or an arbitrary remote SQL Server.
func pcOSItemNameSQLCmdArgs(database, query, certificateHost string) ([]string, error) {
    if database != "SuperMarket" {
        return nil, errors.New("ItemName monitor database must be SuperMarket")
    }
    if strings.TrimSpace(query) == "" {
        return nil, errors.New("ItemName monitor query is empty")
    }
    if len(certificateHost) == 0 || len(certificateHost) > 253 {
        return nil, errors.New("invalid SQL certificate hostname length")
    }
    for _, label := range strings.Split(certificateHost, ".") {
        if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
            return nil, errors.New("invalid SQL certificate hostname label")
        }
        for _, c := range label {
            if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
                (c >= '0' && c <= '9') || c == '-') {
                return nil, errors.New("invalid SQL certificate hostname character")
            }
        }
    }
    // -N asks SQLCMD ODBC for encrypted transport; without -C the driver's
    // normal trust-chain validation remains enabled. -F specifies the name to
    // check in the certificate; an unknown/mismatched certificate must fail.
    return []string{
        "-S", "localhost", "-E", "-N", "-F", certificateHost, "-d", "SuperMarket",
        "-b", "-h", "-1", "-w", "8192", "-y", "8192",
        "-l", "10", "-t", "90", "-r", "1", "-Q", query,
    }, nil
}
