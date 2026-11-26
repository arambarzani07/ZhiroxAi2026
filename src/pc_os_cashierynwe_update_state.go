package main

import (
 "encoding/json"
 "time"
)

const cashierynweUpdateStateLimit = 256 * 1024

// A journal is evidence of a saved app state, NOT proof of a currently running process.
// Never return raw file contents, paths, history, error messages, or credentials.
type cashierynweUpdateStateReport struct {
 Schema int `json:"schema"`
 App string `json:"app"`
 State string `json:"state"`
 Source string `json:"source"`
 ObservedUTC string `json:"observed_utc"`
 JournalModifiedUTC string `json:"journal_modified_utc,omitempty"`
 LastCheckedUTC string `json:"last_checked_utc,omitempty"`
 ReportedVersion string `json:"reported_version,omitempty"`
 AvailableVersion string `json:"available_version,omitempty"`
 DownloadState string `json:"download_state,omitempty"`
 Progress *int `json:"download_progress_percent,omitempty"`
 DeferCode string `json:"defer_reason_code,omitempty"`
 BackupStatus string `json:"backup_status,omitempty"`
 RollbackState string `json:"rollback_state,omitempty"`
 PostUpdateHealthVerified *bool `json:"post_update_health_verified,omitempty"`
 ZeroUI bool `json:"zero_ui"`
 ReadOnly bool `json:"read_only"`
}

func cashierynweEmptyUpdateState(state string, now time.Time) cashierynweUpdateStateReport {
 return cashierynweUpdateStateReport{Schema:1,App:"Cashierynwe",State:state,Source:"persisted_auto_update_journal",ObservedUTC:now.UTC().Format(time.RFC3339),ZeroUI:true,ReadOnly:true}
}
func cashierynweAllowedValue(v string, allowed ...string) bool {
 for _, a := range allowed { if v==a { return true } }
 return false
}
func parseCashierynweUpdateState(raw []byte, modified, now time.Time) cashierynweUpdateStateReport {
 out:=cashierynweEmptyUpdateState("invalid",now)
 if len(raw)==0 || len(raw)>cashierynweUpdateStateLimit || modified.IsZero() || modified.After(now.Add(time.Minute)){return out}
 var in struct {
  CurrentVersion string `json:"currentVersion"`
  AvailableVersion *string `json:"latestAvailableVersion"`
  LastChecked *string `json:"lastCheckedIso"`
  DownloadState string `json:"downloadState"`
  Progress *int `json:"downloadProgressPercent"`
  DeferCode string `json:"deferReasonCode"`
  BackupStatus string `json:"backupStatus"`
  RollbackState string `json:"rollbackState"`
  PostUpdateHealthVerified *bool `json:"postUpdateHealthVerified"`
 }
 if err:=json.Unmarshal(raw,&in);err!=nil{return out}
 if !cashierynweStableVersion.MatchString(in.CurrentVersion) ||
  (in.AvailableVersion!=nil && !cashierynweStableVersion.MatchString(*in.AvailableVersion)) ||
  !cashierynweAllowedValue(in.DownloadState,"idle","checking","available","downloading","downloaded","deferred","installing","up_to_date","offline_retry","blocked_unsigned","error","rolled_back") ||
  in.Progress==nil || *in.Progress<0 || *in.Progress>100 ||
  !cashierynweAllowedValue(in.DeferCode,"NONE","ACTIVE_SALE_IN_PROGRESS","CART_NOT_EMPTY","RECEIPT_PENDING_CONFIRMATION","PRINTING_IN_PROGRESS","ACTIVE_SHIFT_OR_FINANCIAL_PROCESS","ACTIVE_OPERATOR_SESSION","OFFLINE_QUEUE_NON_EMPTY","LAN_TERMINAL_TURN_WAIT","TERMINAL_SAFETY_UNKNOWN","TERMINAL_SAFETY_STALE","TERMINAL_SAFETY_INVALID") ||
  !cashierynweAllowedValue(in.BackupStatus,"idle","creating","verified","failed") ||
  !cashierynweAllowedValue(in.RollbackState,"idle","rollback_required","rollback_in_progress","rollback_failed","rollback_completed","manual_recovery_required") ||
  in.PostUpdateHealthVerified==nil { return out }
 var checked time.Time
 if in.LastChecked!=nil {
  t,err:=time.Parse(time.RFC3339Nano,*in.LastChecked)
  if err!=nil || t.After(now.Add(time.Minute)){return out}
  checked=t
 }
 out.State="journal_observed"
 // Freshness is explicit, and an old journal must never imply the app is running.
 if now.Sub(modified)>15*time.Minute || checked.IsZero() || now.Sub(checked)>15*time.Minute {out.State="journal_stale"}
 out.JournalModifiedUTC=modified.UTC().Format(time.RFC3339)
 if !checked.IsZero(){out.LastCheckedUTC=checked.UTC().Format(time.RFC3339)}
 out.ReportedVersion=in.CurrentVersion
 if in.AvailableVersion!=nil{out.AvailableVersion=*in.AvailableVersion}
 out.DownloadState=in.DownloadState
 out.Progress=in.Progress
 out.DeferCode=in.DeferCode
 out.BackupStatus=in.BackupStatus
 out.RollbackState=in.RollbackState
 out.PostUpdateHealthVerified=in.PostUpdateHealthVerified
 return out
}
