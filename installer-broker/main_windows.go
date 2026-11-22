//go:build windows

// Zhirox attended broker: locally confirmed, fixed-artifact, single-user
// install only. This UNSIGNED CANDIDATE is intentionally not production-enabled.
package main

import (
    "context"
    "crypto/sha256"
    "crypto/subtle"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io"
    "os"
    "os/exec"
    "os/user"
    "path/filepath"
    "strings"
    "syscall"
    "time"
    "unsafe"
)

const (
    brokerProductionEnabled = false
    brokerTicketFile = `C:\ProgramData\ZhiroxAI\InstallRequests\cashierynwe-1.0.10.json`
    brokerStagingFolder = `C:\ProgramData\ZhiroxAI\ApprovedInstallers`
    brokerNoWindow = 0x08000000
)

func brokerSessionOK() bool {
    currentUser, err := user.Current()
    if err != nil || currentUser == nil {return false}
    name:=strings.ToLower(currentUser.Username)
    if strings.Contains(name,"system") || strings.EqualFold(os.Getenv("USERNAME"),"SYSTEM") {
        return false
    }
    home:=os.Getenv("LOCALAPPDATA")
    if home=="" || !filepath.IsAbs(home) {return false}
    kernel:=syscall.NewLazyDLL("kernel32.dll")
    idProc:=kernel.NewProc("ProcessIdToSessionId")
    activeProc:=kernel.NewProc("WTSGetActiveConsoleSessionId")
    active,_,_:=activeProc.Call()
    if uint32(active)==0 || uint32(active)==0xFFFFFFFF {return false}
    var current uint32
    ok,_,_:=idProc.Call(uintptr(os.Getpid()),uintptr(unsafe.Pointer(&current)))
    return ok!=0 && current==uint32(active)
}
func brokerHidden(cmd *exec.Cmd) {
    cmd.SysProcAttr=&syscall.SysProcAttr{
        HideWindow:true,
        CreationFlags:brokerNoWindow,
    }
}
func brokerConsent() bool {
    question,err1:=syscall.UTF16PtrFromString("Cashierynwe v1.0.10\nڕەزامەندیت بە دامەزراندنی ئەم ئەپەی پشکنراوە؟")
    title,err2:=syscall.UTF16PtrFromString("ZHIROX — ڕەزامەندی دامەزراندن")
    if err1!=nil || err2!=nil{return false}
    dlg:=syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
    // MB_YESNO | MB_ICONQUESTION | MB_DEFBUTTON2 : No is default.
    answer,_,_:=dlg.Call(0,uintptr(unsafe.Pointer(question)),uintptr(unsafe.Pointer(title)),0x4|0x20|0x100)
    return answer==6
}

func brokerVerifyTrustedPublisher(file string) error {
    systemRoot:=os.Getenv("SystemRoot")
    if systemRoot=="" {return errors.New("windows_system_root_missing")}
    ps:=filepath.Join(systemRoot,"System32","WindowsPowerShell","v1.0","powershell.exe")
    escaped:=strings.ReplaceAll(file,"'","''")
    verifyScript:="$ErrorActionPreference='Stop'; $s=Get-AuthenticodeSignature -LiteralPath '"+escaped+"'; "+
        "if($s.Status -ne 'Valid' -or !$s.SignerCertificate -or $s.SignerCertificate.Subject -cne '"+brokerPublisher+"'){exit 41}; "+
        "$e=@($s.SignerCertificate.Extensions|Where-Object{$_.Oid.Value -eq '2.5.29.37'}); "+
        "if($e.Count -ne 1){exit 42}; "+
        "$o=@(([System.Security.Cryptography.X509Certificates.X509EnhancedKeyUsageExtension]$e[0]).EnhancedKeyUsages|ForEach-Object{$_.Value}); "+
        "if($o -notcontains '1.3.6.1.5.5.7.3.3'){exit 43}; exit 0"
    ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
    defer cancel()
    cmd:=exec.CommandContext(ctx,ps,"-NoLogo","-NoProfile","-NonInteractive","-Command",verifyScript)
    brokerHidden(cmd)
    if err:=cmd.Run();err!=nil{return errors.New("authenticode_trust_or_publisher_mismatch")}
    return nil
}

func brokerVerifyPrivateCopy() (string, func(), error) {
    staged:=filepath.Join(brokerStagingFolder,brokerFileName)
    dir,err:=os.Lstat(brokerStagingFolder)
    if err!=nil||!dir.IsDir()||dir.Mode()&os.ModeSymlink!=0 {
        return "",nil,errors.New("approved_staging_folder_unavailable")
    }
    info,err:=os.Lstat(staged)
    if err!=nil||!info.Mode().IsRegular()||info.Size()!=brokerFileSize {
        return "",nil,errors.New("signed_installer_missing_or_size_mismatch")
    }
    tmp,err:=os.MkdirTemp("","zhirox-broker-verified-")
    if err!=nil{return "",nil,errors.New("private_directory_creation_failed")}
    cleanup:=func(){_ = os.RemoveAll(tmp)}
    dst:=filepath.Join(tmp,brokerFileName)
    source,err:=os.Open(staged)
    if err!=nil{cleanup();return "",nil,errors.New("signed_installer_unreadable")}
    target,err:=os.OpenFile(dst,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
    if err!=nil{source.Close();cleanup();return "",nil,errors.New("private_copy_open_failed")}
    h:=sha256.New()
    n,copyErr:=io.Copy(io.MultiWriter(target,h),io.LimitReader(source,brokerFileSize+1))
    closeErr:=target.Close()
    source.Close()
    expected,decodeErr:=hex.DecodeString(brokerSHA256)
    if copyErr!=nil||closeErr!=nil||decodeErr!=nil||n!=brokerFileSize||
        subtle.ConstantTimeCompare(h.Sum(nil),expected)!=1 {
        cleanup()
        return "",nil,errors.New("verified_copy_sha256_or_size_mismatch")
    }
    if err:=brokerVerifyTrustedPublisher(dst);err!=nil{
        cleanup();return "",nil,err
    }
    return dst,cleanup,nil
}

func brokerCurrentAppActive() bool {
    sys:=os.Getenv("SystemRoot")
    if sys=="" {return true}
    tasklist:=filepath.Join(sys,"System32","tasklist.exe")
    ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
    defer cancel()
    cmd:=exec.CommandContext(ctx,tasklist,"/FI","IMAGENAME eq Cashierynwe POS.exe","/NH","/FO","CSV")
    brokerHidden(cmd)
    output,err:=cmd.Output()
    if err!=nil{return true} // failure to detect is hard blocker
    return strings.Contains(strings.ToLower(string(output)),"cashierynwe pos.exe")
}

func brokerConsumeRequest(requestID string) error {
    root:=filepath.Join(os.Getenv("LOCALAPPDATA"),"ZhiroxAI","InstallerBroker","Consumed")
    if err:=os.MkdirAll(root,0700);err!=nil{return errors.New("local_replay_store_unavailable")}
    lock:=filepath.Join(root,requestID+".lock")
    f,err:=os.OpenFile(lock,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
    if err!=nil{return errors.New("request_consumed_or_replay_store_inaccessible")}
    if err:=f.Close();err!=nil{return errors.New("replay_lock_close_failed")}
    return nil
}

func brokerInstall(t InstallTicket) string {
    if !brokerProductionEnabled {return "broker_production_gate_disabled"}
    if !brokerSessionOK(){return "interactive_console_user_required"}
    localApp:=filepath.Join(os.Getenv("LOCALAPPDATA"),"Programs","Cashierynwe POS")
    if _,err:=os.Stat(localApp);err==nil{return "already_installed_use_app_updater"}
    if brokerCurrentAppActive(){return "cashier_active_or_undetermined"}
    if !brokerConsent(){return "local_consent_declined"}
    // Consumes the exact server-approved request before any installer launch.
    if err:=brokerConsumeRequest(t.RequestID);err!=nil{return "replay_or_local_lock_failed"}
    verified,cleanup,err:=brokerVerifyPrivateCopy()
    if err!=nil{return err.Error()}
    defer cleanup()
    ctx,cancel:=context.WithTimeout(context.Background(),5*time.Minute)
    defer cancel()
    install:=exec.CommandContext(ctx,verified,"/S")
    brokerHidden(install)
    if err:=install.Run();err!=nil{return "signed_installer_execution_failed"}
    // A zero exit code is NOT acceptance; separate app smoke test required.
    return "installer_exited_zero_needs_app_verification"
}

func brokerWriteStatus(state string) {
    local:=os.Getenv("LOCALAPPDATA")
    if local==""||!filepath.IsAbs(local){return}
    folder:=filepath.Join(local,"ZhiroxAI","InstallerBroker")
    if os.MkdirAll(folder,0700)!=nil{return}
    report:=map[string]interface{}{
        "app_id":brokerAppID,"version":brokerVersion,"state":state,
        "checked_utc":time.Now().UTC().Format(time.RFC3339),
        "physical_install_verified":false,
    }
    data,err:=json.Marshal(report)
    if err!=nil{return}
    tmp:=filepath.Join(folder,"status.json.tmp")
    target:=filepath.Join(folder,"status.json")
    if err:=os.WriteFile(tmp,data,0600);err==nil{_ = os.Rename(tmp,target)}
}

func main() {
    if len(os.Args)!=1 {
        brokerWriteStatus("arbitrary_arguments_rejected")
        return
    }
    // Hard source-level rollout gate: unsigned CI artifacts never install.
    if !brokerProductionEnabled {
        brokerWriteStatus("broker_production_gate_disabled")
        return
    }
    if !brokerSessionOK() {
        brokerWriteStatus("interactive_console_user_required")
        return
    }
    ticket,err:=readTicket(brokerTicketFile,time.Now().UTC())
    if err!=nil{
        brokerWriteStatus("approved_ticket_invalid_or_unavailable")
        return
    }
    brokerWriteStatus(brokerInstall(ticket))
}
