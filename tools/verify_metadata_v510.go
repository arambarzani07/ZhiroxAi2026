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
    Version   string `json:"version"`
    File      string `json:"file"`
    SHA256    string `json:"sha256"`
    Size      int64  `json:"size"`
    Signature string `json:"signature"`
    Algorithm string `json:"algorithm"`
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
    if m.Version != "5.1.0" || m.File != "ZhiroxAISupervisor.exe" || !strings.EqualFold(m.Algorithm, "Ed25519") {
        fatal("metadata identity/algorithm mismatch")
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

    publicRaw, err := os.ReadFile(filepath.Clean("channel/update-public-key.hex"))
    if err != nil {
        fatal("read public key marker: " + err.Error())
    }
    publicKey, err := hex.DecodeString(strings.TrimSpace(string(publicRaw)))
    if err != nil || len(publicKey) != ed25519.PublicKeySize {
        fatal("invalid Ed25519 public key")
    }
    sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(m.Signature))
    if err != nil || len(sig) != ed25519.SignatureSize {
        fatal("invalid Ed25519 signature encoding/size")
    }
    canonical := []byte(m.Version + "|" + strings.ToLower(gotHash) + "|" + strconv.FormatInt(m.Size, 10))
    if !ed25519.Verify(ed25519.PublicKey(publicKey), canonical, sig) {
        fatal("Ed25519 release signature verification failed")
    }
    fmt.Println("verified", artifactPath, gotHash, "with Ed25519")
}

func fatal(msg string) {
    fmt.Fprintln(os.Stderr, msg)
    os.Exit(1)
}
