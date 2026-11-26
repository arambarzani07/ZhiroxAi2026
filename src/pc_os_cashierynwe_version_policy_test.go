package main

import (
 "encoding/json"
 "strings"
 "testing"
 "time"
)

func TestCashierynweRegistryVersionPolicy(t *testing.T) {
 for _,c:=range []struct{name string;items []cashierynweVersionObservation;state,version string}{
  {"missing",nil,"unknown",""},
  {"registered",[]cashierynweVersionObservation{{Present:true,Version:"1.0.14"}},"registered","1.0.14"},
  {"duplicate views",[]cashierynweVersionObservation{{Present:true,Version:"1.0.14"},{Present:true,Version:"1.0.14"}},"registered","1.0.14"},
  {"conflict",[]cashierynweVersionObservation{{Present:true,Version:"1.0.13"},{Present:true,Version:"1.0.14"}},"conflicting_versions",""},
  {"incomplete",[]cashierynweVersionObservation{{Present:true,Version:"1.0.14"},{Incomplete:true}},"incomplete",""},
  {"invalid",[]cashierynweVersionObservation{{Present:true,Version:"1.0.14-secret"}},"incomplete",""},
 }{
  t.Run(c.name,func(t *testing.T){
   r:=summarizeCashierynweVersions(c.items,time.Unix(0,0))
   if r.State!=c.state || r.RegisteredVersion!=c.version || !r.ZeroUI || !r.ReadOnly {t.Fatalf("unexpected report: %+v",r)}
   raw,_:=json.Marshal(r)
   if strings.Contains(string(raw),"secret") || strings.Contains(string(raw),"S-1-5") {t.Fatal("unexpected data in report")}
  })
 }
}
