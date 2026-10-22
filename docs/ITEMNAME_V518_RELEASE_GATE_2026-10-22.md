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
