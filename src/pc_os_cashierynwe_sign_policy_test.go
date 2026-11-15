package main

import (
	"strings"
	"testing"
)

func TestCashierynweSigningApprovalArguments(t *testing.T) {
	const commit = "a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4"
	const id = "12345678-1234-4234-8234-1234567890ab"
	valid := map[string]interface{}{
		"tag": "v1.0.1",
		"approval_id": id,
		"commit_sha": commit,
	}
	gotCommit, gotID, err := cashierynweValidateSignArgs(valid)
	if err != nil || gotCommit != commit || gotID != id {
		t.Fatalf("expected pinned main SHA and one-time approval UUID, got %q / %q / %v", gotCommit, gotID, err)
	}
	fail := []map[string]interface{}{
		nil,
		{},
		{"tag": "v1.0.1"},
		{"tag":"v1.0.2", "approval_id":id, "commit_sha":commit},
		{"tag":"v1.0.1", "approval_id":"approved=true", "commit_sha":commit},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":"main"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":commit+"; powershell"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":"A1B2C3D4A1B2C3D4A1B2C3D4A1B2C3D4A1B2C3D4"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":commit, "script":"evil.ps1"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":commit, "pfx_path":"C:\\secret.pfx"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":commit, "git_url":"https://evil.example"},
		{"tag":"v1.0.1", "approval_id":id, "commit_sha":commit, "approval_phrase":"SIGN_CASHIERYNWE_V1_0_1_ON_OWNER_PC"},
	}
	for i, args := range fail {
		if _, _, err := cashierynweValidateSignArgs(args); err == nil {
			t.Errorf("invalid arguments #%d accepted: %#v", i, args)
		}
	}
}

func TestCashierynweHeadlessEnvironmentOverridesInheritedPrompts(t *testing.T) {
	base := []string{
		"PATH=C:\\Windows",
		"gcm_interactive=true",
		"GIT_TERMINAL_PROMPT=1",
		"GCM_GUI_PROMPT=1",
		"GH_PROMPT_DISABLED=0",
		"CI=false",
		"GIT_ASKPASS=C:\\bad\\credential-helper.exe",
		"SSH_ASKPASS=C:\\bad\\ssh-askpass.exe",
		"CUSTOM_KEEP=safe",
	}
	result := cashierynweHeadlessEnv(base)
	expect := map[string]string{
		"CI": "true",
		"GIT_TERMINAL_PROMPT": "0",
		"GCM_INTERACTIVE": "0",
		"GCM_GUI_PROMPT": "0",
		"GH_PROMPT_DISABLED": "1",
		"GIT_ASKPASS": "",
		"SSH_ASKPASS": "",
		"CUSTOM_KEEP": "safe",
		"PATH": "C:\\Windows",
	}
	seen := map[string]bool{}
	for _, entry := range result {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid environment entry: %q", entry)
		}
		key := strings.ToUpper(parts[0])
		if seen[key] {
			t.Fatalf("inherited interactive environment duplicate: %s", key)
		}
		seen[key] = true
		if val, ok := expect[key]; !ok || val != parts[1] {
			t.Fatalf("unexpected environment variable %s=%q", key, parts[1])
		}
	}
	for key := range expect {
		if !seen[key] { t.Errorf("headless signing environment omitted %s", key) }
	}
}
