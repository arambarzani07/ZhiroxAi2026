# Zhirox Full Remote Control v1

Status: **hardened screen-only candidate built and CI-verified — not production enabled**

Base: `main` / Zhirox PC OS v5.1.6

## Verified candidate provenance

Runtime candidate source commit: `1a5ea351576ef2c4c20e53ad80148788bd45573f`

GitHub Actions candidate artifact:
- workflow run: `38519547860`
- artifact id: `11866050037`
- name: `Zhirox-Remote-Control-v1-screen-only-1a5ea351576ef2c4c20e53ad80148788bd45573f`
- size: `5,270,372` bytes
- artifact digest: `sha256:8db27f233d81247d213a7a3782476c4643205fc7af5990909c355253cfedeb34`
- manifest remains `publishable:false` and `production_enabled:false`

Current PR head `0a1c99df6c84c1dace8467ccc92034acb7822c4a` changes only CI workflow files after the runtime candidate commit; runtime source is unchanged from the verified candidate.

## Architecture

1. **Control plane** — Supabase Edge Function handles bounded WebRTC signaling only.
2. **Media plane** — Pion WebRTC carries the H.264 screen stream peer-to-peer, with TURN fallback supported by configuration.
3. **Interactive capture plane** — service-side auth/signaling/WebRTC remains outside the signed-in user desktop; a separate media-only helper is launched into the active interactive Windows session and returns bounded H.264 access units through one inherited anonymous pipe.
4. **Input plane** — disabled. Mouse/keyboard injection is not present in this candidate.

Supabase never proxies live screen/video frames.

## Security policy

- Remote control defaults disabled.
- Every session requires attended approval on the target PC.
- Approval expires after 2 minutes.
- Maximum session duration is 30 minutes.
- Maximum simultaneous controllers is 1.
- Visible session indicator is mandatory.
- Local emergency disconnect is mandatory.
- Unattended access is disabled.
- UAC/secure-desktop control is disabled.
- Signaling does not carry media frames.
- Current capability is screen-stream only.
- Mouse, keyboard, clipboard, file transfer and audio are disabled.
- No `SendInput`, shell, PowerShell, `cmd.exe`, keylogger, stealth capture or security bypass is enabled.
- Screen-only v1 does **not** claim a server-side idle timeout; media is direct WebRTC, so `idle_timeout_seconds=0` until an explicit reviewed end-to-end activity signal exists.

## Dedicated controller authorization

Remote-control browser sessions no longer reuse generic relay connector tokens.

`pc_remote_controller_tokens` provides dedicated, short-lived controller authorization:
- token stored as SHA-256 only;
- token is bound to exactly one `agent_id`;
- screen-stream permission is explicit;
- maximum token TTL is 24 hours;
- revoked/expired tokens fail closed.

PostgreSQL also enforces controller-agent scope with a composite foreign key, so a token for PC-A cannot create a session for PC-B even if an application-layer regression occurs.

## Database defense in depth

The remote-control schema includes:
- `pc_remote_controller_tokens`
- `pc_remote_sessions`
- `pc_remote_signals`
- `pc_remote_audit`

Controls include:
- RLS enabled on all remote tables;
- no direct client RLS policies;
- `anon` and `authenticated` privileges revoked;
- one live session per agent via partial unique index;
- 30-minute session TTL constraint;
- screen-only capability constraints in PostgreSQL;
- signal JSON payload capped at 256 KiB;
- audit metadata bounded;
- controller token TTL capped at 24 hours;
- cross-agent token/session binding enforced in PostgreSQL.

The schema is **not deployed in production**. It was executed successfully in an isolated PostgreSQL 17 GitHub Actions service, including executable tests for RLS/revokes, one-live-session, cross-agent scope, capability refusal and token TTL.

## Signaling hardening

The Edge Function candidate:
- pins `@supabase/supabase-js` to an exact version;
- uses a configured exact browser Origin allowlist;
- native-agent requests without browser Origin remain supported;
- rejects oversized request bodies;
- validates screen-only capabilities;
- authenticates dedicated agent-scoped controller tokens;
- reuses existing agent relay authentication;
- checks target agent freshness and health;
- expires stale approval/session rows;
- uses conditional state transitions;
- emits opaque HTTP 500 responses with request IDs rather than database details;
- uses `Cache-Control: no-store` and browser hardening headers.

## Credential transport

The standalone service-side remote agent refuses secret-bearing environment configuration.

Agent secret and TURN credentials are supplied only through a bounded anonymous-pipe/stdin bootstrap:
- maximum bootstrap size 64 KiB;
- unknown JSON fields are rejected;
- only STUN/STUNS/TURN/TURNS ICE schemes are allowed;
- TURN relay-only mode requires an ICE/TURN server;
- secret-bearing environment variables are explicitly refused.

A sanitized source audit verified that the existing v5.1.6 Supervisor loads relay credentials locally with `pcOSLoadRelayCredentials()` and keeps them in memory for relay requests. The credential-audit workflow passed without printing secret values.

## Interactive capture isolation

Direct service/Session-0 desktop capture is not used by the Windows orchestrator.

`ZhiroxRemoteScreenAgent.exe` launches the same SHA-pinned executable into the active signed-in Windows session in media-only helper mode:
- WTS active-console token;
- `CreateProcessAsUserW` on `winsta0\default`;
- explicit `PROC_THREAD_ATTRIBUTE_HANDLE_LIST` limits inherited handles to the H.264 pipe;
- `CreateEnvironmentBlock` creates the signed-in user's environment instead of inheriting the service environment;
- executable SHA-256 is reverified immediately before launch;
- helper accepts only pipe handle, encoder path/hash, FPS and bitrate;
- helper has no signaling URL, agent secret, controller token or TURN credential;
- helper has no input/clipboard/file-transfer capability.

Local media protocol:
- magic/version `ZHXH2641`;
- bounded width/height/FPS header;
- length-prefixed H.264 access units;
- 4 MiB maximum access unit;
- invalid magic/geometry/FPS/length fails before unsafe allocation.

## Verified media path

- reusable top-down 32-bit DIB capture using `CreateDIBSection` + `BitBlt`;
- BGRA -> Media Foundation H.264;
- bounded FPS, bitrate and keyframe cadence;
- Pion H.264 WebRTC track with packetization-mode=1;
- real Windows encoder probe passed;
- direct-ICE H.264 RTP delivery test passed;
- consent helper build and Windows compile/vet passed;
- interactive-helper isolation candidate build passed.

Credentialed forced-TURN relay has **not** yet been verified and must not be reported as passed.

## Current production gates

Production remains unchanged at Zhirox v5.1.6. The remote-control control-plane tables/function are not deployed.

Latest live-PC preflight showed:
- version: `5.1.6`
- health score: `85`
- alert: `disk_low`
- C: free space: approximately `15.97 GB` / `14.97%`

Therefore remote-control deployment/signing/promotion is fail-closed until the PC health/disk gate is restored.

No Supabase preview branch has been created. Creating a Supabase branch may incur resource cost, so branch creation requires an explicit cost-approved gate.

## Remaining gates before production

1. Restore live PC disk/health gate.
2. Obtain an explicitly approved isolated Supabase branch/project and deploy the hardened schema/signaling function there.
3. Run Supabase security/performance advisors on that isolated deployment.
4. Run a real browser -> real target Windows attended screen-stream session using the verified candidate runtime.
5. Run credentialed forced-TURN relay testing.
6. Verify controller hangup, local emergency disconnect, approval expiry, session expiry, failure and reconnect behavior on the real target.
7. Build a new immutable production release identity (do not replace v5.1.6), sign under the current Ed25519 trust anchor and pass existing updater/rollback health gates.
8. Controlled screen-only rollout followed by live audit.
9. Only after screen-only stability: separately reviewed mouse/keyboard input-control phase.
