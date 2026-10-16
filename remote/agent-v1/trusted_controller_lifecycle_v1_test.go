package remoteagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestTrustedControllerEnrollmentAndRevocationLifecycle(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)
	enrollment, audit, err := EnrollTrustedController(TrustedControllerEnrollmentRequest{
		ControllerID:         "controller-owner-1",
		Label:                "Owner browser",
		PublicKey:            pub,
		OwnerAuthorizationID: "owner-auth-123",
		Permissions: TrustedControllerPermissions{
			Screen: true, Mouse: true, Keyboard: true,
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Record.Revoked || enrollment.Fingerprint == "" || enrollment.OwnerAuthorizationID != "owner-auth-123" {
		t.Fatalf("unexpected enrollment: %+v", enrollment)
	}
	if audit.Action != TrustedAuditEnrolled || audit.ControllerID != enrollment.Record.ID || audit.Fingerprint != enrollment.Fingerprint {
		t.Fatalf("unexpected enrollment audit: %+v", audit)
	}

	revoked, revokeAudit, err := RevokeTrustedController(enrollment, "owner-auth-456", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !revoked.Record.Revoked || revoked.RevokedAt == nil {
		t.Fatalf("expected revoked enrollment: %+v", revoked)
	}
	if revokeAudit.Action != TrustedAuditRevoked || revokeAudit.OwnerAuthorizationID != "owner-auth-456" {
		t.Fatalf("unexpected revoke audit: %+v", revokeAudit)
	}
}

func TestTrustedControllerEnrollmentRequiresOwnerAuthorization(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = EnrollTrustedController(TrustedControllerEnrollmentRequest{
		ControllerID: "controller-1",
		Label:        "Owner browser",
		PublicKey:    pub,
		Permissions: TrustedControllerPermissions{Screen: true},
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected missing owner authorization to be rejected")
	}
}

func TestTrustedControllerEnrollmentRequiresScreenPermission(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = EnrollTrustedController(TrustedControllerEnrollmentRequest{
		ControllerID:         "controller-1",
		Label:                "Owner browser",
		PublicKey:            pub,
		OwnerAuthorizationID: "owner-auth-1",
		Permissions: TrustedControllerPermissions{
			Mouse: true, Keyboard: true,
		},
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected screen-less remote controller enrollment to be rejected")
	}
}

func TestTrustedControllerCannotBeRevokedTwice(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	enrollment, _, err := EnrollTrustedController(TrustedControllerEnrollmentRequest{
		ControllerID:         "controller-1",
		Label:                "Owner browser",
		PublicKey:            pub,
		OwnerAuthorizationID: "owner-auth-1",
		Permissions:          TrustedControllerPermissions{Screen: true},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	revoked, _, err := RevokeTrustedController(enrollment, "owner-auth-2", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := RevokeTrustedController(revoked, "owner-auth-3", now.Add(2*time.Second)); err == nil {
		t.Fatal("expected second revocation to be rejected")
	}
}

func TestTrustedControllerEnrollmentCopiesPublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enrollment, _, err := EnrollTrustedController(TrustedControllerEnrollmentRequest{
		ControllerID:         "controller-1",
		Label:                "Owner browser",
		PublicKey:            pub,
		OwnerAuthorizationID: "owner-auth-1",
		Permissions:          TrustedControllerPermissions{Screen: true},
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	before := enrollment.Record.PublicKey[0]
	pub[0] ^= 0xff
	if enrollment.Record.PublicKey[0] != before {
		t.Fatal("enrollment must own an immutable copy of the public key")
	}
}
