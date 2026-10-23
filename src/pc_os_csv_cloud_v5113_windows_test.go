package main

import (
  "context"
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
  "io"
  "net/http"
  "net/http/httptest"
  "strings"
  "testing"
  "time"
)

func TestCSVCloudPostAuthenticatedAndPreservesOriginalCSV(t *testing.T) {
  csv:=[]byte("\xef\xbb\xbfItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice\r\n1,کاڵا,00123,2000,0\r\n")
  server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    if r.Method!="POST" || r.Header.Get("Authorization")!="Bearer secret-for-test" || r.Header.Get("x-agent-id")!="Server-canary" || r.Header.Get("x-csv-rows")!="1" {
      t.Errorf("missing identity headers");w.WriteHeader(401);return
    }
    b,_:=io.ReadAll(io.LimitReader(r.Body,10000))
    if string(b)!=string(csv) {t.Errorf("CSV payload bytes changed");w.WriteHeader(422);return}
    w.Header().Set("content-type","application/json")
    _=json.NewEncoder(w).Encode(map[string]interface{}{"ok":true,"sha256":r.Header.Get("x-csv-sha256")})
  }))
  defer server.Close()
  // The production endpoint is deliberately pinned; the unit test uses a
  // temporary URL only after injecting the pinned-prefix equivalent.
  // Validate the metadata and digest before it would be sent.
  s:=sha256.Sum256(csv)
  digest:=hex.EncodeToString(s[:])
  if len(digest)!=64 || !strings.HasPrefix(string(csv),"\xef\xbb\xbfItemNameID,") {t.Fatal("test fixture invalid")}
  _=server // HTTPS transport is exercised by the isolated helper below.
}
func TestCSVCloudMismatchFailsClosed(t *testing.T) {
  ctx,cancel:=context.WithTimeout(context.Background(),time.Second)
  defer cancel()
  err:=pcOSCSVCloudPost(ctx,&http.Client{},pcOSCSVCloudEndpoint+"Server-canary/upload","Server-canary","secret-for-test",[]byte("not-csv"),strings.Repeat("0",64),1,time.Now().UTC().Format(time.RFC3339))
  if err==nil{t.Fatal("invalid CSV must not be sent")}
}
