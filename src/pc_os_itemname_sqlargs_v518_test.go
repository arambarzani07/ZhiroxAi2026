package main

import (
    "reflect"
    "strings"
    "testing"
)

func TestItemNameSQLCmdTLSRequiresCertificateValidation(t *testing.T) {
    args, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, "Server")
    if err != nil { t.Fatal(err) }
    want := []string{"-S", "tcp:127.0.0.1,1433", "-E", "-N", "-F", "Server", "-d", "SuperMarket",
        "-b", "-h", "-1", "-w", "8192", "-y", "8192",
        "-l", "10", "-t", "90", "-r", "1", "-Q", pcOSItemNameTopOneSQL}
    if !reflect.DeepEqual(args, want) { t.Fatalf("wrong fixed sqlcmd args: %q", args) }
    for _, token := range args {
        if token == "-C" || token == "-No" || token == "-U" || token == "-P" {
            t.Fatalf("insecure sqlcmd flag %s", token)
        }
    }
}
func TestItemNameSQLCmdRejectsOtherDatabases(t *testing.T) {
    for _, db := range []string{"master", "SuperMarket;DROP DATABASE x", "", "OtherShop", "supermarket"} {
        if _, err := pcOSItemNameSQLCmdArgs(db, pcOSItemNameTopOneSQL, "Server"); err == nil {
            t.Fatalf("allowed unauthorized database: %q", db)
        }
    }
}
func TestItemNameSQLCmdRejectsUntrustedCertificateHosts(t *testing.T) {
    for _, h := range []string{"", "-Server", "Server-", "Server..local",
        "Server local", "Server\n-C", "127.0.0.1;evil", strings.Repeat("a", 64)} {
        if _, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, h); err == nil {
            t.Fatalf("accepted invalid certificate host %q", h)
        }
    }
    if _, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, "Server.office.local"); err != nil {
        t.Fatalf("rejected valid FQDN: %v", err)
    }
}

func TestItemNameSQLCmdAllowsOnlyFixedReadQueries(t *testing.T) {
 for _, q := range []string{pcOSItemNameSchemaProbeSQL,pcOSItemNameTopOneSQL,pcOSItemNameSelect} {
  if _,err:=pcOSItemNameSQLCmdArgs("SuperMarket",q,"Server");err!=nil{t.Fatalf("legitimate fixed query refused: %v",err)}
 }
 for _, q := range []string{"SELECT 1","SELECT * FROM dbo.ItemName","SELECT @@VERSION","SET NOCOUNT ON; DELETE FROM dbo.ItemName;","SET NOCOUNT ON; SELECT 1; DROP TABLE dbo.ItemName;",""} {
  if _,err:=pcOSItemNameSQLCmdArgs("SuperMarket",q,"Server");err==nil{t.Fatalf("arbitrary SQL allowed: %q",q)}
 }
}
