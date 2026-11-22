package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "io"
    "os"
    "regexp"
    "time"
)

const (
    brokerAgentID = "Server-c24b5c0f"
    brokerAppID = "cashierynwe"
    brokerVersion = "1.0.10"
    brokerFileName = "Cashierynwe-Setup-1.0.10-x64.exe"
    brokerSHA256 = "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b"
    brokerFileSize int64 = 158128464
    brokerPublisher = "CN=Cashierynwe Single-PC Updates"
    brokerTicketSchema = "zhirox.signed-install-ticket.v1"
    brokerTicketMaxBytes = 4096
)
var brokerUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type InstallTicket struct {
    Schema string `json:"schema"`
    AgentID string `json:"agent_id"`
    AppID string `json:"app_id"`
    Version string `json:"version"`
    Filename string `json:"filename"`
    SHA256 string `json:"sha256"`
    Size int64 `json:"size"`
    ApprovalID string `json:"approval_id"`
    RequestID string `json:"request_id"`
    IssuedUTC time.Time `json:"issued_utc"`
    ExpiresUTC time.Time `json:"expires_utc"`
    OneTimeApprovalConsumed bool `json:"one_time_approval_consumed"`
}

func readAndValidateInstallTicket(raw []byte, now time.Time) (InstallTicket, error) {
    if len(raw) == 0 || len(raw) > brokerTicketMaxBytes {
        return InstallTicket{}, errors.New("ticket_size_out_of_bounds")
    }
    var ticket InstallTicket
    dec := json.NewDecoder(bytes.NewReader(raw))
    dec.DisallowUnknownFields()
    if err := dec.Decode(&ticket); err != nil {
        return InstallTicket{}, errors.New("ticket_json_invalid")
    }
    if dec.Decode(new(interface{})) != io.EOF {
        return InstallTicket{}, errors.New("ticket_has_trailing_data")
    }
    if ticket.Schema != brokerTicketSchema || ticket.AgentID != brokerAgentID ||
       ticket.AppID != brokerAppID || ticket.Version != brokerVersion ||
       ticket.Filename != brokerFileName || ticket.SHA256 != brokerSHA256 ||
       ticket.Size != brokerFileSize || !ticket.OneTimeApprovalConsumed ||
       !brokerUUID.MatchString(ticket.ApprovalID) || !brokerUUID.MatchString(ticket.RequestID) {
        return InstallTicket{}, errors.New("ticket_not_exactly_approved")
    }
    if ticket.IssuedUTC.IsZero() || ticket.ExpiresUTC.IsZero() ||
       ticket.ExpiresUTC.Before(ticket.IssuedUTC) ||
       ticket.ExpiresUTC.Sub(ticket.IssuedUTC) > 15*time.Minute ||
       now.Before(ticket.IssuedUTC) || !now.Before(ticket.ExpiresUTC) {
        return InstallTicket{}, errors.New("ticket_expired_or_clock_invalid")
    }
    return ticket, nil
}

// The ticket must be emitted by a protected service-owned directory ONLY
// after Supabase consumes a one-time owner approval. This parser is NOT an
// authorization oracle. The broker additionally requires visible local consent.
func readTicket(path string, now time.Time) (InstallTicket, error) {
    info, err := os.Lstat(path)
    if err != nil || !info.Mode().IsRegular() || info.Size() > brokerTicketMaxBytes {
        return InstallTicket{}, errors.New("ticket_unavailable_or_untrusted_type")
    }
    f, err := os.Open(path)
    if err != nil { return InstallTicket{}, errors.New("ticket_unreadable") }
    defer f.Close()
    raw, err := io.ReadAll(io.LimitReader(f, brokerTicketMaxBytes+1))
    if err != nil { return InstallTicket{}, errors.New("ticket_unreadable") }
    return readAndValidateInstallTicket(raw, now.UTC())
}
