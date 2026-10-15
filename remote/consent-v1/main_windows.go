//go:build windows

package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	mbOK                  = 0x00000000
	mbYesNo               = 0x00000004
	mbIconWarning         = 0x00000030
	mbSetForeground       = 0x00010000
	mbTopMost             = 0x00040000
	idYes                 = 6
	maxPairingStdinBytes  = 128
)

var (
	user32      = syscall.NewLazyDLL("user32.dll")
	messageBoxW = user32.NewProc("MessageBoxW")
)

func argValue(prefix string) string {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
	}
	return ""
}

func decodedDisplayArg(b64Prefix, legacyPrefix string) string {
	if raw := argValue(b64Prefix); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(decoded) > 512 {
			return ""
		}
		return string(decoded)
	}
	return argValue(legacyPrefix)
}

func showMessage(title, body string, flags uintptr) (int, error) {
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0, err
	}
	bodyPtr, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return 0, err
	}
	ret, _, callErr := messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(bodyPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		flags,
	)
	if ret == 0 {
		return 0, callErr
	}
	return int(ret), nil
}

func readPairingCodeFromStdin() string {
	if argValue("--pairing-stdin=") != "1" {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxPairingStdinBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxPairingStdinBytes {
		return ""
	}
	return sanitizePairingCode(strings.TrimSpace(string(data)))
}

func runPairing() int {
	// Pairing codes are secret-like one-time values. They are accepted only via
	// inherited stdin and are deliberately refused from command-line arguments.
	if argValue("--pairing-code=") != "" || argValue("--pairing-code-b64=") != "" {
		return 32
	}
	code := readPairingCodeFromStdin()
	if code == "" {
		return 32
	}
	body := fmt.Sprintf(
		"CANARY / SCREEN ONLY\r\n\r\nPairing code:\r\n\r\n%s\r\n\r\nEnter this one-time code in the controller browser. The code expires quickly and does not grant control by itself. A separate Yes/No approval will still be required before screen viewing starts.\r\n\r\nMouse and keyboard control are disabled.\r\n\r\nClick OK (or close this window) to cancel pairing.",
		code,
	)
	_, err := showMessage(
		"Zhirox Remote Canary Pairing",
		body,
		mbOK|mbIconWarning|mbSetForeground|mbTopMost,
	)
	if err != nil {
		return 32
	}
	return 30
}

func runApprove() int {
	mode, err := parseConsentCapabilityMode(argValue("--capability-mode="))
	if err != nil {
		return 12
	}
	label := sanitizeDisplayText(decodedDisplayArg("--controller-label-b64=", "--controller-label="), maxDisplayLabelChars)
	fingerprint := sanitizeDisplayText(decodedDisplayArg("--controller-fingerprint-b64=", "--controller-fingerprint="), maxFingerprintChars)
	if label == "" {
		label = "Unknown controller"
	}
	if fingerprint == "" {
		fingerprint = "Not provided"
	}
	body := fmt.Sprintf(
		"A remote session has been requested.\r\n\r\nController: %s\r\nFingerprint: %s\r\n\r\n%s\r\n\r\nAllow this session?",
		label,
		fingerprint,
		consentApprovalNotice(mode),
	)
	ret, err := showMessage(
		"Zhirox Remote Session Request",
		body,
		mbYesNo|mbIconWarning|mbSetForeground|mbTopMost,
	)
	if err != nil {
		return 12
	}
	if ret == idYes {
		return 0
	}
	return 10
}

func runIndicator() int {
	mode, err := parseConsentCapabilityMode(argValue("--capability-mode="))
	if err != nil {
		return 22
	}
	label := sanitizeDisplayText(decodedDisplayArg("--controller-label-b64=", "--controller-label="), maxDisplayLabelChars)
	if label == "" {
		label = "Remote controller"
	}
	body := fmt.Sprintf(
		"%s\r\n\r\nController: %s\r\n\r\n%s\r\n\r\nClick OK (or close this window) to disconnect the remote session immediately.",
		map[bool]string{true: "REMOTE CONTROL SESSION IS ACTIVE", false: "REMOTE SCREEN SESSION IS ACTIVE"}[mode != consentScreenOnly],
		label,
		consentActiveNotice(mode),
	)
	_, err = showMessage(
		"Zhirox Remote Session Active — Disconnect",
		body,
		mbOK|mbIconWarning|mbSetForeground|mbTopMost,
	)
	if err != nil {
		return 22
	}
	return 20
}

func main() {
	mode, err := validateMode(argValue("--mode="))
	if err != nil {
		os.Exit(2)
	}
	switch mode {
	case "pairing":
		os.Exit(runPairing())
	case "approve":
		os.Exit(runApprove())
	case "indicator":
		os.Exit(runIndicator())
	default:
		os.Exit(2)
	}
}
