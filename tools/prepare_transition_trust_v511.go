package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	transitionSeed := mustSeed("ZHIROX_V510_TRANSITION_ED25519_SEED_B64")
	transitionPub := ed25519.NewKeyFromSeed(transitionSeed).Public().(ed25519.PublicKey)
	transitionHex := strings.ToLower(hex.EncodeToString(transitionPub))
	nextHex := mustNextPublicKeyHex()

	if transitionHex == nextHex {
		fatal("transition signing key and next trust key must be different")
	}

	if expected := strings.ToLower(strings.TrimSpace(os.Getenv("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX"))); expected != "" {
		if len(expected) != ed25519.PublicKeySize*2 {
			fatal("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX must contain exactly 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(expected); err != nil {
			fatal("ZHIROX_V510_ACCEPTED_PUBLIC_KEY_HEX is not valid hexadecimal")
		}
		if transitionHex != expected {
			fatal("transition signing key does not match the v5.1.0 accepted production trust anchor")
		}
	}

	if err := os.MkdirAll("buildsrc", 0755); err != nil {
		fatal(err.Error())
	}
	if err := os.MkdirAll("channel", 0755); err != nil {
		fatal(err.Error())
	}

	trustSource := "package main\n\n// Generated for the v5.1.1 trust transition. The binary accepts future\n// releases only from the NEW trust anchor; the old v5.1.0 key is used solely\n// to authenticate this one transition release.\nconst pcOSPinnedUpdatePublicKeyHex = \"" + nextHex + "\"\n"
	if err := os.WriteFile(filepath.Clean("buildsrc/update_trust_v510.go"), []byte(trustSource), 0600); err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(filepath.Clean("channel/update-public-key.hex"), []byte(nextHex+"\n"), 0644); err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(filepath.Clean("channel/transition-signing-public-key.hex"), []byte(transitionHex+"\n"), 0644); err != nil {
		fatal(err.Error())
	}

	fmt.Println("prepared v5.1.1 one-way Ed25519 trust transition; next private key was not required or emitted")
}

func mustNextPublicKeyHex() string {
	publicHex := strings.ToLower(strings.TrimSpace(os.Getenv("ZHIROX_V511_NEXT_PUBLIC_KEY_HEX")))
	ephemeralSeedB64 := strings.TrimSpace(os.Getenv("ZHIROX_V511_NEXT_ED25519_SEED_B64"))

	if publicHex != "" && ephemeralSeedB64 != "" {
		fatal("provide either ZHIROX_V511_NEXT_PUBLIC_KEY_HEX or an ephemeral next seed, never both")
	}

	if publicHex != "" {
		if len(publicHex) != ed25519.PublicKeySize*2 {
			fatal("ZHIROX_V511_NEXT_PUBLIC_KEY_HEX must contain exactly 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(publicHex); err != nil {
			fatal("ZHIROX_V511_NEXT_PUBLIC_KEY_HEX is not valid hexadecimal")
		}
		return publicHex
	}

	if ephemeralSeedB64 == "" {
		fatal("ZHIROX_V511_NEXT_PUBLIC_KEY_HEX is required; production trust transition is fail-closed")
	}

	// Feature CI may derive a throw-away public key from an ephemeral seed so
	// the two-key transition path can be tested end-to-end. Production/main is
	// explicitly forbidden from receiving the next private seed: the real next
	// key is generated and retained by the server-side KMS and only its public
	// half may enter the release build.
	if strings.TrimSpace(os.Getenv("GITHUB_REF")) == "refs/heads/main" {
		fatal("next Ed25519 private seed is forbidden on main; provide only ZHIROX_V511_NEXT_PUBLIC_KEY_HEX from the server-side KMS")
	}

	seed, err := base64.StdEncoding.DecodeString(ephemeralSeedB64)
	if err != nil || len(seed) != ed25519.SeedSize {
		fatal("ephemeral ZHIROX_V511_NEXT_ED25519_SEED_B64 must be a base64-encoded 32-byte Ed25519 seed")
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	return strings.ToLower(hex.EncodeToString(pub))
}

func mustSeed(name string) []byte {
	seedB64 := strings.TrimSpace(os.Getenv(name))
	if seedB64 == "" {
		fatal(name + " is required; trust transition is fail-closed")
	}
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil || len(seed) != ed25519.SeedSize {
		fatal(name + " must be a base64-encoded 32-byte Ed25519 seed")
	}
	return seed
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
