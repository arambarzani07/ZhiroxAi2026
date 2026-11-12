package basketmerge

import (
 "math"
 "testing"
)
func scan(id,ticket string) Scan {
 return Scan{EventID:id,TicketID:ticket,ProductID:"16167",Barcode:"670237553985",Unit:"piece",UnitPriceIQD:500,Quantity:1,PriceVersion:"p1"}
}
func session(t *testing.T, ids ...string) Session {
 t.Helper()
 s,e:=NewSession(ids...);if e!=nil{t.Fatal(e)};return s
}
func apply(t *testing.T,s Session,x Scan,want Outcome) Session {
 t.Helper()
 out,got,err:=ApplyScan(s,x)
 if err!=nil||got!=want{t.Fatalf("outcome=%s error=%v want=%s",got,err,want)}
 return out
}
func TestSameBarcodeOneLineAndCorrectTotal(t *testing.T){
 s:=session(t,"T1")
 s=apply(t,s,scan("scan1","T1"),Added)
 s=apply(t,s,scan("scan2","T1"),Merged)
 if len(s.Tickets[0].Lines)!=1 || s.Tickets[0].Lines[0].Quantity!=2 {t.Fatalf("should be 1 line quantity 2: %+v",s)}
 total,err:=TotalIQD(s.Tickets[0]);if err!=nil||total!=1000 {t.Fatalf("total=%d error=%v",total,err)}
 s=apply(t,s,scan("scan3","T1"),Merged)
 if s.Tickets[0].Lines[0].Quantity!=3 {t.Fatal("third scan not merged")}
}
func TestDuplicateScanIDIgnored(t *testing.T){
 s:=session(t,"T1");s=apply(t,s,scan("scan1","T1"),Added)
 s=apply(t,s,scan("scan1","T1"),AlreadyProcessed)
 if s.Tickets[0].Lines[0].Quantity!=1 {t.Fatal("double-charged retry")}
}
func TestFiveHeldTicketsRemainSeparate(t *testing.T){
 s:=session(t,"1","2","3","4","5")
 s=apply(t,s,scan("e1","1"),Added)
 s=apply(t,s,scan("e2","2"),Added)
 s=apply(t,s,scan("e3","1"),Merged)
 if s.Tickets[0].Lines[0].Quantity!=2||s.Tickets[1].Lines[0].Quantity!=1||len(s.Tickets[2].Lines)!=0 {t.Fatal("tickets are not isolated")}
 if _,err:=NewSession("1","2","3","4","5","6");err==nil {t.Fatal("six tickets allowed")}
}
func TestStrictMergeIdentity(t *testing.T){
 cases:=[]struct{name string;edit func(*Scan)}{
  {"unit",func(x *Scan){x.Unit="carton"}},
  {"price",func(x *Scan){x.UnitPriceIQD=750}},
  {"price version",func(x *Scan){x.PriceVersion="p2"}},
  {"discount",func(x *Scan){x.DiscountID="promotion"}},
  {"tax",func(x *Scan){x.TaxCode="tax"}},
  {"batch",func(x *Scan){x.BatchID="other"}},
  {"product",func(x *Scan){x.ProductID="other"}},
  {"barcode",func(x *Scan){x.Barcode="other"}},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
  s:=session(t,"T1");s=apply(t,s,scan("e1","T1"),Added)
  x:=scan("e2","T1");tc.edit(&x)
  s=apply(t,s,x,Added)
  if len(s.Tickets[0].Lines)!=2 {t.Fatal("merged unlike products or terms")}
 })}
 s:=session(t,"T1");s=apply(t,s,scan("e1","T1"),Added)
 s.Tickets[0].Lines[0].Locked=true
 s=apply(t,s,scan("e2","T1"),Added)
 if len(s.Tickets[0].Lines)!=2 {t.Fatal("merged locked line")}
}
func TestUnsafeInputRejectedWithoutMutation(t *testing.T){
 s:=session(t,"T1");s=apply(t,s,scan("e1","T1"),Added)
 invalid:=[]Scan{}
 x:=scan("e2","T1");x.Unit="kg";invalid=append(invalid,x)
 x=scan("e2","T1");x.Quantity=0;invalid=append(invalid,x)
 x=scan("e2","T1");x.UnitPriceIQD=-1;invalid=append(invalid,x)
 x=scan("e2","T1");x.IsReturn=true;invalid=append(invalid,x)
 x=scan("e2","T1");x.IsManualOverride=true;invalid=append(invalid,x)
 x=scan("e2","missing");invalid=append(invalid,x)
 x=scan("e2","T1");x.UnitPriceIQD=math.MaxInt64;x.Quantity=2;invalid=append(invalid,x)
 for _,x:=range invalid {
  got,_,err:=ApplyScan(s,x)
  if err==nil||got.Tickets[0].Lines[0].Quantity!=1 {t.Fatalf("accepted %+v, err=%v",x,err)}
 }
}
func TestCopyOnWriteInputIntact(t *testing.T) {
 s:=session(t,"T1");s=apply(t,s,scan("e1","T1"),Added)
 out:=apply(t,s,scan("e2","T1"),Merged)
 if s.Tickets[0].Lines[0].Quantity!=1||out.Tickets[0].Lines[0].Quantity!=2||len(s.Tickets[0].SeenEventIDs)!=1 {t.Fatal("input session was mutated")}
}
func TestTotalOverflowRejected(t *testing.T) {
 s:=session(t,"T1")
 x:=scan("e1","T1");x.UnitPriceIQD=math.MaxInt64/2;x.Quantity=2
 s=apply(t,s,x,Added)
 y:=scan("e2","T1");y.Barcode="other";y.UnitPriceIQD=42
 got,_,err:=ApplyScan(s,y)
 if err==nil||len(got.Tickets[0].Lines)!=1 {t.Fatal("accepted overflow")}
}

func TestGlobalScanEventDedupeAcrossTickets(t *testing.T){
 s:=session(t,"T1","T2")
 s=apply(t,s,scan("event-001","T1"),Added)
 // A late retry after customer/ticket switch must never create a charge.
 s=apply(t,s,scan("event-001","T2"),AlreadyProcessed)
 if len(s.Tickets[1].Lines)!=0 {t.Fatal("duplicate physical scan charged other customer")}
 s=apply(t,s,scan("event-002","T2"),Added)
 if len(s.Tickets[0].Lines)!=1||len(s.Tickets[1].Lines)!=1 {t.Fatal("new physical event did not reach separate ticket")}
}
