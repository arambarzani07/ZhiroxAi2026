# Zhirox PC OS v5.1.0

## Objective

Turn the existing Supervisor into an enterprise-safe PC management layer while preserving the Critical Protection Lock, Zero-UI operation, structured actions, remote reconnect behavior, explicit approval for mutations, signed-update trust, and rollback discipline.

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
- New remote `pc_os_*` diagnostic commands remain read-only.
- Generic structured mutations are default-deny and require a one-time exact-args approval record.
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

### One-time v5.0.8 -> v5.1.0 bootstrap boundary

The currently deployed v5.0.8 updater predates Ed25519 verification. Therefore the first transition into v5.1.0 cannot truthfully claim agent-side Ed25519 verification before the new binary is running.

That single bootstrap transition is allowed only under all of these gates:

- manual production manifest promotion;
- authenticated existing relay path;
- existing trusted Supabase relay host restriction;
- exact release SHA-256 and exact artifact size recorded before promotion;
- feature/unsigned artifacts remain non-publishable;
- signed main artifact and rollback behavior verified before promotion;
- production remains pinned to known-good v5.0.8 until explicit cutover approval;
- after v5.1.0 is running, all later automatic updates are Ed25519 fail-closed.

This bootstrap boundary is a migration constraint, not a permanent weakening of the v5.1+ trust model.

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

Read-only PC OS commands:

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

The relay also exposes existing bounded structured read actions such as metrics, processes, services, disks, drivers, startup items, Windows Update, security status, app status and file hash. Text reading of protected binaries/database files is explicitly refused where it is not an appropriate inspection method.

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

Production currently runs relay-v15 logic as Supabase Edge Function deployment version 16. It keeps the production Supervisor update channel fixed at the known-good `5.0.8` while adding PC OS endpoints gated to Agent version `>=5.1.0`. It also sanitizes PostgreSQL-bound payloads and caps oversized command results.

Live verification after deployment confirmed the existing Agent remained on `5.0.8`, health remained `100`, alerts remained empty, and heartbeats continued normally. Deploying the relay therefore did not promote or install v5.1.0 on the PC.

## Structured mutation approval gate

Read-only relay actions remain directly queueable through the authenticated connector path. Structured mutation actions are separately gated and cannot be queued from the relay without an approval record.

A mutation approval is bound to:

- one exact Agent ID;
- one exact action;
- one exact JSON argument object;
- one use only;
- a maximum validity window of 15 minutes.

The database RPC locks the approval row with `FOR UPDATE`, rejects missing/expired/used/mismatched approvals, atomically queues the exact command, and immediately marks the approval as used with the resulting command ID.

`public.enqueue_approved_pc_relay_command(uuid,text,text,jsonb)` runs as `SECURITY INVOKER`, not `SECURITY DEFINER`. The relay service role already has the exact table privileges needed, so no privilege elevation is required. `anon` and `authenticated` cannot execute the RPC and cannot perform DML on the approval table.

The approval TTL is enforced by the validated database constraint `pc_relay_command_approvals_ttl_check`, which requires `expires_at <= approved_at + 15 minutes`.

Current production verification shows zero active approvals and zero queued mutation commands.

## Native SQL backup policy

A raw copy of live `.mdf/.ndf/.ldf` files is never used as the business backup mechanism.

The Windows Smart Backup Vault provides three distinct SQL Server operations:

- **Readiness** — read-only discovery of `sqlcmd.exe`, connectivity and user databases.
- **Verify** — `RESTORE VERIFYONLY FROM DISK ... WITH CHECKSUM` plus SHA-256 of an existing `.bak` outside `D:\DataBase`.
- **Execute** — approval-bound `BACKUP DATABASE ... WITH COPY_ONLY, CHECKSUM, COMPRESSION` followed immediately by verification and SHA-256.

Execution rules:

- local confirmation must exactly equal `BACKUP <database>`;
- remote execution also requires the one-time exact-args relay approval;
- backup destination cannot be inside `D:\DataBase`;
- destination must be an absolute `.bak` path;
- existing backup files are never overwritten;
- the default local backup root is `D:\ZhiroxBackups`;
- the SQL backup command is launched directly with Zero-UI/background process flags, not through a shell;
- restore is not implemented as an automatic background action.

Retention baseline:

- 7 daily
- 4 weekly
- 3 monthly
- at least one immutable/offline copy

A local hash-verified `.bak` is **not** equivalent to WORM/immutable storage. True immutable backup requires a separate immutable/offline target.

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
2. Pass unit tests, relay type checks, mutation-approval assertions, security invariants, `go vet`, and Windows GUI compile.
3. Feature artifact must remain unsigned/non-publishable and fail-closed for automatic update.
4. Keep Supabase PC OS schema service-role-only.
5. Keep production relay backward compatible with v5.0.8 and the production update channel pinned to v5.0.8 until signed promotion.
6. Keep structured mutation execution behind one-time, exact-agent/action/args approvals with a maximum 15-minute TTL.
7. Provision `ZHIROX_UPDATE_ED25519_SEED_B64` only in GitHub Actions secret storage; never commit it or send it through the relay/database.
8. Merge to `main` only after all verification gates pass and the release signing secret is securely available.
9. Main build derives and compiles the pinned public trust anchor, then signs the exact built artifact.
10. Verify exact release SHA-256, size, signature, updater behavior, health marker and rollback artifact before production manifest promotion.
11. Treat v5.0.8 -> v5.1.0 as the single controlled bootstrap boundary described above.
12. After v5.1.0 proves stable, all later automatic updates remain Ed25519 fail-closed and use the embedded companion rollback updater.
