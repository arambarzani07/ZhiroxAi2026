package main

import "testing"

func TestCashierynweSigningApprovalArguments(t *testing.T) {
	const commit = "a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4a1b2c3d4"
	valid := map[string]interface{}{
		"release_tag": "v1.0.1",
		"approval_phrase": "SIGN_CASHIERYNWE_V1_0_1_ON_OWNER_PC",
		"approved_main_commit": commit,
	}
	got, err := cashierynweValidateSignArgs(valid)
	if err != nil || got != commit {
		t.Fatalf("expected approved exact commit, got %q / %v", got, err)
	}
	fail := []map[string]interface{}{
		nil,
		{},
		{"release_tag":"v1.0.1"},
		{"release_tag":"v1.0.2", "approval_phrase":valid["approval_phrase"], "approved_main_commit":commit},
		{"release_tag":"v1.0.1", "approval_phrase":"SIGN_ANY_VERSION", "approved_main_commit":commit},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":"main"},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":commit+"; powershell"},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":"A1B2C3D4A1B2C3D4A1B2C3D4A1B2C3D4A1B2C3D4"},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":commit, "script":"evil.ps1"},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":commit, "pfx_path":"C:\\secret.pfx"},
		{"release_tag":"v1.0.1", "approval_phrase":valid["approval_phrase"], "approved_main_commit":commit, "git_url":"https://evil.example"},
	}
	for i, args := range fail {
		if _, err := cashierynweValidateSignArgs(args); err == nil {
			t.Errorf("invalid arguments #%d accepted: %#v", i, args)
		}
	}
}
