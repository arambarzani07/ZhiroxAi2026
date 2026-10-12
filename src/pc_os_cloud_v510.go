package main

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "time"
)

const pcOSDefaultRelay = "https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/zhirox-pc-relay"

type pcOSRelayCredentials struct {
    AgentID string
    Secret  string
    Relay   string
}

func pcOSNormalizeConfigKey(k string) string {
    k = strings.ToLower(strings.TrimSpace(k))
    replacer := strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
    return replacer.Replace(k)
}

func pcOSReadString(raw json.RawMessage) string {
    var s string
    if json.Unmarshal(raw, &s) == nil {
        return strings.TrimSpace(s)
    }
    return ""
}

func pcOSLoadRelayCredentials() (pcOSRelayCredentials, error) {
    cfgPath := filepath.Join(pcOSActiveRoot(), "config.json")
    raw, err := os.ReadFile(cfgPath)
    if err != nil {
        return pcOSRelayCredentials{}, fmt.Errorf("read agent config: %w", err)
    }

    var doc map[string]json.RawMessage
    if err := json.Unmarshal(raw, &doc); err != nil {
        return pcOSRelayCredentials{}, fmt.Errorf("parse agent config: %w", err)
    }

    creds := pcOSRelayCredentials{Relay: pcOSDefaultRelay}
    for key, value := range doc {
        normalized := pcOSNormalizeConfigKey(key)
        switch normalized {
        case "agentid", "relayagentid":
            if creds.AgentID == "" {
                creds.AgentID = pcOSReadString(value)
            }
        case "agentsecret", "relayagentsecret", "secret":
            if creds.Secret == "" {
                creds.Secret = pcOSReadString(value)
            }
        case "relayurl", "relaybaseurl", "relayendpoint", "endpoint":
            if v := pcOSReadString(value); v != "" {
                creds.Relay = v
            }
        }
    }

    if creds.AgentID == "" || creds.Secret == "" {
        return pcOSRelayCredentials{}, errors.New("agent relay credentials unavailable in config")
    }

    creds.Relay = strings.TrimRight(strings.TrimSpace(creds.Relay), "/")
    creds.Relay = strings.TrimSuffix(creds.Relay, "/api")
    if creds.Relay == "" {
        creds.Relay = pcOSDefaultRelay
    }
    if u, err := url.Parse(creds.Relay); err != nil || u.Scheme != "https" || u.Host == "" {
        return pcOSRelayCredentials{}, errors.New("relay URL must be a valid HTTPS endpoint")
    }
    return creds, nil
}

func pcOSPublishCloudSnapshot(payload map[string]interface{}) error {
    creds, err := pcOSLoadRelayCredentials()
    if err != nil {
        return err
    }

    body, err := json.Marshal(map[string]interface{}{
        "module":   "agent_health",
        "severity": "info",
        "payload":  payload,
    })
    if err != nil {
        return err
    }

    endpoint := creds.Relay + "/api/agent/" + url.PathEscape(creds.AgentID) + "/pc-os/snapshot"
    req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(body)))
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Agent-ID", creds.AgentID)
    req.Header.Set("Authorization", "Bearer "+creds.Secret)
    req.Header.Set("User-Agent", "Zhirox-PC-OS/5.1.0")

    client := &http.Client{Timeout: 8 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("pc os snapshot relay returned HTTP %d", resp.StatusCode)
    }
    return nil
}

func pcOSCloudLoop() {
    // Let the pre-existing Supervisor heartbeat establish the new agent version first.
    time.Sleep(45 * time.Second)
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        snapshot := pcOSBackgroundSnapshot()
        err := pcOSPublishCloudSnapshot(snapshot)
        status := map[string]interface{}{
            "timestamp_utc": time.Now().UTC().Format(time.RFC3339),
            "ok":            err == nil,
            "zero_ui":       true,
        }
        if err != nil {
            status["error"] = err.Error()
        }
        _ = pcOSWriteAtomicJSON(filepath.Join(pcOSActiveRoot(), "pc-os-cloud-status.json"), status)
        <-ticker.C
    }
}
