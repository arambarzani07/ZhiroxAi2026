package main

// Feature builds intentionally carry no release trust anchor and therefore
// cannot accept automatic signed updates. The main release workflow replaces
// this value at build time from the Ed25519 signing seed's public key.
const pcOSPinnedUpdatePublicKeyHex = ""
