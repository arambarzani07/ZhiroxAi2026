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
)

type CanarySignalingClient struct {
	baseURL   string
	pairingID string
	ticket    string
	http      *http.Client
}

type CanaryRegistration struct {
	PairingID string `json:"pairing_id"`
	ExpiresAt string `json:"expires_at"`
}

func RegisterCanarySignaling(ctx context.Context, baseURL, pairingCode, agentLabel string, client *http.Client) (*CanarySignalingClient, CanaryRegistration, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, CanaryRegistration{}, errors.New("canary signaling base url must be an absolute https URL")
	}
	pairingHash, err := CanaryPairingHash(pairingCode)
	if err != nil {
		return nil, CanaryRegistration{}, err
	}
	agentLabel = strings.TrimSpace(agentLabel)
	if agentLabel == "" || len(agentLabel) > 120 {
		return nil, CanaryRegistration{}, errors.New("canary agent label is invalid")
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	payload := map[string]any{
		"pairing_hash": pairingHash,
		"agent_label":  agentLabel,
		"capabilities": map[string]bool{
			"screen_stream": true,
			"mouse":         false,
			"keyboard":      false,
			"clipboard":     false,
			"file_transfer": false,
			"audio":         false,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, CanaryRegistration{}, err
	}
	if len(encoded) > 16*1024 {
		return nil, CanaryRegistration{}, errors.New("canary registration payload exceeds safety limit")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/canary/pair/register", bytes.NewReader(encoded))
	if err != nil {
		return nil, CanaryRegistration{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, CanaryRegistration{}, err
	}
	defer resp.Body.Close()
	data, err := readBoundedCanaryBody(resp.Body)
	if err != nil {
		return nil, CanaryRegistration{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, CanaryRegistration{}, canaryHTTPError(resp.StatusCode, data)
	}
	var reply struct {
		PairingID  string `json:"pairing_id"`
		AgentTicket string `json:"agent_ticket"`
		ExpiresAt  string `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return nil, CanaryRegistration{}, errors.New("invalid canary registration response")
	}
	if err := pcOSRemoteValidateSessionID(reply.PairingID); err != nil {
		return nil, CanaryRegistration{}, errors.New("canary registration returned invalid pairing id")
	}
	if strings.TrimSpace(reply.AgentTicket) == "" || len(reply.AgentTicket) > 4096 {
		return nil, CanaryRegistration{}, errors.New("canary registration returned invalid agent ticket")
	}
	if strings.TrimSpace(reply.ExpiresAt) == "" {
		return nil, CanaryRegistration{}, errors.New("canary registration returned no expiry")
	}
	c := &CanarySignalingClient{
		baseURL: baseURL,
		pairingID: reply.PairingID,
		ticket: reply.AgentTicket,
		http: client,
	}
	return c, CanaryRegistration{PairingID: reply.PairingID, ExpiresAt: reply.ExpiresAt}, nil
}

func readBoundedCanaryBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxHTTPResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxHTTPResponseBytes {
		return nil, errors.New("canary signaling response exceeds local safety limit")
	}
	return data, nil
}

func canaryHTTPError(status int, data []byte) error {
	var apiErr struct{ Error string `json:"error"` }
	_ = json.Unmarshal(data, &apiErr)
	if apiErr.Error == "" {
		apiErr.Error = http.StatusText(status)
	}
	return fmt.Errorf("canary signaling request failed (%d): %s", status, apiErr.Error)
}

func (c *CanarySignalingClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	if c == nil || c.baseURL == "" || c.ticket == "" {
		return errors.New("canary signaling client is not initialized")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if len(encoded) > pcOSRemoteMaxSignalBytes {
			return errors.New("canary signaling request exceeds local safety limit")
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.ticket)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := readBoundedCanaryBody(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return canaryHTTPError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return errors.New("invalid canary signaling response")
		}
	}
	return nil
}

func (c *CanarySignalingClient) Pending(ctx context.Context) ([]PendingSession, error) {
	var reply struct {
		Sessions []PendingSession `json:"sessions"`
	}
	path := "/api/canary/agent/pairings/" + url.PathEscape(c.pairingID) + "/pending"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &reply); err != nil {
		return nil, err
	}
	return reply.Sessions, nil
}

func (c *CanarySignalingClient) sessionPath(sessionID, suffix string) (string, error) {
	if err := pcOSRemoteValidateSessionID(sessionID); err != nil {
		return "", err
	}
	return "/api/canary/agent/sessions/" + url.PathEscape(sessionID) + suffix, nil
}

func (c *CanarySignalingClient) Approve(ctx context.Context, sessionID string) error {
	path, err := c.sessionPath(sessionID, "/approve")
	if err != nil { return err }
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (c *CanarySignalingClient) Reject(ctx context.Context, sessionID string) error {
	path, err := c.sessionPath(sessionID, "/reject")
	if err != nil { return err }
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (c *CanarySignalingClient) MarkActive(ctx context.Context, sessionID string) error {
	path, err := c.sessionPath(sessionID, "/active")
	if err != nil { return err }
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{}, nil)
}

func (c *CanarySignalingClient) PostSignal(ctx context.Context, sessionID, kind string, payload any) error {
	if err := pcOSRemoteValidateSignal(kind, payloadSizeJSON(payload)); err != nil {
		return err
	}
	path, err := c.sessionPath(sessionID, "/signals")
	if err != nil { return err }
	return c.doJSON(ctx, http.MethodPost, path, map[string]any{"kind": kind, "payload": payload}, nil)
}

func (c *CanarySignalingClient) PollSignals(ctx context.Context, sessionID string, after int64) ([]Signal, error) {
	if after < 0 {
		return nil, errors.New("signal cursor cannot be negative")
	}
	path, err := c.sessionPath(sessionID, "/signals")
	if err != nil { return nil, err }
	path += "?after=" + fmt.Sprint(after)
	var reply struct { Signals []Signal `json:"signals"` }
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &reply); err != nil {
		return nil, err
	}
	return reply.Signals, nil
}
