package main

import "testing"

func TestRemoteControlV1DefaultsFailClosed(t *testing.T) {
	p := pcOSRemoteControlPolicy()
	if p.Enabled {
		t.Fatal("remote control must default disabled")
	}
	if !p.AttendedApprovalRequired || p.UnattendedAccessEnabled {
		t.Fatal("v1 must require attended approval and refuse unattended access")
	}
	if !p.VisibleSessionIndicator || !p.EmergencyDisconnect || !p.AuditRequired {
		t.Fatal("visible indicator, emergency disconnect and audit are mandatory")
	}
	if p.SecureDesktopControl {
		t.Fatal("secure desktop/UAC control must remain disabled")
	}
	if p.SignalingCarriesMedia {
		t.Fatal("signaling plane must not carry screen/media payloads")
	}
	if !p.WebRTCDTLSEncryption {
		t.Fatal("WebRTC DTLS encryption is mandatory")
	}
	if !p.Capabilities.ScreenStream {
		t.Fatal("screen streaming capability must be modeled")
	}
	if p.Capabilities.Mouse || p.Capabilities.Keyboard || p.Capabilities.Clipboard || p.Capabilities.FileTransfer || p.Capabilities.Audio {
		t.Fatal("phase 1 must not enable input/clipboard/file/audio control")
	}
}

func TestRemoteSessionTransitions(t *testing.T) {
	allowed := [][2]pcOSRemoteSessionState{
		{pcOSRemoteStateRequested, pcOSRemoteStateApproved},
		{pcOSRemoteStateApproved, pcOSRemoteStateConnecting},
		{pcOSRemoteStateConnecting, pcOSRemoteStateActive},
		{pcOSRemoteStateActive, pcOSRemoteStateEnded},
	}
	for _, pair := range allowed {
		if !pcOSRemoteTransitionAllowed(pair[0], pair[1]) {
			t.Fatalf("expected transition %s -> %s", pair[0], pair[1])
		}
	}
	if pcOSRemoteTransitionAllowed(pcOSRemoteStateEnded, pcOSRemoteStateActive) {
		t.Fatal("ended sessions must never reactivate")
	}
	if pcOSRemoteTransitionAllowed(pcOSRemoteStateRequested, pcOSRemoteStateActive) {
		t.Fatal("approval/connection states must not be bypassed")
	}
}

func TestRemoteSignalValidation(t *testing.T) {
	for _, kind := range []string{"offer", "answer", "ice_candidate", "hangup"} {
		if err := pcOSRemoteValidateSignal(kind, 1024); err != nil {
			t.Fatalf("%s should be accepted: %v", kind, err)
		}
	}
	if err := pcOSRemoteValidateSignal("input_event", 10); err == nil {
		t.Fatal("input events must not be accepted by signaling contract")
	}
	if err := pcOSRemoteValidateSignal("offer", pcOSRemoteMaxSignalBytes+1); err == nil {
		t.Fatal("oversized signaling payload must be refused")
	}
}

func TestRemoteSessionIDValidation(t *testing.T) {
	if err := pcOSRemoteValidateSessionID("RcV1_0123456789abcdefghijklmnop"); err != nil {
		t.Fatalf("valid id rejected: %v", err)
	}
	for _, bad := range []string{"short", "../../remote-session-0123456789", "remote session 01234567890123456789"} {
		if err := pcOSRemoteValidateSessionID(bad); err == nil {
			t.Fatalf("bad session id accepted: %q", bad)
		}
	}
}
