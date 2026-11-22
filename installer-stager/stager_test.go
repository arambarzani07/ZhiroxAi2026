package stager

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

type fakeTransport func(*http.Request) (*http.Response, error)
func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func bodyResponse(status int, data []byte, length int64) *http.Response {
    return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)),
       Header: make(http.Header), ContentLength:length}
}
const safeURL = "https://release-assets.githubusercontent.com/signed-asset?token=short"

func TestFixedArtifactPolicy(t *testing.T) {
    if FileName != "Cashierynwe-Setup-1.0.10-x64.exe" || FileSize != 158128464 ||
       FileSHA256 != "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b" {
        t.Fatal("production release pinned artifact modified")
    }
    good := []string{
        safeURL, "https://objects.githubusercontent.com/asset?sig=x",
    }
    for _,url:=range good {
        if err:=ValidateCDNURL(url);err!=nil { t.Fatalf("unexpected trusted CDN refusal: %v",err) }
    }
    bad := []string{
        "", "https://evil.example/asset", "http://release-assets.githubusercontent.com/asset",
        "https://release-assets.githubusercontent.com.evil.example/asset",
        "https://user:pass@release-assets.githubusercontent.com/file",
        "https://release-assets.githubusercontent.com:8443/file",
        "https://127.0.0.1/a", "file:///C:/Windows/a.exe",
        "https://release-assets.githubusercontent.com/a#fragment",
        "https://release-assets.githubusercontent.com/a%2fb",
    }
    for _,url:=range bad {
        if ValidateCDNURL(url)==nil { t.Errorf("untrusted CDN URL accepted: %s",url) }
    }
    if !ValidApprovalID("12345678-1234-4234-8234-1234567890ab") ||
        ValidApprovalID("x") {t.Fatal("approval id grammar invalid")}
}

func TestControlPlaneTokenNotForwardedAndExactResponse(t *testing.T) {
    approval:="12345678-1234-4234-8234-1234567890ab"
    response:=fmt.Sprintf(
      `{"url":%q,"app_id":"cashierynwe","version":"1.0.10","filename":"Cashierynwe-Setup-1.0.10-x64.exe","sha256":"%s","size":158128464}`,
      safeURL, FileSHA256)
    transport:=fakeTransport(func(req *http.Request)(*http.Response,error){
        if req.URL.Hostname()!="vllwundrtrgdqmkqbduy.supabase.co" ||
           req.Header.Get("Authorization")!="Bearer agent-secret" ||
           req.Header.Get("X-Agent-ID")!=AgentID ||
           req.URL.Query().Get("approval_id")!=approval {
            t.Fatal("control plane request not exactly agent-bound")
        }
        return bodyResponse(http.StatusOK,[]byte(response),-1),nil
    })
    url,err:=FetchApprovedLocation(context.Background(),&http.Client{Transport:transport},"agent-secret",approval)
    if err!=nil || url!=safeURL {t.Fatalf("valid response refused: %v",err)}
    for _,bad:=range []string{
        strings.Replace(response,`"size":158128464`,`"size":1`,1),
        strings.Replace(response,`"app_id":"cashierynwe"`,`"app_id":"other"`,1),
        strings.Replace(response,`"url":`,`"evil":"x","url":`,1),
        response+"{}",
    } {
        denied:=&http.Client{Transport:fakeTransport(func(_ *http.Request)(*http.Response,error){
            return bodyResponse(http.StatusOK,[]byte(bad),-1),nil
        })}
        if _,e:=FetchApprovedLocation(context.Background(),denied,"agent-secret",approval);e==nil {
            t.Fatal("unsafe control plane response accepted")
        }
    }
    if _,err:=FetchApprovedLocation(context.Background(),nil,"",approval);err==nil {
        t.Fatal("blank agent token accepted")
    }
    if _,err:=FetchApprovedLocation(context.Background(),nil,"agent-secret","wrong");err==nil {
        t.Fatal("forged approval accepted")
    }
}

func TestStreamingHashSizeAndAtomicCommit(t *testing.T) {
    content:=[]byte("this is a synthetic test installer")
    expectedSize:=int64(len(content))
    sum:=sha256.Sum256(content)
    digest:=hex.EncodeToString(sum[:])
    dir:=t.TempDir()
    var calls int
    transport:=fakeTransport(func(req *http.Request)(*http.Response,error){
        calls++
        if req.URL.Hostname()!="release-assets.githubusercontent.com" ||
           req.Header.Get("Authorization")!="" ||
           req.Header.Get("Accept-Encoding")!="identity" {
           t.Fatal("cdn must never receive agent secret and must be uncompressed")
        }
        return bodyResponse(200,content,expectedSize),nil
    })
    client:=&http.Client{Transport:transport}
    if e:=downloadVerifiedWithSpec(context.Background(),client,safeURL,dir,"test-installer.exe",expectedSize,digest);e!=nil {
        t.Fatalf("valid synthetic installer rejected: %v",e)
    }
    data,err:=os.ReadFile(filepath.Join(dir,"test-installer.exe"))
    if err!=nil || !bytes.Equal(data,content) {t.Fatal("committed file bytes wrong")}
    if e:=downloadVerifiedWithSpec(context.Background(),client,safeURL,dir,"test-installer.exe",expectedSize,digest);e==nil {
        t.Fatal("existing artifact overwritten")
    }
    if calls!=1 {t.Fatal("no network call should occur after destination already exists")}

    for _,test:=range []struct{
        name string
        httpStatus int
        bytes []byte
        contentLength int64
        expectedSHA string
    }{
        {"mismatch-hash",200,content,expectedSize,strings.Repeat("0",64)},
        {"oversize",200,append(bytes.Clone(content),'X'),-1,digest},
        {"truncate",200,content[:3],-1,digest},
        {"redirect",302,content,expectedSize,digest},
        {"wrong-content-length",200,content,1,digest},
    }{
        destDir:=t.TempDir()
        custom:=&http.Client{Transport:fakeTransport(func(_ *http.Request)(*http.Response,error){
            return bodyResponse(test.httpStatus,test.bytes,test.contentLength),nil
        })}
        if e:=downloadVerifiedWithSpec(context.Background(),custom,safeURL,destDir,"candidate.exe",expectedSize,test.expectedSHA);e==nil {
            t.Errorf("%s unexpectedly accepted",test.name)
        }
        if _,err:=os.Stat(filepath.Join(destDir,"candidate.exe"));!os.IsNotExist(err) {
            t.Errorf("%s left a published executable",test.name)
        }
    }
}
