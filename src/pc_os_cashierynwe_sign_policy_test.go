package main

import "testing"

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
