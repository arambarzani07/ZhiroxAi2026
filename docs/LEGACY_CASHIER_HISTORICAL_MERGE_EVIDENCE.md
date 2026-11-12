# Legacy Cashier.exe — historical basket line-merging evidence (READ ONLY)

**Source:** Previously extracted historical SuperMarket SQL Server backup: `SuperMarket_Sales_Transactions_Full.csv`. This is **not** a current/live SQL query and does not contain current cashier UI telemetry.

**Target binary identity** as established through PC Relay SHA-256 and earlier independent exactness-evidence record:
- `D:\DataBase\Cashier.exe`: `64b886b04b345bd63d8f76e693ed224958816463535a556208f88daa9cd39efb`, 376,832 bytes.
- `D:\DataBase\Admin.exe`: `020f5f1939871b4d52b889c81ca07f0881c14d68edbc7535be3bc765ac23252b`, 1,708,032 bytes.

These hashes match the historical `EXACTNESS_EVIDENCE_RAW_V10.json` Library evidence; they verify binary *identity*, not that its internals contain a merge toggle.

## Reproducible historical CSV analysis

Read-only Python stdlib (`csv`, `decimal`, `datetime`, `collections`) scan; no private customer/product fields are emitted. Groups are defined by `(BillNo, EnterDate calendar date, ItemNameID, UnitPrice)`, excluding canceled/nonpositive rows.

| Metric | Measured count |
|---|---:|
| Total exported CashierForm transaction rows | 73,501 |
| Distinct TransactionID | 73,501 |
| Duplicate TransactionID in export | 0 |
| Rows with UnitQuantity > 1 | 9,480 |
| Rows with integer UnitQuantity > 1 | 5,591 |
| Distinct (BillNo, calendar date) groups | 12,417 |
| Bill/day groups with repeated ItemNameID+UnitPrice | 5,982 |
| Repeated product+price groups within bill/day | 10,713 |
| Extra individual rows in these repeated groups | 14,590 |
| Repeated groups in which every row has UnitQuantity=1 | 10,408 |
| Repeated groups whose records have identical timestamp (second) | 9,194 |
| Maximum time span within a repeated group | 3 seconds |

**Interpretation:** Existing CashierForm schema and historical records can represent quantity greater than one in a single transaction row. Distinct TransactionID records also repeatedly exist for the same bill/day/product/price, including 10,408 all-unit-quantity-1 groups, consistent with an application that frequently leaves separate basket/sale records rather than always consolidating them.

**DO NOT OVERCLAIM:** These rows cannot distinguish repeated physical barcode scans from manual line entry, same-day bill-number reuse, later edits or operational conventions. The evidence does not establish whether legacy Cashier.exe has a hidden settings toggle. `F8` quantity support, if verified, is not proof of auto-merging. Historical data aggregation in a report/receipt is not proof of consolidated *live basket* lines.

## Technical consequence

- **SQL schema migration is not an identified prerequisite** for representing one line of quantity 2 in the historical `CashierForm` schema.
- **Changing SQL history or adding an aggregate view will not reliably change the in-memory live basket grid of Cashier.exe.** It risks stock/receipt/cash reconciliation.
- To implement native auto-merging safely requires verified legacy executable code behavior (or original source), plus a controlled, independent test environment. The read-only PC Relay does **not** provide binary bytes or an allowed route to inject/patch executable code.
- Do not replace, patch, reconfigure or restart production Admin.exe/Cashier.exe, SQL Server or DVR.
- Treat `27...` weighed barcode formats separately from fixed-unit merges; reusing a physical label's scan may be accidental and quantity encoding differs.
- The in-memory Go prototype in Draft PR #25 is **not** integrated with the original legacy executable.

## Next actionable gate

Obtain an **offline copy** of the exact binary, hash-check it against the approved SHA-256 and perform static PE/VB6 resource/control/scan-event analysis **without executing the copy against live SQL**. Only if a genuine native toggle or legally maintainable method is demonstrated should a separate nonproduction test be considered. If it is not, the reliable route is an actual vendor revision or replacement application, not an unsafe external key hook or DB rewrite.

**User-facing summary (Sorani):** لە پاشەکەوتی فرۆشتنی کۆندا بەڵگەی بەهێز هەیە کە هەمان کاڵا لە هەمان پسووڵەدا بە ڕیزی جیاواز تۆمار کراوە، هەرچەندە داتابەیس بڕی 2 لە یەک ڕیزدا دەتوانێت تۆمار بکات. تا ناوخۆی فایلە ڕەسەنەکە پشکنرێت، جێبەجێکردنی خودکاری Merge لە Cashier.exe پشتڕاست نەکراوە.
