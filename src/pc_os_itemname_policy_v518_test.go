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

func TestItemNameDecimalPricesForAlerts(t *testing.T){
 cases:=[]struct{input,expected string}{
  {"00100.2500","100.25"},
  {"100.25","100.25"},
  {"0000.0000","0"},
  {"00100.0000","100"},
  {"0000.5000","0.5"},
  {" +0005.0900 ","5.09"},
  {"9007199254740993.123456789","9007199254740993.123456789"},
 }
 for _,tc:=range cases {
  got,err:=pcOSItemNameCanonicalPrice(tc.input)
  if err!=nil||got!=tc.expected {t.Fatalf("canonical price %q: got=%q want=%q err=%v",tc.input,got,tc.expected,err)}
 }
}

func TestItemNameRejectNonDecimalPrices(t *testing.T){
 for _,input:=range []string{"-0.01","-0","1/3","1e3","NaN","1,000","12.","0..1","abc"}{
  if _,err:=pcOSItemNameCanonicalPrice(input);err==nil {t.Fatalf("accepted invalid price %q",input)}
 }
}

func TestItemNamePriceEventUsesDecimalNotRational(t *testing.T){
 before:=pcOSIR("7","1200.25","24000.5000")
 after:=pcOSIR("7","1200.50","24000.5")
 changes,err:=pcOSItemNameDiff([]pcOSItemNameRow{before},[]pcOSItemNameRow{after})
 if err!=nil||len(changes)!=1 {t.Fatalf("unexpected events: %+v err=%v",changes,err)}
 if changes[0].OldPrice!="1200.25"||changes[0].NewPrice!="1200.5"||changes[0].PriceField!="SalePrice"{
  t.Fatalf("prices must be normal base-10 decimal strings: %+v",changes[0])
 }
}
