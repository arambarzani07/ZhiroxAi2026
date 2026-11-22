# Windows attended installer broker — source-only candidate

**Status: compiled candidate only. Not code-signed, installed, authorized to
run, or production-enabled.**

## Why this separate process exists

- The store's `ZhiroxAISupervisor.exe` runs as `NT AUTHORITY\\SYSTEM`
  in Windows Session 0.
- Cashierynwe's NSIS setup uses `oneClick: true` and
  `perMachine: false`. Launching it from Session 0 would target the
  wrong Windows user's profile.
- Microsoft's guidance for interactive Windows services requires a separate
  GUI process in the authenticated user's session.
- Remote code-execution modes, arbitrary scripts or unsandboxed PowerShell
  would violate the Relay's existing deny-by-default design.

## New isolated Go component

The proposed `installer-broker/` binary is user-session-only; it has no
agent cloud secret and no remote command receiver. It only accepts a
fixed, short-lived JSON ticket and the pinned Cashierynwe v1.0.10 installer.

Required ticket fields:

- `schema = zhirox.signed-install-ticket.v1`
- `agent_id = Server-c24b5c0f`
- `app_id = cashierynwe`
- `version = 1.0.10`
- `filename = Cashierynwe-Setup-1.0.10-x64.exe`
- `sha256 = e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b`
- `size = 158128464`
- Canonical request and approval UUID, issuance and expiry no more than
  15 minutes apart, and a consumed approval marker.

Before any prospective install it validates:
1. Interactive Windows console session (not SYSTEM/Session 0).
2. Exact ticket fields, size, freshness and unknown-field rejection.
3. Existing POS install/running cashier must be absent.
4. Explicit visible Yes/No consent, with No selected by default.
5. One-time replay guard in the user profile.
6. Exact binary bytes SHA-256, size and trusted Authenticode publisher in a
   private temporary copy, using a fixed noninteractive verification script.
7. Only the pinned EXE is invoked with fixed `/S`; no downloaded commands,
   custom flags, arbitrary URL, arbitrary file or service control.
8. A zero installer exit code is only `installer_exited_zero_needs_app_verification`.
   Full app launch and data check must be separately observed.

## Why the new helper deliberately cannot install yet

The constant `brokerProductionEnabled = false` is mandatory in unsigned
GitHub Actions outputs. DO NOT switch this without an independent review and:

- A SYSTEM-owned, read-only-to-Users NTFS ACL for InstallRequests and
  ApprovedInstallers folders, verified against symlink/junction attacks.
- A service-side emitter that writes the exact request ticket only after
  `enqueue_approved_pc_relay_command` consumes authenticated owner approval.
- A signed and authenticated installer transport, verified on the actual
  market Windows desktop.
- A correctly signed, canary-tested broker bootstrap in the intended
  interactive user account. The unsigned candidate **must not be
  installed** on the market PC.
- A receipt/status handshake from broker to service with server-independent
  confirmation of the POS installation, safe backup and rollback.
- A separate owner-approved promotion of the signed Relay, no forced
  replacement of `D:\\DataBase\\Admin.exe` or `D:\\DataBase\\Cashier.exe`.

## No production mutations

This PR does not deploy a Supabase function, issue a remote command, install
software, modify Windows Registry, create scheduled tasks, change service
rights, touch SQL Server or interrupt the legacy cashier.
