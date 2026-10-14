//go:build windows

package remoteagent

import "fmt"

const remoteConsentPairingCancel = 30

// StartPairing displays the one-time canary pairing code on the attended target
// desktop. Closing the window cancels pairing. Stop is used after a controller
// claims the code so a separate Yes/No approval can be shown next.
func (l *ConsentLauncher) StartPairing(pairingCode string) (SessionIndicator, error) {
	if _, err := NormalizeCanaryPairingCode(pairingCode); err != nil {
		return nil, err
	}
	encoded, err := encodeConsentArg(pairingCode)
	if err != nil {
		return nil, err
	}
	proc, err := l.startInteractive([]string{
		"--mode=pairing",
		"--pairing-code-b64=" + encoded,
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
		case remoteConsentPairingCancel, remoteConsentAgentStop:
			indicator.done <- nil
		default:
			indicator.done <- fmt.Errorf("consent helper pairing display exited with code %d", exitCode)
		}
	}()
	return indicator, nil
}
