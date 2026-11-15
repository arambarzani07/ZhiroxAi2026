package main

import (
	"errors"
	"regexp"
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
