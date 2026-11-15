package main

import (
	"errors"
	"regexp"
	"strings"
)

const cashierynweApprovedTag = "v1.0.1"

var (
	cashierynweHexCommit = regexp.MustCompile("^[a-f0-9]{40}$")
	cashierynweApprovalUUID = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$")
)

// This is NOT itself an approval verifier. The privileged Supabase RPC must
// atomically consume the owner approval and bind it to this exact command
// before the relay is allowed to receive it. The local script pins the same
// approved SHA, tag and one-time approval identifier.
// Refuse all extra fields, including any executable, script, path, shell
// command, secret or PFX supplied by a remote caller.
func cashierynweValidateSignArgs(args map[string]interface{}) (string, string, error) {
	if len(args) != 3 {
		return "", "", errors.New("sign_args_not_exact")
	}
	tag, _ := args["tag"].(string)
	commit, _ := args["commit_sha"].(string)
	approvalID, _ := args["approval_id"].(string)
	if tag != cashierynweApprovedTag ||
		!cashierynweHexCommit.MatchString(commit) ||
		!cashierynweApprovalUUID.MatchString(approvalID) {
		return "", "", errors.New("sign_args_rejected")
	}
	return commit, approvalID, nil
}

// cashierynweHeadlessEnv overrides inherited interactive credential helpers.
// The approved Windows agent must never display a Git, GCM or GitHub login
// dialog while signing. Missing stored credentials cause a failed operation.
func cashierynweHeadlessEnv(base []string) []string {
	pinned := []string{
		"CI=true",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=0",
		"GCM_GUI_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
	}
	blocked := make(map[string]bool, len(pinned))
	for _, item := range pinned {
		blocked[strings.ToUpper(strings.SplitN(item, "=", 2)[0])] = true
	}
	result := make([]string, 0, len(base)+len(pinned))
	for _, item := range base {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || blocked[strings.ToUpper(parts[0])] {
			continue
		}
		result = append(result, item)
	}
	return append(result, pinned...)
}
