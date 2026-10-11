package main

import (
    "strings"
    "testing"
)

func TestPCOSBackupDatabaseNamePolicy(t *testing.T) {
    valid := []string{"BusinessDB", "Business_DB-1", "Cashier DB (Main)"}
    for _, v := range valid {
        if got, err := pcOSValidateDatabaseName(v); err != nil || got != v {
            t.Fatalf("expected valid database %q, got %q err=%v", v, got, err)
        }
    }
    invalid := []string{"", "db/name", "db;drop", "db]x", strings.Repeat("a", 129)}
    for _, v := range invalid {
        if _, err := pcOSValidateDatabaseName(v); err == nil {
            t.Fatalf("expected database name %q to be rejected", v)
        }
    }
}

func TestPCOSBackupConfirmationIsExact(t *testing.T) {
    if err := pcOSValidateBackupConfirm("BusinessDB", "BACKUP BusinessDB"); err != nil {
        t.Fatal(err)
    }
    for _, v := range []string{"backup BusinessDB", "BACKUP  BusinessDB", "BACKUP OtherDB", "BACKUP BusinessDB now"} {
        if err := pcOSValidateBackupConfirm("BusinessDB", v); err == nil {
            t.Fatalf("expected confirmation %q to be rejected", v)
        }
    }
}

func TestPCOSBackupPathVaultBoundary(t *testing.T) {
    allowed := []string{
        `D:\ZhiroxBackups\BusinessDB\a.bak`,
        `d:/ZhiroxBackups/BusinessDB/a.bak`,
        `D:\ZhiroxBackups\BusinessDB\folder\..\a.bak`,
    }
    for _, v := range allowed {
        got, err := pcOSValidateBackupPath(v)
        if err != nil {
            t.Fatalf("expected path %q to be allowed: %v", v, err)
        }
        if !strings.HasPrefix(strings.ToLower(got), `d:\zhiroxbackups\`) || !strings.HasSuffix(strings.ToLower(got), ".bak") {
            t.Fatalf("unexpected normalized path %q", got)
        }
    }

    rejected := []string{
        `D:\DataBase\BusinessDB.bak`,
        `C:\Temp\BusinessDB.bak`,
        `D:\ZhiroxBackups\..\Windows\BusinessDB.bak`,
        `\\server\share\BusinessDB.bak`,
        `D:\ZhiroxBackups\BusinessDB\a.txt`,
        `D:\ZhiroxBackups\BusinessDB\a.bak:stream`,
        `relative\BusinessDB.bak`,
    }
    for _, v := range rejected {
        if _, err := pcOSValidateBackupPath(v); err == nil {
            t.Fatalf("expected path %q to be rejected", v)
        }
    }
}

func TestPCOSLocalSQLServerPolicy(t *testing.T) {
    hostname := "Server"
    allowed := []string{"", "localhost", ".", "(local)", "127.0.0.1", "Server", `.\SQLEXPRESS`, `Server\SQL2019`}
    for _, v := range allowed {
        if _, err := pcOSValidateLocalSQLServer(v, hostname); err != nil {
            t.Fatalf("expected SQL target %q to be allowed: %v", v, err)
        }
    }
    rejected := []string{"10.0.0.2", "remote-host", "localhost,1433", "tcp:localhost", `Server\bad instance!`, `\\server\pipe`}
    for _, v := range rejected {
        if _, err := pcOSValidateLocalSQLServer(v, hostname); err == nil {
            t.Fatalf("expected SQL target %q to be rejected", v)
        }
    }
}

func TestPCOSSQLQuoting(t *testing.T) {
    if got := pcOSQuoteSQLIdentifier("a]b"); got != "[a]]b]" {
        t.Fatalf("unexpected identifier quote %q", got)
    }
    if got := pcOSQuoteSQLString("O'Brien"); got != "N'O''Brien'" {
        t.Fatalf("unexpected string quote %q", got)
    }
}
