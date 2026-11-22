package stager

// The module is a library only, not a PC Relay command or installer.
// Its caller MUST separately enforce trusted Windows staging directory ACLs.
// The installed Relay does not yet link this package or execute any download.

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "time"
)

type approvedLocation struct {
    URL string `json:"url"`
    AppID string `json:"app_id"`
    Version string `json:"version"`
    Filename string `json:"filename"`
    SHA256 string `json:"sha256"`
    Size int64 `json:"size"`
}

func approvedClient(base *http.Client) *http.Client {
    var c http.Client
    if base == nil {
        c = http.Client{Timeout: 5 * time.Minute}
    } else {
        c = *base
        if c.Timeout == 0 || c.Timeout > 5*time.Minute {
            c.Timeout = 5*time.Minute
        }
    }
    // Never forward the Bearer header to any redirects, including Github.
    c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
        return http.ErrUseLastResponse
    }
    return &c
}

// FetchApprovedLocation sends the Relay agent secret only to a compiled,
// fixed Supabase HTTPS endpoint. The exact one-time approval was previously
// consumed by the owner's authenticated server-side RPC. The agent can
// neither select a release nor supply an alternative URL.
func FetchApprovedLocation(ctx context.Context, client *http.Client, agentSecret, approvalID string) (string, error) {
    if !ValidApprovalID(approvalID) || agentSecret == "" || len(agentSecret) > 4096 {
        return "", errors.New("stager_credentials_or_approval_invalid")
    }
    endpoint, err := url.Parse(FixedRelayEndpoint)
    if err != nil || endpoint.Scheme != "https" ||
        endpoint.Hostname() != "vllwundrtrgdqmkqbduy.supabase.co" || endpoint.Port() != "" {
        return "", errors.New("stager_endpoint_not_pinned")
    }
    q := endpoint.Query()
    q.Set("approval_id", approvalID)
    endpoint.RawQuery = q.Encode()
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
    if err != nil { return "", errors.New("stager_request_invalid") }
    req.Header.Set("Authorization", "Bearer "+agentSecret)
    req.Header.Set("X-Agent-ID", AgentID)
    req.Header.Set("Accept", "application/json")
    req.Header.Set("Cache-Control", "no-store")
    res, err := approvedClient(client).Do(req)
    if err != nil { return "", errors.New("stager_control_plane_unavailable") }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK {
        return "", errors.New("stager_control_plane_denied")
    }
    if res.ContentLength > MaxControlResponse {
        return "", errors.New("stager_control_plane_response_too_large")
    }
    raw, err := io.ReadAll(io.LimitReader(res.Body, MaxControlResponse+1))
    if err != nil || len(raw) > MaxControlResponse {
        return "", errors.New("stager_control_plane_response_invalid")
    }
    var loc approvedLocation
    decoder := json.NewDecoder(bytes.NewReader(raw))
    decoder.DisallowUnknownFields()
    if decoder.Decode(&loc) != nil || decoder.Decode(new(interface{})) != io.EOF ||
        loc.AppID != AppID || loc.Version != Version ||
        loc.Filename != FileName || loc.Size != FileSize || loc.SHA256 != FileSHA256 {
        return "", errors.New("stager_artifact_identity_invalid")
    }
    if err := ValidateCDNURL(loc.URL); err != nil {
        return "", err
    }
    // Short-lived bearer-style CDN URL must not appear in logs or error text.
    return loc.URL, nil
}

// DownloadVerified stages a known-size immutable asset without following
// redirects. It never supplies the Relay/GitHub Authorization token to CDN.
// If destination already exists, it refuses to overwrite. The caller MUST
// supply a service-owned directory with a separately verified protective ACL.
func DownloadVerified(ctx context.Context, client *http.Client, cdnURL, outputDir string) error {
    return downloadVerifiedWithSpec(ctx, client, cdnURL, outputDir, FileName, FileSize, FileSHA256)
}

// A package-private small-fixture variant allows bounded end-to-end CI
// tests without transferring 158 MB or changing production constants.
func downloadVerifiedWithSpec(ctx context.Context, client *http.Client, cdnURL, outputDir, filename string, expectedSize int64, expectedSHA256 string) error {
    if expectedSize <= 0 || expectedSize > FileSize ||
        filename == "" || filepath.Base(filename) != filename || filename == "." {
        return errors.New("stager_invalid_fixed_test_spec")
    }
    if err := ValidateCDNURL(cdnURL); err != nil { return err }
    if outputDir == "" || !filepath.IsAbs(outputDir) {
        return errors.New("staging_root_not_absolute")
    }
    dir, err := os.Lstat(outputDir)
    if err != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 {
        return errors.New("staging_root_missing_or_reparse_point")
    }
    dest := filepath.Join(outputDir, filename)
    if _, err := os.Lstat(dest); err == nil {
        return errors.New("staged_artifact_already_exists")
    } else if !os.IsNotExist(err) {
        return errors.New("staged_artifact_destination_unavailable")
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnURL, nil)
    if err != nil { return errors.New("stager_cdn_request_invalid") }
    req.Header.Set("Accept-Encoding", "identity")
    res, err := approvedClient(client).Do(req)
    if err != nil { return errors.New("stager_cdn_unavailable") }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK || res.Uncompressed ||
        (res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity") {
        return errors.New("stager_cdn_body_not_raw_binary")
    }
    if res.ContentLength >= 0 && res.ContentLength != expectedSize {
        return errors.New("stager_cdn_length_wrong")
    }
    f, err := os.CreateTemp(outputDir, ".zhirox-verified-installer-*")
    if err != nil { return errors.New("stager_temp_creation_failed") }
    tmp := f.Name()
    defer os.Remove(tmp)
    h := sha256.New()
    count, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(res.Body, expectedSize+1))
    syncErr := f.Sync()
    closeErr := f.Close()
    if copyErr != nil || syncErr != nil || closeErr != nil ||
        count != expectedSize || !ExactDigest(h.Sum(nil), expectedSHA256) {
        return errors.New("stager_asset_sha256_size_mismatch")
    }
    // Exclusive hard-link commit: unlike a rename, this cannot overwrite a
    // concurrently created destination on Windows or POSIX. Same directory
    // guarantees one filesystem/volume. The temp link is always removed.
    if err := os.Link(tmp, dest); err != nil {
        return errors.New("stager_exclusive_commit_failed")
    }
    return nil
}
