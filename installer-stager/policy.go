package stager

import (
    "crypto/subtle"
    "encoding/hex"
    "errors"
    "net/url"
    "regexp"
    "strings"
)

const (
    AppID = "cashierynwe"
    Version = "1.0.10"
    AgentID = "Server-c24b5c0f"
    FileName = "Cashierynwe-Setup-1.0.10-x64.exe"
    FileSHA256 = "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b"
    FileSize int64 = 158128464
    FixedRelayEndpoint = "https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/zhirox-pc-relay/api/agent/Server-c24b5c0f/signed-app-download"
    MaxControlResponse = 4096
)

var approvalUUID = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")

func ValidApprovalID(id string) bool {
    return approvalUUID.MatchString(id)
}

// ValidateCDNURL rejects every location except a trusted, exact HTTPS GitHub
// release-asset CDN host. The pinned SHA256 still determines exact file identity.
func ValidateCDNURL(location string) error {
    if len(location) < 20 || len(location) > 8192 {
        return errors.New("cdn_location_invalid_length")
    }
    u, err := url.Parse(location)
    if err != nil { return errors.New("cdn_location_invalid") }
    if u.Scheme != "https" || u.Port() != "" || u.User != nil ||
        u.Fragment != "" || u.Opaque != "" ||
        u.Hostname() != strings.ToLower(u.Hostname()) ||
        (u.Hostname() != "release-assets.githubusercontent.com" &&
         u.Hostname() != "objects.githubusercontent.com") ||
        u.Path == "" || u.Path == "/" {
        return errors.New("cdn_location_not_allowlisted")
    }
    if strings.Contains(u.EscapedPath(), "%2f") || strings.Contains(u.EscapedPath(), "%2F") ||
       strings.Contains(u.EscapedPath(), "%5c") || strings.Contains(u.EscapedPath(), "%5C") {
        return errors.New("cdn_location_encoded_path_rejected")
    }
    return nil
}

func ExactDigest(actual []byte, expected string) bool {
    decoded, err := hex.DecodeString(expected)
    return err == nil && len(decoded) == 32 &&
        subtle.ConstantTimeCompare(actual, decoded) == 1
}
