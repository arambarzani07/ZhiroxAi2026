package remoteagent

import (
	"errors"
	"fmt"
)

// RemoteCapabilities is the explicit capability contract for attended remote
// sessions. Phase 2 permits screen viewing with optional mouse/keyboard input,
// while clipboard, file transfer, and audio remain disabled.
type RemoteCapabilities struct {
	ScreenStream bool
	Mouse        bool
	Keyboard     bool
	Clipboard    bool
	FileTransfer bool
	Audio        bool
}

func ParseRemoteCapabilitiesV2(raw map[string]any) (RemoteCapabilities, error) {
	if len(raw) == 0 {
		return RemoteCapabilities{}, errors.New("remote capabilities are required")
	}
	allowed := map[string]bool{
		"screen_stream": true,
		"mouse":         true,
		"keyboard":      true,
		"clipboard":     true,
		"file_transfer": true,
		"audio":         true,
	}
	for key := range raw {
		if !allowed[key] {
			return RemoteCapabilities{}, fmt.Errorf("unsupported remote capability %q", key)
		}
	}
	readBool := func(name string) (bool, error) {
		value, ok := raw[name]
		if !ok {
			return false, fmt.Errorf("remote capability %q is missing", name)
		}
		b, ok := value.(bool)
		if !ok {
			return false, fmt.Errorf("remote capability %q must be boolean", name)
		}
		return b, nil
	}
	caps := RemoteCapabilities{}
	var err error
	if caps.ScreenStream, err = readBool("screen_stream"); err != nil { return RemoteCapabilities{}, err }
	if caps.Mouse, err = readBool("mouse"); err != nil { return RemoteCapabilities{}, err }
	if caps.Keyboard, err = readBool("keyboard"); err != nil { return RemoteCapabilities{}, err }
	if caps.Clipboard, err = readBool("clipboard"); err != nil { return RemoteCapabilities{}, err }
	if caps.FileTransfer, err = readBool("file_transfer"); err != nil { return RemoteCapabilities{}, err }
	if caps.Audio, err = readBool("audio"); err != nil { return RemoteCapabilities{}, err }

	if !caps.ScreenStream {
		return RemoteCapabilities{}, errors.New("screen_stream must be enabled for attended remote control")
	}
	if caps.Clipboard || caps.FileTransfer || caps.Audio {
		return RemoteCapabilities{}, errors.New("clipboard, file transfer, and audio are disabled in phase 2")
	}
	return caps, nil
}

func (c RemoteCapabilities) RequiresInputApproval() bool {
	return c.Mouse || c.Keyboard
}

func (c RemoteCapabilities) ConsentSummary() string {
	if c.Mouse && c.Keyboard {
		return "Screen viewing + mouse and keyboard control"
	}
	if c.Mouse {
		return "Screen viewing + mouse control"
	}
	if c.Keyboard {
		return "Screen viewing + keyboard control"
	}
	return "Screen viewing only"
}
