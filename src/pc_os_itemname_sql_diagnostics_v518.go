package main

import (
  "errors"
  "fmt"
  "strings"
)

// Extract only SQLCMD error text from process output; never echo table rows,
// product names, barcodes, query contents, or local authentication tokens.
func pcOSItemNameSQLCmdDiagnostic(stdout []string, stderr string, exitErr error) error {
 if exitErr == nil {return nil}
 out:=make([]string,0,4)
 candidates:=append(strings.Split(stderr,"\n"),stdout...)
 total:=0
 for _,item:=range candidates {
  item=strings.TrimSpace(item)
  lower:=strings.ToLower(item)
  if !(strings.HasPrefix(lower,"sqlcmd:")||
       strings.HasPrefix(lower,"msg ")||
       strings.HasPrefix(lower,"sqlstate")||
       strings.HasPrefix(lower,"ssl provider:")||
       strings.HasPrefix(lower,"error:")){continue}
  if len(item)>512{item=item[:512]+" [truncated]"}
  if total+len(item)>2048 {break}
  out=append(out,item)
  total+=len(item)
  if len(out)>=5 {break}
 }
 if len(out)==0 {return fmt.Errorf("read-only SQLCMD failed (%v); no safe diagnostics available",exitErr)}
 return errors.New("read-only SQLCMD failed: "+strings.Join(out,"; "))
}
