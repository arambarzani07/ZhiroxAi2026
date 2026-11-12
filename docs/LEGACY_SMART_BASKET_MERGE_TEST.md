# Smart Basket Merge — Safe test prototype

**Status:** Development branch only. Not merged to `main`, installed on Windows, or connected to production SQL.

**Legacy system boundary:** `Admin.exe`, `Cashier.exe`, `D:\DataBase`, SQL Server, printer drivers and DVR are untouched. This standalone package has no database/network/process or filesystem I/O.

## Confirmed example from the cashier screen

The same barcode `670237553985` scanned twice in ticket 1 at 500 IQD/piece should yield exactly one line, quantity 2, total 1,000 IQD. Two *different physical scans* must have different `EventID` values. A repeated transmission of the same `EventID` is treated as a retry and ignored.

## Rules implemented

- Distinct ticket states; up to 5 tickets, no cross-ticket merge.
- Merge requires matching product ID, barcode, selling unit, unit price, price version, discount rule, tax code and batch.
- Never merge into a locked line.
- Price/quantity overflow is rejected without mutating the original session.
- Returns, manual price overrides and variable-weight sales are blocked pending a separate authorized POS workflow.
- No changes to receipt, stock, database or financial ledger.

## Test

```bash
cd internal/basketmerge
GO111MODULE=off go test -race -count=1 -v .
GO111MODULE=off go vet .
```

## Before any deployment

1. Verify authentic staging copy and original Admin/Cashier program behavior.
2. Verify real scanner event framing, unit conversions, cart/receipt parity and printing.
3. Test crash/restart, multiple cashiers, stock decrement and exactly-once checkout on a test market.
4. Confirm native source/integration path; read-only PC Relay cannot inject this logic into a closed-source Cashier.exe.
5. Explicit release approval, verified backup, rollback and signed update.

**Sorani:** ئەم کۆدە تەنها بۆ تاقیکردنەوەی کۆکردنەوەی بارکۆدی دووبارەیە؛ لە کاشێری ڕاستەقینە دامەزراو نییە.

## Phase 2: HID input and receipt parity (prototype only)

- `internal/basketmerge/scanner.go`: deterministic scanner parser; Enter sends a complete barcode immediately, while no-Enter scanner input is only finalized after a caller-provided quiet-time observation.
- The prototype quiet threshold is 250 ms and needs calibration with the physical scanner on an isolated test PC. It is NOT proof of compatibility with the deployed cashier.
- Accept only ASCII digit input of bounded length. Do not install system-wide keyboard hooks, monitor cashiers' typing, or capture user passwords.
- Retain leading zeros. Never equate barcode value with scan EventID. No auto-merge for unverified prefix-27 variable-weight labels.
- Event IDs are globally deduplicated within the in-memory five-ticket session. A late retry after switching tickets must not charge a new customer.
- `internal/basketmerge/receipt.go`: a read-only in-memory receipt preview and exact parity check; this does not interact with the real receipt printer or legacy accounting.
- The `ApplyScan` caller is responsible for resolving products and verifying unit price, barcode alias, permissions, pricing version and unit conversion against a trusted catalog. No authorization bypass is implied by the Go API.

### New checks

- Enter and CRLF versus no-Enter scanners; explicit gap handling and invalid/short/oversized barcode rejection.
- Two independent scans from the photographed legacy basket example result in one line with quantity 2 and total 1,000 IQD.
- A duplicated network/input event is not counted twice, even across tickets.
- Receipt line and grand total arithmetic cannot drift from the current basket snapshot.
- No file writes, SQL commands, sale recording, stock decrement, printing, or Windows input capture.

### Remaining gates before deployment

1. Establish a Windows test copy of the *actual* scanner, legacy database schema, and existing Cashier.exe with zero production side effects.
2. Prove scan segmentation using the real barcode reader (Enter/no-Enter and optional prefixes) under Kurdish keyboard layouts.
3. Verify catalog and pricing mapping, including weight-scale prefix 27 and separate product alias / carton semantics.
4. Demonstrate basket-display/physical-receipt parity and exactly-once persisted sales on a **test database**.
5. Keep this feature as Draft PR only until genuine end-to-end staging evidence exists.
