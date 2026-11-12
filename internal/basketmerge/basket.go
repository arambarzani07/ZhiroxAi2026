// Package basketmerge is an isolated, side-effect-free barcode basket prototype.
// It has no access to legacy Cashier.exe, Admin.exe, SQL Server or PC Relay.
package basketmerge

import (
 "errors"
 "fmt"
 "math"
 "strings"
)

const (
 MaxTickets = 5
 MaxLinesPerTicket = 400
 MaxScansPerTicket = 10000
 MaxQuantityPerLine int64 = 100000
)

// EventID is unique per physical scan; barcode itself is NOT the event ID.
// Product and price values must be validated against a trusted catalog first.
type Scan struct {
 EventID, TicketID, ProductID, Barcode, Unit string
 UnitPriceIQD, Quantity int64
 PriceVersion, DiscountID, TaxCode, BatchID string
 IsReturn, IsManualOverride bool
}
type Line struct {
 ProductID, Barcode, Unit string
 UnitPriceIQD, Quantity int64
 PriceVersion, DiscountID, TaxCode, BatchID string
 Locked bool
}
type Ticket struct {
 ID string
 Lines []Line
 SeenEventIDs []string
}
type Session struct { Tickets []Ticket }
type Outcome string
const (
 Added Outcome = "added"
 Merged Outcome = "merged"
 AlreadyProcessed Outcome = "already_processed"
)
func NewSession(ids ...string) (Session,error) {
 if len(ids)==0 || len(ids)>MaxTickets {return Session{},fmt.Errorf("ticket count must be 1..%d",MaxTickets)}
 seen:=map[string]bool{}
 s:=Session{Tickets:make([]Ticket,0,len(ids))}
 for _,id:=range ids {
  if strings.TrimSpace(id)=="" || seen[id] {return Session{},errors.New("ticket IDs must be non-empty and unique")}
  seen[id]=true
  s.Tickets=append(s.Tickets,Ticket{ID:id})
 }
 return s,nil
}
func validate(x Scan) error {
 if strings.TrimSpace(x.EventID)=="" || strings.TrimSpace(x.TicketID)=="" || strings.TrimSpace(x.ProductID)=="" || strings.TrimSpace(x.Barcode)=="" {
  return errors.New("event, ticket, product and barcode IDs required")
 }
 switch x.Unit {
 case "piece","carton","pack","set":
 default: return errors.New("weight/variable-unit scans need a separate verified workflow")
 }
 if x.Quantity<=0 || x.Quantity>MaxQuantityPerLine {return errors.New("invalid quantity")}
 if x.UnitPriceIQD<0 {return errors.New("negative price")}
 if x.IsReturn || x.IsManualOverride {return errors.New("special transactions require separate authorization")}
 if x.UnitPriceIQD>math.MaxInt64/x.Quantity {return errors.New("line overflow")}
 return nil
}
func matches(l Line,x Scan) bool {
 return !l.Locked && l.ProductID==x.ProductID && l.Barcode==x.Barcode &&
 l.Unit==x.Unit && l.UnitPriceIQD==x.UnitPriceIQD &&
 l.PriceVersion==x.PriceVersion && l.DiscountID==x.DiscountID &&
 l.TaxCode==x.TaxCode && l.BatchID==x.BatchID
}
// ApplyScan returns a new session. Invalid events never mutate the source.
// Repeated EventID is not a new scan; same barcode with a NEW EventID is.
func ApplyScan(before Session,x Scan) (Session,Outcome,error) {
 if err:=validate(x);err!=nil{return before,"",err}
 if len(before.Tickets)==0 || len(before.Tickets)>MaxTickets {return before,"",errors.New("invalid session")}
 idx:=-1
 for i:=range before.Tickets {if before.Tickets[i].ID==x.TicketID {idx=i;break}}
 if idx<0 {return before,"",errors.New("ticket not found")}
 t:=before.Tickets[idx]
 for _,event:=range t.SeenEventIDs {if event==x.EventID {return before,AlreadyProcessed,nil}}
 if len(t.SeenEventIDs)>=MaxScansPerTicket{return before,"",errors.New("scan history limit")}
 if _,err:=TotalIQD(t);err!=nil{return before,"",err}
 out:=Session{Tickets:make([]Ticket,len(before.Tickets))}
 for i,v:=range before.Tickets {out.Tickets[i]=Ticket{ID:v.ID,Lines:append([]Line(nil),v.Lines...),SeenEventIDs:append([]string(nil),v.SeenEventIDs...)}}
 target:=&out.Tickets[idx]
 outcome:=Added
 for i:=range target.Lines {
  l:=&target.Lines[i]
  if !matches(*l,x){continue}
  if l.Quantity<=0 || l.Quantity>MaxQuantityPerLine-x.Quantity{return before,"",errors.New("merged quantity limit")}
  l.Quantity+=x.Quantity
  outcome=Merged
  break
 }
 if outcome==Added {
  if len(target.Lines)>=MaxLinesPerTicket{return before,"",errors.New("line count limit")}
  target.Lines=append(target.Lines,Line{
   ProductID:x.ProductID,Barcode:x.Barcode,Unit:x.Unit,UnitPriceIQD:x.UnitPriceIQD,
   Quantity:x.Quantity,PriceVersion:x.PriceVersion,DiscountID:x.DiscountID,
   TaxCode:x.TaxCode,BatchID:x.BatchID,
  })
 }
 if _,err:=TotalIQD(*target);err!=nil{return before,"",err}
 target.SeenEventIDs=append(target.SeenEventIDs,x.EventID)
 return out,outcome,nil
}
func TotalIQD(t Ticket) (int64,error) {
 if len(t.Lines)>MaxLinesPerTicket{return 0,errors.New("line count limit")}
 var total int64
 for _,l:=range t.Lines {
  if l.Quantity<=0 || l.Quantity>MaxQuantityPerLine || l.UnitPriceIQD<0 {return 0,errors.New("invalid line")}
  if l.UnitPriceIQD>math.MaxInt64/l.Quantity{return 0,errors.New("line total overflow")}
  amount:=l.UnitPriceIQD*l.Quantity
  if amount>math.MaxInt64-total{return 0,errors.New("ticket total overflow")}
  total+=amount
 }
 return total,nil
}
