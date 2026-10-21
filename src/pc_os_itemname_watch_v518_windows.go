//go:build windows
package main
import (
 "bufio"
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "time"
 "unicode/utf16"
)
const pcOSItemNameMonitorInterval = 30*time.Minute
var pcOSItemNameStatusMu sync.Mutex
var pcOSItemNameCurrentStatus = map[string]interface{}{"active":false,"baseline_ready":false,"mode":"read_only_ItemName"}
type pcOSItemNameState struct {
 Version int
 Database string
 CapturedUTC string
 Rows []pcOSItemNameRow
}
func pcOSItemNameStatusSet(values map[string]interface{}){
 pcOSItemNameStatusMu.Lock();defer pcOSItemNameStatusMu.Unlock()
 for k,v:=range values{pcOSItemNameCurrentStatus[k]=v}
}
func pcOSItemNameStatusAction()(interface{},error){
 pcOSItemNameStatusMu.Lock();defer pcOSItemNameStatusMu.Unlock()
 x:=make(map[string]interface{},len(pcOSItemNameCurrentStatus))
 for k,v:=range pcOSItemNameCurrentStatus{x[k]=v}
 return x,nil
}
type pcOSItemNameWriter struct{out *bytes.Buffer}
func(w *pcOSItemNameWriter)Write(p []byte)(int,error){
 n:=len(p);if w.out.Len()<8192{
   take:=p;if len(take)>8192-w.out.Len(){take=take[:8192-w.out.Len()]}
   _,_=w.out.Write(take)
 }
 return n,nil
}
// Fixed local-host SQL only, no general SQL execution API and no SQL mutation.
func pcOSItemNameQuery(ctx context.Context,db,q string)([]string,error){
 if db==""||len(db)>128||strings.ContainsAny(db,"\r\n\x00"){return nil,errors.New("database name invalid")}
 bin,err:=pcOSFindSQLCmd();if err!=nil{return nil,err}
 cmd:=hiddenCommandContext(ctx,bin,"-S","localhost","-E","-C","-d",db,
  "-b","-h","-1","-w","8192","-y","8192","-l","10","-t","90","-r","1","-Q",q)
 stdout,err:=cmd.StdoutPipe();if err!=nil{return nil,err}
 stderr,err:=cmd.StderrPipe();if err!=nil{return nil,err}
 if err=cmd.Start();err!=nil{return nil,err}
 var stderrOut bytes.Buffer
 stderrDone:=make(chan struct{})
 go func(){_,_=io.Copy(&pcOSItemNameWriter{out:&stderrOut},stderr);close(stderrDone)}()
 scanner:=bufio.NewScanner(stdout);scanner.Buffer(make([]byte,8192),65536)
 lines:=make([]string,0,4096)
 tooMany:=false
 for scanner.Scan(){
  line:=strings.TrimSpace(scanner.Text())
  if line==""{continue}
  if len(lines)>=150000{tooMany=true;_=cmd.Process.Kill();break}
  lines=append(lines,line)
 }
 scanErr:=scanner.Err()
 waitErr:=cmd.Wait()
 <-stderrDone
 if scanErr!=nil{return nil,scanErr}
 if tooMany{return nil,errors.New("ItemName row limit reached")}
 if waitErr!=nil{return nil,fmt.Errorf("SQL read-only query failed: %s",strings.TrimSpace(stderrOut.String()))}
 return lines,nil
}
func pcOSItemNameFindDB(ctx context.Context)(string,error){
 names,err:=pcOSItemNameQuery(ctx,"master","SET NOCOUNT ON; SELECT name FROM sys.databases WHERE database_id>4 AND state_desc='ONLINE' ORDER BY name;")
 if err!=nil{return "",err}
 found:=[]string{}
 for _,db:=range names{
  if len(db)>128||strings.ContainsAny(db,"\r\n\x00"){return "",errors.New("unsafe database metadata")}
  x,e:=pcOSItemNameQuery(ctx,db,"SET NOCOUNT ON; SELECT CASE WHEN OBJECT_ID(N'dbo.ItemName',N'U') IS NOT NULL THEN '1' ELSE '0' END;")
  if e==nil&&len(x)==1&&x[0]=="1"{found=append(found,db)}
 }
 if len(found)==1{return found[0],nil}
 if len(found)==0{return "",errors.New("dbo.ItemName not accessible")}
 for _,db:=range found{if strings.EqualFold(db,"SuperMarket"){return db,nil}}
 return "",errors.New("multiple ItemName databases: cannot choose safely")
}
func pcOSItemNameDecodeHex(v string)(string,error){
 b,err:=hex.DecodeString(strings.TrimPrefix(v,"0x"));if err!=nil{return "",err}
 if len(b)%2!=0{return "",errors.New("UTF16LE odd bytes")}
 xs:=make([]uint16,len(b)/2)
 for i:=range xs{xs[i]=uint16(b[2*i])|uint16(b[2*i+1])<<8}
 return string(utf16.Decode(xs)),nil
}
const pcOSItemNameSelect = "SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT CONVERT(VARCHAR(40),ItemNameID) + '|' + COALESCE(CONVERT(VARCHAR(MAX),CONVERT(VARBINARY(MAX),CONVERT(NVARCHAR(512),ItemName)),2),'') + '|' + COALESCE(CONVERT(VARCHAR(MAX),CONVERT(VARBINARY(MAX),CONVERT(NVARCHAR(128),ItemCode)),2),'') + '|' + COALESCE(CONVERT(VARCHAR(64),SalePrice),'') + '|' + COALESCE(CONVERT(VARCHAR(64),WholeSalePrice),'') FROM dbo.ItemName ORDER BY ItemNameID;"
func pcOSItemNameFetch(ctx context.Context,db string)([]pcOSItemNameRow,error){
 lines,err:=pcOSItemNameQuery(ctx,db,pcOSItemNameSelect);if err!=nil{return nil,err}
 rows:=make([]pcOSItemNameRow,0,len(lines))
 for _,line:=range lines{
  p:=strings.Split(line,"|");if len(p)!=5{return nil,errors.New("ItemName output malformed or truncated")}
  name,e:=pcOSItemNameDecodeHex(p[1]);if e!=nil{return nil,e}
  code,e:=pcOSItemNameDecodeHex(p[2]);if e!=nil{return nil,e}
  rows=append(rows,pcOSItemNameRow{ID:p[0],Name:name,Barcode:code,SalePrice:p[3],CartonPrice:p[4]})
 }
 if len(rows)<1000{return nil,errors.New("ItemName snapshot unexpectedly small: quarantine")}
 if _,err=pcOSItemNameValidate(rows);err!=nil{return nil,err}
 return rows,nil
}
func pcOSItemNameVerifiedSnapshot(ctx context.Context,db string)([]pcOSItemNameRow,error){
 a,err:=pcOSItemNameFetch(ctx,db);if err!=nil{return nil,err}
 b,err:=pcOSItemNameFetch(ctx,db);if err!=nil{return nil,err}
 if !reflect.DeepEqual(a,b){return nil,errors.New("ItemName rows changed between verification passes")}
 return b,nil
}
func pcOSItemNameBaselinePath()string{return filepath.Join(pcOSActiveRoot(),"pc-os-itemname-baseline-v1.json")}
func pcOSItemNameReadBaseline()(pcOSItemNameState,error){
 var b pcOSItemNameState
 raw,err:=os.ReadFile(pcOSItemNameBaselinePath());if err!=nil{return b,err}
 if len(raw)>100*1024*1024{return b,errors.New("baseline oversized")}
 if err=json.Unmarshal(raw,&b);err!=nil{return b,err}
 if b.Version!=1{return b,errors.New("unsupported ItemName baseline version")}
 if _,err=pcOSItemNameValidate(b.Rows);err!=nil{return b,err}
 return b,nil
}
func pcOSItemNameStoreBaseline(b pcOSItemNameState)error{
 return pcOSWriteAtomicJSON(pcOSItemNameBaselinePath(),b)
}
func pcOSItemNameEvent(delta []pcOSItemNameChange, db,from,to string,offset int)error{
 creds,err:=pcOSLoadRelayCredentials();if err!=nil{return err}
 // Idempotency key is stable across retry after a lost server acknowledgement.
 canonical,_:=json.Marshal(delta)
 keyData:=sha256.Sum256(append([]byte(db+"|"+from+"|"+fmt.Sprint(offset)+"|"),canonical...))
 payload:=map[string]interface{}{
  "event_key":hex.EncodeToString(keyData[:]),"database":db,
  "previous_snapshot_utc":from,"current_snapshot_utc":to,
  "changes":delta,"read_only":true,
 }
 body,err:=json.Marshal(map[string]interface{}{"event_type":"itemname_delta","severity":"info","payload":payload})
 if err!=nil{return err}
 url:=creds.Relay+"/api/agent/"+creds.AgentID+"/pc-os/event"
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,url,bytes.NewReader(body));if err!=nil{return err}
 req.Header.Set("Content-Type","application/json");req.Header.Set("X-Agent-ID",creds.AgentID)
 req.Header.Set("Authorization","Bearer "+creds.Secret)
 res,err:=(&http.Client{Timeout:15*time.Second}).Do(req);if err!=nil{return err};defer res.Body.Close()
 if res.StatusCode!=202{return fmt.Errorf("relay refused ItemName event: %d",res.StatusCode)}
 return nil
}
func pcOSItemNameWatchCycle()error{
 ctx,cancel:=context.WithTimeout(context.Background(),6*time.Minute);defer cancel()
 db,err:=pcOSItemNameFindDB(ctx);if err!=nil{return err}
 now,err:=pcOSItemNameVerifiedSnapshot(ctx,db);if err!=nil{return err}
 b,err:=pcOSItemNameReadBaseline()
 next:=pcOSItemNameState{Version:1,Database:db,CapturedUTC:time.Now().UTC().Format(time.RFC3339),Rows:now}
 if os.IsNotExist(err){
  if err=pcOSItemNameStoreBaseline(next);err!=nil{return err}
  pcOSItemNameStatusSet(map[string]interface{}{"baseline_ready":true,"database":db,"item_count":len(now),"last_success_utc":next.CapturedUTC,"changes":0})
  return nil
 }
 if err!=nil{return err}
 if b.Database!=db{return errors.New("SQL ItemName database identity changed; refusing baseline advance")}
 if len(now)*10<len(b.Rows)*9{return errors.New("unexpected large product count decrease; quarantine")}
 changes,err:=pcOSItemNameDiff(b.Rows,now);if err!=nil{return err}
 if len(changes)>500{return errors.New("too many simultaneous changes; refusing unverified bulk alert")}
 for i:=0;i<len(changes);i+=25{
  j:=i+25;if j>len(changes){j=len(changes)}
  if err=pcOSItemNameEvent(changes[i:j],db,b.CapturedUTC,next.CapturedUTC,i);err!=nil{return err}
 }
 if err=pcOSItemNameStoreBaseline(next);err!=nil{return err}
 pcOSItemNameStatusSet(map[string]interface{}{"baseline_ready":true,"database":db,"item_count":len(now),"last_success_utc":next.CapturedUTC,"changes":len(changes)})
 return nil
}
func pcOSItemNameProbe()(interface{},error){
 ctx,cancel:=context.WithTimeout(context.Background(),50*time.Second);defer cancel()
 db,err:=pcOSItemNameFindDB(ctx)
 if err!=nil{return map[string]interface{}{"probe_ok":false,"error":err.Error(),"read_only":true},nil}
 lines,err:=pcOSItemNameQuery(ctx,db,"SET NOCOUNT ON; SELECT TOP(1) CONVERT(VARCHAR(40),ItemNameID) FROM dbo.ItemName ORDER BY ItemNameID;")
 if err!=nil{return map[string]interface{}{"probe_ok":false,"database":db,"error":err.Error(),"read_only":true},nil}
 return map[string]interface{}{"probe_ok":len(lines)==1,"database":db,"read_only":true,"zero_ui":true},nil
}
func pcOSItemNameWatchLoop(){
 time.Sleep(90*time.Second)
 run:=func(){
  pcOSItemNameStatusSet(map[string]interface{}{"active":true,"last_attempt_utc":time.Now().UTC().Format(time.RFC3339)})
  if err:=pcOSItemNameWatchCycle();err!=nil{pcOSItemNameStatusSet(map[string]interface{}{"last_error":err.Error()})}else{pcOSItemNameStatusSet(map[string]interface{}{"last_error":""})}
 }
 run()
 ticker:=time.NewTicker(pcOSItemNameMonitorInterval);defer ticker.Stop()
 for range ticker.C{run()}
}
