package main

import (
    "bytes"
    "encoding/csv"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func csvFixture() []pcOSItemNameRow {
    return []pcOSItemNameRow{
        {ID:"2",Name:"فازلين فارينا 180مل",Barcode:"0012345009876",SalePrice:"3000.00",CartonPrice:"0.000"},
        {ID:"1",Name:"=HYPERLINK(\"bad\")",Barcode:"8690000000001",SalePrice:"1000.50",CartonPrice:"12000"},
    }
}
func readExportCSV(t *testing.T, p string) [][]string {
    t.Helper()
    data,err:=os.ReadFile(p)
    if err!=nil {t.Fatal(err)}
    if !bytes.HasPrefix(data,[]byte{0xEF,0xBB,0xBF}){t.Fatal("missing Excel UTF-8 BOM")}
    reader:=csv.NewReader(bytes.NewReader(data[3:]))
    records,err:=reader.ReadAll()
    if err!=nil {t.Fatal(err)}
    return records
}
func TestFullLiveCSVIsUnicodeAndBarcodePreserving(t *testing.T) {
    path:=filepath.Join(t.TempDir(),pcOSItemNameCSVName)
    sha,updated,err:=pcOSItemNameEnsureLiveCSV(path,csvFixture())
    if err!=nil||!updated||len(sha)!=64 {t.Fatalf("export failed: %v %v %q",err,updated,sha)}
    rows:=readExportCSV(t,path)
    if len(rows)!=3 || len(rows[0])!=5 {t.Fatalf("expected 2 item rows: %v",rows)}
    if rows[1][1]!="'=HYPERLINK(\"bad\")" {t.Fatal("Excel formula injection unguarded",rows[1][1])}
    if rows[2][1]!="فازلين فارينا 180مل"||rows[2][2]!="0012345009876" {t.Fatal("unicode or leading zero barcode corrupted",rows[2])}
    if rows[2][3]!="3000"||rows[2][4]!="0" {t.Fatal("prices not canonical",rows[2])}
}
func TestLiveCSVIdempotentAndUpdatesOnPriceOrNewProduct(t *testing.T) {
    path:=filepath.Join(t.TempDir(),pcOSItemNameCSVName)
    rows:=csvFixture()
    first,updated,err:=pcOSItemNameEnsureLiveCSV(path,rows)
    if err!=nil||!updated {t.Fatal(err)}
    again,updated,err:=pcOSItemNameEnsureLiveCSV(path,rows)
    if err!=nil||updated||again!=first {t.Fatalf("unnecessary CSV rewrite: %v %v",updated,err)}
    rows[0].SalePrice="3300"
    latest,updated,err:=pcOSItemNameEnsureLiveCSV(path,rows)
    if err!=nil||!updated||latest==first {t.Fatal("price change not exported",err)}
    rows=append(rows,pcOSItemNameRow{ID:"3",Name:"شامپو",Barcode:"123",SalePrice:"7000",CartonPrice:"0"})
    _,updated,err=pcOSItemNameEnsureLiveCSV(path,rows)
    if err!=nil||!updated {t.Fatal("new product not exported",err)}
    out:=readExportCSV(t,path)
    if len(out)!=4||out[2][3]!="3300" {t.Fatal("CSV not refreshed correctly",out)}
}
func TestLiveCSVFailClosed(t *testing.T) {
    path:=filepath.Join(t.TempDir(),pcOSItemNameCSVName)
    rows:=csvFixture()
    if _,_,err:=pcOSItemNameEnsureLiveCSV(strings.TrimSuffix(path,".csv")+".bak",rows);err==nil{t.Fatal("unexpected name accepted")}
    _,_,err:=pcOSItemNameEnsureLiveCSV(path,rows)
    if err!=nil{t.Fatal(err)}
    rows[0].SalePrice="invalid"
    if _,_,err:=pcOSItemNameEnsureLiveCSV(path,rows);err==nil{t.Fatal("invalid price accepted")}
    saved:=readExportCSV(t,path)
    if saved[2][3]!="3000"{t.Fatal("original complete CSV should survive validation failure")}
}
