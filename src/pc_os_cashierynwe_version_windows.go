//go:build windows

package main

import (
 "regexp"
 "syscall"
 "time"
 "unsafe"
)

var cashierynweUserSID = regexp.MustCompile(`^S-1-5-(21-[0-9]+-[0-9]+-[0-9]+-[0-9]+|[0-9]+)$`)

func cashierynweRegistryString(h syscall.Handle, name string) (string, error) {
 p, err := syscall.UTF16PtrFromString(name)
 if err != nil { return "",err }
 var typ uint32
 var buffer [512]uint16
 size:=uint32(len(buffer)*2)
 err=syscall.RegQueryValueEx(h,p,nil,&typ,(*byte)(unsafe.Pointer(&buffer[0])),&size)
 if err!=nil{return "",err}
 if typ!=syscall.REG_SZ || size<2 || size%2!=0 || size>uint32(len(buffer)*2) || buffer[size/2-1]!=0 {return "",syscall.Errno(13)}
 for i:=uint32(0);i<size/2-1;i++{if buffer[i]==0{return "",syscall.Errno(13)}}
 return syscall.UTF16ToString(buffer[:size/2]),nil
}

func cashierynweReadRegisteredVersion(root syscall.Handle, key string, view uint32) cashierynweVersionObservation {
 ptr,err:=syscall.UTF16PtrFromString(key)
 if err!=nil{return cashierynweVersionObservation{Incomplete:true}}
 var h syscall.Handle
 err=syscall.RegOpenKeyEx(root,ptr,0,syscall.KEY_QUERY_VALUE|view,&h)
 if err==syscall.ERROR_FILE_NOT_FOUND || err==syscall.Errno(3){return cashierynweVersionObservation{}}
 if err!=nil{return cashierynweVersionObservation{Incomplete:true}}
 defer syscall.RegCloseKey(h)
 version,err:=cashierynweRegistryString(h,"DisplayVersion")
 if err!=nil{return cashierynweVersionObservation{Incomplete:true}}
 return cashierynweVersionObservation{Present:true,Version:version}
}

// No arbitrary command, path or registry-key input; never loads offline user
// hives, launches a process, uses the POS database, or returns user SIDs.
func pcOSCashierynweRegisteredVersion() cashierynweVersionReport {
 obs:=[]cashierynweVersionObservation{}
 views:=[]uint32{syscall.KEY_WOW64_64KEY,syscall.KEY_WOW64_32KEY}
 for _,view:=range views{
  obs=append(obs,cashierynweReadRegisteredVersion(syscall.HKEY_LOCAL_MACHINE,cashierynweUninstallKey,view))
 }
 // Service runs as LocalSystem: its HKCU is NOT the cashier user's HKCU.
 // Enumerate only loaded HKU subkey names, then query the fixed app key.
 // A bounded enumeration failure is unknown, not an invented version.
 var users syscall.Handle
 err:=syscall.RegOpenKeyEx(syscall.HKEY_USERS,nil,0,syscall.KEY_ENUMERATE_SUB_KEYS,&users)
 if err!=nil{return summarizeCashierynweVersions(append(obs,cashierynweVersionObservation{Incomplete:true}),time.Now())}
 defer syscall.RegCloseKey(users)
 complete:=false
 for i:=uint32(0);i<128;i++{
  var name [256]uint16
  n:=uint32(len(name))
  err=syscall.RegEnumKeyEx(users,i,&name[0],&n,nil,nil,nil,nil)
  if err==syscall.Errno(259){complete=true;break}
  if err!=nil{break}
  sid:=syscall.UTF16ToString(name[:])
  if !cashierynweUserSID.MatchString(sid){continue}
  for _,view:=range views{
   obs=append(obs,cashierynweReadRegisteredVersion(users,sid+`\`+cashierynweUninstallKey,view))
  }
 }
 if !complete{obs=append(obs,cashierynweVersionObservation{Incomplete:true})}
 return summarizeCashierynweVersions(obs,time.Now())
}
