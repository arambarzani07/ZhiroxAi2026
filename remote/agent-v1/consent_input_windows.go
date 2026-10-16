//go:build windows

package remoteagent

import (
	"errors"
	"fmt"
)

func remoteInputCapabilityMode(mouse, keyboard bool) (string, error) {
	switch {
	case mouse && keyboard:
		return "screen-input", nil
	case mouse:
		return "screen-mouse", nil
	case keyboard:
		return "screen-keyboard", nil
	default:
		return "", errors.New("remote input consent requires mouse or keyboard capability")
	}
}

// ApproveInput launches the pinned consent helper in the active interactive
// desktop with an exact disclosure of the requested input capabilities.
// The helper exit code is only an explicit local Yes/No decision; the caller
// must still bind that decision to the trusted permit and session.
func (l *ConsentLauncher) ApproveInput(controllerLabel, controllerFingerprint string, mouse, keyboard bool) (bool, error) {
	mode, err := remoteInputCapabilityMode(mouse, keyboard)
	if err != nil {
		return false, err
	}
	label, err := encodeConsentArg(controllerLabel)
	if err != nil {
		return false, err
	}
	fingerprint, err := encodeConsentArg(controllerFingerprint)
	if err != nil {
		return false, err
	}
	proc, err := l.startInteractive([]string{
		"--mode=approve",
		"--capability-mode=" + mode,
		"--controller-label-b64=" + label,
		"--controller-fingerprint-b64=" + fingerprint,
	})
	if err != nil {
		return false, err
	}
	defer proc.close()
	exitCode, timedOut, err := proc.wait(remoteConsentApprovalTTLMS)
	if err != nil {
		return false, err
	}
	if timedOut {
		_ = proc.stop(124)
		_, _, _ = proc.wait(remoteConsentKillWaitMS)
		return false, errors.New("input consent helper timed out after the 2-minute approval window")
	}
	switch exitCode {
	case remoteConsentApproveYes:
		return true, nil
	case remoteConsentApproveNo:
		return false, nil
	default:
		return false, fmt.Errorf("input consent helper approval exited with code %d", exitCode)
	}
}

// StartInputIndicator launches the visible attended-control indicator with the
// exact approved input capability disclosure. Closing it disconnects the
// session through the same SessionIndicator contract used by screen-only mode.
func (l *ConsentLauncher) StartInputIndicator(controllerLabel string, mouse, keyboard bool) (SessionIndicator, error) {
	mode, err := remoteInputCapabilityMode(mouse, keyboard)
	if err != nil {
		return nil, err
	}
	label, err := encodeConsentArg(controllerLabel)
	if err != nil {
		return nil, err
	}
	proc, err := l.startInteractive([]string{
		"--mode=indicator",
		"--capability-mode=" + mode,
		"--controller-label-b64=" + label,
	})
	if err != nil {
		return nil, err
	}
	indicator := &windowsSessionIndicator{proc: proc, done: make(chan error, 1)}
	go func() {
		defer proc.close()
		defer close(indicator.done)
		exitCode, _, waitErr := proc.wait(remoteInfinite)
		if waitErr != nil {
			indicator.done <- waitErr
			return
		}
		switch exitCode {
		case remoteConsentDisconnect, remoteConsentAgentStop:
			indicator.done <- nil
		default:
			indicator.done <- fmt.Errorf("input consent helper indicator exited with code %d", exitCode)
		}
	}()
	return indicator, nil
}
