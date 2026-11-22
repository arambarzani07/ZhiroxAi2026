package main

import (
    "errors"
    "regexp"
)

// SignedAppInstallSpec is a compiled immutable allowlist, NOT a remote shell,
// command, user-supplied URL, or user-supplied filesystem path.
// Add future products only by reviewed source+CI+signed Relay releases.
type SignedAppInstallSpec struct {
    AppID, Version, Filename, SHA256, PublisherSubject string
    Size int64
}

const signedAppInstallMinAgent = "5.1.16"

var (
    signedAppApprovalUUID = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")
    signedAppCatalog = map[string]SignedAppInstallSpec{
        "cashierynwe@1.0.10": {
            AppID: "cashierynwe",
            Version: "1.0.10",
            Filename: "Cashierynwe-Setup-1.0.10-x64.exe",
            SHA256: "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b",
            Size: 158128464,
            PublisherSubject: "CN=Cashierynwe Single-PC Updates",
        },
    }
)

// The authenticated backend must atomically consume a separate, single-use,
// owner approval bound to the identical (agent, action, args, command ID).
// This syntax validator is NEVER an authorization decision by itself.
func validateSignedAppInstallArgs(args map[string]interface{}) (SignedAppInstallSpec, string, error) {
    if len(args) != 3 {
        return SignedAppInstallSpec{}, "", errors.New("signed_install_args_not_exact")
    }
    app, appOK := args["app_id"].(string)
    version, verOK := args["version"].(string)
    approval, approvalOK := args["approval_id"].(string)
    if !appOK || !verOK || !approvalOK || !signedAppApprovalUUID.MatchString(approval) {
        return SignedAppInstallSpec{}, "", errors.New("signed_install_args_rejected")
    }
    spec, ok := signedAppCatalog[app+"@"+version]
    if !ok || spec.AppID != app || spec.Version != version {
        return SignedAppInstallSpec{}, "", errors.New("signed_install_not_allowlisted")
    }
    return spec, approval, nil
}
