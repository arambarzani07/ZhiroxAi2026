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
