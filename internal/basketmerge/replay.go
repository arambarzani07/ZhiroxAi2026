package basketmerge

import (
 "errors"
 "fmt"
 "math"
)

// ReplayFixture is an entirely in-memory compatibility test. It neither opens
// Cashier.exe nor reads the scanner or SQL database. It accepts synthetic,
// catalog-verified test cases only; event IDs must be assigned by the caller.
type ReplayFixture struct {
 TicketIDs []string
 Catalog []ReplayCatalogItem
 Scans []ReplayScan
}

type ReplayCatalogItem struct {
 Barcode, ProductID, Unit string
 UnitPriceIQD int64
 PriceVersion, DiscountID, TaxCode, BatchID string
}

type ReplayScan struct {
 EventID, TicketID, Barcode string
 StartMS, KeyGapMS int64
 WithEnter bool
}

type ReplayStep struct {
 EventID, TicketID string
 Outcome Outcome
 Barcode string
 Receipt ReceiptPreview
}

type ReplayResult struct {
 Steps []ReplayStep
 Tickets []ReceiptPreview
}

// SyntheticMaxInterKeyMS is a conservative upper bound for TEST FIXTURES.
// It is NOT calibrated against the user's physical HID scanner. In the actual
// application, scanner-only focus, provenance and timing must be verified.
const SyntheticMaxInterKeyMS int64 = 80
const MaxReplayEvents = 1000

func ReplayBasketFixture(f ReplayFixture) (ReplayResult, error) {
 result:=ReplayResult{}
 if len(f.Scans)>MaxReplayEvents {return result,errors.New("test replay event limit exceeded")}
 session,err:=NewSession(f.TicketIDs...)
 if err!=nil{return result,err}
 catalog:=make(map[string]ReplayCatalogItem,len(f.Catalog))
 for _,entry:=range f.Catalog {
  if !BarcodeCanBeAutoMerged(entry.Barcode){return result,fmt.Errorf("invalid/variable-weight catalog barcode")}
  if _,exists:=catalog[entry.Barcode];exists{return result,errors.New("duplicate barcode in test catalog")}
  x:=Scan{EventID:"fixture-validation",TicketID:session.Tickets[0].ID,ProductID:entry.ProductID,Barcode:entry.Barcode,Unit:entry.Unit,UnitPriceIQD:entry.UnitPriceIQD,Quantity:1,PriceVersion:entry.PriceVersion}
  if err:=validate(x);err!=nil{return result,fmt.Errorf("invalid test catalog item: %w",err)}
  catalog[entry.Barcode]=entry
 }
 type eventIdentity struct{ barcode string }
 seen:=map[string]eventIdentity{}
 for _,sample:=range f.Scans {
  if sample.EventID==""||sample.TicketID=="" {return ReplayResult{},errors.New("event and ticket ID required")}
  if sample.KeyGapMS<0||sample.KeyGapMS>SyntheticMaxInterKeyMS{return ReplayResult{},errors.New("unverified scanner timing in fixture")}
  if sample.StartMS<0||sample.StartMS>math.MaxInt64-10000 {return ReplayResult{},errors.New("invalid scanner timestamp")}
  if previous,exists:=seen[sample.EventID];exists&&previous.barcode!=sample.Barcode {
   return ReplayResult{},errors.New("replayed scan event changed its barcode")
  }
  var state ScannerState
  var code string
  var done bool
  var err error
  at:=sample.StartMS
  for _,digit:=range sample.Barcode {
   state,code,done,err=FeedScannerKey(state,digit,at)
   if err!=nil||done{return ReplayResult{},errors.New("scanner stream rejected or emitted prematurely")}
   at+=sample.KeyGapMS
  }
  if sample.WithEnter {
   state,code,done,err=FeedScannerKey(state,'\r',at)
  } else {
   state,code,done,err=FlushScannerIdle(state,at-sample.KeyGapMS+HIDIdleMS)
  }
  if err!=nil||!done||state.Active||code!=sample.Barcode {
   return ReplayResult{},errors.New("scanner framing mismatch")
  }
  if !BarcodeCanBeAutoMerged(code){return ReplayResult{},errors.New("barcode requires special workflow")}
  item,exists:=catalog[code]
  if !exists{return ReplayResult{},errors.New("barcode missing from staged trusted catalog")}
  x:=Scan{
   EventID:sample.EventID,TicketID:sample.TicketID,
   ProductID:item.ProductID,Barcode:code,Unit:item.Unit,
   UnitPriceIQD:item.UnitPriceIQD,Quantity:1,PriceVersion:item.PriceVersion,
   DiscountID:item.DiscountID,TaxCode:item.TaxCode,BatchID:item.BatchID,
  }
  var outcome Outcome
  session,outcome,err=ApplyScan(session,x)
  if err!=nil{return ReplayResult{},fmt.Errorf("test apply scan: %w",err)}
  seen[sample.EventID]=eventIdentity{barcode:code}
  var ticket Ticket
  for _,t:=range session.Tickets{if t.ID==sample.TicketID{ticket=t;break}}
  receipt,err:=BuildReceiptPreview(ticket)
  if err!=nil{return ReplayResult{},err}
  if err:=CheckReceiptParity(ticket,receipt);err!=nil{return ReplayResult{},err}
  result.Steps=append(result.Steps,ReplayStep{EventID:sample.EventID,TicketID:sample.TicketID,Outcome:outcome,Barcode:code,Receipt:receipt})
 }
 for _,t:=range session.Tickets {
  preview,err:=BuildReceiptPreview(t)
  if err!=nil{return ReplayResult{},err}
  if err:=CheckReceiptParity(t,preview);err!=nil{return ReplayResult{},err}
  result.Tickets=append(result.Tickets,preview)
 }
 return result,nil
}
