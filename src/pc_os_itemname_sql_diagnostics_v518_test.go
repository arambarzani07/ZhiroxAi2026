package main

import (
 "errors"
 "strings"
 "testing"
)

func TestItemNameSafeSQLCmdDiagnostic(t *testing.T){
 got:=pcOSItemNameSQLCmdDiagnostic([]string{
  "2700002029709 | Secret Product Name | 100.25",
  "Sqlcmd: Error: Microsoft ODBC Driver 18: The target principal name is incorrect",
 },"",errors.New("exit status 1"))
 if got==nil {t.Fatal("expected error")}
 if !strings.Contains(got.Error(),"target principal name"){t.Fatalf("missing TLS diagnostic: %v",got)}
 for _,secret:=range []string{"Secret Product","2700002029709","100.25"}{
  if strings.Contains(got.Error(),secret){t.Fatalf("leaked product row: %s",secret)}
 }
}
func TestItemNameUnrecognizedOutputIsNotLeaked(t *testing.T){
 err:=pcOSItemNameSQLCmdDiagnostic([]string{"کالای نهێنی|450|4999"},"",errors.New("exit status 1"))
 if err==nil || strings.Contains(err.Error(),"4999"){t.Fatalf("unsafe fallback: %v",err)}
 if !strings.Contains(err.Error(),"no safe diagnostics"){t.Fatalf("missing safe fallback: %v",err)}
}
func TestItemNameSQLCmdErrorIsCapped(t *testing.T){
 err:=pcOSItemNameSQLCmdDiagnostic(nil,strings.Repeat("Sqlcmd: Error: "+strings.Repeat("x",1000)+"\n",20),errors.New("failed"))
 if err==nil||len(err.Error())>2600 {t.Fatalf("error not safely bounded: %d",len(err.Error()))}
}
func TestItemNameSQLCmdSuccessNoError(t *testing.T){
 if err:=pcOSItemNameSQLCmdDiagnostic([]string{"any"},"",nil);err!=nil{t.Fatal(err)}
}
