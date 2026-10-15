package remoteagent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	AttendedExecutionReviewTTL      = 5 * time.Second
	AttendedExecutionContractV1     = "attended-input-execution/v1"
	attendedExecutionReviewIDBytes  = 16
)

// ReviewedAttendedExecutor describes metadata that a separately reviewed
// platform executor must expose before it can ever be considered for binding.
// This interface deliberately contains no execution method: this phase only
// binds identity/capability metadata and never dispatches or injects OS input.
type ReviewedAttendedExecutor interface {
	ExecutorID() string
	SupportsAttendedCapabilities(mouse, keyboard bool) bool
}

// AttendedExecutionReview is a short-lived, non-executing binding between one
// consumed attended activation grant and one reviewed executor contract.
// Producing or validating this object never activates ControlDispatcher and
// never invokes operating-system input APIs.
type AttendedExecutionReview struct {
	ReviewID          string
	ContractVersion   string
	ExecutorID        string
	GrantReadinessID  string
	ApprovalID        string
	ControllerID      string
	AgentID           string
	SessionID         string
	Fingerprint       string
	Mouse             bool
	Keyboard          bool
	ArbiterGeneration uint64
	IssuedAt           time.Time
	ExpiresAt          time.Time
}

func mintAttendedExecutionReviewID() (string, error) {
	var raw [attendedExecutionReviewIDBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", errors.New("failed to mint attended execution review id")
	}
	return hex.EncodeToString(raw[:]), nil
}

func validateAttendedExecutionReviewID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) != attendedExecutionReviewIDBytes*2 {
		return errors.New("attended execution review id is invalid")
	}
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != attendedExecutionReviewIDBytes {
		return errors.New("attended execution review id is invalid")
	}
	return nil
}

// PrepareReviewedAttendedExecution binds a valid one-time activation grant to
// reviewed executor metadata. The dispatcher remains inactive and the executor
// is never asked to perform input.
func PrepareReviewedAttendedExecution(runtime *AttendedControlRuntime, grant AttendedActivationGrant, executor ReviewedAttendedExecutor, now time.Time) (AttendedExecutionReview, error) {
	if runtime == nil {
		return AttendedExecutionReview{}, errors.New("attended control runtime is required")
	}
	if executor == nil {
		return AttendedExecutionReview{}, errors.New("reviewed attended executor is required")
	}

	now = trustedArbiterNow(now)
	if err := grant.Validate(runtime, now); err != nil {
		return AttendedExecutionReview{}, err
	}

	executorID := strings.TrimSpace(executor.ExecutorID())
	if executorID == "" {
		return AttendedExecutionReview{}, errors.New("reviewed attended executor id is required")
	}
	if !executor.SupportsAttendedCapabilities(grant.Mouse, grant.Keyboard) {
		return AttendedExecutionReview{}, errors.New("reviewed attended executor capability mismatch")
	}

	expiresAt := now.Add(AttendedExecutionReviewTTL)
	if grant.ExpiresAt.Before(expiresAt) {
		expiresAt = grant.ExpiresAt.UTC()
	}
	if !expiresAt.After(now) {
		return AttendedExecutionReview{}, errors.New("attended execution review would already be expired")
	}

	reviewID, err := mintAttendedExecutionReviewID()
	if err != nil {
		return AttendedExecutionReview{}, err
	}

	return AttendedExecutionReview{
		ReviewID:          reviewID,
		ContractVersion:   AttendedExecutionContractV1,
		ExecutorID:        executorID,
		GrantReadinessID:  grant.ReadinessID,
		ApprovalID:        grant.ApprovalID,
		ControllerID:      grant.ControllerID,
		AgentID:           grant.AgentID,
		SessionID:         grant.SessionID,
		Fingerprint:       strings.ToLower(strings.TrimSpace(grant.Fingerprint)),
		Mouse:             grant.Mouse,
		Keyboard:          grant.Keyboard,
		ArbiterGeneration: grant.ArbiterGeneration,
		IssuedAt:          now,
		ExpiresAt:         expiresAt,
	}, nil
}

// Validate re-checks the consumed grant, runtime liveness, executor identity,
// capabilities and the short review TTL. It has no execution side effects.
func (r AttendedExecutionReview) Validate(runtime *AttendedControlRuntime, grant AttendedActivationGrant, executor ReviewedAttendedExecutor, now time.Time) error {
	if runtime == nil {
		return errors.New("attended control runtime is required")
	}
	if executor == nil {
		return errors.New("reviewed attended executor is required")
	}
	if err := validateAttendedExecutionReviewID(r.ReviewID); err != nil {
		return err
	}
	if r.ContractVersion != AttendedExecutionContractV1 {
		return errors.New("attended execution contract version mismatch")
	}

	now = trustedArbiterNow(now)
	if r.IssuedAt.IsZero() || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(r.IssuedAt) || !r.ExpiresAt.After(now) {
		return errors.New("attended execution review is expired or invalid")
	}
	if r.ExpiresAt.Sub(r.IssuedAt) > AttendedExecutionReviewTTL+time.Second {
		return errors.New("attended execution review TTL exceeds policy")
	}
	if err := grant.Validate(runtime, now); err != nil {
		return err
	}

	executorID := strings.TrimSpace(executor.ExecutorID())
	if executorID == "" || executorID != r.ExecutorID {
		return errors.New("reviewed attended executor identity mismatch")
	}
	if !executor.SupportsAttendedCapabilities(r.Mouse, r.Keyboard) {
		return errors.New("reviewed attended executor capability mismatch")
	}
	if r.GrantReadinessID != grant.ReadinessID || r.ApprovalID != grant.ApprovalID || r.ControllerID != grant.ControllerID || r.AgentID != grant.AgentID || r.SessionID != grant.SessionID || r.ArbiterGeneration != grant.ArbiterGeneration {
		return errors.New("attended execution review grant identity mismatch")
	}
	if strings.ToLower(strings.TrimSpace(r.Fingerprint)) != strings.ToLower(strings.TrimSpace(grant.Fingerprint)) {
		return errors.New("attended execution review fingerprint mismatch")
	}
	if r.Mouse != grant.Mouse || r.Keyboard != grant.Keyboard || (!r.Mouse && !r.Keyboard) {
		return errors.New("attended execution review capability mismatch")
	}
	return nil
}
