package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "strconv"
    "testing"
)

func TestSignedUpdatePayloadTrustGate(t *testing.T) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        t.Fatal(err)
    }
    payload := []byte("zhirox-pc-os-signed-update")
    sum := sha256.Sum256(payload)
    hash := hex.EncodeToString(sum[:])
    size := int64(len(payload))
    canonical := []byte("5.1.1|" + hash + "|" + strconv.FormatInt(size, 10))
    signature := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical))

    got, err := pcOSVerifySignedUpdatePayloadWithKey("5.1.1", hash, size, signature, payload, pub)
    if err != nil {
        t.Fatal(err)
    }
    if got != hash {
        t.Fatalf("unexpected verified hash: %s", got)
    }

    if _, err := pcOSVerifySignedUpdatePayloadWithKey("5.1.1", hash, size, "", payload, pub); err == nil {
        t.Fatal("missing signature must fail closed")
    }
    if _, err := pcOSVerifySignedUpdatePayloadWithKey("5.1.1", hash, size+1, signature, payload, pub); err == nil {
        t.Fatal("size mismatch must fail closed")
    }

    tampered := append([]byte(nil), payload...)
    tampered[0] ^= 0x01
    if _, err := pcOSVerifySignedUpdatePayloadWithKey("5.1.1", hash, size, signature, tampered, pub); err == nil {
        t.Fatal("payload tampering must fail closed")
    }

    _, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        t.Fatal(err)
    }
    wrongSig := base64.StdEncoding.EncodeToString(ed25519.Sign(wrongPriv, canonical))
    if _, err := pcOSVerifySignedUpdatePayloadWithKey("5.1.1", hash, size, wrongSig, payload, pub); err == nil {
        t.Fatal("wrong signer must fail closed")
    }
}
