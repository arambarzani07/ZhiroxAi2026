# v5.1.14 Cashierynwe single-action signer — UNSIGNED BUILD CANDIDATE

This is a source/build candidate ONLY, not a deployable PC Relay upgrade.

## Provenance and compatibility

The installed relay reports version **5.1.13**. Its latest promoted
production record in Supabase has the exact source commit:

`46b208507293d677428a25b146599eb537c532b9`

The new workflow `.github/workflows/build-v5114-cashierynwe-signer.yml`
checks out that immutable historical commit separately under `baseline/`.
It executes the historical v5.1.13 source restoration and build pipeline,
including v5.1.7–v5.1.13 remote screen canary, attended consent, diagnostics,
SQL comparison, item watch and private CSV cloud upload. Then it overlays
ONLY the newly reviewed Cashierynwe typed signing source from the new
workflow commit. Its version transition script refuses to relabel a
v5.1.6 or incomplete baseline as v5.1.14.

The Go signing action is strictly for
`cashierynwe_sign_release` (`Server-c24b5c0f`) with exact
`commit_sha`, `tag` and `approval_id`. Go unit tests reject
unexpected executable, path, shell, PFX and password input. The cloud
connector must separately verify that the command was enqueued with a
**privileged, exact-args, single-use owner approval**.

## Trust and privacy

- The current v5.1.13 production Relay and Supabase function are unchanged.
- The build artifact is explicitly `publishable=false` and
  `signed_update_published=false`; the workflow only uploads an artifact.
- No Ed25519 private key, Authenticode PFX, password or GitHub token is
  inserted into the code, runner job or artifact.
- The installed updater must validate a future Ed25519 signature against
  its existing pinned public key before a **separate, approved** rollout.
- Remote input and unattended access remain disabled. Existing SQL,
  CSV, DVR, Admin.exe and Cashier.exe behavior must be preserved.
- The Relay Windows service account might not own the Windows certificate
  `CurrentUser\My` or the authenticated `gh` profile. This must be
  verified on a controlled PC test before releasing a signed POS installer.
- A green CI workflow establishes build readiness; it is **not evidence**
  of an actual v5.1.14 signed rollout or signed Cashierynwe v1.0.1.

## Remaining release gates

1. Verify all inherited modules and build provenance in GitHub Actions.
2. Obtain a legitimate, trusted Ed25519 signing operation for this exact
   v5.1.14 candidate and immutable SHA-256. Do not bypass/rotate the key.
3. Stage in a read-only canary with rollback and validated owner approval.
4. Only after a verified rollout to the intended PC should the
   Cashierynwe signing action be allowed by the production control plane.
5. Confirm the correct Windows account and its existing code-signing key;
   then perform signing with that key staying local.
