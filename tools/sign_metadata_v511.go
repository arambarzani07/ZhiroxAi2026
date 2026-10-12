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

func main() {
	seedB64 := strings.TrimSpace(os.Getenv("ZHIROX_V510_TRANSITION_ED25519_SEED_B64"))
	if seedB64 == "" {
		fatal("ZHIROX_V510_TRANSITION_ED25519_SEED_B64 is required; transition signing is fail-closed")
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil || len(seed) != ed25519.SeedSize {
		fatal("transition signing seed must be a base64-encoded 32-byte Ed25519 seed")
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	signingPublicKey := privateKey.Public().(ed25519.PublicKey)
	signingPublicHex := strings.ToLower(hex.EncodeToString(signingPublicKey))

	transitionMarker := mustPublicKeyFile("channel/transition-signing-public-key.hex")
	if transitionMarker != signingPublicHex {
		fatal("transition signing seed does not match the prepared transition public-key marker")
	}
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

	canonical := []byte("5.1.1|" + hash + "|" + strconv.FormatInt(size, 10))
	sig := ed25519.Sign(privateKey, canonical)

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
	if err := os.WriteFile("channel/ZhiroxAISupervisor.exe.sha256", []byte(hash+"  ZhiroxAISupervisor.exe\n"), 0644); err != nil {
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
