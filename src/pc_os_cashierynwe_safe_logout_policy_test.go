package main
import ("testing";"time";"strings")
func logoutB(v bool)*bool{return &v}
func logoutN(v int)*int{return &v}
func logoutFixture()(map[string]interface{},cashierLogoutObservation,time.Time){
 now:=time.Date(2026,1,1,12,0,0,0,time.UTC)
 nonce:=strings.Repeat("a",64)
 args:=map[string]interface{}{"market_id":"MKT-000000000001","device_id":"ZRX-0000-0000-0001","session_nonce":nonce,
 "approval_id":"00000000-0000-4000-8000-000000000001","command_id":"00000000-0000-4000-8000-000000000002","expires_at":now.Add(time.Minute).Format(time.RFC3339)}
 obs:=cashierLogoutObservation{MarketID:"MKT-000000000001",DeviceID:"ZRX-0000-0000-0001",SessionNonce:nonce,CapturedAt:now,
 OperatorUnlocked:logoutB(true),SaleInProgress:logoutB(false),CartItems:logoutN(0),PendingReceipts:logoutN(0),PrintingInProgress:logoutB(false),FinancialProcessActive:logoutB(false),PendingOfflineQueue:logoutN(0)}
 return args,obs,now
}
func TestLogoutStrictArgs(t *testing.T){
 for _,key:=range []string{"force","path","shell","process_id","windows_session","extra"}{
 a,_,n:=logoutFixture();a[key]=true;if _,err:=parseCashierSafeLogout(a,n);err==nil{t.Fatal(key)}
 }
 for _,key:=range []string{"market_id","device_id","session_nonce","approval_id","command_id","expires_at"}{
 a,_,n:=logoutFixture();delete(a,key);if _,err:=parseCashierSafeLogout(a,n);err==nil{t.Fatal(key)}
 }
 for _,delta:=range []time.Duration{-time.Second,0,3*time.Minute}{
 a,_,n:=logoutFixture();a["expires_at"]=n.Add(delta).Format(time.RFC3339);if _,err:=parseCashierSafeLogout(a,n);err==nil{t.Fatal(delta)}
 }
}
func TestLogoutBusinessSafety(t *testing.T){
 a,s,n:=logoutFixture();q,err:=parseCashierSafeLogout(a,n);if err!=nil{t.Fatal(err)}
 if got:=evaluateCashierSafeLogout(q,s,n);got!="eligible_for_app_recheck"{t.Fatal(got)}
 cases:=[]struct{name string;change func(*cashierLogoutObservation);want string}{
 {"sale",func(x *cashierLogoutObservation){x.SaleInProgress=logoutB(true)},"business_busy"},
 {"cart",func(x *cashierLogoutObservation){x.CartItems=logoutN(1)},"business_busy"},
 {"receipt",func(x *cashierLogoutObservation){x.PendingReceipts=logoutN(1)},"business_busy"},
 {"print",func(x *cashierLogoutObservation){x.PrintingInProgress=logoutB(true)},"business_busy"},
 {"finance",func(x *cashierLogoutObservation){x.FinancialProcessActive=logoutB(true)},"business_busy"},
 {"queue",func(x *cashierLogoutObservation){x.PendingOfflineQueue=logoutN(1)},"business_busy"},
 {"missing",func(x *cashierLogoutObservation){x.SaleInProgress=nil},"safety_invalid"},
 {"negative",func(x *cashierLogoutObservation){x.CartItems=logoutN(-1)},"safety_invalid"},
 {"stale",func(x *cashierLogoutObservation){x.CapturedAt=n.Add(-11*time.Second)},"safety_stale_or_unknown"},
 {"future",func(x *cashierLogoutObservation){x.CapturedAt=n.Add(time.Second)},"safety_stale_or_unknown"},
 {"market",func(x *cashierLogoutObservation){x.MarketID="MKT-000000000002"},"target_mismatch"},
 {"device",func(x *cashierLogoutObservation){x.DeviceID="ZRX-0000-0000-0002"},"target_mismatch"},
 {"session",func(x *cashierLogoutObservation){x.SessionNonce=strings.Repeat("b",64)},"target_mismatch"},
 {"logged out",func(x *cashierLogoutObservation){x.OperatorUnlocked=logoutB(false)},"already_logged_out"},
 }
 for _,c:=range cases{t.Run(c.name,func(t *testing.T){x:=s;c.change(&x);if got:=evaluateCashierSafeLogout(q,x,n);got!=c.want{t.Fatalf("got %s want %s",got,c.want)}})}
 if got:=evaluateCashierSafeLogout(q,s,q.ExpiresAt);got!="request_invalid_or_expired"{t.Fatal(got)}
}
