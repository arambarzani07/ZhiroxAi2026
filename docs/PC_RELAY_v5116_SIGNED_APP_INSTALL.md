# Zhirox PC Relay v5.1.16 — fixed signed-app installation

## Status

**Source/candidate only; NOT signed, NOT installed on the store PC, NOT production-enabled.**
The already deployed PC Relay v5.1.15 is not changed by this PR. No Supabase
Edge Function or live command is deployed; no installation is triggered.

## Target environment (read-only evidence)

- Target agent: `Server-c24b5c0f`, hostname `Server`, Windows 10 Pro.
- Reported deployed Relay version: `5.1.15`.
- Agent service reported as `LocalSystem` in previous authenticated diagnostics.
- Protected legacy production executables `D:\DataBase\Admin.exe` and
  `D:\DataBase\Cashier.exe` must remain unaffected.
- Selected signed app: **Cashierynwe v1.0.10** with release asset
  `Cashierynwe-Setup-1.0.10-x64.exe`.
- GitHub release asset fixed SHA-256:
  `e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b`,
  size **158128464 bytes**.

## Capability added to the reviewed source

1. `signed_app_install_status`: read-only sanitized progress.
2. `signed_app_install`: strictly typed allowlisted, fresh-install request.
3. Exact command args ONLY: `{ "app_id": "cashierynwe", "version": "1.0.10", "approval_id": "<canonical UUID>" }`.
4. Caller cannot supply a binary, URL, path, process name, script, shell,
   PowerShell command, signer, signing certificate, arbitrary flags or an
   unapproved version.
5. Before delivery, a separately authenticated owner approval must be
   consumed atomically by the existing `enqueue_approved_pc_relay_command` RPC
   for the exact agent ID, action and args. The UUID does not authenticate
   or approve anything on its own.
6. The agent **independently** verifies the compiled size/SHA256 pin of a
   local staged installer, then checks Windows Authenticode Valid status,
   the exact `CN=Cashierynwe Single-PC Updates` publisher subject, and
   code-signing EKU. No certificate trust-store changes or bypasses.
7. Copy to an agent-controlled temporary file and rehash that same copy;
   execute only the verified copy with the fixed NSIS `/S` argument.
8. A completed installer process is reported as
   `installer_exited_zero_needs_app_verification`, **not** as a confirmed
   successful installation, until Windows and app-level acceptance tests pass.
9. No anonymous token, generic remote process execution, shell or remote
   interactive desktop control has been added.
10. All requests fail closed if the service is Session 0 / SYSTEM, if the
    app is running or already installed, if the artifact is missing, if
    Windows Authenticode is untrusted, or if the approval is absent.

## Important deployment blockers

### 1. Per-user Windows installer from a SYSTEM service

The present Relay runs as `NT AUTHORITY\SYSTEM` in Windows Session 0.
Cashierynwe's current NSIS EXE is **per-user** (`perMachine: false`).
Silently launching it from the SYSTEM service would install into the
wrong user profile and could produce a false success. The v5.1.16 code
therefore **intentionally refuses** that operation with
`interactive_user_broker_required`.

A separately reviewed, least-privilege, locally authenticated
**attended user-session broker** must be built and Windows-canary-tested
before the store PC can perform remote first-time installation.

### 2. Installer transport

The Cashierynwe GitHub repository is private and no usable scoped
GitHub artifact downloader is provisioned for the Relay service.
No token may be copied from ChatGPT, GitHub CLI credentials or user
profiles into the service. The exact installer must be staged in
`C:\ProgramData\ZhiroxAI\ApprovedInstallers` via a separately
authenticated and verified transport. The Relay cannot install an
unstaged asset.

### 3. Existing Cashierynwe installs

The new typed action is restricted to **fresh installs**. An installed
Cashierynwe instance must use its own authenticated
`electron-updater` update path (with verified backup and safe checkout
gate); Relay must not overwrite a live per-user POS or its SQLite DB.

### 4. Production release of v5.1.16

The Windows GitHub Actions build emits an **unsigned candidate only**;
`publishable=false` and `signed_update_published=false`.
Promoting to the PC requires:

- Windows candidate source/build/test provenance verification.
- A *fresh* Ed25519 signature for **the exact** `5.1.16|sha256|size`
  using the already pinned production key; no generated or reused keys.
- Signature and SHA readback, a healthy canary and rollback to v5.1.15.
- Deployment of the separately reviewed v22 connector to the intended
  Supabase project, after owner-authenticated approval.
- Physical Windows tests confirming session identity, installer trust,
  correct Windows user, app data integrity and actual app launch.

## Source files

- `src/pc_os_signed_app_install_policy.go`: immutable compiled catalog and
  exact approval-shaped args.
- `src/pc_os_signed_app_install_policy_test.go`: rejection matrix.
- `src/pc_os_signed_app_install_windows.go`: fail-closed preflight,
  Authenticode check, and bounded per-user NSIS execution.
- `tools/patch_v5116_installer.py`: exact source route injection and
  version bump without losing the historical signed baseline.
- `ops/supabase/zhirox-pc-relay-v22-signed-installer.ts`: **NOT DEPLOYED**
  future backend strict typed action and approval RPC gate.
- `.github/workflows/build-v5116-signed-app-installer.yml`: Windows
  candidate build and quality gates; no production publish step.

Do not claim the installed PC Relay has this ability until these
hardware and signing blockers are actually resolved.
