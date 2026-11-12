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
