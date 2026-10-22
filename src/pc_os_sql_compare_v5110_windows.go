//go:build windows
package main

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "time"
 "fmt"
)

type sqlCompareClient struct { Name string; Path string; OptionalEncryption bool }
type sqlCompareStage struct { Name string; Database string; Query string; Expected string }

var sqlCompareClients=[]sqlCompareClient{
 {"ODBC18_sqlcmd", `C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\180\Tools\Binn\SQLCMD.EXE`,true},
 {"ODBC17_sqlcmd", `C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\170\Tools\Binn\SQLCMD.EXE`,false},
 {"NativeClient10_sqlcmd", `C:\Program Files\Microsoft SQL Server\100\Tools\Binn\SQLCMD.EXE`,false},
}
var sqlCompareStages=[]sqlCompareStage{
 {"master","master","SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT 1;","1"},
 {"SuperMarket","SuperMarket","SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT 1;","1"},
 {"ItemNameVisible","SuperMarket","SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT CASE WHEN OBJECT_ID(N'dbo.ItemName',N'U') IS NOT NULL THEN 1 ELSE 0 END;","1"},
 {"SharedMemory","SuperMarket","SET NOCOUNT ON; SET LOCK_TIMEOUT 3000; SELECT CONVERT(VARCHAR(40),CONNECTIONPROPERTY('net_transport'));","Shared memory"},
}

// Fixed command, fixed local SQL endpoint, fixed SELECT-only probes. No raw
// SQL/driver output or business data returned through the relay.
func pcOSSQLCompareV2Action()(interface{},error) {
 type stageOutcome struct { Stage string `json:"stage"`; Passed bool `json:"passed"`; Category string `json:"category"` }
 type clientOutcome struct { Client string `json:"client"`; Found bool `json:"found"`; Completed bool `json:"completed"`; Category string `json:"classification"`; Stages []stageOutcome `json:"stages"` }
 result:=map[string]interface{}{
   "diagnostic":"Zhirox fixed read-only SQLCMD comparison v2",
   "target":"lpc:Server",
   "required_transport":"Shared memory",
   "read_only":true,
   "sql_writes":false,
   "arbitrary_sql":false,
   "results":[]clientOutcome{},
 }
 host,e:=os.Hostname()
 if e!=nil || !strings.EqualFold(host,"Server") {result["blocked"]="UNEXPECTED_LOCAL_HOST";return result,nil}
 ctx,stop:=context.WithTimeout(context.Background(),80*time.Second);defer stop()
 entries:=make([]clientOutcome,0,len(sqlCompareClients))
 for _,client:=range sqlCompareClients {
   entry:=clientOutcome{Client:client.Name,Stages:[]stageOutcome{},Category:"NOT_TESTED"}
   info,err:=os.Lstat(client.Path)
   if err!=nil {entry.Category="CLIENT_BINARY_NOT_INSTALLED";entries=append(entries,entry);continue}
   if info.Mode()&os.ModeSymlink!=0 || !info.Mode().IsRegular() || filepath.Clean(client.Path)!=client.Path {entry.Category="CLIENT_BINARY_NOT_REGULAR";entries=append(entries,entry);continue}
   entry.Found=true
   for _,probe:=range sqlCompareStages {
     if ctx.Err()!=nil {entry.Category="PROBE_TOTAL_TIMEOUT";break}
     per,cancel:=context.WithTimeout(ctx,12*time.Second)
     args:=[]string{"-S","lpc:Server","-E"}
     if client.OptionalEncryption {args=append(args,"-No")}
     args=append(args,"-d",probe.Database,"-b","-h","-1","-l","5","-t","9","-r","1","-Q",probe.Query)
     cmd:=hiddenCommandContext(per,client.Path,args...)
     output,runErr:=cmd.CombinedOutput()
     category:="NONE"
     pass:=false
     if per.Err()!=nil {category="SQLCMD_TIMEOUT"} else if runErr!=nil {category=pcOSSQLCompareCategory(output)} else if strings.TrimSpace(string(output))==probe.Expected {pass=true} else {category="UNEXPECTED_FIXED_RESULT"}
     cancel()
     entry.Stages=append(entry.Stages,stageOutcome{Stage:probe.Name,Passed:pass,Category:category})
     if !pass {entry.Category=category;break}
   }
   if len(entry.Stages)==len(sqlCompareStages) && entry.Stages[len(entry.Stages)-1].Passed {entry.Completed=true;entry.Category="PASS"}
   entries=append(entries,entry)
 }
 result["results"]=entries
 // Deliberately no product names, SQL statements, stdout/stderr, computer
 // credentials or raw error messages in the JSON response.
 return result,nil
}

func pcOSSQLCompareCategory(raw []byte)string{
 if len(raw)>8192 {raw=raw[:8192]}
 s:=strings.ToLower(string(raw))
 switch {
 case strings.Contains(s,"unknown option"),strings.Contains(s,"invalid option"),strings.Contains(s,"unrecognized option"),strings.Contains(s,"unrecognized command"),strings.Contains(s,"option not recognized"):
   return "ARGUMENT_UNSUPPORTED"
 case strings.Contains(s,"data source name not found"),strings.Contains(s,"specified driver could not be loaded"),strings.Contains(s,"driver not found"),strings.Contains(s,"im002"):
   return "CLIENT_DRIVER"
 case strings.Contains(s,"target principal name"),strings.Contains(s,"hostname in certificate"),strings.Contains(s,"certificate name mismatch"):
   return "TLS_CERT_NAME"
 case strings.Contains(s,"certificate chain"),strings.Contains(s,"certificate verify"),strings.Contains(s,"self-signed"),strings.Contains(s,"not trusted"):
   return "TLS_CERT_TRUST"
 case strings.Contains(s,"ssl provider"),strings.Contains(s,"tls handshake"),strings.Contains(s,"handshake failure"),strings.Contains(s,"protocol version"):
   return "TLS_PROTOCOL"
 case strings.Contains(s,"login failed"),strings.Contains(s,"18456"):
   return "WINDOWS_LOGIN"
 case strings.Contains(s,"cannot open database"),strings.Contains(s,"4060"):
   return "DATABASE_ACCESS"
 case strings.Contains(s,"shared memory provider"),strings.Contains(s,"no process is on the other end of the pipe"):
   return "LOCAL_SHARED_MEMORY"
 case strings.Contains(s,"network-related"),strings.Contains(s,"08001"),strings.Contains(s,"server was not found"),strings.Contains(s,"connection timeout"):
   return "LOCAL_CONNECTION"
 case strings.Contains(s,"invalid object name"),strings.Contains(s,"permission denied"),strings.Contains(s,"permission was denied"):
   return "SCHEMA_ACCESS"
 }
 if len(strings.TrimSpace(s))==0{return "SILENT_EXIT"}
 _=fmt.Sprintf // ensure no formatting of raw text gets exposed.
 return "OTHER_REDACTED"
}
