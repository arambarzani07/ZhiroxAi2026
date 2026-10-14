package remoteagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxHTTPResponseBytes = 512 * 1024
	defaultHTTPTimeout   = 10 * time.Second
)

type PendingSession struct {
	ID                    string         `json:"id"`
	State                 string         `json:"state"`
	RequestedAt           string         `json:"requested_at"`
	ExpiresAt             string         `json:"expires_at"`
	RequestedCapabilities map[string]any `json:"requested_capabilities"`
	ControllerLabel       string         `json:"controller_label"`
	ControllerFingerprint string         `json:"controller_fingerprint"`
}

type Signal struct {
	ID        int64           `json:"id"`
	Sender    string          `json:"sender"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

type SignalingClient struct {
	baseURL string
	agentID string
	secret  string
	http    *http.Client
}

func NewSignalingClient(baseURL, agentID, secret string, client *http.Client) (*SignalingClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	agentID = strings.TrimSpace(agentID)
	secret = strings.TrimSpace(secret)
	if baseURL == "" || agentID == "" || secret == "" {
		return nil, errors.New("signaling base url, agent id and agent secret are required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("signaling base url must be an absolute https URL")
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &SignalingClient{baseURL: baseURL, agentID: agentID, secret: secret, http: client}, nil
}

func (c *SignalingClient) agentPath(suffix string) string {
	return fmt.Sprintf("/api/agent/%s%s", url.PathEscape(c.agentID), suffix)
}

func (c *SignalingClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if len(encoded) > pcOSRemoteMaxSignalBytes {
			return errors.New("signaling request exceeds local safety limit")
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("X-Agent-ID", c.agentID)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxHTTPResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(data) > maxHTTPResponseBytes {
		return errors.New("signaling response exceeds local safety limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr struct {
			Error  string `json:"error"`
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(data, &apiErr)
		if apiErr.Error == "" {
			apiErr.Error = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("signaling request failed (%d): %s", resp.StatusCode, apiErr.Error)
	}
	if out != nil && len(data) != 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("invalid signaling response: %w", err)
		}
	}
	return nil
}

func (c *SignalingClient) Pending(ctx context.Context) ([]PendingSession, error) {
	var reply struct {
		Sessions []PendingSession `json:"sessions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.agentPath("/sessions/pending"), nil, &reply); err != nil {
		return nil, err
	}
	return reply.Sessions, nil
}

func (c *SignalingClient) Approve(ctx context.Context, sessionID string) error {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodPost, c.agentPath("/sessions/"+url.PathEscape(sessionID)+"/approve"), map[string]any{}, nil)
}

func (c *SignalingClient) Reject(ctx context.Context, sessionID string) error {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodPost, c.agentPath("/sessions/"+url.PathEscape(sessionID)+"/reject"), map[string]any{}, nil)
}

func (c *SignalingClient) MarkActive(ctx context.Context, sessionID string) error {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodPost, c.agentPath("/sessions/"+url.PathEscape(sessionID)+"/active"), map[string]any{}, nil)
}

func (c *SignalingClient) PostSignal(ctx context.Context, sessionID, kind string, payload any) error {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return err
	}
	if err := pcOSRemoteValidateSignal(kind, payloadSizeJSON(payload)); err != nil {
		return err
	}
	return c.doJSON(ctx, http.MethodPost, c.agentPath("/sessions/"+url.PathEscape(sessionID)+"/signals"), map[string]any{
		"kind":    kind,
		"payload": payload,
	}, nil)
}

func (c *SignalingClient) PollSignals(ctx context.Context, sessionID string, after int64) ([]Signal, error) {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return nil, err
	}
	if after < 0 {
		return nil, errors.New("signal cursor cannot be negative")
	}
	var reply struct {
		Signals []Signal `json:"signals"`
	}
	path := c.agentPath("/sessions/"+url.PathEscape(sessionID)+"/signals") + "?after=" + fmt.Sprint(after)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &reply); err != nil {
		return nil, err
	}
	return reply.Signals, nil
}

func payloadSizeJSON(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return pcOSRemoteMaxSignalBytes + 1
	}
	return len(data)
}
