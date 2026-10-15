package remoteagent

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"time"
)

const MaxTrustedControllerLabelBytes = 96

type TrustedControllerPermissions struct {
	Screen   bool
	Mouse    bool
	Keyboard bool
}

type TrustedControllerEnrollmentRequest struct {
	ControllerID         string
	Label                string
	PublicKey            ed25519.PublicKey
	OwnerAuthorizationID string
	Permissions          TrustedControllerPermissions
}

type TrustedControllerEnrollment struct {
	Record               TrustedControllerRecord
	Fingerprint          string
	OwnerAuthorizationID string
	EnrolledAt           time.Time
	RevokedAt            *time.Time
}

type TrustedControllerAuditAction string

const (
	TrustedAuditEnrolled TrustedControllerAuditAction = "controller_enrolled"
	TrustedAuditRevoked  TrustedControllerAuditAction = "controller_revoked"
)

type TrustedControllerAuditEvent struct {
	Action               TrustedControllerAuditAction
	ControllerID         string
	Fingerprint          string
	OwnerAuthorizationID string
	OccurredAt           time.Time
}

// TrustedControllerLifecycleStore is deliberately narrow: the control plane can
// persist owner-authorized enrollment/revocation state and append tamper-evident
// audit records without learning or storing a controller private key.
type TrustedControllerLifecycleStore interface {
	PutTrustedControllerEnrollment(enrollment TrustedControllerEnrollment) error
	MarkTrustedControllerRevoked(controllerID string, revokedAt time.Time) error
	AppendTrustedControllerAudit(event TrustedControllerAuditEvent) error
}

func EnrollTrustedController(req TrustedControllerEnrollmentRequest, now time.Time) (TrustedControllerEnrollment, TrustedControllerAuditEvent, error) {
	controllerID := strings.TrimSpace(req.ControllerID)
	label := strings.TrimSpace(req.Label)
	ownerAuthorizationID := strings.TrimSpace(req.OwnerAuthorizationID)
	if controllerID == "" || len(controllerID) > 160 {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("trusted controller id is invalid")
	}
	if label == "" || len([]byte(label)) > MaxTrustedControllerLabelBytes {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("trusted controller label is invalid")
	}
	if ownerAuthorizationID == "" || len(ownerAuthorizationID) > 200 {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("owner authorization is required")
	}
	if len(req.PublicKey) != ed25519.PublicKeySize {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("trusted controller public key is invalid")
	}
	if !req.Permissions.Screen {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("screen permission is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	fingerprint, err := TrustedControllerFingerprint(req.PublicKey)
	if err != nil {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, err
	}
	record := TrustedControllerRecord{
		ID:              controllerID,
		Label:           label,
		PublicKey:       append(ed25519.PublicKey(nil), req.PublicKey...),
		Revoked:         false,
		AllowedScreen:   req.Permissions.Screen,
		AllowedMouse:    req.Permissions.Mouse,
		AllowedKeyboard: req.Permissions.Keyboard,
	}
	enrollment := TrustedControllerEnrollment{
		Record:               record,
		Fingerprint:          fingerprint,
		OwnerAuthorizationID: ownerAuthorizationID,
		EnrolledAt:           now,
	}
	audit := TrustedControllerAuditEvent{
		Action:               TrustedAuditEnrolled,
		ControllerID:         controllerID,
		Fingerprint:          fingerprint,
		OwnerAuthorizationID: ownerAuthorizationID,
		OccurredAt:           now,
	}
	return enrollment, audit, nil
}

func RevokeTrustedController(enrollment TrustedControllerEnrollment, ownerAuthorizationID string, now time.Time) (TrustedControllerEnrollment, TrustedControllerAuditEvent, error) {
	ownerAuthorizationID = strings.TrimSpace(ownerAuthorizationID)
	if ownerAuthorizationID == "" || len(ownerAuthorizationID) > 200 {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("owner authorization is required")
	}
	if enrollment.Record.ID == "" || enrollment.Fingerprint == "" || enrollment.EnrolledAt.IsZero() {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("trusted controller enrollment is invalid")
	}
	if enrollment.Record.Revoked || enrollment.RevokedAt != nil {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("trusted controller is already revoked")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if now.Before(enrollment.EnrolledAt) {
		return TrustedControllerEnrollment{}, TrustedControllerAuditEvent{}, errors.New("revocation time predates enrollment")
	}
	revokedAt := now
	enrollment.Record.Revoked = true
	enrollment.RevokedAt = &revokedAt
	audit := TrustedControllerAuditEvent{
		Action:               TrustedAuditRevoked,
		ControllerID:         enrollment.Record.ID,
		Fingerprint:          enrollment.Fingerprint,
		OwnerAuthorizationID: ownerAuthorizationID,
		OccurredAt:           now,
	}
	return enrollment, audit, nil
}
