package main

import (
 "encoding/json"
 "strings"
 "testing"
 "time"
)

func TestCashierynweUpdateJournal(t *testing.T){
 now:=time.Date(2026,11,26,12,0,0,0,time.UTC)
 raw:=[]byte(`{"currentVersion":"1.0.13","latestAvailableVersion":"1.0.14","lastCheckedIso":"2026-11-26T11:59:00.000Z","downloadState":"deferred","downloadProgressPercent":100,"deferReasonCode":"ACTIVE_OPERATOR_SESSION","backupStatus":"verified","rollbackState":"idle","postUpdateHealthVerified":false,"history":[{"messageKu":"PRIVATE_HISTORY"}],"pendingInstallerPath":"PRIVATE_PATH","lastErrorKu":"PRIVATE_ERROR","backupChecksum":"PRIVATE_CHECKSUM"}`)
 got:=parseCashierynweUpdateState(raw,now.Add(-time.Minute),now)
 if got.State!="journal_observed"||got.ReportedVersion!="1.0.13"||got.AvailableVersion!="1.0.14"||got.DeferCode!="ACTIVE_OPERATOR_SESSION"||got.Progress==nil||*got.Progress!=100{t.Fatalf("unexpected report: %+v",got)}
 payload,_:=json.Marshal(got)
 if strings.Contains(string(payload),"PRIVATE_"){t.Fatal("private field leaked")}
 old:=parseCashierynweUpdateState(raw,now.Add(-time.Hour),now)
 if old.State!="journal_stale"{t.Fatal("old journal accepted as fresh")}
 for _,bad:=range [][]byte{[]byte("{}"),[]byte("not json"),[]byte(strings.ReplaceAll(string(raw),"ACTIVE_OPERATOR_SESSION","PRIVATE_UNKNOWN")),[]byte(strings.ReplaceAll(string(raw),"100","101")),make([]byte,cashierynweUpdateStateLimit+1)}{
  rejected:=parseCashierynweUpdateState(bad,now,now)
  if rejected.State!="invalid"||rejected.ReportedVersion!=""{t.Fatalf("invalid journal leaked: %+v",rejected)}
 }
}
