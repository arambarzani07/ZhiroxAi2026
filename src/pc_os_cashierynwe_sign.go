//go:build windows

// A single, explicitly approved PC Relay action for creating Cashierynwe v1.0.1.
// It is NOT a generic script runner or remote PowerShell service.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	cashierynweRemote = "https://github.com/arambarzani07/Cashierynwe.git"
	cashierynweScriptGitPath = "scripts/windows/publish-local-signed-release.ps1"
)

func pcOSCashierynweSignStatus() (interface{}, error) {
	cashierynweSignLock.Lock()
	defer cashierynweSignLock.Unlock()
	return map[string]interface{}{
		"action": "cashierynwe_sign_release",
		"state": cashierynweSignState.State,
		"started": cashierynweSignState.Started,
		"finished": cashierynweSignState.Finished,
		"approved_commit": cashierynweSignState.Commit,
		"error_code": cashierynweSignState.ErrorCode,
		"keys_exported": false,
	}, nil
}

func pcOSCashierynweSignRelease(args map[string]interface{}) (interface{}, error) {
	commit, err := cashierynweValidateSignArgs(args)
	if err != nil { return nil, err }
	cashierynweSignLock.Lock()
	if cashierynweSignState.Running {
		cashierynweSignLock.Unlock()
		return nil, errors.New("signing_already_running")
	}
	cashierynweSignState.Running = true
	cashierynweSignState.State = "accepted"
	cashierynweSignState.Started = time.Now().UTC().Format(time.RFC3339)
	cashierynweSignState.Finished = ""
	cashierynweSignState.Commit = commit
	cashierynweSignState.ErrorCode = ""
	cashierynweSignLock.Unlock()

	// Return before the Relay's bounded response timeout. Read completion via
	// the read-only cashierynwe_sign_status action; never return build logs/tokens.
	go func() {
		err := cashierynweExecuteSignedRelease(commit)
		cashierynweSignLock.Lock()
		defer cashierynweSignLock.Unlock()
		cashierynweSignState.Running = false
		cashierynweSignState.Finished = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			cashierynweSignState.State = "failed"
			// Do not return raw subprocess errors: they can contain credentials.
			cashierynweSignState.ErrorCode = "local_sign_or_publish_failed"
		} else {
			cashierynweSignState.State = "published"
			cashierynweSignState.ErrorCode = ""
		}
	}()
	return map[string]interface{}{
		"action": "cashierynwe_sign_release",
		"state": "accepted",
		"release_tag": cashierynweApprovedTag,
		"approved_commit": commit,
		"status_action": "cashierynwe_sign_status",
		"keys_exported": false,
	}, nil
}

func cashierynweExecuteSignedRelease(commit string) error {
	// Require same logged-in Windows user that owns the signing certificate,
	// GitHub CLI credentials, and private Cashierynwe source checkout.
	profile := os.Getenv("USERPROFILE")
	programFiles := os.Getenv("ProgramFiles")
	systemRoot := os.Getenv("SystemRoot")
	if profile == "" || programFiles == "" || systemRoot == "" ||
		!filepath.IsAbs(profile) || !filepath.IsAbs(programFiles) || !filepath.IsAbs(systemRoot) {
		return errors.New("windows_paths_missing")
	}
	root := filepath.Join(profile, "Cashierynwe")
	dir, err := os.Lstat(root)
	if err != nil || !dir.IsDir() || (dir.Mode() & os.ModeSymlink) != 0 {
		return errors.New("owner_checkout_missing_or_symlinked")
	}
	gitExe := filepath.Join(programFiles, "Git", "cmd", "git.exe")
	psExe := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	for _, exe := range []string{gitExe, psExe} {
		if stat, err := os.Stat(exe); err != nil || stat.IsDir() { return errors.New("approved_tool_missing") }
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	gitOutput := func(args ...string) (string, error) {
		cmd := hiddenCommandContext(ctx, gitExe, append([]string{"-C", root}, args...)...)
		out, err := cmd.Output()
		if err != nil || len(out) > 65536 { return "", errors.New("git_preflight_failed") }
		return strings.TrimSpace(string(out)), nil
	}
	remote, err := gitOutput("remote", "get-url", "origin")
	if err != nil || (remote != cashierynweRemote && remote != "https://github.com/arambarzani07/Cashierynwe") {
		return errors.New("unapproved_repo_origin")
	}
	branch, err := gitOutput("branch", "--show-current")
	if err != nil || branch != "main" { return errors.New("unapproved_checkout_branch") }
	// This fetch is restricted to the fixed, previously verified Git origin.
	if _, err := gitOutput("fetch", "origin", "main", "--quiet"); err != nil { return err }
	head, err := gitOutput("rev-parse", "origin/main")
	if err != nil || head != commit { return errors.New("approved_main_commit_mismatch") }
	// Fetch the script as an immutable blob of the explicitly approved commit,
	// not from a possibly modified local worktree.
	script, err := gitOutput("show", commit + ":" + cashierynweScriptGitPath)
	if err != nil || len(script) < 1000 ||
		!strings.Contains(script, "[switch]$UseWindowsCertificateStore") ||
		!strings.Contains(script, "[string]$ExpectedCommit") {
		return errors.New("approved_signer_script_missing")
	}
	tmp, err := os.MkdirTemp("", "zhirox-cashierynwe-sign-")
	if err != nil { return errors.New("signer_temp_dir_failed") }
	defer os.RemoveAll(tmp)
	scriptPath := filepath.Join(tmp, "approved-cashierynwe-sign.ps1")
	// Windows PowerShell 5.1 requires a BOM to interpret Sorani text as UTF-8.
	if err := os.WriteFile(scriptPath, append([]byte{0xEF, 0xBB, 0xBF}, []byte(script)...), 0600); err != nil {
		return errors.New("signer_temp_file_failed")
	}
	cmd := hiddenCommandContext(ctx, psExe,
		"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "RemoteSigned",
		"-File", scriptPath, "-UseWindowsCertificateStore", "-ExpectedCommit", commit,
	)
	cmd.Dir = root
	// SECURITY: Do not capture stdout/stderr from signing or GitHub CLI.
	// The legacy market Admin.exe/Cashier.exe and their databases are untouched.
	if err := cmd.Run(); err != nil { return errors.New("local_sign_or_publish_failed") }
	return nil
}
