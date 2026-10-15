# Zhirox Remote Input v2 — Attended Control Contract

Status: development branch only. Production v5.1.7 remains screen-only.

## Scope

Phase 2 may add mouse and keyboard control only to an already approved WebRTC screen session. It must not add unattended access, clipboard access, file transfer, audio, arbitrary command execution, PowerShell/cmd execution, secure-desktop/UAC control, or hidden operation.

## Required gates

1. A fresh one-time pairing code is generated on the target PC.
2. The controller claims that code before its short expiry.
3. The pairing window closes before the separate local approval prompt appears.
4. The local user explicitly approves the session.
5. The visible active-session indicator remains on screen for the entire session and closing it disconnects immediately.
6. Input messages are accepted only after the session is active and only on the authenticated WebRTC DataChannel.
7. Input messages use bounded batches, monotonically increasing sequence numbers, normalized pointer coordinates, and a fixed event allowlist.
8. Invalid, oversized, replayed, or out-of-order control traffic fails closed.
9. Input capability is removed immediately on disconnect, expiry, WebRTC failure, local indicator close, or process shutdown.
10. The maximum session duration remains 30 minutes.

## Protocol scaffold

`remote/agent-v1/control_protocol_v2.go` defines protocol version 2 and validates pointer move, pointer button, pointer wheel, and key events. The current scaffold does not execute OS input.

## Release discipline

The input execution layer must be a separately reviewed, CI-tested and signed candidate. It must not modify or overwrite the existing v5.1.7 signed artifact. A new immutable version is required, with the same Ed25519 trust anchor, SHA-256 and size pinning, staging, rollback, and post-update health gates.
