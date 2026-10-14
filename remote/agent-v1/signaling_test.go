package remoteagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSessionID = "123e4567-e89b-42d3-a456-426614174000"

func TestSignalingClientRequiresHTTPSAndCredentials(t *testing.T) {
	if _, err := NewSignalingClient("http://example.test", "agent", "secret", nil); err == nil {
		t.Fatal("non-HTTPS signaling URL must be refused")
	}
	if _, err := NewSignalingClient("https://example.test", "", "secret", nil); err == nil {
		t.Fatal("missing agent id must be refused")
	}
}

func TestSignalingClientPendingAndApprove(t *testing.T) {
	secret := "agent-test-secret"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/Server-c24b5c0f/sessions/pending", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("X-Agent-ID") != "Server-c24b5c0f" {
			t.Error("agent authentication headers are missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []map[string]any{{
			"id": testSessionID, "state": "requested", "controller_label": "Browser",
		}}})
	})
	mux.HandleFunc("/api/agent/Server-c24b5c0f/sessions/"+testSessionID+"/approve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()

	client, err := NewSignalingClient(server.URL, "Server-c24b5c0f", secret, server.Client())
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	pending, err := client.Pending(context.Background())
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != testSessionID {
		t.Fatalf("unexpected pending sessions: %+v", pending)
	}
	if err := client.Approve(context.Background(), testSessionID); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func TestSignalingClientRejectsOversizedOrInvalidSignals(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("invalid signal must be rejected before HTTP request")
	}))
	defer server.Close()
	client, err := NewSignalingClient(server.URL, "Server-c24b5c0f", "secret", server.Client())
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if err := client.PostSignal(context.Background(), testSessionID, "input_event", map[string]any{}); err == nil {
		t.Fatal("non-signaling event must be refused")
	}
	large := make([]byte, pcOSRemoteMaxSignalBytes+1)
	if err := client.PostSignal(context.Background(), testSessionID, "offer", large); err == nil {
		t.Fatal("oversized signal must be refused")
	}
}
