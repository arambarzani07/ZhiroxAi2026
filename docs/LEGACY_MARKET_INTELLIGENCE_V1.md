# ZHIROX Legacy Market Intelligence — Phase 1 (Read-Only)

## Status
Feature branch only; **not installed, not deployed, not merged to main**.
Existing production PC Relay v5.1.13, Admin.exe, Cashier.exe, SQL Server and DVR remain unchanged.

## Verified source boundary
- Existing legacy SQL Server database: `SuperMarket`; previous PC Relay read-only ItemName probe succeeded.
- Supabase `public.pc_os_events` already receives `itemname_delta` event payloads with `changes[]`, `event_key`, and `read_only=true`.
- Observed change kinds include `added` and `price_changed`; observed price fields include `SalePrice` and `WholeSalePrice`.
- Other sales, basket, debt, purchase, customer, supplier and stock table schemas have **not been validated** here; do not claim corresponding features implemented.

## Implemented in this branch
`internal/marketintel/pulse.go` is a side-effect-free Go package that:
1. Deduplicates event snapshots using `event_key` (fallback event ID).
2. Counts unique new items and items with price changes within a supplied event window.
3. Counts retail vs. wholesale price changes separately.
4. Flags retail price increases of at least 35% and nonpositive retail prices as **advisory** warnings.
5. Rejects invalid/non-read-only events, oversized inputs and malformed payloads.
6. Caps alert output and never issues SQL queries, network requests, Windows commands or writes.

Run unit tests from `internal/marketintel`:
```sh
GO111MODULE=off go test -v .
```

## Future connection (NOT yet implemented)
Add an authenticated, audited, SELECT-only server adapter to supply bounded event JSON from `public.pc_os_events`. Never expose service-role keys or raw business event data to anonymous clients. Do not store phone numbers, customer notes, payment credentials or other private records.

## Next planned capability groups
- **Safe companion, read-only:** product/price history, sale anomaly analysis once permitted sale data fields are verified, inventory status from authorized read views, system and printer diagnostics, iPhone owner dashboard.
- **Requires staging database, source access and business rules:** five held tickets, multi-unit barcode scanning, basket edit/locking, split payments, customer debt, stock mutation, receipt/refund, protected price edits, employee roles.
- These transactional features **cannot** be added to the existing closed-source `Admin.exe` and `Cashier.exe` by read-only PC Relay alone.

## Production safety gates
1. Preserve `D:\DataBase`, both legacy executables, production SQL and the DVR.
2. No DDL, INSERT, UPDATE, DELETE, BACKUP/RESTORE or live transaction changes.
3. Build and test under a separate development branch and test database.
4. Keep zero-UI, signed release, rollback and explicit approval guarantees.
5. Promote only after an end-to-end compatibility check and separate controlled deployment decision.

## Kurdish user-facing explanation
ئەم قۆناغە تەنها بۆ شیکردنەوەی کاڵا و گۆڕانی نرخە؛ هیچ فرۆشتنێک، داتابەیسێک یان فایلی کاشێر ناگۆڕێت. لە لقێکی پەرەپێدانە و هێشتا لە کۆمپیوتەری مارکێت دامەزراو نییە.
