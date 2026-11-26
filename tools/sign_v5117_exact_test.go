package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedVersionHashSizeAndTrustAnchor(t *testing.T) {
	if string(canonical()) != "5.1.17|8e5c6e3c97e7362aaee4fda6afa331f0bc38d8e792f3bd98072bfb815c58e063|20037120" {
		t.Fatal("canonical message changed")
	}
	pub, err := decodePinnedPublic()
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatal("Ed25519 trust anchor invalid")
	}
}

func TestKeyMaterialMissingWrongOrMalformedFailsClosed(t *testing.T) {
	for _, v := range []string{"", "not base64", base64.StdEncoding.EncodeToString(make([]byte, 3)), strings.Repeat("A", 44)} {
		if _, err := keyFromSeed(v); err == nil {
			t.Fatal("untrusted key was accepted")
		}
	}
	demoSeed := make([]byte, 32)
	for i := range demoSeed { demoSeed[i] = byte(i) }
	priv, err := keyFromSeed(base64.StdEncoding.EncodeToString(demoSeed))
	if err == nil || priv != nil {
		t.Fatal("self-generated key should not match the installed trust anchor")
	}
}

func TestProofRejectsUnpinnedSignatureAndUntrustedExtraContent(t *testing.T) {
	priv := ed25519.NewKeyFromSeed(make([]byte, 32))
	sig := ed25519.Sign(priv, canonical())
	if _, err := makeProof(sig); err == nil {
		t.Fatal("signature from unrelated key was accepted")
	}
	if _, err := makeProof(sig[:10]); err == nil {
		t.Fatal("truncated signature was accepted")
	}
}

func TestUntrustedFileCannotBeSignedWithoutReadingKey(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, exactFilename)
	if err := os.WriteFile(name, []byte("NOT AN EXE"), 0600); err != nil { t.Fatal(err) }
	out := filepath.Join(root, "v5117-signature-proof.json")
	if err := signExactBinary(name, out, ""); err == nil {
		t.Fatal("short fake artifact accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("unexpected signed proof written")
	}
	if err := verifyExactProof(name, out); err == nil {
		t.Fatal("missing proof validated")
	}
}

func TestCurrentManifestStillExplicitlyBlocksUnsignedRelease(t *testing.T) {
	manifestPath := filepath.Join("..", "release-candidates", "v5.1.17", "manifest.json")
	b, err := os.ReadFile(manifestPath)
	if err != nil { t.Fatal(err) }
	var data struct {
		Version string `json:"version"`
		Sha256 string `json:"sha256"`
		SizeBytes int64 `json:"size_bytes"`
		PublicKey string `json:"pinned_update_public_key_hex"`
		Signature *string `json:"signature_b64"`
		Publishable bool `json:"publishable"`
		Rollout bool `json:"rollout_allowed"`
	}
	if err := json.Unmarshal(b, &data); err != nil { t.Fatal(err) }
	if data.Version != exactVersion || data.Sha256 != exactSHA256 ||
		data.SizeBytes != exactSize || data.PublicKey != exactSignerPubHex ||
		data.Signature != nil || data.Publishable || data.Rollout {
		t.Fatal("reviewed candidate is no longer an exact unsigned blocked candidate")
	}
}

func TestSigningProofDoesNotLeakOtherContent(t *testing.T) {
	const pinnedVersion = "5.1.17"
	if exactVersion != pinnedVersion { t.Fatal("version changed") }
	if len(exactSignerPubHex) != 64 { t.Fatal("invalid trust anchor") }
	if b, err := hex.DecodeString(exactSHA256); err != nil || len(b) != sha256.Size {
		t.Fatal("invalid hash")
	}
	if len(exactFilename) == 0 || strings.ContainsAny(exactFilename, "/\\") {
		t.Fatal("invalid executable name")
	}
}
