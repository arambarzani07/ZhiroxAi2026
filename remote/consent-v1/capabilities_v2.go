package main

import (
	"errors"
	"strings"
)

type consentCapabilityMode string

const (
	consentScreenOnly     consentCapabilityMode = "screen"
	consentScreenMouse    consentCapabilityMode = "screen-mouse"
	consentScreenKeyboard consentCapabilityMode = "screen-keyboard"
	consentScreenInput    consentCapabilityMode = "screen-input"
)

func parseConsentCapabilityMode(value string) (consentCapabilityMode, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return consentScreenOnly, nil
	}
	switch consentCapabilityMode(value) {
	case consentScreenOnly:
		return consentScreenOnly, nil
	case consentScreenMouse:
		return consentScreenMouse, nil
	case consentScreenKeyboard:
		return consentScreenKeyboard, nil
	case consentScreenInput:
		return consentScreenInput, nil
	default:
		return "", errors.New("unsupported consent capability mode")
	}
}

func consentApprovalNotice(mode consentCapabilityMode) string {
	switch mode {
	case consentScreenMouse:
		return "The remote side can view this screen and control the mouse. Keyboard control is disabled."
	case consentScreenKeyboard:
		return "The remote side can view this screen and control the keyboard. Mouse control is disabled."
	case consentScreenInput:
		return "The remote side can view this screen and control the mouse and keyboard."
	default:
		return "The remote side can view this screen. Mouse and keyboard control are disabled."
	}
}

func consentActiveNotice(mode consentCapabilityMode) string {
	switch mode {
	case consentScreenMouse:
		return "REMOTE CONTROL IS ACTIVE: screen viewing and mouse control are enabled. Keyboard control is disabled."
	case consentScreenKeyboard:
		return "REMOTE CONTROL IS ACTIVE: screen viewing and keyboard control are enabled. Mouse control is disabled."
	case consentScreenInput:
		return "REMOTE CONTROL IS ACTIVE: screen viewing, mouse, and keyboard are enabled for the approved controller."
	default:
		return "REMOTE SCREEN IS ACTIVE: screen viewing only; mouse and keyboard are disabled."
	}
}
