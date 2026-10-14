package remoteagent

import (
	"errors"
	"fmt"
	"strings"
)

const pcOSRemoteMaxSignalBytes = 256 * 1024

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
