# Zhirox Live ItemName CSV — v5.1.12 candidate

**State:** candidate only; not deployed to protected PC. Production v5.1.11 remains unchanged.

## Permanent file (on protected PC, Agent-owned storage only)

`C:\ProgramData\ZhiroxAI\exports\SuperMarket_ItemName_latest.csv`

**Schema:** `ItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice`

- Initial CSV derives from a verified, double-read `SuperMarket.dbo.ItemName` snapshot (43,961 rows at last verified check; live counts can change).
- The existing 30-minute watch cycle recomputes the CSV after a successful verified snapshot. On each addition, unit price change, or carton price change, the CSV is atomically replaced. Identical bytes leave the file untouched.
- Rows retain distinct `ItemNameID` values; **never merge** duplicate names or prices. Barcode strings preserve leading zeros at the CSV byte level; Excel may require importing the barcode column explicitly as text.
- UTF-8 with BOM and CRLF, quotation rules from Go `encoding/csv`. Spreadsheet-formula-like item names are escaped with a leading apostrophe.
- The exporter writes exclusively under Agent's own ProgramData root, not `D:\DataBase`, `D:\Backup`, `Admin.exe`, `Cashier.exe`, SQL Server database tables or service settings.
- CSV generation occurs **after** the existing cloud change event has been accepted and **before** advancing the existing JSON baseline. Errors stop baseline advancement and retain the previous complete CSV. The existing relay event deduplication guards retries.
- On upgrade, the CSV is bootstrapped on the next normal watch cycle even if no new product/price change occurs.
- The CSV includes only the five fields that the existing authorized SELECT reads. This is **not** an SQL database backup, nor a full export of all tables.
- This first implementation is **local-only**. Remote iPhone downloads or automatic cloud CSV copies require a separate authenticated transport and exposure review.

## Safety and release gates

1. Isolated policy + CSV tests pass, along with read-only code invariants.
2. Run signed Windows candidate build with the CSV source explicitly included in the build manifest (not done by isolated tests alone).
3. Canary verify hash/signature and initial local CSV count/path under Agent-owned ProgramData only.
4. Check SQL load, normal cashier operation, atomic replace and rollback; only then promote.
5. Do not weaken the protected legacy POS Read-Only Lock.

## 2026-10-22 23:40 Baghdad — Signed staging and Canary PASS

- Exact Windows v5.1.12 candidate built successfully in GitHub Actions run **39848546756**; artifact ID **12455743312**. The exact 19,992,576-byte executable has SHA-256 `a7f64c11e03fc4ed385e41331f224700c40a19df4472df8597b2d0b1457cfb11`.
- Public GitHub prerelease `v5.1.12-candidate-f68799de` published through passing exact-transport readback run **39849107565**.
- Supabase `pc_update_releases` row for 5.1.12: `status=staged`, `publishable=false`, `blocked_reason` intentionally non-null. No production manifest/cutover.
- Ed25519 signature generated on the server using the existing pinned update key, with no signing seed returned or embedded in the release; normalized signature string then independently verified in an isolated container against the exact binary and canonical `5.1.12|sha256|19992576`.
- Separate Supabase `zhirox-pc-update-v5112-csv-canary` deployed. The independent GitHub Actions verification run **39850601547** PASSED: canary health fetched the exact 20 MB public prerelease binary, verified full-byte SHA-256 and Ed25519, confirmed the DB gate is staged and non-publishable, and unauthenticated manifest/binary endpoints both returned 401.
- Read-only PC action `pc_os_update_chain` completed: production agent reported 5.1.11 with pinned Ed25519 trust enabled, staging and rollback required, post-update cloud health gate of three successful heartbeats. This verifies reported readiness; **it is not a production rollback execution test**.
- Latest read-only `pc_os_itemname_status` confirmed 43,967 ItemName rows and healthy baseline, with last successful ItemName snapshot at **2026-10-22T20:34:31Z**.
- Production PC remains **v5.1.11** and continuously monitors price/new-item changes. New CSV creation on PC is NOT yet active. No legacy SQL Server/Admin.exe/Cashier.exe updates, restarts, restores, or data edits were made.

### Remaining release gates

1. Owner-controlled protected-PC candidate activation without changing old SQL/POS; confirm explicit approval for agent binary update under the existing Read-Only Lock.
2. Verify `C:\ProgramData\ZhiroxAI\exports\SuperMarket_ItemName_latest.csv` exists with **current** row count, distinct ItemNameIDs, valid UTF-8, barcodes unchanged, and SHA-256 status.
3. Confirm normal Admin/Cashier latency/CPU and monitor connectivity under ordinary market load, and check 3 cloud heartbeat success gate.
4. Verify signed rollback restores known-good Supervisor on controlled update failure, without disrupting the original POS.
5. Optional later phase: authenticated cloud copy and iPhone download of latest CSV; local CSV by itself does not become a downloadable ChatGPT attachment.

**Do not promote while any release gate remains unverified.**
