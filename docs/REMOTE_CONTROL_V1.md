# Zhirox Full Remote Control v1

Status: **Phase 1 foundation — not production enabled**

Base: `main` / Zhirox PC OS v5.1.6

## Goal

Build a TeamViewer/AnyDesk-class remote desktop path without weakening the existing signed-update, watchdog, rollback, and audit boundaries.

The architecture separates three planes:

1. **Control plane** — Supabase Edge Function creates/approves/expires sessions and relays only WebRTC signaling (offer/answer/ICE/hangup).
2. **Media plane** — WebRTC peer connection carries the screen stream directly between controller and agent, with TURN fallback when direct ICE connectivity is unavailable.
3. **Input plane** — WebRTC DataChannel will carry bounded mouse/keyboard events only after a separate reviewed release enables input injection.

Supabase must never proxy the live screen/video stream.

## WebRTC implementation target

The Windows agent is planned to use `github.com/pion/webrtc/v4`. At the time this branch was created, the current upstream v4 release is `v4.2.23`. Dependency pinning must be explicit in the release build and reviewed before production rollout.

WebRTC DataChannel traffic is encrypted by DTLS. Screen media must use the WebRTC media security stack rather than custom plaintext transports.

## Phase 1 security policy

- Remote control defaults to disabled.
- Every session requires attended approval on the target PC.
- Approval expires after 2 minutes if not accepted.
- Maximum session duration is 30 minutes.
- Idle timeout is 5 minutes.
- Maximum simultaneous controllers is 1.
- A visible session indicator is mandatory.
- An emergency local disconnect path is mandatory.
- Every request/approval/rejection/connect/end event is audited.
- Unattended access is disabled.
- UAC/secure desktop control is disabled.
- Signaling carries no screen/media frames.
- Phase 1 allows only screen-stream capability negotiation.
- Mouse, keyboard, clipboard, file transfer and audio remain disabled.
- No `cmd.exe`, PowerShell, keylogger, hidden capture or security bypass is part of this design.

## Session lifecycle

`requested -> approved -> connecting -> active -> ended`

Alternative terminal states:

- `requested -> rejected`
- any non-terminal state -> `expired`

No terminal state can become active again. A new session must be created.

## Control-plane tables

`pc_remote_sessions`
- session identity, agent, state, approval timestamps, expiry and approved capabilities.
- controller token is stored only as a SHA-256 hash.

`pc_remote_signals`
- bounded SDP/ICE signaling records only.

`pc_remote_audit`
- immutable-style session security events for operational review.

Direct `anon` / `authenticated` access is revoked in the schema. The Edge Function accesses the tables only after custom controller/agent authentication.

## Planned next phases

### Phase 2 — live screen streaming

- Integrate pinned Pion WebRTC v4 into the Windows agent.
- Capture the already-tested interactive desktop through the v5.1.6 DIB capture path.
- Add an adaptive encoder path with bounded resolution/FPS/bitrate.
- Support ICE/STUN and credentialed TURN fallback.
- Expose session status through the existing Zero-UI/health telemetry.
- Real-Windows latency and reconnect tests.

### Phase 3 — explicit input control

Only after Phase 2 passes:

- Mouse move/click/wheel.
- Keyboard key down/up and approved shortcuts.
- Rate limits and strict event schema.
- Local emergency disconnect immediately disables input.
- Secure desktop/UAC remains outside scope unless a separate explicit design is reviewed.

### Phase 4 — optional capabilities

Clipboard, file transfer, multi-monitor, and audio are separate capability gates. None are implicitly enabled by remote desktop access.

## Production gate

Do not merge or deploy remote-control runtime merely because policy/signaling CI passes. Production enablement requires:

1. Go policy tests PASS.
2. Deno signaling type-check PASS.
3. Database migration review and advisor checks.
4. Real Windows WebRTC screen-stream test.
5. Local approval UX/indicator/emergency disconnect test.
6. TURN fallback test.
7. Session-expiry and reconnect tests.
8. Signed Windows artifact under the current Ed25519 trust anchor.
9. Existing update health/rollback gate PASS.
10. Controlled rollout followed by live audit.
