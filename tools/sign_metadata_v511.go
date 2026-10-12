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
    Version   string `json:"version"`
    File      string `json:"file"`
    SHA256    string `json:"sha256"`
    Size      int64  `json:"size"`
    Signature string `json:"signature"`
    Algorithm string `json:"algorithm"`
    Notes     string `json:"notes"`
}

func main() {
    seedB64 := strings.TrimSpace(os.Getenv("ZHIROX_UPDATE_ED25519_SEED_B64"))
    if seedB64 == "" {
        fatal("ZHIROX_UPDATE_ED25519_SEED_B64 is required; release signing is fail-closed")
    }
    seed, err := base64.StdEncoding.DecodeString(seedB64)
    if err != nil || len(seed) != ed25519.SeedSize {
        fatal("release signing seed must be base64-encoded 32-byte Ed25519 seed")
    }

    privateKey := ed25519.NewKeyFromSeed(seed)
    publicKey := privateKey.Public().(ed25519.PublicKey)
    publicHex := hex.EncodeToString(publicKey)

    pinnedRaw, err := os.ReadFile(filepath.Clean("channel/update-public-key.hex"))
    if err != nil {
        fatal("compiled trust anchor marker is missing: " + err.Error())
    }
    pinnedHex := strings.ToLower(strings.TrimSpace(string(pinnedRaw)))
    if pinnedHex == "" || pinnedHex != strings.ToLower(publicHex) {
        fatal("release signing key does not match the compiled pinned Ed25519 trust anchor")
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
        Version:   "5.1.1",
        File:      "ZhiroxAISupervisor.exe",
        SHA256:    hash,
        Size:      size,
        Signature: base64.StdEncoding.EncodeToString(sig),
        Algorithm: "Ed25519",
        Notes:     "Zhirox PC OS v5.1.1 self-healing hardening; critical business lock preserved; Zero-UI; pinned Ed25519 fail-closed update chain",
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
    fmt.Println("signed", artifact, hash, "with pinned Ed25519 trust anchor")
}

func fatal(msg string) {
    fmt.Fprintln(os.Stderr, msg)
    os.Exit(1)
}
