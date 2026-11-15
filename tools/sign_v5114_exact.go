//go:build !windows || windows

// A manual, fail-closed one-shot signer for the EXACT already-verified
// v5.1.14 Zhirox PC Relay binary. This does NOT publish or deploy an update.
// Its public signature can subsequently be reviewed by the release gate.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	exactVersion = "5.1.14"
	exactFilename = "ZhiroxAISupervisor.exe"
	exactSHA256 = "371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa"
	exactSize int64 = 20025856
	exactSignerPubHex = "19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143"
	secretEnv = "ZHIROX_UPDATE_ED25519_SEED_B64"
)

func canonical() []byte {
	return []byte(exactVersion + "|" + exactSHA256 + "|" + strconv.FormatInt(exactSize, 10))
}

func failCategory(err error) {
	// NEVER print the Ed25519 seed, environment variables, file contents or full exceptions.
	fmt.Fprintln(os.Stderr, "SIGNED_PROOF_REJECTED:", err.Error())
	os.Exit(1)
}

func checkExactBinary(file string) error {
	if filepath.Base(file) != exactFilename {
		return errors.New("candidate_filename_mismatch")
	}
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() != exactSize {
		return errors.New("candidate_size_or_type_mismatch")
	}
	f, err := os.Open(file)
	if err != nil {
		return errors.New("candidate_unreadable")
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil || n != exactSize {
		return errors.New("candidate_read_incomplete")
	}
	if hex.EncodeToString(sum.Sum(nil)) != exactSHA256 {
		return errors.New("candidate_sha256_mismatch")
	}
	return nil
}

func decodePinnedPublic() (ed25519.PublicKey, error) {
	pub, err := hex.DecodeString(exactSignerPubHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("pinned_key_invalid")
	}
	return ed25519.PublicKey(pub), nil
}

func keyFromSeed(seedB64 string) (ed25519.PrivateKey, error) {
	if seedB64 == "" {
		return nil, errors.New("signing_key_unavailable")
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(seedB64)
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("signing_key_invalid_encoding")
	}
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	priv := ed25519.NewKeyFromSeed(raw)
	derived := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if derived != exactSignerPubHex {
		return nil, errors.New("signing_key_not_matching_installed_trust_anchor")
	}
	return priv, nil
}

type signedProof struct {
	Version string `json:"version"`
	Filename string `json:"filename"`
	SHA256 string `json:"sha256"`
	SizeBytes int64 `json:"size_bytes"`
	PublicKeyHex string `json:"public_key_hex"`
	Canonical string `json:"canonical"`
	Algorithm string `json:"algorithm"`
	SignatureB64 string `json:"signature_b64"`
	Publishable bool `json:"publishable"`
	RolloutAllowed bool `json:"rollout_allowed"`
}

func makeProof(sig []byte) (signedProof, error) {
	pub, err := decodePinnedPublic()
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, canonical(), sig) {
		return signedProof{}, errors.New("signature_verification_failed")
	}
	return signedProof{
		Version: exactVersion, Filename: exactFilename, SHA256: exactSHA256,
		SizeBytes: exactSize, PublicKeyHex: exactSignerPubHex,
		Canonical: string(canonical()), Algorithm: "Ed25519",
		SignatureB64: base64.StdEncoding.EncodeToString(sig),
		Publishable: false, RolloutAllowed: false,
	}, nil
}

func signExactBinary(exePath, outFile, encodedSeed string) error {
	if err := checkExactBinary(exePath); err != nil {
		return err
	}
	priv, err := keyFromSeed(strings.TrimSpace(encodedSeed))
	if err != nil {
		return err
	}
	sig := ed25519.Sign(priv, canonical())
	defer func() {
		for i := range priv {
			priv[i] = 0
		}
	}()
	proof, err := makeProof(sig)
	if err != nil {
		return err
	}
	if filepath.Base(outFile) != "v5114-signature-proof.json" {
		return errors.New("output_filename_not_allowed")
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0700); err != nil {
		return errors.New("output_dir_unavailable")
	}
	if _, err := os.Lstat(outFile); err == nil {
		return errors.New("proof_exists_refuse_overwrite")
	} else if !os.IsNotExist(err) {
		return errors.New("output_stat_error")
	}
	data, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		return errors.New("proof_serialization_failed")
	}
	if err := os.WriteFile(outFile, append(data, '\n'), 0600); err != nil {
		return errors.New("proof_write_failed")
	}
	return nil
}

func verifyExactProof(exePath, proofFile string) error {
	if err := checkExactBinary(exePath); err != nil {
		return err
	}
	if filepath.Base(proofFile) != "v5114-signature-proof.json" {
		return errors.New("proof_filename_not_allowed")
	}
	stat, err := os.Lstat(proofFile)
	if err != nil || !stat.Mode().IsRegular() || stat.Size() > 4096 {
		return errors.New("proof_size_or_type_invalid")
	}
	b, err := os.ReadFile(proofFile)
	if err != nil {
		return errors.New("proof_unreadable")
	}
	var proof signedProof
	if err := json.Unmarshal(b, &proof); err != nil {
		return errors.New("proof_json_invalid")
	}
	sig, err := base64.StdEncoding.Strict().DecodeString(proof.SignatureB64)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("proof_signature_encoding_invalid")
	}
	want, err := makeProof(sig)
	if err != nil {
		return err
	}
	if proof != want {
		return errors.New("proof_metadata_mismatch")
	}
	return nil
}

func main() {
	// Only one version and artifact can be signed. No remote user-controlled
	// version, source branch, signing key path, code, URL or command is accepted.
	if len(os.Args) != 4 || (os.Args[1] != "sign" && os.Args[1] != "verify") {
		failCategory(errors.New("usage_sign_or_verify_exact_only"))
	}
	if os.Args[1] == "sign" {
		if err := signExactBinary(os.Args[2], os.Args[3], os.Getenv(secretEnv)); err != nil {
			failCategory(err)
		}
		fmt.Println("SIGNED_PROOF_CREATED: signature only, update channel unchanged")
	} else {
		if err := verifyExactProof(os.Args[2], os.Args[3]); err != nil {
			failCategory(err)
		}
		fmt.Println("SIGNED_PROOF_VERIFIED: trusted public key matched, no rollout")
	}
}
