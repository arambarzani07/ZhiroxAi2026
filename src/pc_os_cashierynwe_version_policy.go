package main

import (
 "regexp"
 "time"
)

// This is the exact electron-builder 26.0.12 UUIDv5 of
// com.zhirox.cashierynwe.pos, not a caller-supplied registry path.
const cashierynweUninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\2a8a9551-9aff-5f3e-9f5e-71e0ff32278e`
var cashierynweStableVersion = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

type cashierynweVersionObservation struct {
 Present bool
 Version string
 Incomplete bool
}

type cashierynweVersionReport struct {
 Schema int `json:"schema"`
 App string `json:"app"`
 State string `json:"state"`
 RegisteredVersion string `json:"registered_version,omitempty"`
 ObservedUTC string `json:"observed_utc"`
 Source string `json:"source"`
 ZeroUI bool `json:"zero_ui"`
 ReadOnly bool `json:"read_only"`
}

func summarizeCashierynweVersions(items []cashierynweVersionObservation, now time.Time) cashierynweVersionReport {
 out := cashierynweVersionReport{Schema:1, App:"Cashierynwe", State:"unknown", ObservedUTC:now.UTC().Format(time.RFC3339), Source:"nsis_uninstall_registry_loaded_hives", ZeroUI:true, ReadOnly:true}
 v := ""
 for _, x := range items {
  if x.Incomplete || (x.Present && !cashierynweStableVersion.MatchString(x.Version)) {
   out.State = "incomplete"
   return out
  }
  if !x.Present { continue }
  if v != "" && v != x.Version { out.State="conflicting_versions"; return out }
  v=x.Version
 }
 if v != "" { out.State="registered"; out.RegisteredVersion=v }
 return out
}
