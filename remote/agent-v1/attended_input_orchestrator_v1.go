package remoteagent

import (
	"errors"
	"strings"
	"time"
)

// RemoteInputConsent is the narrow attended-input UI boundary. Implementations
// must surface an explicit local approval prompt and a visible active-session
// indicator with exact mouse/keyboard capability disclosure.
type RemoteInputConsent interface {
	ApproveInput(controllerLabel, controllerFingerprint string, mouse, keyboard bool) (bool, error)
	StartInputIndicator(controllerLabel string, mouse, keyboard bool) (SessionIndicator, error)
}

type AttendedInputApprovalResult struct {
	Approval      LocalInputApproval
	Authorization AttendedControlAuthorization
}

// RequestAttendedInputAuthorization is the orchestration boundary between the
// trusted-controller permit and the interactive consent helper. A trusted
// controller is never enough on its own: a fresh explicit local Yes decision
// is converted into a short-lived approval proof bound to the same session.
// This function does not activate a ControlDispatcher and does not execute OS input.
func RequestAttendedInputAuthorization(
	arbiter *TrustedSessionArbiter,
	permit TrustedControlPermit,
	consent RemoteInputConsent,
	controllerLabel string,
	approvalID string,
	now time.Time,
) (AttendedInputApprovalResult, error) {
	if arbiter == nil {
		return AttendedInputApprovalResult{}, errors.New("trusted session arbiter is required")
	}
	if consent == nil {
		return AttendedInputApprovalResult{}, errors.New("interactive input consent provider is required")
	}
	now = trustedArbiterNow(now)
	if err := permit.Validate(arbiter, now); err != nil {
		return AttendedInputApprovalResult{}, err
	}
	if !permit.Mouse && !permit.Keyboard {
		return AttendedInputApprovalResult{}, errors.New("trusted permit does not request input capability")
	}
	approved, err := consent.ApproveInput(
		strings.TrimSpace(controllerLabel),
		strings.ToLower(strings.TrimSpace(permit.Fingerprint)),
		permit.Mouse,
		permit.Keyboard,
	)
	if err != nil {
		return AttendedInputApprovalResult{}, err
	}
	if !approved {
		return AttendedInputApprovalResult{}, errors.New("local user denied remote input control")
	}
	approval, err := NewLocalInputApproval(permit, approvalID, true, now)
	if err != nil {
		return AttendedInputApprovalResult{}, err
	}
	authorization, err := AuthorizeAttendedControl(arbiter, permit, approval, now)
	if err != nil {
		return AttendedInputApprovalResult{}, err
	}
	return AttendedInputApprovalResult{Approval: approval, Authorization: authorization}, nil
}

// StartAttendedInputIndicator re-validates the complete authorization before
// presenting the visible active-control indicator. Closing the indicator is
// handled by the existing SessionIndicator disconnect contract.
func StartAttendedInputIndicator(
	arbiter *TrustedSessionArbiter,
	permit TrustedControlPermit,
	consent RemoteInputConsent,
	result AttendedInputApprovalResult,
	controllerLabel string,
	now time.Time,
) (SessionIndicator, error) {
	if consent == nil {
		return nil, errors.New("interactive input consent provider is required")
	}
	if err := result.Authorization.Validate(arbiter, permit, result.Approval, trustedArbiterNow(now)); err != nil {
		return nil, err
	}
	return consent.StartInputIndicator(
		strings.TrimSpace(controllerLabel),
		result.Authorization.Mouse,
		result.Authorization.Keyboard,
	)
}
