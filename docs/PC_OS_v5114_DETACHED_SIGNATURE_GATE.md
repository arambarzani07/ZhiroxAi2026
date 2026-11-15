# Independent signature verification — Zhirox PC Relay v5.1.14

**Security status: unsigned candidate; deployment NOT authorized.**

The Windows build verified by GitHub Actions run **43708666560** is an
unsigned candidate. This repository deliberately does **not** contain the
Ed25519 private key. Do not invent a signature, replace the trust anchor,
disable the updater signature check, or change the live update channel to
make an unsigned build installable.

Pinned production provenance:
- Version: `5.1.14` (upgrade from installed `5.1.13`)
- Source: `091c7e23ee50807a5c721a45c7b1c7c2267695c0`
- Signed v5.1.13 baseline: `46b208507293d677428a25b146599eb537c532b9`
- Artifact ID: `14159935571`
- File: `ZhiroxAISupervisor.exe`
- Size: `20025856` bytes
- SHA-256: `371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa`
- Ed25519 public key: `19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143`
- Canonical signed message: `5.1.14|371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa|20025856`

## Read-only verification when a trusted detached signature exists

Only the keyholder or an already-approved trusted server-side signing service
may sign **exactly the canonical bytes above** with the existing pinned key.
Provide the resulting 64-byte Ed25519 signature in standard Base64, saved as a
separate text file (e.g. `signature.b64`). This is NOT a request to upload
the private key, seed, PFX or authentication token anywhere.

With the original CI artifact downloaded through an authenticated channel,
run:

```bash
node tools/verify-v5114-signature.mjs \
  --manifest release-candidates/v5.1.14/manifest.json \
  --binary /trusted/path/ZhiroxAISupervisor.exe \
  --signature /trusted/path/signature.b64
```

This tool validates exact manifest schema, version, release provenance,
original binary size, SHA-256, immutable Ed25519 public key and the detached
signature. It is implemented with Node built-ins; no package installation
or external APIs are required. It performs **no writes**.

Positive output means *only* that the signature cryptographically matches
the specified candidate. It does not authorize production deployment,
prove the artifact was transported safely, or establish owner approval.

The CI tests use **ephemeral test-only keys**, never the production private
key; they verify tampering, forged signatures, altered flags, extra fields
and symlink rejection. CI intentionally verifies that the unsigned
candidate is refused.

## Remaining independent production gates

1. An authenticated owner approval authorizing exact candidate SHA/size and
   its one-time scope, verified by trusted server-side code.
2. Production Ed25519 signature from the existing trusted keyholder.
3. Durable authenticated artifact distribution to the existing updater
   *without* changing pinned public keys or reducing signature verification.
4. Canary, service identity/certificate-store checks, health checks and
   verified rollback on the target Windows PC.
5. Only then release promotion to production and the subsequent
   Cashierynwe v1.0.1 Windows code-signing/auto-update validation.

The existing live PC Relay `v5.1.13`, POS, Admin.exe/Cashier.exe,
SQL Server and DVR are left untouched by this verifier.
