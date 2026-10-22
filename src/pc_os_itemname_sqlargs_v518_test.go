package main

import (
    "reflect"
    "strings"
    "testing"
)

func TestItemNameSQLCmdTLSRequiresCertificateValidation(t *testing.T) {
    args, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, "Server")
    if err != nil { t.Fatal(err) }
    want := []string{"-S", "lpc:Server", "-E", "-No", "-d", "SuperMarket",
        "-b", "-h", "-1", "-w", "8192", "-y", "4096",
        "-l", "10", "-t", "90", "-r", "1", "-Q", pcOSItemNameTopOneSQL}
    if !reflect.DeepEqual(args, want) { t.Fatalf("wrong fixed sqlcmd args: %q", args) }
    for _, token := range args {
        if token == "-C" || token == "-N" || token == "-F" || token == "-U" || token == "-P" {
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
    if _, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, "SERVER"); err != nil {
        t.Fatalf("rejected actual local host: %v", err)
    }
    if _, err := pcOSItemNameSQLCmdArgs("SuperMarket", pcOSItemNameTopOneSQL, "Server.office.local"); err == nil {
        t.Fatal("accepted non-local host")
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

func TestItemNameSharedMemoryOnly(t *testing.T) {
 args,err:=pcOSItemNameSQLCmdArgs("SuperMarket",pcOSItemNameSchemaProbeSQL,"Server")
 if err!=nil { t.Fatal(err) }
 if !strings.Contains(pcOSItemNameSchemaProbeSQL,"CONNECTIONPROPERTY('net_transport')") { t.Fatal("server-side local transport verification missing") }
 if !strings.Contains(pcOSItemNameSchemaProbeSQL,"Shared memory") { t.Fatal("shared memory transport not enforced") }
 for i,a:=range args {
  if a=="-S" && (i+1>=len(args)||args[i+1]!="lpc:Server") { t.Fatal("remote target accepted") }
  if a=="-No" && (len(args)<2 || args[1]!="lpc:Server") { t.Fatal("optional encryption outside local shared memory") }
 }
 for _,a:=range args {
  if a=="-C" || a=="-P" || a=="-U" || a=="-N" || a=="-F" {
   t.Fatalf("forbidden remote auth/transport flag %q",a)
  }
  if strings.HasPrefix(a,"tcp:")||strings.HasPrefix(a,"np:"){t.Fatalf("remote SQL transport forbidden: %q",a)}
 }
}
