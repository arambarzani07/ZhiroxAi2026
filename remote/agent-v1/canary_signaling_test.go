package remoteagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCanaryRegistrationSendsHashNotRawCode(t *testing.T) {
	pairingCode := "ABCD-0123-EF45-6789-A0B1-C2D3"
	wantHash, err := CanaryPairingHash(pairingCode)
	if err != nil { t.Fatal(err) }
	const pairingID = "123e4567-e89b-42d3-a456-426614174000"
	const ticket = "canary-agent-ticket"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/canary/pair/register":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), strings.ReplaceAll(pairingCode, "-", "")) || strings.Contains(string(body), pairingCode) {
				t.Error("raw pairing code leaked in registration request")
			}
			var got struct {
				PairingHash string `json:"pairing_hash"`
				Capabilities map[string]bool `json:"capabilities"`
			}
			if err := json.Unmarshal(body, &got); err != nil { t.Fatal(err) }
			if got.PairingHash != wantHash { t.Fatalf("unexpected pairing hash %q", got.PairingHash) }
			if !got.Capabilities["screen_stream"] || got.Capabilities["mouse"] || got.Capabilities["keyboard"] {
				t.Fatalf("unsafe canary capabilities: %#v", got.Capabilities)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pairing_id": pairingID,
				"agent_ticket": ticket,
				"expires_at": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/canary/agent/pairings/"+pairingID+"/pending":
			if r.Header.Get("Authorization") != "Bearer "+ticket { t.Error("agent ticket auth missing") }
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, reg, err := RegisterCanarySignaling(context.Background(), server.URL, pairingCode, "Server canary", server.Client())
	if err != nil { t.Fatal(err) }
	if reg.PairingID != pairingID { t.Fatalf("unexpected pairing id %q", reg.PairingID) }
	pending, err := client.Pending(context.Background())
	if err != nil { t.Fatal(err) }
	if len(pending) != 0 { t.Fatalf("unexpected pending sessions: %#v", pending) }
}

func TestCanarySignalingRejectsPlainHTTP(t *testing.T) {
	code := "ABCD-0123-EF45-6789-A0B1-C2D3"
	if _, _, err := RegisterCanarySignaling(context.Background(), "http://example.test", code, "Server canary", nil); err == nil {
		t.Fatal("plain HTTP canary signaling URL was accepted")
	}
}
