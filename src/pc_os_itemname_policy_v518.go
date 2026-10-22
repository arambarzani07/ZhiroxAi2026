package main

import (
  "errors"
  "sort"
  "strings"
)

type pcOSItemNameRow struct {
  ID string `json:"itemNameID"`
  Name string `json:"itemName"`
  Barcode string `json:"itemCode"`
  SalePrice string `json:"salePrice"`
  CartonPrice string `json:"wholeSalePrice"`
}
type pcOSItemNameChange struct {
  Kind string `json:"kind"`
  ID string `json:"itemNameID"`
  Name string `json:"itemName"`
  Barcode string `json:"itemCode"`
  PriceField string `json:"priceField,omitempty"`
  OldPrice string `json:"oldPrice,omitempty"`
  NewPrice string `json:"newPrice,omitempty"`
  UnitPrice string `json:"unitPrice,omitempty"`
  CartonPrice string `json:"cartonPrice,omitempty"`
}

// SQL Server decimal/money values must remain human-readable base-10 amounts.
// Rational representation (for example 401/4) is unsuitable for price alerts.
func pcOSItemNameCanonicalPrice(v string) (string,error) {
  v=strings.TrimSpace(v)
  if v=="" {return "",nil}
  if len(v)>64 {return "",errors.New("decimal price oversized")}
  if strings.HasPrefix(v,"+") {v=v[1:]}
  if strings.HasPrefix(v,"-") {return "",errors.New("negative price rejected")}
  parts:=strings.Split(v,".")
  if len(parts)>2 || parts[0]=="" || (len(parts)==2 && parts[1]=="") {
    return "",errors.New("invalid decimal price")
  }
  for _,part:=range parts {
    for _,digit:=range part {
      if digit<'0'||digit>'9' {return "",errors.New("invalid decimal price")}
    }
  }
  integer:=strings.TrimLeft(parts[0],"0")
  if integer=="" {integer="0"}
  if len(parts)==1 {return integer,nil}
  fraction:=strings.TrimRight(parts[1],"0")
  if fraction=="" {return integer,nil}
  return integer+"."+fraction,nil
}

func pcOSItemNameValidate(rows []pcOSItemNameRow)(map[string]pcOSItemNameRow,error) {
  if len(rows)==0 || len(rows)>150000{return nil,errors.New("invalid or empty ItemName snapshot")}
  out:=make(map[string]pcOSItemNameRow,len(rows))
  for _,r:=range rows {
    r.ID=strings.TrimSpace(r.ID)
    if r.ID==""||len(r.ID)>50{return nil,errors.New("missing ItemNameID")}
    if _,ok:=out[r.ID];ok{return nil,errors.New("duplicate ItemNameID")}
    if len(r.Name)>2048||len(r.Barcode)>512{return nil,errors.New("oversized product metadata")}
    unit,err:=pcOSItemNameCanonicalPrice(r.SalePrice);if err!=nil{return nil,err}
    carton,err:=pcOSItemNameCanonicalPrice(r.CartonPrice);if err!=nil{return nil,err}
    r.SalePrice,r.CartonPrice=unit,carton
    out[r.ID]=r
  }
  return out,nil
}

// No alerts for deletion, inventory changes, names, barcodes or heartbeat.
func pcOSItemNameDiff(previous,current []pcOSItemNameRow)([]pcOSItemNameChange,error) {
  old,err:=pcOSItemNameValidate(previous);if err!=nil{return nil,err}
  now,err:=pcOSItemNameValidate(current);if err!=nil{return nil,err}
  keys:=make([]string,0,len(now));for id:=range now{keys=append(keys,id)};sort.Strings(keys)
  changes:=make([]pcOSItemNameChange,0)
  for _,id:=range keys {
    n:=now[id];o,found:=old[id]
    if !found {
      changes=append(changes,pcOSItemNameChange{Kind:"added",ID:id,Name:n.Name,Barcode:n.Barcode,UnitPrice:n.SalePrice,CartonPrice:n.CartonPrice})
      continue
    }
    if o.SalePrice!=n.SalePrice {changes=append(changes,pcOSItemNameChange{Kind:"price_changed",ID:id,Name:n.Name,Barcode:n.Barcode,PriceField:"SalePrice",OldPrice:o.SalePrice,NewPrice:n.SalePrice})}
    if o.CartonPrice!=n.CartonPrice {changes=append(changes,pcOSItemNameChange{Kind:"price_changed",ID:id,Name:n.Name,Barcode:n.Barcode,PriceField:"WholeSalePrice",OldPrice:o.CartonPrice,NewPrice:n.CartonPrice})}
  }
  return changes,nil
}
