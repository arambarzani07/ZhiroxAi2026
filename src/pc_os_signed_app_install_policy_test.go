package main

import "testing"

func TestSignedAppInstallerExactAllowlist(t *testing.T) {
    approval := "12345678-1234-4234-8234-1234567890ab"
    ok := map[string]interface{}{"app_id":"cashierynwe","version":"1.0.10","approval_id":approval}
    spec, id, err := validateSignedAppInstallArgs(ok)
    if err != nil || id != approval || spec.SHA256 != "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b" || spec.Size != 158128464 {
        t.Fatalf("exact signed 1.0.10 immutable artifact rejected: %v", err)
    }
    rejected := []map[string]interface{}{
        nil, {}, {"app_id":"cashierynwe"}, {"app_id":"unknown","version":"1.0.10","approval_id":approval},
        {"app_id":"cashierynwe","version":"1.0.11","approval_id":approval},
        {"app_id":"Cashierynwe","version":"1.0.10","approval_id":approval},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":"approved"},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":approval,"url":"https://evil.example/installer.exe"},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":approval,"exe":"cmd.exe"},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":approval,"flags":"/D=C:/Windows"},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":approval,"sha256":"0"},
        {"app_id":"cashierynwe","version":"1.0.10","approval_id":approval,"script":"powershell -enc AAA"},
    }
    for i, arg := range rejected {
        if _, _, err := validateSignedAppInstallArgs(arg); err == nil { t.Errorf("unauthorized parameter variant %d accepted", i) }
    }
    if signedAppInstallMinAgent != "5.1.16" { t.Fatal("version gate altered") }
}
