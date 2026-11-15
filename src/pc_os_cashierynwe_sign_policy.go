package main

import (
	"errors"
	"regexp"
)

const (
	cashierynweApprovedTag = "v1.0.1"
	cashierynweApprovedPhrase = "SIGN_CASHIERYNWE_V1_0_1_ON_OWNER_PC"
)

var cashierynweHexCommit = regexp.MustCompile("^[a-f0-9]{40}$")

// Reject unexpected arguments, including all caller-supplied paths, shell text,
// GitHub hosts, PFX locations, passwords, and replacement version numbers.
// Cloud also requires an authenticated one-time exact-args approval.
func cashierynweValidateSignArgs(args map[string]interface{}) (string, error) {
	if len(args) != 3 {
		return "", errors.New("sign_args_not_exact")
	}
	tag, _ := args["release_tag"].(string)
	commit, _ := args["approved_main_commit"].(string)
	phrase, _ := args["approval_phrase"].(string)
	if tag != cashierynweApprovedTag ||
		phrase != cashierynweApprovedPhrase ||
		!cashierynweHexCommit.MatchString(commit) {
		return "", errors.New("sign_args_rejected")
	}
	return commit, nil
}
