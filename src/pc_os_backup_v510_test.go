package main

import (
    "strings"
    "testing"
)

func TestPCOSBackupDatabaseNameValidation(t *testing.T) {
    good := []string{"BusinessDB", "Cashier 2026", "DB_01", "Sales-Prod"}
    for _, name := range good {
        got, err := pcOSValidateDatabaseName(name)
        if err != nil || got != name {
            t.Fatalf("expected valid database name %q: got=%q err=%v", name, got, err)
        }
    }
    bad := []string{"", "db]; DROP DATABASE master;--", "db' OR 1=1 --", "db/other", strings.Repeat("x", 129)}
    for _, name := range bad {
        if _, err := pcOSValidateDatabaseName(name); err == nil {
            t.Fatalf("expected invalid database name %q to fail", name)
        }
    }
}

func TestPCOSBackupSQLQuoting(t *testing.T) {
    if got := pcOSQuoteSQLIdentifier("A]B"); got != "[A]]B]" {
        t.Fatalf("identifier quote mismatch: %q", got)
    }
    if got := pcOSQuoteSQLString(`D:\Backups\O'Brien.bak`); got != `N'D:\Backups\O''Brien.bak'` {
        t.Fatalf("string quote mismatch: %q", got)
    }
}

func TestPCOSCappedBuffer(t *testing.T) {
    b := &pcOSCappedBuffer{max: 5}
    n, err := b.Write([]byte("123456789"))
    if err != nil || n != 9 {
        t.Fatalf("write contract mismatch: n=%d err=%v", n, err)
    }
    if got := b.String(); got != "12345" {
        t.Fatalf("cap mismatch: %q", got)
    }
}
