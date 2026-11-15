# Zhirox Relay v5.1.15 — Zero-Screen signing candidate

## Status: source/build only — never installed, published or approved

This version is rebuilt from the exact reviewed v5.1.13 historical source baseline
`46b208507293d677428a25b146599eb537c532b9` plus the strictly scoped
Cashierynwe signer source. It contains the prompt-free Git/GCM/GitHub preflight
hardening merged into `main` after the original v5.1.14 proof.

Workflow: `.github/workflows/build-v5115-headless-signer.yml`.
It must remain an **unsigned GitHub Actions artifact** with
`publishable=false`, `signed_update_published=false`,
`remote_input_enabled=false`, and `unattended_access=false`.
The action is restricted to `cashierynwe_sign_release`, the fixed private
repo, approved commit SHA, tag `v1.0.1` and a truly authenticated
single-use owner approval. It must not expose arbitrary shell or file writes.

## Immutable signature trust

The verified v5.1.14 Ed25519 signature covers **only**
`5.1.14|371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa|20025856`.
It is invalid for a v5.1.15 candidate. Never reuse, relabel, or overwrite
that artifact, production release, signature, or live channel.

For v5.1.15, use the **existing** trusted Ed25519 keyholder to sign this
new version's own exact `version|sha256|size` request, verify against the
already pinned public key, and prove authenticated artifact transport.

## Zero-UI gate

Git commands and the fixed PowerShell signer use hidden subprocesses and
no-interaction environment variables. If cached authentication is absent,
the release **fails closed**, with no login screen and no credential export.

This cannot eliminate Windows certificate provider PIN prompts, UAC,
SmartScreen or cross-account certificate-store access. A controlled Windows
canary must verify the intended Relay SID, `CurrentUser\\My` private key,
the signing provider's true no-UI capability, verified backups and rollback.

## Deployment remains blocked until

1. All Windows GitHub Actions tests and provenance checks pass.
2. The immutable v5.1.15 candidate is signed and verified with the existing key.
3. Authenticated download / staging and updater rollback are tested.
4. Exact one-time owner approval is validated, with no self-approval.
5. Physical Windows no-screen signing and nonproduction A→B checks pass.

No Supabase writes, remote PC commands, legacy Admin.exe/Cashier.exe changes,
SQL Server/DVR interruptions, or silent live installation are authorized here.
