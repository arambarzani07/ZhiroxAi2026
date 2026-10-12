package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type manifest struct {
	Version                       string `json:"version"`
	FromVersion                   string `json:"from_version"`
	File                          string `json:"file"`
	SHA256                        string `json:"sha256"`
	Size                          int64  `json:"size"`
	Signature                     string `json:"signature"`
	Algorithm                     string `json:"algorithm"`
	TrustTransition               bool   `json:"trust_transition"`
	TransitionSigningPublicKeyHex string `json:"transition_signing_public_key_hex"`
	NextTrustPublicKeyHex         string `json:"next_trust_public_key_hex"`
	Notes                         string `json:"notes"`
}

type signingRequest struct {
	Version                       string `json:"version"`
	FromVersion                   string `json:"from_version"`
	File                          string `json:"file"`
	SHA256                        string `json:"sha256"`
	Size                          int64  `json:"size"`
	Canonical                     string `json:"canonical"`
	CanonicalSHA256               string `json:"canonical_sha256"`
	TransitionSigningPublicKeyHex string `json:"transition_signing_public_key_hex"`
	NextTrustPublicKeyHex         string `json:"next_trust_public_key_hex"`
}

func main() {
	signingPublicHex := mustPublicKeyFile("channel/transition-signing-public-key.hex")
	if expected := strings.ToLower(strings.TrimSpace(os.Getenv("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX"))); expected != "" && expected != signingPublicHex {
		fatal("transition signing key does not match the v5.1.0 accepted production trust anchor")
	}

	nextTrustHex := mustPublicKeyFile("channel/update-public-key.hex")
	if nextTrustHex == signingPublicHex {
		fatal("refusing transition release: next trust anchor must differ from the v5.1.0 transition signing key")
	}

	artifact := filepath.Clean("channel/ZhiroxAISupervisor.exe")
	raw, err := os.ReadFile(artifact)
	if err != nil {
		fatal(err.Error())
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	size := int64(len(raw))
	canonicalText := "5.1.1|" + hash + "|" + strconv.FormatInt(size, 10)
	canonical := []byte(canonicalText)
	canonicalSum := sha256.Sum256(canonical)

	if err := os.WriteFile("channel/ZhiroxAISupervisor.exe.sha256", []byte(hash+"  ZhiroxAISupervisor.exe\n"), 0644); err != nil {
		fatal(err.Error())
	}

	seedB64 := strings.TrimSpace(os.Getenv("ZHIROX_V510_TRANSITION_ED25519_SEED_B64"))
	externalSigB64 := strings.TrimSpace(os.Getenv("ZHIROX_V510_TRANSITION_SIGNATURE_B64"))
	unsignedRequest := strings.TrimSpace(os.Getenv("ZHIROX_V511_UNSIGNED_REQUEST")) == "1"
	isMain := strings.TrimSpace(os.Getenv("GITHUB_REF")) == "refs/heads/main"

	if isMain && seedB64 != "" {
		fatal("v5.1.0 transition private seed is forbidden on main; use the server-side one-shot signer")
	}
	if seedB64 != "" && externalSigB64 != "" {
		fatal("provide either a local ephemeral transition seed or an external server-side signature, never both")
	}

	var sig []byte
	switch {
	case seedB64 != "":
		seed, err := base64.StdEncoding.DecodeString(seedB64)
		if err != nil || len(seed) != ed25519.SeedSize {
			fatal("transition signing seed must be a base64-encoded 32-byte Ed25519 seed")
		}
		privateKey := ed25519.NewKeyFromSeed(seed)
		derivedPublicHex := strings.ToLower(hex.EncodeToString(privateKey.Public().(ed25519.PublicKey)))
		if derivedPublicHex != signingPublicHex {
			fatal("transition signing seed does not match the prepared transition public-key marker")
		}
		sig = ed25519.Sign(privateKey, canonical)

	case externalSigB64 != "":
		decoded, err := base64.StdEncoding.DecodeString(externalSigB64)
		if err != nil || len(decoded) != ed25519.SignatureSize {
			fatal("external transition signature must be base64-encoded 64-byte Ed25519 signature")
		}
		pub, err := hex.DecodeString(signingPublicHex)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			fatal("invalid transition signing public key")
		}
		if !ed25519.Verify(ed25519.PublicKey(pub), canonical, decoded) {
			fatal("external server-side transition signature verification failed")
		}
		sig = decoded

	case unsignedRequest:
		req := signingRequest{
			Version:                       "5.1.1",
			FromVersion:                   "5.1.0",
			File:                          "ZhiroxAISupervisor.exe",
			SHA256:                        hash,
			Size:                          size,
			Canonical:                     canonicalText,
			CanonicalSHA256:               hex.EncodeToString(canonicalSum[:]),
			TransitionSigningPublicKeyHex: signingPublicHex,
			NextTrustPublicKeyHex:         nextTrustHex,
		}
		encoded, err := json.Marshal(req)
		if err != nil {
			fatal(err.Error())
		}
		if err := os.WriteFile("channel/signing-request.json", append(encoded, '\n'), 0644); err != nil {
			fatal(err.Error())
		}
		fmt.Println("unsigned v5.1.1 signing request", canonicalText)
		return

	default:
		fatal("no transition signature source provided; use server-side signature or explicit unsigned-request mode")
	}

	m := manifest{
		Version:                       "5.1.1",
		FromVersion:                   "5.1.0",
		File:                          "ZhiroxAISupervisor.exe",
		SHA256:                        hash,
		Size:                          size,
		Signature:                     base64.StdEncoding.EncodeToString(sig),
		Algorithm:                     "Ed25519",
		TrustTransition:               true,
		TransitionSigningPublicKeyHex: signingPublicHex,
		NextTrustPublicKeyHex:         nextTrustHex,
		Notes:                         "One-way v5.1.0 to v5.1.1 trust transition; v5.1.0 key authenticates this release only; v5.1.1 pins a distinct next key; Zero-UI and self-healing hardening preserved",
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile("channel/metadata.json", append(encoded, '\n'), 0644); err != nil {
		fatal(err.Error())
	}
	fmt.Println("signed v5.1.1 one-way trust-transition candidate", hash)
}

func mustPublicKeyFile(path string) string {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		fatal("read public-key marker: " + err.Error())
	}
	keyHex := strings.ToLower(strings.TrimSpace(string(raw)))
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != ed25519.PublicKeySize {
		fatal("invalid Ed25519 public-key marker: " + path)
	}
	return keyHex
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
