package main

import (
    "crypto/ed25519"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "errors"
    "strconv"
    "strings"
)

// pcOSVerifySignedUpdatePayload is the mandatory v5.1+ auto-update trust gate.
// It operates entirely in memory so an unsigned/untrusted payload is never even
// staged to disk. The pinned public key is loaded by pcOSLoadPublicKey().
func pcOSVerifySignedUpdatePayload(version, expectedSHA string, expectedSize int64, signatureB64 string, payload []byte) (string, error) {
    version = strings.TrimSpace(version)
    expectedSHA = strings.ToLower(strings.TrimSpace(expectedSHA))
    signatureB64 = strings.TrimSpace(signatureB64)

    if version == "" || expectedSHA == "" || expectedSize <= 0 || signatureB64 == "" {
        return "", errors.New("signed update manifest requires version, sha256, size, and signature")
    }
    if int64(len(payload)) != expectedSize {
        return "", errors.New("signed update payload size mismatch")
    }

    sum := sha256.Sum256(payload)
    actual := hex.EncodeToString(sum[:])
    if actual != expectedSHA {
        return "", errors.New("signed update SHA-256 mismatch")
    }

    pub, err := pcOSLoadPublicKey()
    if err != nil {
        return "", err
    }
    if len(pub) != ed25519.PublicKeySize {
        return "", errors.New("invalid pinned Ed25519 public key")
    }

    sig, err := base64.StdEncoding.DecodeString(signatureB64)
    if err != nil || len(sig) != ed25519.SignatureSize {
        return "", errors.New("invalid Ed25519 update signature encoding")
    }

    canonical := []byte(version + "|" + expectedSHA + "|" + strconv.FormatInt(expectedSize, 10))
    if !ed25519.Verify(pub, canonical, sig) {
        return "", errors.New("Ed25519 update signature verification failed")
    }
    return expectedSHA, nil
}
