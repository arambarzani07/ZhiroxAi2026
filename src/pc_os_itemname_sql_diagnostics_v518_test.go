package main

import (
 "errors"
 "strings"
 "testing"
)

func TestItemNameSafeSQLCmdDiagnostic(t *testing.T){
 got:=pcOSItemNameSQLCmdDiagnostic([]string{
  "2700002029709 | Secret Product Name | 100.25",
  "Sqlcmd: Error: misleading output from database rows",
 },"Sqlcmd: Error: Microsoft ODBC Driver 18: The target principal name is incorrect",errors.New("exit status 1"))
 if got==nil {t.Fatal("expected error")}
 if !strings.Contains(got.Error(),"SQL_TLS_CERT_NAME_MISMATCH"){t.Fatalf("missing TLS diagnostic: %v",got)}
 for _,secret:=range []string{"Secret Product","2700002029709","100.25"}{
  if strings.Contains(got.Error(),secret){t.Fatalf("leaked product row: %s",secret)}
 }
}
func TestItemNameUnrecognizedOutputIsNotLeaked(t *testing.T){
 err:=pcOSItemNameSQLCmdDiagnostic([]string{"کالای نهێنی|450|4999"},"",errors.New("exit status 1"))
 if err==nil || strings.Contains(err.Error(),"4999"){t.Fatalf("unsafe fallback: %v",err)}
 if !strings.Contains(err.Error(),"SQLCMD_ERROR_UNCLASSIFIED"){t.Fatalf("missing safe fallback: %v",err)}
}
func TestItemNameSQLCmdErrorIsCapped(t *testing.T){
 err:=pcOSItemNameSQLCmdDiagnostic(nil,strings.Repeat("Sqlcmd: Error: "+strings.Repeat("x",1000)+"\n",20),errors.New("failed"))
 if err==nil||len(err.Error())>160 {t.Fatalf("error not safely bounded: %d",len(err.Error()))}
}
func TestItemNameSQLCmdSuccessNoError(t *testing.T){
 if err:=pcOSItemNameSQLCmdDiagnostic([]string{"any"},"",nil);err!=nil{t.Fatal(err)}
}

func TestItemNameSQLCMDClassifications(t *testing.T){
 cases:=map[string]string{
  "certificate chain was issued by an authority that is not trusted":"SQL_TLS_CERT_TRUST",
  "TLS handshake failure":"SQL_TLS_HANDSHAKE",
  "Login failed":"SQL_INTEGRATED_LOGIN_FAILED",
  "Cannot open database":"SQL_DATABASE_ACCESS_DENIED",
  "A network-related or instance-specific error":"SQL_CONNECTION_FAILED",
  "Invalid object name":"SQL_SCHEMA_OR_PERMISSION_ERROR",
 }
 for raw,want:=range cases {
  got:=pcOSItemNameSQLCmdDiagnostic([]string{"BARCODE_123456"},raw,errors.New("exit status 1"))
  if got==nil||!strings.Contains(got.Error(),want)||strings.Contains(got.Error(),"BARCODE_123456"){t.Errorf("%s: %v",want,got)}
 }
}
