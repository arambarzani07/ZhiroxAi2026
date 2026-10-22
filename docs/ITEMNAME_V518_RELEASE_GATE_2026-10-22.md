# ItemName Watch v5.1.8 — Release Gate (2026-10-22)

## Scope and invariants
- Legacy Admin.exe, Cashier.exe and SuperMarket SQL Server remain untouched.
- No database UPDATE/INSERT/DELETE/ALTER, SQL RESTORE or SQL service restart on the market PC.
- New-item and price-change monitoring only; ignore name/barcode/stock edits and removals.
- Match records by ItemNameID; preserve distinct products even with identical names or barcodes.
- No production cutover, release promotion or automatic PC update from this branch.

## Evidence already verified
- The connected relay reported one online agent on 2026-10-22 around 00:29 Baghdad time, version 5.1.7, health score 100. A healthy heartbeat is not proof of SQL access.
- Supabase release metadata records v5.1.8 as `staged`, `publishable=false`; source commit `af8394e8583c8cca72ee389d8f5af1a2446d63d8`.
- Supabase private storage records a 19,963,904-byte executable (the executable itself was not independently redownloaded and rehashed in this pass).
- Release metadata Ed25519 signature verified against the recorded public key and canonical `version|sha256|size` string; this verifies the signed manifest, not the independently retrieved executable bytes.
- GitHub Actions run `39675366994` for v5.1.8 succeeded on Windows and uploaded artifact `12381372159`.
- The monitor source hardcodes database `SuperMarket`, checks `dbo.ItemName`, and uses SQL SELECT queries.
- The isolated decimal-format fix on branch `fix/v518-itemname-decimal-price-20261022` preserves ordinary exact base-10 numbers instead of rational strings such as `401/4`. Seven policy tests passed in a local Go harness; this is not a full Windows executable integration test.

## Outstanding blockers and cautions
1. **SQL connectivity unverified:** A v5.1.7 read-only readiness probe failed. On `localhost` the returned ODBC Driver 18 message included `SSL Provider: The target principal name is incorrect`. On host `Server`, it returned SQLCMD exit status 1. These are distinct observations; the exact root cause remains unconfirmed.
2. **TLS bypass risk:** v5.1.8 ItemName code uses SQLCMD `-C` (trust server certificate without validation). Do not treat this as proper TLS certificate validation. Prefer a verified matching certificate/hostname when feasible. No SQL Server or OS configuration changes are approved.
3. **Live ItemName data is unverified:** No successful read-only query of `SuperMarket.dbo.ItemName`, baseline, or real change notification has been demonstrated.
4. **The monitoring baseline creates an agent-owned local JSON file:** It does not change the supermarket SQL database, but it is a local filesystem write. If the required policy prohibits *all* writes even within the agent's own state directory, a different baseline architecture is necessary before deployment.
5. **Backup vault is a separate concern:** `D:\ZhiroxBackups` was not available in the previous backup readiness probe. This blocks that backup workflow but does not by itself establish an ItemName SQL watcher failure.

## Safe next sequence
- Inspect only existing relay observations and GitHub source while the PC remains on v5.1.7.
- Design a read-only, least-privileged SQL probe for `SuperMarket.dbo.ItemName` with proper TLS certificate validation, and test it in an isolated Windows environment first.
- Confirm the product schema and privileges using only SELECT on the authorized database; do not guess columns.
- Confirm independent executable SHA-256 readback and Ed25519 verification before any promotion.
- Test full backend event acceptance/idempotency on test infrastructure and show no false alerts for metadata-only changes.
- Decide explicitly whether agent-owned baseline files are permitted. Do not deploy while this policy remains unresolved.
- Keep release at `staged` and `publishable=false` until all gates pass and the owner authorizes release.

## Status
**NOT approved for production deployment.** No write command or installation was sent to the market PC during this review.

## 2026-10-22 isolated TLS hardening progress

- Implemented `src/pc_os_itemname_sqlargs_v518.go` with a fixed `SuperMarket` target, local SQL Server endpoint (`localhost`), integrated Windows auth (`-E`), encrypted transport request (`-N`) and certificate-name validation (`-F`). No `-C` trust override, no remote SQL target, no SQL username/password or fallback disabling certificate validation.
- Windows monitor now obtains the local OS hostname and passes it as the expected certificate hostname. **This is an explicit initial assumption, not a discovered certificate name**; connection must fail closed if the actual certificate does not match or is not trusted. Confirm the certificate CN/SAN, issuer/trust and SQL Server instance independently before enabling monitoring.
- 10/10 isolated Go unit tests passed under Linux; policy and SQL argument tests also successfully cross-compiled into a Windows x64 test executable. **Neither result verifies an actual SQL connection or the complete Windows agent integration.**
- Added a branch-only GitHub Actions workflow `.github/workflows/itemname-v518-isolated-tests.yml` to run policy and TLS tests on Linux and Windows. **Remote CI conclusion must be checked separately; success is not yet claimed.**
- Independently verified the Ed25519 signature against the recorded public key and the canonical string `5.1.8|469b8758409199fa6b87aad77b78534ce4e127968e4b36a409278bd4acb7d46e|19963904`. No independently fetched binary hash verification is claimed.
- Relay latest observed active agent is still v5.1.7; production v5.1.8 remains staged and not publishable. No ItemName event or ItemName snapshot was present in relay at this verification.
- **Still blocked:** Production certificate and database SELECT connectivity validation; live schema verification; event ingestion and idempotency validation; explicit policy decision for agent-owned local JSON baseline; signed Windows build and approval before cutover.

**Do not promote, merge into a deployed release, or install on the market PC until all release gates are satisfied.**

## 2026-10-22 09:32+03 — New live-read findings and fail-closed SQL boundary

### Confirmed from read-only PC-side inspection

- Relay agent `Server-c24b5c0f` was reporting a fresh v5.1.7 heartbeat (20 seconds old at 09:32 Baghdad), health score 100.
- Windows machine Personal certificate store and Trusted Root certificate store both contained the same legacy SQL Server certificate under SHA-1 thumbprint `35F0CEE5D8F4520E680BAE46E01B32EEB033EA09`.
- The DER certificate was extracted from both registry Blobs (732 bytes each) and compared **byte for byte**, yielding **exact equality**. Registry Blob wrappers themselves differed in size/metadata; the certificate DER did not.
- A prior read-only SQL ERRORLOG showed that SQL Server 2008 R2 instance `MSSQL10_50.MSSQLSERVER` loaded the same thumbprint, started `SuperMarket`, and reported readiness on local SQL TCP ports. These facts do not demonstrate a successful `SELECT`.
- The certificate is self-signed, lacks SAN, and has older SHA-1 signature. Windows Root-store presence makes local trust possible but **DOES NOT establish ODBC authentication success**; validate the exact server/certificate name and trust through a successful real connection without using `-C`.

### New SQL command hardening

- `pcOSItemNameSQLCmdArgs` now rejects arbitrary SQL, including even benign non-allowlisted `SELECT 1`, and only permits three exact built-in statements:
  1. Fixed `OBJECT_ID(N'dbo.ItemName',N'U')` schema check;
  2. Fixed `SELECT TOP(1) ItemNameID ... FROM dbo.ItemName` probe;
  3. Fixed read-only `ItemName` product snapshot query.
- The Windows watcher now uses the shared allowlisted constants rather than independent inline SQL strings.
- Local **Go tests 4/4 passed** in the Linux Go 1.23.2 test harness (TLS flags, database whitelist, certificate hostname validation, SQL allowlist). The previous 7 policy tests and 14 JS guard tests are separate tests; this is not a claim of a full Windows agent build or live database connectivity.

### Remaining mandatory gates

1. Confirm an ODBC `sqlcmd` connection using `-N -F Server` with no `-C`, the authorized integrated Windows identity, and the correct instance. If certificate or protocol negotiation fails, report the blocker; **do not change the live SQL Server configuration** under the read-only lock.
2. Demonstrate a real `SuperMarket.dbo.ItemName` SELECT with minimum impact and confirm expected columns. Do not begin continuous full-table polling until load impact and local-state policy are approved.
3. Integrate and test event uniqueness through a staging relay, then verify owner-facing notification delivery separately. An inserted row in `pc_os_events` is not delivery to the owner.
4. Resolve baseline state: the current watcher writes an agent-owned JSON file, which must be prohibited if the PC zero-write policy covers all local paths.
5. Only after the above gates and owner authorization: signed build, hash verification, staging canary and release promotion. Until then, v5.1.8 must remain `staged`/`publishable=false`.

**No protected PC application, SQL schema, SQL settings, or production Supabase function was modified.**

## 2026-10-22 — requested live SELECT + real notification delivery checks

**Current result: NOT COMPLETED; fail-closed.**

1. The production PC agent was online (v5.1.7; health 100) when checked. A fresh queued **read-only** `pc_os_backup_readiness` command targeting `Server` was delivered and completed, but its `database_probe_ok=false` and `sqlcmd failed: exit status 1`. This action queries `sys.databases`, NOT `dbo.ItemName`. It is **not evidence** of a successful ItemName SELECT. The live agent version does not yet include the unpromoted v5.1.8 `pc_os_itemname_probe` handler.
2. The protected legacy SQL ERRORLOG was read again, confirming the expected SQL Server 2008 R2 certificate loaded at startup; no reliable SQLCMD failure reason was captured from its existing v5.1.7 error reporting. No SQL setting change or certificate bypass was permitted.
3. Added isolated `pcOSItemNameSQLCmdDiagnostic` to the feature branch. It reports only SQLCMD error-prefixed lines, caps output and hides item names, barcodes and arbitrary row data. Four isolated tests passed locally, but the candidate is **not installed on the PC**.
4. The production relay currently has an authenticated `POST /api/agent/:id/pc-os/event` ingestion handler, but there is **no owner delivery channel configured** in this project and no completed end-to-end test. Production `pc_os_events` contained 0 rows at the initial verification. HTTP delivery cannot be asserted from database reads.
5. An isolated synthetic test-agent record was temporarily created in Supabase for a relay ingress verification attempt, but the outbound test request was blocked by the available execution policy; **no HTTP 202 or event arrival was observed**. The exact test-only agent was then deleted. No live agent identity or customer records were altered.
6. The user-facing fallback `Zhirox-ItemName-ReadOnly-Diagnostic.ps1` was prepared separately as a local diagnostic asset (not installed/deployed). It executes fixed SQLCMD SELECT-only checks using Windows integrated authentication and TLS certificate-name verification without `-C`. Execution and output on the authorized PC are still needed for proof.

**Do not mark live ItemName SELECT or owner notification delivery as complete.** The remaining gates are: validated live SQL connection and SELECT, staging event ingress + deduplication, real destination configuration, and a recipient-confirmed notification test. Leave v5.1.8 STAGED and the protected supermarket systems unchanged.

## 2026-10-22 10:30+03 — Live driver inventory and verified protocol blocker

Read-only Windows Registry `registry_get` responses from agent `Server-c24b5c0f` positively confirmed these installed client providers:
- `ODBC Driver 17 for SQL Server` = Installed
- `ODBC Driver 18 for SQL Server` = Installed
- `SQL Server Native Client 10.0` = Installed

A fresh heartbeat at 2026-10-22 07:30:53 UTC confirmed the live agent is **5.1.7**, not 5.1.8. `itemname_delta` database events remain zero. Existing 5.1.7 only exposes the backup readiness probe, which reported `sqlcmd exit status 1` and does **not** expose 5.1.8 `pc_os_itemname_probe` or an arbitrary SQL action. Therefore no live SQL `SELECT` should be claimed.

**Microsoft-confirmed SQL build compatibility:** the observed SQL Server 2008 R2 (RTM) build `10.50.1600.1` predates Microsoft's first documented SQL Server 2008 R2 SP3 TLS 1.2-supporting x64 build `10.50.6542.0`. TLS 1.2 is not assured for this old engine; the ODBC 18 secure connection may fail due to both certificate identity validation and protocol negotiation. See:
- https://learn.microsoft.com/en-us/troubleshoot/sql/database-engine/connect/tls-1-2-support-microsoft-sql-server
- https://learn.microsoft.com/en-us/sql/connect/odbc/connection-troubleshooting
- https://learn.microsoft.com/en-us/sql/tools/sqlcmd/sqlcmd-connect-database-engine

A documented alternative protocol is `sqlcmd -S lpc:Server` (local shared memory, no network), but this is **not** an approved automatic downgrade and has **not** been tested. It may depend on connection encryption policies and local permissions. Do not add a silent plaintext or certificate-trust fallback.

The connected ChatGPT hourly condition-watch was activated to report genuine `itemname_delta` database events if and when they exist. This is a *polling notification route*, not real-time delivery proof and cannot replace testing Agent→Relay→DB or an owner-specific Telegram/email push provider.

### Blocks remaining
1. Run exactly authorized read-only `sqlcmd` probes from the market PC under the correct integrated Windows identity and certificate/trust/protocol constraints, or evaluate local-only shared-memory separately with approval. A failure is **not** a successful `dbo.ItemName` read.
2. Verify agent's actual `SuperMarket.dbo.ItemName` schema, count and first row using only `SELECT`.
3. Deploy and test relay uniqueness/idempotency only after staging; then verify actual owner-visible notifications end-to-end. Supabase `pc_os_events` currently empty.
4. Keep v5.1.8 staged, release gate closed and the original SQL Server/Admin/Cashier read-only.

### 07:37Z protocol-prefix policy probe (read-only)

Agent 5.1.7 accepted the transport readiness action `pc_os_backup_readiness` with `server="lpc:Server"` but correctly **rejected the target** at its server allowlist validation: `server_policy_ok=false`, `server_policy_error="SQL backup target must be a local host or local named instance"`. **No SQL query ran**. Do not loosen the production allowlist as an undocumented workaround.

Registry checks also verified ODBC Driver 17, ODBC Driver 18 and SQL Server Native Client 10.0 all report Installed. The first verified live `ItemName` SELECT and mobile push delivery remain unproven. An hourly ChatGPT condition-watch for non-test ItemName events has been set up; it cannot produce events that the still-running 5.1.7 agent does not emit.

An independent Windows-local PowerShell diagnostic can attempt `tcp:Server,1433` then `lpc:Server`, both using mandatory encryption and real certificate validation, and explicitly stops without a plaintext fallback. It is a **manual diagnostic only**; do not represent preparing it as running it on the PC.

Strict original SQL Server/Admin.exe/Cashier.exe READ-ONLY LOCK remains in effect.

## 2026-10-22 12:40 Baghdad — Verification addendum (STAGED ONLY)

Read-only checks on the active PC relay observed:
- Agent `Server-c24b5c0f` v5.1.7 heartbeat fresh (~19 seconds) and self-reported health 100/100.
- `pc_os_events` contained **0** genuine `itemname_delta` records from that agent.
- v5.1.8 remains `staged`, `publishable=false`; **not installed or promoted**.
- An attempted read-only `registry_get` of `SCHANNEL\\Protocols\\TLS 1.2\\Server` for `Enabled` and `DisabledByDefault` found no explicit registry path. Absence is not proof TLS 1.2 is disabled: Windows defaults matter.
- SQL ERRORLOG previously identified SQL Server 2008 R2 RTM version 10.50.1600.1; Microsoft's [KB3135244](https://learn.microsoft.com/en-us/troubleshoot/sql/database-engine/connect/tls-1-2-support-microsoft-sql-server) lists 10.50.6542.0 as the initial x86/x64 SQL Server 2008 R2 SP3 build with TLS 1.2 support. A modern TLS 1.2 connection cannot be assumed. No legacy engine updates or weaker TLS fallbacks are authorized under the lock.
- Read-only SQLCMD probe in the older v5.1.7 backup module previously returned exit status 1. This is a failure, not proof that `dbo.ItemName` was read.
- Privacy hardening of the v5.1.8 SQL diagnostics now **discards SQLCMD stdout** and returns fixed diagnostic categories from stderr only (TLS name/trust/handshake, integrated login, DB access, network, schema). Five corresponding isolated Go tests passed in a reproduction of the candidate logic in the analyst container. This is **not** a full Windows integration or live DB success.
- The GitHub `ops/diagnostics/ItemName-ReadOnly-Connection-Gate-v3.ps1` probe was hardened for Windows PowerShell 5.1 stderr capture and categorized error reports, remaining read-only.
- One-time ChatGPT push diagnostic task completed at 2026-10-22 09:31:27Z, but `notifications_enabled=false` and delivery to iPhone was not observed; no success claim.
- A separate conversation artifact `Zhirox-ItemName-Live-SELECT-ReadOnly-v4.ps1` carries three hardcoded SQL `SELECT` statements and categorical-only SQLCMD diagnostics. It has been statically inspected, **not executed on the PC**.

Release remains blocked on: (1) authorized validated-TLS successful live `ItemName` SELECT with an appropriate SQL principal; (2) deliberate safe agent update under owner approval without modification of old SQL/POS; (3) real `itemname_delta` -> relay -> notification handoff with verified iPhone receipt.

**Do not mark production READY or update the legacy SQL Server under read-only lock.**

## 2026-10-22 15:06 Baghdad — Real local SQLCMD failure isolated
A live v5.1.7 agent read-only `pc_os_backup_readiness` probe for `localhost` completed with `database_probe_ok=false` and the exact ODBC Driver 18 failure:
```text
SSL Provider: The target principal name is incorrect.
Client unable to establish connection.
```
A second read-only probe with host name `Server` also returned `database_probe_ok=false` (generic exit status 1; detailed diagnostic unavailable). **Neither probe read any ItemName rows.**
The candidate v5.1.8 `pcOSItemNameSQLCmdArgs` now pins TCP transport to `tcp:127.0.0.1,1433` and specifies the machine certificate host with `-F`; `-N` requires TLS and `-C` is absent. Microsoft's SQLCMD documentation confirms `-F` supplies the certificate CN/SAN verification hostname; this is an unproven candidate fix, not proof of handshake success.
The SQL Server engine logged as v10.50.1600.1 (SQL Server 2008 R2 RTM); KB3135244 documents TLS 1.2 as available in x86/x64 SQL Server 2008 R2 SP3 build 10.50.6542.0 and later. This remains a potentially separate blocker.
**Release gate remains BLOCKED, do not promote v5.1.8 or modify legacy SQL server/certificate settings.**

## 2026-10-22 15:33 Baghdad — Local-only LPC candidate and build correction

### Completed, isolated branch only
- Switched v5.1.8 `pcOSItemNameSQLCmdArgs` to fixed `-S lpc:Server -E -No`: no TCP, named pipes or remote fallback. `-No` (optional encryption) is confined to Windows local Shared Memory; DO NOT use this setting for network SQL sessions. Never introduce `-C` or SQL passwords.
- The schema probe now accepts `dbo.ItemName` ONLY when `CONNECTIONPROPERTY('net_transport') = 'Shared memory'`; if the client tries another transport, the schema gate fails closed.
- Added isolated tests that assert LPC-only args and reject other server hosts and arbitrary SQL.
- Fixed the Windows Build workflow to copy AND compile the necessary SQL args and categorized diagnostics helpers; previously these helpers were missing and the build could fail with undefined functions.
- The build remains a candidate: no GitHub Actions job success, signed new binary, Windows runtime SQL probe or production event delivery is claimed.

### Independently verified active PC facts
- Live service `ZhiroxAICore` runs as **LocalSystem**; the active SQL Server service `MSSQLSERVER` also runs as **LocalSystem** (from READ-ONLY Windows registry/service metadata). This does **not** establish that the SQL login `NT AUTHORITY\\SYSTEM` has permission to SELECT from `SuperMarket.dbo.ItemName`; authenticated live query is still required.
- `pc_os_backup_readiness` over `localhost` failed with ODBC Driver 18 certificate principal-name mismatch; host `Server` also failed (generic exit status 1).
- A probe using local IP was rejected by the agent's allowed-server policy, which remained unchanged.
- At 15:00 local, the database engine SQL ERRORLOG recorded successfully completed backup `D:\\Backup\\SuperMarket_backup_2026_10_22_150004_3880297.bak`. The agent safely hashed it: 20,009,472 bytes; SHA256 `b9cee29ea2f470f5356ff1fbcccfc238b790afebad68e78f47c939fb3796a02a`. A backup on O: was denied by `allowed_roots`. No backup was copied off-site; a hash does not constitute restore verification or ItemName data extraction.

### Next hard gates
1. Run the local `lpc:Server` SQLCMD probe **under the same identity that the eventual agent will use**; prove actual SELECT against ItemName, row count, and local `net_transport`. Any authentication/permission failure requires an explicit owner-approved least-privilege access design; do not silently grant LocalSystem sysadmin.
2. Test changed SQL args on a compatible Windows Server/SQL Server 2008 R2 environment without modifying the protected POS host. Verify CI and a signed release artifact. Leave active agent 5.1.7 untouched until signoff.
3. Migrate+integrate the relay's event-key idempotency only after isolated staging E2E. Confirm actual production events and downstream notification receipt separately. No synthetic test data in user-facing reports.
4. Keep v5.1.8 `staged` and `publishable=false` until all gates are met.

**Stop condition:** The active v5.1.7 agent cannot execute arbitrary scripts, so a remote live LPC SELECT is not authorized through existing relay capabilities. Do not bypass command allowlists or mutate legacy SQL engine/services.
