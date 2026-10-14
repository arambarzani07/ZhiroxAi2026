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

Post-candidate changes have been limited to CI and documentation. Runtime source remains unchanged from the verified candidate.

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

Remote-control browser sessions do not reuse generic relay connector tokens.

`pc_remote_controller_tokens` provides dedicated, short-lived controller authorization:
- token stored as SHA-256 only;
- token bound to exactly one `agent_id`;
- screen-stream permission explicit;
- maximum token TTL 24 hours;
- revoked/expired tokens fail closed.

PostgreSQL also enforces controller-agent scope with a composite foreign key.

## Database defense in depth

The remote-control schema includes:
- `pc_remote_controller_tokens`
- `pc_remote_sessions`
- `pc_remote_signals`
- `pc_remote_audit`

Controls include RLS on every table, no direct client policies, revoked `anon`/`authenticated` access, one live session per agent, 30-minute session TTL, screen-only capability checks, bounded signal/audit payloads, 24-hour controller-token TTL, and DB-level cross-agent scope enforcement.

The schema is **not deployed in production**. It passed executable tests in isolated PostgreSQL 17 CI for RLS/revokes, single-live-session, cross-agent scope, capability refusal and token TTL.

## Signaling hardening

The Edge Function candidate:
- pins `@supabase/supabase-js` exactly;
- exact browser Origin allowlist;
- bounded request bodies;
- dedicated agent-scoped controller tokens;
- existing agent relay authentication;
- target-agent freshness/health checks;
- approval/session expiry;
- conditional state transitions;
- opaque internal errors;
- `no-store` and browser hardening headers.

## Credential transport

The service-side remote agent refuses secret-bearing environment configuration.

Agent secret and TURN credentials are supplied only through a bounded anonymous-pipe/stdin bootstrap. Unknown fields and unsupported ICE schemes fail closed.

A sanitized v5.1.6 source audit verified that the Supervisor loads relay credentials locally through `pcOSLoadRelayCredentials()` and retains them in memory for relay requests. The corrected credential-source audit workflow passed without printing secret values.

## Interactive capture isolation

Direct service/Session-0 desktop capture is not used.

The SHA-pinned remote-agent executable launches itself in media-only helper mode into the active signed-in Windows session:
- WTS active-console token;
- `CreateProcessAsUserW` on `winsta0\default`;
- explicit handle list: H.264 output pipe only;
- signed-in user environment instead of service environment;
- executable SHA reverified before launch;
- helper receives only pipe handle, encoder path/hash, FPS and bitrate;
- helper receives no auth/signaling/TURN credentials;
- helper has no input, clipboard or file-transfer capability.

The local media IPC uses bounded `ZHXH2641` framing and caps H.264 access units at 4 MiB.

## Verified media path

- DIB `CreateDIBSection` + `BitBlt` capture;
- BGRA -> Media Foundation H.264;
- bounded FPS/bitrate/keyframe cadence;
- Pion H.264 WebRTC track, packetization-mode=1;
- Windows encoder probe passed;
- direct-ICE RTP delivery passed;
- consent helper, compile/vet and interactive-helper isolation candidate passed.

Credentialed forced-TURN relay has **not** yet been verified.

## Current production gates

Production remains unchanged at Zhirox v5.1.6. Remote-control database objects/function are not deployed.

Latest live-PC preflight:
- version `5.1.6`
- health `85`
- `disk_low`
- C: approximately `15.97 GB` / `14.97%` free

Remote-control signing/promotion/deployment remains blocked until the live health/disk gate is restored.

No Supabase preview branch has been created because branch/resource creation can incur cost and requires explicit cost approval.

## Remaining gates

1. Restore live PC disk/health gate.
2. Obtain cost-approved isolated Supabase branch/project.
3. Deploy hardened control plane there and run Supabase advisors.
4. Real browser -> real Windows attended screen stream.
5. Credentialed forced-TURN relay test.
6. Real hangup/emergency-disconnect/expiry/failure/reconnect tests.
7. New immutable signed release identity after v5.1.6, preserving Ed25519 trust/rollback gates.
8. Controlled screen-only rollout + live audit.
9. Mouse/keyboard only as a later separately reviewed phase.
