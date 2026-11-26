//go:build windows

package main
import "testing"
func TestCashierynweNativeProbeReadOnly(t *testing.T){
 r:=pcOSCashierynweRegisteredVersion()
 if !r.ZeroUI || !r.ReadOnly || r.Source!="nsis_uninstall_registry_loaded_hives" || r.ObservedUTC=="" {t.Fatal("invalid native report")}
 if r.State=="registered" && !cashierynweStableVersion.MatchString(r.RegisteredVersion){t.Fatal("invalid reported version")}
}
