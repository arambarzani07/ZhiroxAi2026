# Zhirox Full Remote Control v1

Status: **Phase 1/2 development — not production enabled**

Base: `main` / Zhirox PC OS v5.1.6

## Goal

Build a TeamViewer/AnyDesk-class remote desktop path without weakening the existing signed-update, watchdog, rollback, and audit boundaries.

The architecture separates three planes:

1. **Control plane** — Supabase Edge Function creates/approves/expires sessions and relays only WebRTC signaling (offer/answer/ICE/hangup).
2. **Media plane** — WebRTC peer connection carries the screen stream directly between controller and agent, with TURN fallback when direct ICE connectivity is unavailable.
3. **Input plane** — WebRTC DataChannel will carry bounded mouse/keyboard events only after a separate reviewed release enables input injection.

Supabase must never proxy the live screen/video stream.

## WebRTC implementation target

The remote-agent module pins `github.com/pion/webrtc/v4 v4.2.23` and uses Go 1.24 because this Pion release requires Go 1.24 or newer.

The screen track is H.264 with an explicit WebRTC profile/packetization contract. The Windows production encoder target is the native Microsoft Media Foundation H.264 encoder (`Mfh264enc.dll`) rather than an external `ffmpeg` process. The encoder accepts formats including NV12/I420 and produces H.264. The existing v5.1.6 DIB desktop capture will feed a bounded color-conversion/Media Foundation encode path.

The browser controller prefers H.264 during codec negotiation. WebRTC transports screen media over its encrypted media stack, while the control DataChannel remains non-operative for input in Phase 1/2.

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
- Phase 1/2 allows only screen-stream capability negotiation.
- Mouse, keyboard, clipboard, file transfer and audio remain disabled.
- No `cmd.exe`, PowerShell, keylogger, hidden capture or security bypass is part of this design.

## Session lifecycle

`requested -> approved -> connecting -> active -> ended`

Alternative terminal states:

- `requested -> rejected`
- any non-terminal state -> `expired`

No terminal state can become active again. A new session must be created.

## Control plane

`pc_remote_sessions`
- session identity, agent, state, approval timestamps, expiry and approved capabilities.
- controller token is stored only as a SHA-256 hash.

`pc_remote_signals`
- bounded SDP/ICE signaling records only.

`pc_remote_audit`
- session security events for operational review.

Direct `anon` / `authenticated` access is revoked in the schema. The Edge Function accesses the tables only after custom controller/agent authentication.

The signaling contract now includes:
- controller session request and status read
- agent pending-request polling
- attended approve/reject
- agent active-state confirmation
- controller/agent offer, answer, ICE and hangup exchange

## Browser controller

`remote/controller-v1` contains the screen-only WebRTC controller prototype.

- Controller credentials remain in memory only and are cleared on disconnect.
- It waits for approval on the target PC before creating the WebRTC offer.
- It prefers H.264 and displays the received screen track.
- It accepts ICE/TURN configuration supplied for the session.
- It does not register keyboard/mouse input handlers.
- It does not persist credentials to localStorage/sessionStorage.

## Remote agent transport

`remote/agent-v1` contains the Pion peer-connection transport.

- H.264 screen track
- offer/answer and ICE support
- TURN-capable ICE configuration
- RTCP draining
- screen access-unit writer
- DataChannel present but incoming control messages intentionally ignored

## Phase 2 remaining work — live screen encoding

- Connect the already-tested v5.1.6 interactive DIB capture to a bounded frame pipeline.
- Convert captured BGRA/RGBA frames to an input format accepted by the Media Foundation H.264 encoder, preferably NV12.
- Configure low-latency H.264 output, bounded resolution/FPS/bitrate, frequent IDR frames and packetization compatible with the Pion H.264 track.
- Add a visible local approval/session indicator and emergency disconnect implementation.
- Bind the agent to pending-session polling and signaling endpoints.
- Run real-Windows H.264 screen-stream, reconnect and TURN tests.

## Phase 3 — explicit input control

Only after Phase 2 passes:

- Mouse move/click/wheel.
- Keyboard key down/up and approved shortcuts.
- Rate limits, anti-replay sequencing and strict event schema.
- Local emergency disconnect immediately disables input.
- Secure desktop/UAC remains outside scope unless a separate explicit design is reviewed.

The input event protocol is already modeled and tested, but `pcOSRemoteInputRuntimeEnabled` remains `false` and no Windows input-injection API exists in this branch.

## Phase 4 — optional capabilities

Clipboard, file transfer, multi-monitor, and audio are separate capability gates. None are implicitly enabled by remote desktop access.

## Production gate

Do not merge or deploy remote-control runtime merely because foundation CI passes. Production enablement requires:

1. Go policy/protocol tests PASS.
2. Pion transport tests/vet PASS.
3. Browser-controller syntax/security invariants PASS.
4. Deno signaling type-check PASS.
5. Database migration review and Supabase advisor checks.
6. Real Windows H.264 screen-stream test.
7. Local approval UX/indicator/emergency disconnect test.
8. TURN fallback test.
9. Session-expiry and reconnect tests.
10. Signed Windows artifact under the current Ed25519 trust anchor.
11. Existing update health/rollback gate PASS.
12. Controlled rollout followed by live audit.
