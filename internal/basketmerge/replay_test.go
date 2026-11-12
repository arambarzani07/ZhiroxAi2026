package basketmerge

import (
 "math"
 "strings"
 "testing"
)

func fixture() ReplayFixture {
 return ReplayFixture{
  TicketIDs:[]string{"T1","T2","T3","T4","T5"},
  Catalog:[]ReplayCatalogItem{
   {Barcode:"123456789012",ProductID:"P1",Unit:"piece",UnitPriceIQD:500,PriceVersion:"v1"},
   {Barcode:"098765432109",ProductID:"P2",Unit:"carton",UnitPriceIQD:5500,PriceVersion:"v1"},
  },
  Scans:[]ReplayScan{
   {EventID:"physical-1",TicketID:"T1",Barcode:"123456789012",StartMS:100,KeyGapMS:10,WithEnter:true},
   {EventID:"physical-2",TicketID:"T1",Barcode:"123456789012",StartMS:500,KeyGapMS:10,WithEnter:false},
   {EventID:"physical-3",TicketID:"T2",Barcode:"098765432109",StartMS:1000,KeyGapMS:12,WithEnter:true},
   {EventID:"physical-1",TicketID:"T2",Barcode:"123456789012",StartMS:1400,KeyGapMS:10,WithEnter:true},
  },
 }
}

func TestOfflineReplayEnterNoEnterAndTicketIsolation(t *testing.T){
 result,err:=ReplayBasketFixture(fixture())
 if err!=nil{t.Fatal(err)}
 if len(result.Steps)!=4||len(result.Tickets)!=5{t.Fatalf("unexpected replay sizes: %+v",result)}
 if result.Steps[0].Outcome!=Added||result.Steps[1].Outcome!=Merged||
    result.Steps[2].Outcome!=Added||result.Steps[3].Outcome!=AlreadyProcessed {
  t.Fatalf("wrong replay outcomes: %+v",result.Steps)
 }
 a:=result.Tickets[0]
 if len(a.Lines)!=1||a.Lines[0].Quantity!=2||a.TotalIQD!=1000||a.Lines[0].LineTotalIQD!=1000{
  t.Fatalf("same code not coalesced correctly: %+v",a)
 }
 b:=result.Tickets[1]
 if len(b.Lines)!=1||b.Lines[0].Quantity!=1||b.TotalIQD!=5500{
  t.Fatalf("cross-ticket replay corrupted T2: %+v",b)
 }
 for _,unused:=range result.Tickets[2:]{
  if unused.TotalIQD!=0||len(unused.Lines)!=0{t.Fatalf("untouched ticket changed: %+v",unused)}
 }
}

func TestOfflineReplayRejectsUnverifiedCatalogAndScanner(t *testing.T){
 makeSample:=func(f ReplayFixture) ReplayFixture{f.Scans=f.Scans[:1];return f}
 tests:=[]struct{name string;mutate func(*ReplayFixture)}{
  {"unknown barcode",func(f *ReplayFixture){f.Scans[0].Barcode="888888888888"}},
  {"scale prefix-27",func(f *ReplayFixture){f.Scans[0].Barcode="2701234567890"}},
  {"negative clock",func(f *ReplayFixture){f.Scans[0].StartMS=-1}},
  {"slow synthetic timing",func(f *ReplayFixture){f.Scans[0].KeyGapMS=SyntheticMaxInterKeyMS+1}},
  {"nondigit",func(f *ReplayFixture){f.Scans[0].Barcode="12-45678"}},
  {"short",func(f *ReplayFixture){f.Scans[0].Barcode="123"}},
  {"overlong",func(f *ReplayFixture){f.Scans[0].Barcode=strings.Repeat("1",MaxBarcodeDigits+1)}},
  {"no event",func(f *ReplayFixture){f.Scans[0].EventID=""}},
  {"unknown ticket",func(f *ReplayFixture){f.Scans[0].TicketID="T6"}},
  {"duplicate catalog",func(f *ReplayFixture){f.Catalog=append(f.Catalog,f.Catalog[0])}},
  {"negative price",func(f *ReplayFixture){f.Catalog[0].UnitPriceIQD=-50}},
  {"unverified unit",func(f *ReplayFixture){f.Catalog[0].Unit="kg"}},
  {"empty catalog",func(f *ReplayFixture){f.Catalog=nil}},
  {"more than five tickets",func(f *ReplayFixture){f.TicketIDs=append(f.TicketIDs,"T6")}},
 }
 for _,tc:=range tests{t.Run(tc.name,func(t *testing.T){
  f:=makeSample(fixture())
  tc.mutate(&f)
  got,err:=ReplayBasketFixture(f)
  if err==nil||len(got.Steps)>0||len(got.Tickets)>0{t.Fatalf("unsafe input accepted: %+v err=%v",got,err)}
 })}
}

func TestOfflineReplayRejectsConflictingEventReuse(t *testing.T){
 f:=fixture()
 f.Scans[3].Barcode="098765432109" // same ID as first, different barcode
 r,err:=ReplayBasketFixture(f)
 if err==nil||len(r.Steps)>0{t.Fatalf("conflicting event ID should fail closed: %+v %v",r,err)}
}

func TestOfflineReplayLimitAndOverflow(t *testing.T){
 f:=fixture()
 f.Scans=make([]ReplayScan,MaxReplayEvents+1)
 if _,err:=ReplayBasketFixture(f);err==nil{t.Fatal("oversized fixture accepted")}
 f=fixture()
 f.Catalog[0].UnitPriceIQD=math.MaxInt64
 f.Scans=f.Scans[:2] // second scan would overflow same line, fail closed
 r,err:=ReplayBasketFixture(f)
 if err==nil||len(r.Steps)!=0{t.Fatalf("overflow should fail closed: %+v err=%v",r,err)}
}

func TestLeadingZeroPreserved(t *testing.T){
 f:=fixture()
 f.Scans=f.Scans[:1]
 f.Scans[0].Barcode="012345678901"
 f.Catalog[0].Barcode="012345678901"
 r,err:=ReplayBasketFixture(f)
 if err!=nil||r.Tickets[0].Lines[0].Barcode!="012345678901"{
  t.Fatalf("leading zero lost: %+v err=%v",r,err)
 }
}
