package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	pcOSRemoteControlProtocolVersion = "1"
	pcOSRemoteMaxSessionDuration    = 30 * time.Minute
	pcOSRemoteApprovalTTL           = 2 * time.Minute
	pcOSRemoteMaxControllers        = 1
	pcOSRemoteMaxInputEventsPerSec  = 240
	pcOSRemoteMaxSignalBytes        = 256 * 1024
)

type pcOSRemoteSessionState string

const (
	pcOSRemoteStateRequested  pcOSRemoteSessionState = "requested"
	pcOSRemoteStateApproved   pcOSRemoteSessionState = "approved"
	pcOSRemoteStateConnecting pcOSRemoteSessionState = "connecting"
	pcOSRemoteStateActive     pcOSRemoteSessionState = "active"
	pcOSRemoteStateEnded      pcOSRemoteSessionState = "ended"
	pcOSRemoteStateRejected   pcOSRemoteSessionState = "rejected"
	pcOSRemoteStateExpired    pcOSRemoteSessionState = "expired"
)

type pcOSRemoteCapabilities struct {
	ScreenStream bool `json:"screen_stream"`
	Mouse        bool `json:"mouse"`
	Keyboard     bool `json:"keyboard"`
	Clipboard    bool `json:"clipboard"`
	FileTransfer bool `json:"file_transfer"`
	Audio        bool `json:"audio"`
}

type pcOSRemoteSessionPolicy struct {
	ProtocolVersion          string                 `json:"protocol_version"`
	Enabled                  bool                   `json:"enabled"`
	AttendedApprovalRequired bool                   `json:"attended_approval_required"`
	UnattendedAccessEnabled  bool                   `json:"unattended_access_enabled"`
	VisibleSessionIndicator  bool                   `json:"visible_session_indicator"`
	EmergencyDisconnect      bool                   `json:"emergency_disconnect"`
	AuditRequired            bool                   `json:"audit_required"`
	SecureDesktopControl     bool                   `json:"secure_desktop_control"`
	SignalingCarriesMedia    bool                   `json:"signaling_carries_media"`
	WebRTCDTLSEncryption     bool                   `json:"webrtc_dtls_encryption"`
	TURNFallbackAllowed      bool                   `json:"turn_fallback_allowed"`
	MaxControllers          int                    `json:"max_controllers"`
	MaxSessionSeconds       int                    `json:"max_session_seconds"`
	IdleTimeoutSeconds      int                    `json:"idle_timeout_seconds"`
	ApprovalTTLSeconds      int                    `json:"approval_ttl_seconds"`
	MaxInputEventsPerSecond int                    `json:"max_input_events_per_second"`
	Capabilities            pcOSRemoteCapabilities `json:"capabilities"`
}

func pcOSRemoteControlPolicy() pcOSRemoteSessionPolicy {
	return pcOSRemoteSessionPolicy{
		ProtocolVersion:          pcOSRemoteControlProtocolVersion,
		Enabled:                  false,
		AttendedApprovalRequired: true,
		UnattendedAccessEnabled:  false,
		VisibleSessionIndicator:  true,
		EmergencyDisconnect:      true,
		AuditRequired:            true,
		SecureDesktopControl:     false,
		SignalingCarriesMedia:    false,
		WebRTCDTLSEncryption:     true,
		TURNFallbackAllowed:      true,
		MaxControllers:          pcOSRemoteMaxControllers,
		MaxSessionSeconds:       int(pcOSRemoteMaxSessionDuration / time.Second),
		// Screen-only media is peer-to-peer WebRTC and does not traverse the
		// signaling service, so a server-side "idle" claim would be false.
		// Keep it disabled until a reviewed end-to-end activity signal exists.
		IdleTimeoutSeconds:      0,
		ApprovalTTLSeconds:      int(pcOSRemoteApprovalTTL / time.Second),
		MaxInputEventsPerSecond: pcOSRemoteMaxInputEventsPerSec,
		Capabilities: pcOSRemoteCapabilities{
			ScreenStream: true,
			Mouse:        false,
			Keyboard:     false,
			Clipboard:    false,
			FileTransfer: false,
			Audio:        false,
		},
	}
}

func pcOSRemoteValidateSessionID(v string) error {
	v = strings.TrimSpace(v)
	if len(v) < 24 || len(v) > 128 {
		return errors.New("remote session id length is invalid")
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errors.New("remote session id contains unsupported characters")
	}
	return nil
}

func pcOSRemoteValidateSignal(kind string, payloadBytes int) error {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "offer", "answer", "ice_candidate", "hangup":
	default:
		return fmt.Errorf("unsupported remote signaling kind %q", kind)
	}
	if payloadBytes < 0 || payloadBytes > pcOSRemoteMaxSignalBytes {
		return fmt.Errorf("remote signaling payload exceeds %d bytes", pcOSRemoteMaxSignalBytes)
	}
	return nil
}

func pcOSRemoteTransitionAllowed(from, to pcOSRemoteSessionState) bool {
	switch from {
	case pcOSRemoteStateRequested:
		return to == pcOSRemoteStateApproved || to == pcOSRemoteStateRejected || to == pcOSRemoteStateExpired
	case pcOSRemoteStateApproved:
		return to == pcOSRemoteStateConnecting || to == pcOSRemoteStateEnded || to == pcOSRemoteStateExpired
	case pcOSRemoteStateConnecting:
		return to == pcOSRemoteStateActive || to == pcOSRemoteStateEnded || to == pcOSRemoteStateExpired
	case pcOSRemoteStateActive:
		return to == pcOSRemoteStateEnded || to == pcOSRemoteStateExpired
	case pcOSRemoteStateEnded, pcOSRemoteStateRejected, pcOSRemoteStateExpired:
		return false
	default:
		return false
	}
}
