# Zhirox PC OS v5.1.0

## Objective

Turn the existing Supervisor into an enterprise-safe PC management layer while preserving the Critical Protection Lock, Zero-UI operation, structured actions, remote reconnect behavior, and rollback discipline.

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
- PC OS diagnostic modules do not call `cmd.exe`, PowerShell, or a visible shell.
- The rollback updater launches only the fixed Supervisor executable and is itself GUI/hidden-process based.
- The binary remains built with `-H=windowsgui`.
- Background telemetry writes only Zhirox-owned status files under the active ZhiroxAI root and service-role-only Supabase control-plane rows.
- Unknown files are never auto-deleted by Disk Intelligence.
- Production database files are never treated as safe live-file-copy backup targets.
- Companion updater refresh keeps timestamped backups; it does not automatically delete the previous updater.

## Update trust chain

The v5.1.0 auto-update path rejects a newer release unless all of these pass before staging:

- SHA-256 match
- exact artifact size
- Ed25519 signature
- supported signature algorithm
- pinned Ed25519 public trust anchor compiled into the release binary

Verification is fail-closed. A feature build intentionally carries an empty trust anchor and therefore cannot accept automatic signed updates.

Canonical signed string:

`version|lowercase_sha256|size`

The main release workflow uses the GitHub Actions secret `ZHIROX_UPDATE_ED25519_SEED_B64`, which must contain a base64-encoded 32-byte Ed25519 seed. Before compilation it derives the public half and generates `buildsrc/update_trust_v510.go`; the public key is therefore compiled into `ZhiroxAISupervisor.exe`. The private seed is never written to the repository or release channel.

`channel/update-public-key.hex` is retained only as a release/audit marker. Runtime verification does **not** trust or read that mutable file. The signing step refuses the release if the signing seed does not match the public key that was prepared before compilation.

## Atomic updater and rollback

After v5.1.0 has proved cloud health with three consecutive heartbeats, it refreshes `ZhiroxUpdater.exe` from the verified running Supervisor binary. This creates a known-source companion updater for later v5.1+ updates.

The companion updater:

- runs with no visible console/UI;
- accepts only the fixed `ZhiroxAI` installation root;
- accepts only `ZhiroxAISupervisor.exe` and `ZhiroxAISupervisor.exe.new`;
- re-verifies the staged SHA-256 before replacement;
- moves the old Supervisor to a timestamped rollback backup;
- promotes the verified staged binary by rename;
- launches the new Supervisor hidden;
- requires a matching fresh `update-health.json` marker and 90 seconds of process survival;
- automatically restores and relaunches the prior Supervisor if startup/health fails;
- preserves rollback/failed binaries for audit instead of silently deleting them.

The existing three-heartbeat cleanup gate remains the later stability gate for eligible legacy Supervisor artifacts.

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

## Windows live-diagnostic integration

The read-only Windows layer reuses the hardened Supervisor's structured collectors for:

- hardware
- disks
- network adapters
- drivers
- startup items
- scheduled tasks
- Windows Update
- Defender / Firewall security state
- metrics and health score
- processes and services
- Cashier/Admin application status
- network status
- recent Windows event errors

These collectors do not invoke the generic mutating owner-action surface.

## Supabase control plane

The additive PC OS schema contains:

- `pc_os_snapshots`
- `pc_os_events`
- `pc_os_integrity_baselines`
- `pc_os_backup_catalog`
- `pc_os_update_audit`

RLS is enabled on all five tables. `anon` and `authenticated` have no DML privileges. `service_role` receives explicit `SELECT/INSERT/UPDATE/DELETE` grants for relay operation.

Production relay v14 keeps the existing v5.0.8 update channel unchanged while adding PC OS endpoints gated to agent version `>=5.1.0`. It also sanitizes PostgreSQL-bound payloads and caps oversized command results.

## Backup policy

Agent state uses versioned/checksummed snapshots and the backup catalog. The business database requires a database-native backup provider. A raw copy of a live SQL database is intentionally not implemented as a backup mechanism.

Retention baseline:

- 7 daily
- 4 weekly
- 3 monthly
- at least one immutable/offline copy

Destructive restore remains disabled. Restore is a controlled disaster-recovery operation, never a background action.

## Disk policy

Disk Intelligence performs bounded read-only scans and reports storage pressure/largest files. It never deletes unknown files automatically.

Free-space policy:

- warning: 15%
- critical: 10%

## Network policy

Network Control Center enumerates interfaces and performs bounded reachability checks. It does not rewrite adapter, DNS, IP, or routing configuration.

## Security policy

Security Sentinel combines Windows Defender/Firewall/startup/driver/event state with SHA-256 integrity information for critical Zhirox/business executables. It reports deviations and refuses mutation rather than auto-remediating protected business assets.

## Rollout gates

1. Build on `feature/pc-os-v510`.
2. Pass unit tests, relay type checks, security invariants, `go vet`, and Windows GUI compile.
3. Feature artifact must remain unsigned/non-publishable and fail-closed for automatic update.
4. Keep Supabase PC OS schema service-role-only.
5. Keep production relay backward compatible with v5.0.8.
6. Provision `ZHIROX_UPDATE_ED25519_SEED_B64` only in GitHub Actions secret storage; never commit it.
7. Merge to `main` only after all verification gates pass.
8. Main build derives and compiles the pinned public trust anchor, then signs the exact built artifact.
9. Do not promote the production update manifest until the signed artifact and rollback path are verified.
10. Initial v5.0.8 -> v5.1.0 cutover uses the existing staged updater; after v5.1 proves three heartbeats, the new self-updater replaces the legacy companion for subsequent releases.
