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

type releaseManifest struct {
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
}

func main() {
	rawManifest, err := os.ReadFile(filepath.Clean("channel/metadata.json"))
	if err != nil {
		fatal("read metadata: " + err.Error())
	}
	var m releaseManifest
	if err := json.Unmarshal(rawManifest, &m); err != nil {
		fatal("parse metadata: " + err.Error())
	}
	if m.Version != "5.1.1" || m.FromVersion != "5.1.0" || m.File != "ZhiroxAISupervisor.exe" || !strings.EqualFold(m.Algorithm, "Ed25519") || !m.TrustTransition {
		fatal("metadata identity/algorithm/trust-transition mismatch")
	}
	if m.Size <= 0 || len(m.SHA256) != 64 || strings.TrimSpace(m.Signature) == "" {
		fatal("metadata is incomplete")
	}

	artifactPath := filepath.Clean(filepath.Join("channel", m.File))
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		fatal("read artifact: " + err.Error())
	}
	if int64(len(artifact)) != m.Size {
		fatal("artifact size mismatch")
	}
	sum := sha256.Sum256(artifact)
	gotHash := hex.EncodeToString(sum[:])
	if !strings.EqualFold(gotHash, m.SHA256) {
		fatal("artifact SHA-256 mismatch")
	}

	transitionPubHex := mustPublicKeyFile("channel/transition-signing-public-key.hex")
	nextTrustHex := mustPublicKeyFile("channel/update-public-key.hex")
	if transitionPubHex == nextTrustHex {
		fatal("trust transition is invalid: transition signer and next trust anchor are identical")
	}
	if expected := strings.ToLower(strings.TrimSpace(os.Getenv("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX"))); expected != "" {
		if len(expected) != ed25519.PublicKeySize*2 {
			fatal("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX must contain exactly 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(expected); err != nil {
			fatal("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX is not valid hexadecimal")
		}
		if transitionPubHex != expected {
			fatal("transition verifier public key does not match the v5.1.0 accepted production trust anchor")
		}
	}
	if !strings.EqualFold(strings.TrimSpace(m.TransitionSigningPublicKeyHex), transitionPubHex) {
		fatal("metadata transition-signing public key does not match marker")
	}
	if !strings.EqualFold(strings.TrimSpace(m.NextTrustPublicKeyHex), nextTrustHex) {
		fatal("metadata next-trust public key does not match marker")
	}

	transitionPub, _ := hex.DecodeString(transitionPubHex)
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(m.Signature))
	if err != nil || len(sig) != ed25519.SignatureSize {
		fatal("invalid Ed25519 signature encoding/size")
	}
	canonical := []byte(m.Version + "|" + strings.ToLower(gotHash) + "|" + strconv.FormatInt(m.Size, 10))
	if !ed25519.Verify(ed25519.PublicKey(transitionPub), canonical, sig) {
		fatal("Ed25519 transition release signature verification failed")
	}
	fmt.Println("verified v5.1.1 one-way trust transition", artifactPath, gotHash)
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
