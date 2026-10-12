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
	modules, ok := m["modules"].([]string)
	if !ok || len(modules) != 12 {
		t.Fatalf("expected exactly 12 PC OS modules, got %#v", m["modules"])
	}
	seen := map[string]bool{}
	for _, name := range modules {
		if name == "" || seen[name] {
			t.Fatalf("module catalog must contain 12 unique non-empty names: %#v", modules)
		}
		seen[name] = true
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
	if m["critical_mutations_locked"] != true {
		t.Fatal("critical mutations must remain locked")
	}
}

func TestPCOSBackupVaultSafety(t *testing.T) {
	raw, err := pcOSBackupVault()
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(map[string]interface{})
	if m["destructive_restore"] != false {
		t.Fatal("backup vault must not expose automatic destructive restore")
	}
	db := m["business_database"].(map[string]interface{})
	if db["live_file_copy"] != false {
		t.Fatal("production database live file copy must stay disabled")
	}
	if db["strategy"] != "database-native backup required" {
		t.Fatalf("unexpected production DB backup strategy: %#v", db["strategy"])
	}
}

func TestPCOSBusinessContinuitySafety(t *testing.T) {
	raw, err := pcOSBusinessContinuity()
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(map[string]interface{})
	policy := m["policy"].(map[string]interface{})
	for _, key := range []string{"critical_process_stop", "critical_process_restart", "critical_service_disable", "database_mutation", "hikvision_mutation"} {
		if policy[key] != false {
			t.Fatalf("%s must remain false", key)
		}
	}
	if policy["critical_service_start_for_recovery"] != true {
		t.Fatal("start-only recovery must remain available")
	}
}

func TestPCOSDisasterRecoverySafety(t *testing.T) {
	raw, err := pcOSDisasterRecovery()
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(map[string]interface{})
	if m["automatic_destructive_restore"] != false {
		t.Fatal("automatic destructive restore must remain disabled")
	}
	if m["mode"] != "plan_and_verify" {
		t.Fatalf("unexpected disaster recovery mode: %#v", m["mode"])
	}
}

func TestPCOSReadOnlyPathRejectsOutsideManagedRoots(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if _, err := pcOSReadOnlyPath(outside); err == nil {
		t.Fatal("read-only file manager must reject paths outside managed roots")
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
		Version:   "5.1.0",
		File:      "ZhiroxAISupervisor.exe",
		SHA256:    hash,
		Size:      int64(len(data)),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
	result, err := pcOSVerifyUpdateWithKey(manifest, artifact, pub)
	if err != nil {
		t.Fatal(err)
	}
	if result["verified"] != true || result["signature"] != "ed25519_verified" {
		t.Fatalf("verification did not produce trusted result: %#v", result)
	}

	badHash := manifest
	badHash.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := pcOSVerifyUpdateWithKey(badHash, artifact, pub); err == nil {
		t.Fatal("hash mismatch must fail closed")
	}

	badSize := manifest
	badSize.Size++
	if _, err := pcOSVerifyUpdateWithKey(badSize, artifact, pub); err == nil {
		t.Fatal("size mismatch must fail closed")
	}

	missingSignature := manifest
	missingSignature.Signature = ""
	if _, err := pcOSVerifyUpdateWithKey(missingSignature, artifact, pub); err == nil {
		t.Fatal("missing signature must fail closed")
	}

	wrongSig := manifest
	wrongSig.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, err := pcOSVerifyUpdateWithKey(wrongSig, artifact, pub); err == nil {
		t.Fatal("invalid signature must fail closed")
	}
}
