package main

import (
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
  "strings"
  "time"
)

// Upload-only channel for the Agent-owned CSV. Never reads/writes the legacy
// SuperMarket SQL DB or calls Admin.exe / Cashier.exe / Windows services.
const pcOSCSVCloudEndpoint = "https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/zhirox-live-csv-sync/api/agent/"
const pcOSCSVCloudLimit = 8388608

type pcOSCSVCloudState struct {
  SHA256 string `json:"sha256"`
  Rows int `json:"rows"`
  UploadedUTC string `json:"uploaded_utc"`
}

func pcOSCSVCloudPost(ctx context.Context, client *http.Client, endpoint, agentID, secret string, data []byte, digest string, rows int, capturedUTC string) error {
  if !strings.HasPrefix(endpoint, pcOSCSVCloudEndpoint) || !strings.HasSuffix(endpoint, "/upload") {
    return errors.New("CSV cloud endpoint not allowlisted")
  }
  if len(data) < 80 || len(data) > pcOSCSVCloudLimit {
    return errors.New("CSV cloud size rejected")
  }
  if rows < 1 || rows > 150000 || len(agentID)<6 || len(secret)<1 {
    return errors.New("CSV cloud metadata rejected")
  }
  sum:=sha256.Sum256(data)
  if hex.EncodeToString(sum[:]) != digest {
    return errors.New("CSV cloud digest mismatch")
  }
  if !bytes.HasPrefix(data,[]byte("\xef\xbb\xbfItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice\r\n")) {
    return errors.New("CSV cloud header rejected")
  }
  req,err:=http.NewRequestWithContext(ctx,http.MethodPost,endpoint,bytes.NewReader(data))
  if err!=nil{return err}
  req.Header.Set("x-agent-id",agentID)
  req.Header.Set("Authorization","Bearer "+secret)
  req.Header.Set("Content-Type","text/csv")
  req.Header.Set("x-csv-sha256",digest)
  req.Header.Set("x-csv-rows",fmt.Sprint(rows))
  req.Header.Set("x-csv-captured-utc",capturedUTC)
  res,err:=client.Do(req)
  if err!=nil{return err}
  defer res.Body.Close()
  if res.StatusCode!=http.StatusOK{
    return fmt.Errorf("private CSV sync rejected HTTP %d",res.StatusCode)
  }
  var reply struct {OK bool `json:"ok"`; SHA256 string `json:"sha256"`}
  if err=json.NewDecoder(io.LimitReader(res.Body,8192)).Decode(&reply);err!=nil{return err}
  if !reply.OK || reply.SHA256!=digest{return errors.New("CSV sync acknowledgement mismatch")}
  return nil
}

func pcOSItemNameCloudUpload(csvPath,digest string,rows int) error {
  if filepath.Clean(csvPath)!=filepath.Join(pcOSActiveRoot(),"exports",pcOSItemNameCSVName){
    return errors.New("CSV cloud upload path not allowlisted")
  }
  statePath:=filepath.Join(pcOSActiveRoot(),"pc-os-csv-cloud-ack.json")
  stateFile,err:=os.ReadFile(statePath)
  if err==nil{
    var prior pcOSCSVCloudState
    if json.Unmarshal(stateFile,&prior)==nil && prior.SHA256==digest && prior.Rows==rows {
      pcOSItemNameStatusSet(map[string]interface{}{"csv_cloud_ready":true,"csv_cloud_sha256":digest,"csv_cloud_error":""})
      return nil
    }
  }
  fi,err:=os.Lstat(csvPath);if err!=nil{return err}
  if !fi.Mode().IsRegular() || fi.Size()<80 || fi.Size()>pcOSCSVCloudLimit{
    return errors.New("CSV cloud source not a bounded regular file")
  }
  csvData,err:=os.ReadFile(csvPath);if err!=nil{return err}
  creds,err:=pcOSLoadRelayCredentials();if err!=nil{return err}
  if creds.Relay!="https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/zhirox-pc-relay" {
    return errors.New("CSV private sync requires pinned Supabase project endpoint")
  }
  ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel()
  client:= &http.Client{Timeout:40*time.Second}
  capturedUTC:=time.Now().UTC().Format(time.RFC3339)
  endpoint:=pcOSCSVCloudEndpoint+creds.AgentID+"/upload"
  if err:=pcOSCSVCloudPost(ctx,client,endpoint,creds.AgentID,creds.Secret,csvData,digest,rows,capturedUTC);err!=nil{return err}
  if err:=pcOSWriteAtomicJSON(statePath,pcOSCSVCloudState{SHA256:digest,Rows:rows,UploadedUTC:capturedUTC});err!=nil{return err}
  pcOSItemNameStatusSet(map[string]interface{}{"csv_cloud_ready":true,"csv_cloud_sha256":digest,"csv_cloud_error":"","csv_cloud_uploaded_utc":capturedUTC})
  return nil
}
