# Zhirox Full Remote Control v1

Status: **screen-only candidate development — not production enabled**

Base: `main` / Zhirox PC OS v5.1.6

## Goal

Build a TeamViewer/AnyDesk-class remote desktop path without weakening the existing signed-update, watchdog, rollback, and audit boundaries.

The architecture separates three planes:

1. **Control plane** — Supabase Edge Function creates/approves/expires sessions and relays only WebRTC signaling (offer/answer/ICE/hangup).
2. **Media plane** — WebRTC peer connection carries the H.264 screen stream directly between controller and agent, with TURN fallback when direct ICE connectivity is unavailable.
3. **Input plane** — WebRTC DataChannel may carry bounded mouse/keyboard events only after a separate reviewed release enables input injection.

Supabase must never proxy the live screen/video stream.

## Verified implementation

The branch now contains and CI verifies the complete screen-only component path:

- Pion WebRTC v4.2.23 on Go 1.24.
- H.264 WebRTC screen track with packetization-mode=1.
- Native Windows Media Foundation H.264 encoder; no external ffmpeg process.
- Reusable top-down 32-bit DIB desktop capture using `CreateDIBSection` + `BitBlt`.
- Direct BGRA capture -> Media Foundation encode -> Pion H.264 track pipeline.
- Bounded FPS (default 15, maximum 30), bitrate and keyframe cadence.
- Fail-closed geometry/stride/buffer validation.
- Real Windows H.264 encoder probe.
- Direct-ICE Pion screen-track delivery test.
- Credentialed relay-only TURN test path gated by runtime CI secrets/environment.
- Browser screen-only controller with H.264 preference.
- Agent signaling client for pending/approve/reject/active/offer/answer/ICE/hangup.
- Consent-first agent lifecycle orchestration.
- Active Windows interactive-session consent launched with `WTSQueryUserToken` + `CreateProcessAsUserW` onto `winsta0\\default`.
- Consent helper and encoder DLL require absolute paths and SHA-256 pins.
- Consent helper SHA is re-verified before every launch.
- Local approval prompt hard-times out after the 2-minute server approval window.
- Visible active-session indicator is explicitly bound to session lifetime and is terminated on controller hangup, timeout, agent shutdown or failure.
- Local indicator close/OK provides emergency disconnect.
- Server `expires_at` and local 30-minute maximum are both enforced; the earlier deadline wins.
- Pre-offer ICE is bounded, duplicate offers fail closed, unsupported signal kinds fail closed.
- Requested capabilities other than screen viewing are rejected before local consent.

## Phase 1 security policy

- Remote control defaults to disabled.
- Every session requires attended approval on the target PC.
- Approval expires after 2 minutes if not accepted.
- Maximum session duration is 30 minutes.
- Maximum simultaneous controllers is 1.
- A visible session indicator is mandatory.
- An emergency local disconnect path is mandatory.
- Every request/approval/rejection/connect/end event is auditable.
- Unattended access is disabled.
- UAC/secure desktop control is disabled.
- Signaling carries no screen/media frames.
- Current candidate allows only screen-stream capability negotiation.
- Mouse, keyboard, clipboard, file transfer and audio remain disabled.
- No `SendInput`, `cmd.exe`, PowerShell, keylogger, hidden capture or security bypass is part of the current candidate.

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

The signaling contract includes:
- controller session request and status read
- agent pending-request polling
- attended approve/reject
- agent active-state confirmation
- controller/agent offer, answer, ICE and hangup exchange

## Browser controller

`remote/controller-v1` contains the screen-only WebRTC controller.

- Controller credentials remain in memory only and are cleared on disconnect.
- It waits for approval on the target PC before creating the WebRTC offer.
- It prefers H.264 and displays the received screen track.
- It accepts ICE/TURN configuration supplied for the session.
- It does not register keyboard/mouse input handlers.
- It does not persist credentials to localStorage/sessionStorage.

## Remote agent

`remote/agent-v1` contains the Pion transport, Windows capture/encoder pipeline and consent-first orchestrator.

An isolated Windows candidate entrypoint is provided at `remote/agent-v1/cmd/zhirox-remote-screen` for controlled pre-production testing. It reads signaling/agent authentication and runtime paths from environment variables; credentials are never embedded into the binary or manifest.

The candidate bundle workflow builds:
- `ZhiroxRemoteScreenAgent.exe`
- `ZhiroxRemoteConsent.exe`
- `ZhiroxMFH264Encoder.dll`
- SHA-256/size manifest with `publishable:false` and `production_enabled:false`

## Current CI gates

Verified gates include:

1. Go policy/protocol/orchestrator tests.
2. Pion transport tests and `go vet`.
3. Browser-controller syntax/security invariants.
4. Deno signaling type-check.
5. Windows Media Foundation encoder compile and real encode probe.
6. SHA-pinned Go -> encoder DLL bridge.
7. Reusable Windows DIB capture compile/vet checks.
8. Capture -> H.264 -> WebRTC pipeline tests.
9. Direct-ICE H.264 RTP delivery test.
10. Visible consent-helper build/policy tests.
11. Interactive-session consent launcher and fail-closed lifecycle compile/tests.

## Remaining pre-production gates

The following are intentionally not claimed complete yet:

1. Build and verify the immutable screen-only candidate bundle from the exact reviewed source commit.
2. Deploy the remote signaling schema/function to an isolated/canary control plane and run Supabase security/advisor checks.
3. Run a real browser -> real target Windows screen-stream session using the candidate binaries.
4. Run the same real session with credentialed TURN forced (`relay` policy), not merely direct ICE.
5. Verify controller hangup, local emergency disconnect, approval expiry, 30-minute/server expiry and reconnect/failure behavior on the real target.
6. Produce a separately reviewed signed production candidate under the current Ed25519 trust anchor; do not replace or mutate the promoted v5.1.6 artifact.
7. Pass the existing update health/rollback gate.
8. Controlled screen-only rollout followed by live audit.

## Explicit input-control phase

Mouse and keyboard are a later, separately reviewed capability gate. The input event protocol is modeled and tested, but `pcOSRemoteInputRuntimeEnabled` remains `false`, the current agent does not execute DataChannel control messages, and no Windows input-injection API exists in this branch.

Clipboard, file transfer, multi-monitor expansion and audio are also separate capability gates and are not implicitly enabled by screen viewing.

## Production rule

Do not merge, sign for production, promote or deploy this remote-control runtime merely because component CI passes. The current candidate is screen-viewing only, attended, visible and non-publishable until the remaining real-target and release-security gates pass.
