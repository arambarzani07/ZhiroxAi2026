package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "os"
    "path/filepath"
    "strconv"
    "testing"
)

func TestPCOSModulesSafetyFlags(t *testing.T) {
    raw, err := pcOSModules()
    if err != nil {
        t.Fatal(err)
    }
    m := raw.(map[string]interface{})
    if m["zero_ui"] != true {
        t.Fatal("zero_ui must be true")
    }
    if m["arbitrary_shell"] != false {
        t.Fatal("arbitrary_shell must be false")
    }
    if m["critical_protection_lock"] != true {
        t.Fatal("critical protection must remain enabled")
    }
}

func TestPCOSZeroUIInvariant(t *testing.T) {
    raw, err := pcOSZeroUI()
    if err != nil {
        t.Fatal(err)
    }
    m := raw.(map[string]interface{})
    if m["shell_execution"] != false || m["cmd_window"] != false || m["powershell_window"] != false {
        t.Fatal("shell/UI execution invariant failed")
    }
    if m["structured_actions_only"] != true {
        t.Fatal("structured actions must remain mandatory")
    }
}

func TestPCOSHashFile(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "sample.bin")
    data := []byte("zhirox-integrity-test")
    if err := os.WriteFile(path, data, 0600); err != nil {
        t.Fatal(err)
    }
    got := pcOSHashFile(path, 1024)
    sum := sha256.Sum256(data)
    if got["sha256"] != hex.EncodeToString(sum[:]) {
        t.Fatalf("unexpected hash: %v", got["sha256"])
    }
}

func TestPCOSVerifyUpdateEd25519(t *testing.T) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        t.Fatal(err)
    }
    dir := t.TempDir()
    artifact := filepath.Join(dir, "ZhiroxAISupervisor.exe")
    data := []byte("signed-zhirox-test-artifact")
    if err := os.WriteFile(artifact, data, 0600); err != nil {
        t.Fatal(err)
    }
    sum := sha256.Sum256(data)
    hash := hex.EncodeToString(sum[:])
    canonical := []byte("5.1.0|" + hash + "|" + strconv.Itoa(len(data)))
    sig := ed25519.Sign(priv, canonical)
    manifest := pcOSUpdateManifest{
        Version: "5.1.0",
        File: "ZhiroxAISupervisor.exe",
        SHA256: hash,
        Size: int64(len(data)),
        Signature: base64.StdEncoding.EncodeToString(sig),
    }
    result, err := pcOSVerifyUpdateWithKey(manifest, artifact, pub)
    if err != nil {
        t.Fatal(err)
    }
    if result["verified"] != true || result["signature"] != "ed25519_verified" {
        t.Fatalf("verification did not produce trusted result: %#v", result)
    }

    manifest.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
    if _, err := pcOSVerifyUpdateWithKey(manifest, artifact, pub); err == nil {
        t.Fatal("hash mismatch must fail closed")
    }
}
