# Zhirox PC OS v5.1.0

## Objective

Turn the existing Supervisor into an enterprise-safe PC management layer while preserving the existing Critical Protection Lock, Zero-UI operation, structured actions, remote reconnect behavior, and update rollback discipline.

## Modules

1. Guardian
2. Smart Backup Vault
3. Business Continuity
4. Signed Update Manager
5. Disk Intelligence
6. Network Control Center
7. Security Sentinel
8. Remote File Manager (read-only surface)
9. Inventory & Asset Manager
10. Local AI Operator (deterministic safety rules)
11. Zero-UI Automation Engine
12. Disaster Recovery

## Safety invariants

- `D:\DataBase` remains protected.
- `Admin.exe` and `Cashier.exe` are not modified.
- Critical SQL/business services cannot be stopped/restarted through generic actions; start-only recovery behavior from v5.0.7 is preserved.
- Hikvision/DVR is not modified by PC OS modules.
- No arbitrary shell command is introduced.
- New remote `pc_os_*` commands are read-only.
- The PC OS module does not call `cmd.exe`, PowerShell, or any visible shell.
- The binary remains built with `-H=windowsgui`.
- Background PC OS telemetry writes only `pc-os-health.json` atomically under the active ZhiroxAI root.
- Unknown files are never auto-deleted by Disk Intelligence.
- Production database files are never treated as safe live-file-copy backup targets.

## Update trust chain

The v5.1.0 verifier requires:

- SHA-256 match
- exact artifact size
- Ed25519 signature
- a locally pinned `update-public-key.hex`

Verification is fail-closed: a missing public key, signature, size mismatch, hash mismatch, or invalid signature rejects the update.

Canonical signed string:

`version|lowercase_sha256|size`

The release workflow uses the GitHub Actions secret `ZHIROX_UPDATE_ED25519_SEED_B64`, which must contain a base64-encoded 32-byte Ed25519 seed. The seed is never written to the repository or release channel.

## New structured commands

- `pc_os_overview`
- `pc_os_modules`
- `pc_os_guardian`
- `pc_os_backup`
- `pc_os_continuity`
- `pc_os_update_chain`
- `pc_os_verify_update`
- `pc_os_disk`
- `pc_os_network`
- `pc_os_security`
- `pc_os_files`
- `pc_os_inventory`
- `pc_os_ai`
- `pc_os_zero_ui`
- `pc_os_disaster_recovery`

`pc_os_verify_update` accepts `manifest_path` and `artifact_path`. `pc_os_files` accepts `path`. Both are restricted to managed roots.

## Backup policy

Agent state can use versioned/checksummed snapshots. The business database requires a database-native backup provider. A raw copy of a live SQL database is intentionally not implemented as a backup mechanism.

Recommended retention baseline:

- 7 daily
- 4 weekly
- 3 monthly
- at least one immutable/offline copy

Restore remains non-automatic and destructive restore is disabled by default.

## Disk policy

Disk Intelligence performs bounded read-only scans (maximum 2,000 entries per target, depth 2) and reports the largest files. It never deletes unknown files automatically.

Recommended free-space policy:

- warning: 15%
- critical: 10%

## Network policy

Network Control Center enumerates interfaces and performs a bounded TCP reachability probe to the Supabase relay endpoint. It does not rewrite adapter, DNS, IP, or routing configuration.

## Security policy

Security Sentinel computes SHA-256 integrity information for:

- active `ZhiroxAISupervisor.exe`
- `D:\DataBase\Admin.exe`
- `D:\DataBase\Cashier.exe`

It reports and refuses mutation rather than auto-remediating protected assets.

## Rollout

1. Build on `feature/pc-os-v510`.
2. Pass security invariants and Windows compile check.
3. Review the unsigned feature artifact only; do not deploy it as a release.
4. Provision an Ed25519 signing seed as GitHub Actions secret.
5. Pin the matching public key on the PC under the protected ZhiroxAI installation.
6. Merge to `main` only after verification.
7. Main build signs metadata and publishes the release channel.
8. Deploy through the existing staged updater.
9. Require post-update health gate and rollback on failure.
10. Only after stable health should legacy updater artifacts be cleaned.
