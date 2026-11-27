package main

import (
 "errors"
 "regexp"
 "time"
)

// Source-only v5.1.19 candidate policy. This is not a command dispatcher.
// Do not expose the mutation until the application-owned authenticated
// handshake, durable replay protection and real Windows tests are complete.
const cashierSafeLogoutAction = "cashierynwe_request_safe_logout"
const cashierSafeLogoutTTL = 2*time.Minute
const cashierSafeLogoutSafetyTTL = 10*time.Second

var logoutUUID = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
var logoutMarket = regexp.MustCompile("^MKT-[0-9A-F]{12}$")
var logoutDevice = regexp.MustCompile("^ZRX-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$")
var logoutSession = regexp.MustCompile("^[a-f0-9]{64}$")

type cashierLogoutTarget struct {
 MarketID string
 DeviceID string
 SessionNonce string
 ApprovalID string
 CommandID string
 ExpiresAt time.Time
}

// Exact typed args only. No shell, path, process id, Windows session id,
// credentials, force flag, or request to close a different operator session.
func parseCashierSafeLogout(args map[string]interface{}, now time.Time) (cashierLogoutTarget,error) {
 var t cashierLogoutTarget
 if now.IsZero() || len(args)!=6 {return t,errors.New("invalid_logout_args")}
 get:=func(k string)string{v,_:=args[k].(string);return v}
 t.MarketID=get("market_id")
 t.DeviceID=get("device_id")
 t.SessionNonce=get("session_nonce")
 t.ApprovalID=get("approval_id")
 t.CommandID=get("command_id")
 expiry,err:=time.Parse(time.RFC3339Nano,get("expires_at"))
 if err!=nil || !expiry.After(now) || expiry.After(now.Add(cashierSafeLogoutTTL)) ||
 !logoutMarket.MatchString(t.MarketID) || !logoutDevice.MatchString(t.DeviceID) ||
 !logoutSession.MatchString(t.SessionNonce) || !logoutUUID.MatchString(t.ApprovalID) ||
 !logoutUUID.MatchString(t.CommandID) {return cashierLogoutTarget{},errors.New("invalid_or_expired_logout_args")}
 t.ExpiresAt=expiry
 return t,nil
}

// These observations MUST be from a fresh authenticated app session,
// never supplied by the remote caller or inferred from the update journal.
// Pointer flags/counts are mandatory so missing data cannot become safe.
type cashierLogoutObservation struct {
 MarketID string
 DeviceID string
 SessionNonce string
 CapturedAt time.Time
 OperatorUnlocked *bool
 SaleInProgress *bool
 CartItems *int
 PendingReceipts *int
 PrintingInProgress *bool
 FinancialProcessActive *bool
 PendingOfflineQueue *int
}

func evaluateCashierSafeLogout(t cashierLogoutTarget, s cashierLogoutObservation, now time.Time) string {
 if now.IsZero() || !t.ExpiresAt.After(now) || t.ExpiresAt.After(now.Add(cashierSafeLogoutTTL)) ||
 !logoutMarket.MatchString(t.MarketID) || !logoutDevice.MatchString(t.DeviceID) ||
 !logoutSession.MatchString(t.SessionNonce) || !logoutUUID.MatchString(t.ApprovalID) ||
 !logoutUUID.MatchString(t.CommandID) {return "request_invalid_or_expired"}
 if s.CapturedAt.IsZero() || s.CapturedAt.After(now) || now.Sub(s.CapturedAt)>cashierSafeLogoutSafetyTTL {return "safety_stale_or_unknown"}
 if t.MarketID!=s.MarketID || t.DeviceID!=s.DeviceID || t.SessionNonce!=s.SessionNonce {return "target_mismatch"}
 if s.OperatorUnlocked==nil || s.SaleInProgress==nil || s.CartItems==nil || s.PendingReceipts==nil ||
 s.PrintingInProgress==nil || s.FinancialProcessActive==nil || s.PendingOfflineQueue==nil ||
 *s.CartItems<0 || *s.PendingReceipts<0 || *s.PendingOfflineQueue<0 {return "safety_invalid"}
 if *s.SaleInProgress || *s.CartItems!=0 || *s.PendingReceipts!=0 || *s.PrintingInProgress ||
 *s.FinancialProcessActive || *s.PendingOfflineQueue!=0 {return "business_busy"}
 if !*s.OperatorUnlocked {return "already_logged_out"}
 // Only permits the application to re-evaluate and invoke its existing safe
 // logout path. It is not permission to terminate a process or log off Windows.
 return "eligible_for_app_recheck"
}
