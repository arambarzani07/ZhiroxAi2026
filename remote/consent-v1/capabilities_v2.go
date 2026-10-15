package main

import (
	"errors"
	"strings"
)

type consentCapabilityMode string

const (
	consentScreenOnly consentCapabilityMode = "screen"
	consentScreenInput consentCapabilityMode = "screen-input"
)

func parseConsentCapabilityMode(value string) (consentCapabilityMode, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return consentScreenOnly, nil
	}
	switch consentCapabilityMode(value) {
	case consentScreenOnly:
		return consentScreenOnly, nil
	case consentScreenInput:
		return consentScreenInput, nil
	default:
		return "", errors.New("unsupported consent capability mode")
	}
}

func consentApprovalNotice(mode consentCapabilityMode) string {
	if mode == consentScreenInput {
		return "The remote side can view this screen and control the mouse and keyboard."
	}
	return "The remote side can view this screen. Mouse and keyboard control are disabled."
}

func consentActiveNotice(mode consentCapabilityMode) string {
	if mode == consentScreenInput {
		return "REMOTE CONTROL IS ACTIVE: screen viewing, mouse, and keyboard are enabled for the approved controller."
	}
	return "REMOTE SCREEN IS ACTIVE: screen viewing only; mouse and keyboard are disabled."
}
