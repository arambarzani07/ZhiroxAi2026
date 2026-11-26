//go:build windows

package main

import (
 "io"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

const cashierynweFileFlagOpenReparsePoint = 0x00200000
const cashierynweFileAttributeReparsePoint = 0x400

var cashierynweGetFinalPath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// Only fixed storage roots documented by Cashierynwe. No discovery of arbitrary
// user paths, no shell, no SQLite access and no directory enumeration.
var cashierynweJournalPaths = [...]string{
 "D:\\ZHIROX_POS\\Updates\\auto_update_state.json",
 "C:\\ProgramData\\ZHIROX_POS\\Updates\\auto_update_state.json",
}

func cashierynweReadFixedJournal(path string, now time.Time) cashierynweUpdateStateReport {
 out:=cashierynweEmptyUpdateState("unavailable",now)
 // Reject any intermediate reparse point. A final handle path check also rejects
 // redirection between attribute checks and open. Never read an unverified handle.
 for p:=path; ; p=filepath.Dir(p) {
  ptr,err:=syscall.UTF16PtrFromString(p)
  if err!=nil{return out}
  attrs,err:=syscall.GetFileAttributes(ptr)
  if err==syscall.ERROR_FILE_NOT_FOUND||err==syscall.Errno(3){out.State="not_found";return out}
  if err!=nil||attrs&cashierynweFileAttributeReparsePoint!=0{return out}
  if filepath.Dir(p)==p{break}
 }
 ptr,err:=syscall.UTF16PtrFromString(path)
 if err!=nil{return out}
 h,err:=syscall.CreateFile(ptr,syscall.GENERIC_READ,syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,nil,syscall.OPEN_EXISTING,cashierynweFileFlagOpenReparsePoint,0)
 if err!=nil{return out}
 file:=os.NewFile(uintptr(h),"fixed-cashierynwe-journal")
 if file==nil{syscall.CloseHandle(h);return out}
 defer file.Close()
 var info syscall.ByHandleFileInformation
 if syscall.GetFileInformationByHandle(h,&info)!=nil||info.FileAttributes&(syscall.FILE_ATTRIBUTE_DIRECTORY|cashierynweFileAttributeReparsePoint)!=0{return out}
 var buf [1024]uint16
 n,_,_:=cashierynweGetFinalPath.Call(uintptr(h),uintptr(unsafe.Pointer(&buf[0])),uintptr(len(buf)),0)
 if n==0||n>=uintptr(len(buf)){return out}
 actual:=syscall.UTF16ToString(buf[:n])
 if !strings.EqualFold(actual,"\\\\?\\"+path){return out}
 stat,err:=file.Stat()
 if err!=nil||!stat.Mode().IsRegular()||stat.Size()<=0||stat.Size()>cashierynweUpdateStateLimit{return out}
 raw,err:=io.ReadAll(io.LimitReader(file,cashierynweUpdateStateLimit+1))
 if err!=nil{return out}
 end,err:=file.Stat()
 if err!=nil||stat.Size()!=end.Size()||stat.ModTime()!=end.ModTime()||int64(len(raw))!=end.Size(){return out}
 return parseCashierynweUpdateState(raw,end.ModTime(),now)
}

func pcOSCashierynweUpdateState() cashierynweUpdateStateReport {
 now:=time.Now()
 out:=cashierynweEmptyUpdateState("not_found",now)
 found:=0
 for _,path:=range cashierynweJournalPaths {
  candidate:=cashierynweReadFixedJournal(path,now)
  if candidate.State=="not_found"{continue}
  found++
  if found>1{return cashierynweEmptyUpdateState("multiple_or_inaccessible_journals",now)}
  out=candidate
 }
 return out
}
