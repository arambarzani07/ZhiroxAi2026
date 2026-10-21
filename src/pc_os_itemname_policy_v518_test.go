package main
import "testing"
func pcOSIR(id,unit,carton string) pcOSItemNameRow {return pcOSItemNameRow{ID:id,Name:"کاڵا",Barcode:"8697426455846",SalePrice:unit,CartonPrice:carton}}
func TestItemNameAddedAndPrice(t *testing.T){
 a:=pcOSIR("1","1000.00","12000")
 b:=pcOSIR("1","1000.000","14000")
 c:=pcOSIR("2","2000","24000")
 changes,err:=pcOSItemNameDiff([]pcOSItemNameRow{a},[]pcOSItemNameRow{b,c})
 if err!=nil||len(changes)!=2{t.Fatalf("expected 2 changes: %v %v",changes,err)}
 if changes[0].PriceField!="WholeSalePrice"||changes[1].Kind!="added"{t.Fatalf("wrong events: %+v",changes)}
 if changes[1].UnitPrice==""||changes[1].CartonPrice==""{t.Fatal("added item prices absent")}
}
func TestItemNameIgnoreMetadataAndRemoval(t *testing.T){
 a:=pcOSIR("1","100","1000")
 b:=a;b.Name="ناوی نوێ";b.Barcode="12345"
 x,e:=pcOSItemNameDiff([]pcOSItemNameRow{a},[]pcOSItemNameRow{b});if e!=nil||len(x)!=0{t.Fatal(x,e)}
 c:=pcOSIR("2","20","200")
 x,e=pcOSItemNameDiff([]pcOSItemNameRow{a,c},[]pcOSItemNameRow{a});if e!=nil||len(x)!=0{t.Fatal(x,e)}
}
func TestItemNameFailClosed(t *testing.T){
 a:=pcOSIR("1","100","1000")
 if _,e:=pcOSItemNameDiff([]pcOSItemNameRow{a},[]pcOSItemNameRow{a,a});e==nil{t.Fatal("duplicate ItemNameID")}
 if _,e:=pcOSItemNameDiff([]pcOSItemNameRow{a},nil);e==nil{t.Fatal("empty snapshot")}
 a.SalePrice="invalid"
 if _,e:=pcOSItemNameDiff([]pcOSItemNameRow{a},[]pcOSItemNameRow{a});e==nil{t.Fatal("invalid price")}
}
func TestItemNameDecimalAndNegative(t *testing.T){
 a,_:=pcOSItemNameCanonicalPrice("00100.2500")
 b,_:=pcOSItemNameCanonicalPrice("100.25")
 if a!=b{t.Fatalf("%q != %q",a,b)}
 if _,e:=pcOSItemNameCanonicalPrice("-3");e==nil{t.Fatal("negative")}
}
