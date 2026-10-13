//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	mbOK            = 0x00000000
	mbYesNo         = 0x00000004
	mbIconWarning   = 0x00000030
	mbSetForeground = 0x00010000
	mbTopMost       = 0x00040000
	idYes           = 6
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

func runApprove() int {
	label := sanitizeDisplayText(argValue("--controller-label="), maxDisplayLabelChars)
	fingerprint := sanitizeDisplayText(argValue("--controller-fingerprint="), maxFingerprintChars)
	if label == "" {
		label = "Unknown controller"
	}
	if fingerprint == "" {
		fingerprint = "Not provided"
	}
	body := fmt.Sprintf(
		"A remote screen session has been requested.\r\n\r\nController: %s\r\nFingerprint: %s\r\n\r\nAllow this screen-viewing session?\r\n\r\nMouse and keyboard control are disabled in this version.",
		label,
		fingerprint,
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
	label := sanitizeDisplayText(argValue("--controller-label="), maxDisplayLabelChars)
	if label == "" {
		label = "Remote controller"
	}
	body := fmt.Sprintf(
		"REMOTE SCREEN SESSION IS ACTIVE\r\n\r\nController: %s\r\n\r\nThe remote side can view this screen. Mouse and keyboard control are disabled.\r\n\r\nClick OK (or close this window) to disconnect the remote session immediately.",
		label,
	)
	_, err := showMessage(
		"Zhirox Remote Session Active — Disconnect",
		body,
		mbOK|mbIconWarning|mbSetForeground|mbTopMost,
	)
	if err != nil {
		return 22
	}
	// The agent treats helper exit code 20 as a local emergency disconnect.
	return 20
}

func main() {
	mode, err := validateMode(argValue("--mode="))
	if err != nil {
		os.Exit(2)
	}
	switch mode {
	case "approve":
		os.Exit(runApprove())
	case "indicator":
		os.Exit(runIndicator())
	default:
		os.Exit(2)
	}
}
