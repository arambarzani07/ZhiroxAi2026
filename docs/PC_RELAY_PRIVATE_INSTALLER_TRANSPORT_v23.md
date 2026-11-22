# PC Relay — owner-approved private Cashierynwe installer transport v23

## Status
**Source only. Never deployed; no production Supabase function or store PC changed.**

This design closes the GitHub private-release transport *control-plane* gap,
but does not perform or authorize a Windows installation. The running PC Relay
still reports v5.1.15. A signed v5.1.16 agent, attended user-session broker,
Windows canary, backup and rollback remain required.

## Fixed, immutable release

- Repository: `arambarzani07/Cashierynwe` (private)
- Tag: `v1.0.10`; asset ID `779202065`
- Filename: `Cashierynwe-Setup-1.0.10-x64.exe`
- Size: `158128464` bytes
- SHA256: `e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b`
- Authorized relay agent: `Server-c24b5c0f`, minimum `5.1.16`

## Exact request / approval sequence

1. Authenticated owner approves the exact agent, action
   `signed_app_install` and args containing only fixed app ID/version plus
   one-time approval UUID.
2. Privileged `enqueue_approved_pc_relay_command` RPC atomically consumes
   the approval and binds `command_id`, no self-approval.
3. The trusted agent, authenticated by its existing secret, requests
   `GET /api/agent/Server-c24b5c0f/signed-app-download?approval_id=<uuid>`.
4. Supabase verifies matching consumed approval, command ID, exact args,
   agent ID, and recent delivery (15 minute bound). A UUID by itself is not
   authorization.
5. The Edge function uses its own limited, server-side GitHub token to
   request only the exact asset ID. It rejects unexpected HTTP status or
   non-HTTPS/non-GitHub CDN redirects.
6. It returns the *ephemeral GitHub CDN URL* and immutable expected size/hash
   to the authenticated agent. The GitHub token never leaves Supabase.
7. The Windows Relay must download with strict TLS/hostname checks, a bounded
   streaming size, checksum verification and a locked staging directory.
   The per-user broker must independently validate Authenticode and SHA256
   before any NSIS execution.

## Required configuration before a future canary

- Provision an account-scoped **read-only** GitHub fine-grained token, limited
  to `arambarzani07/Cashierynwe` contents/releases read. Store it only as
  `CASHIERYNWE_PRIVATE_RELEASES_READ_TOKEN` in the correct Supabase Edge
  Function environment; do not copy to the PC, chat, source, logs or a
  public-facing website.
- Deploy v23 only after independent code review, staged endpoint tests,
  owner-approved authorization verification, and explicit promotion.
- If the token is not set, or GitHub serves the artifact directly with HTTP
  200 instead of a validated short-lived CDN 302, fail closed: do not
  fall back to proxying 158 MB through the Edge function.
- No browser CORS access is permitted for the response containing the
  temporary CDN URL.
- Use `ZhiroxAI` service's existing agent secret to authenticate; never
  issue a separate generic anonymous download API.

## Remaining gap

The current source has NO active Windows user-session broker, no proven
unprivileged staging path, and no production signing/rollout of v5.1.16.
**Do not claim installer success** based only on a signed URL, download hash
or successful NSIS exit status. Verify application launch, correct Windows
profile, receipt printing, SQLite data integrity and rollback on a Windows
test PC before production use.
