package main

import (
    "encoding/json"
    "testing"
    "time"
)

func TestBrokerRejectsAllUnapprovedVariants(t *testing.T) {
    now := time.Date(2026,11,22,15,0,0,0,time.UTC)
    ticket := InstallTicket{
        Schema:brokerTicketSchema, AgentID:brokerAgentID, AppID:brokerAppID,
        Version:brokerVersion,Filename:brokerFileName,SHA256:brokerSHA256,
        Size:brokerFileSize,ApprovalID:"12345678-1234-4234-8234-1234567890ab",
        RequestID:"32345678-1234-4234-8234-1234567890ab",
        IssuedUTC:now.Add(-time.Minute),ExpiresUTC:now.Add(time.Minute),
        OneTimeApprovalConsumed:true,
    }
    encode := func(v any) []byte { b, e := json.Marshal(v); if e != nil { t.Fatal(e) }; return b }
    if _, err := readAndValidateInstallTicket(encode(ticket),now); err != nil {
        t.Fatalf("valid one-time ticket rejected: %v",err)
    }
    mutate := []func(*InstallTicket){
        func(v *InstallTicket){v.AppID="evil"},
        func(v *InstallTicket){v.Version="1.0.11"},
        func(v *InstallTicket){v.Filename="cmd.exe"},
        func(v *InstallTicket){v.SHA256="0"},
        func(v *InstallTicket){v.Size=1},
        func(v *InstallTicket){v.AgentID="other-agent"},
        func(v *InstallTicket){v.OneTimeApprovalConsumed=false},
        func(v *InstallTicket){v.ApprovalID="bypass"},
        func(v *InstallTicket){v.RequestID=""},
        func(v *InstallTicket){v.IssuedUTC=now.Add(time.Second)},
        func(v *InstallTicket){v.ExpiresUTC=now.Add(-time.Second)},
        func(v *InstallTicket){v.ExpiresUTC=now.Add(16*time.Minute)},
    }
    for i,fn:=range mutate {
        v:=ticket
        fn(&v)
        if _,err:=readAndValidateInstallTicket(encode(v),now);err==nil {
            t.Errorf("unapproved ticket variant %d accepted",i)
        }
    }
    var extra map[string]any
    if err:=json.Unmarshal(encode(ticket),&extra);err!=nil {t.Fatal(err)}
    extra["exe"]="powershell.exe"
    if _,err:=readAndValidateInstallTicket(encode(extra),now);err==nil {t.Fatal("arbitrary executable field accepted")}
    if _,err:=readAndValidateInstallTicket(append(encode(ticket),[]byte(" {}")...),now);err==nil {t.Fatal("trailing JSON accepted")}
    if _,err:=readAndValidateInstallTicket(make([]byte,4097),now);err==nil {t.Fatal("oversized ticket accepted")}
}
