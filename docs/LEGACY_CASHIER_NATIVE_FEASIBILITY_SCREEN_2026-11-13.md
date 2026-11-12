# Legacy Cashier.exe — native feasibility screen observation (2026-11-13)

**Target:** production legacy binary at `D:\DataBase\Cashier.exe`, not the new ZHIROX371 app.

## Confirmed passive observation
- Read-only PC Relay `pc_os_active_window` returned `process_name=Cashier.exe`, `window_title=المبيعات`, and the exact executable path.
- On-demand read-only primary-display screenshot at `2026-11-12T21:19:18Z` (2026-11-13 00:19 local) showed the actual original cashier sales interface.
- Sales grid visibly has item identifier/name, **quantity**, unit price, and line total columns. There is a **F8 quantity** control in the interface.
- Basket was empty at capture (displayed zero total). No barcode input was sent and no sale was created.
- No explicit **Merge** or **merge identical scanned items** toggle was visible in the sales window. This does **not** rule out a different admin/settings dialog.
- This observation does not establish that pressing F8 is equivalent to automatic duplicate-scan merging.

## Offline binary identity evidence
The same production file was independently hashed via PC Relay:
`sha256=64b886b04b345bd63d8f76e693ed224958816463535a556208f88daa9cd39efb`;
`size=376832` bytes. These match historical binary-analysis evidence, but **no binary payload** is currently available in the assistant's workspace for PE import inspection, disassembly, or event-handler tracing.

## Safe user-assisted exact-file gate
A **standalone local HTML inspector** was prepared outside the protected installation. It reads a file explicitly selected with the local browser file picker, checks SHA-256 against the independently verified production hash, and summarizes PE architecture, Visual Basic runtime markers and section metadata. It has an explicit no-network Content Security Policy. It does not execute any user-selected EXE file, inject events, change registry, start/stop PC services, or touch production SQL.

The inspector was tested against a **synthetic PE sample only**; there is no claim of testing it against the genuine production executable. Its result is strictly **metadata**, not a proof that a native scan-merge event handler can be modified.

## Decision
- **Feasible in principle** to represent two pieces as one quantity-2 basket line, consistent with the production UI's quantity field and the historical SQL schema.
- **Not yet verified** that the existing closed-source legacy Cashier.exe supports a merge setting, safe compiled-code patch, or supported extension.
- No SQL rewrite, receipt-only aggregation, key hook, DLL injection or binary replacement should be deployed into the single live store PC.
- Draft PR #25 remains **development-only** pending genuine offline copy inspection and protected staging/replay tests.
