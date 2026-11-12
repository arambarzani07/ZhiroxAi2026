# Legacy Cashier Smart Basket Merge — one-PC safe testing plan

**User constraint:** Only one actual shop PC is available. No second PC is required for initial **isolated** testing.

**State:** Development-only PR #25. No live Cashier.exe integration. No SQL write/migration. Do not merge/deploy without live-operation safeguards.

## What can be tested on the SAME physical computer

After business hours, after closing all outstanding sales and **normally** exiting Cashier.exe, open a self-contained local HTML simulator in Microsoft Edge / Chrome. The test page has no network dependencies, no persistent storage, no SQL access, no printer commands and no keyboard hooks. It has five in-memory test tickets and a test price explicitly unrelated to catalog prices.

### Procedure

1. Finish the current shift and make sure outstanding sales are finalized, with the cashier operator's confirmation. Never remote-kill Cashier.exe.
2. Leave Admin.exe, real SQL services, Windows tasks, printer services and DVR entirely unchanged.
3. Open the **separately supplied** `ZHIROX_Single_PC_Basket_Test.html` from a local nonbusiness folder (e.g., `Downloads`) in Edge/Chrome. This HTML demo is a conversation artifact, **not installed or served by this repository**.
4. Focus the barcode test field. Scan the same *ordinary fixed-unit* barcode twice. It should show one row with quantity 2, using the explicitly selected **test price**.
5. Switch to a second test ticket; its contents must remain separate. Use a barcode with Enter or without Enter, if supported by the physical scanner configuration.
6. If the scanner does not populate the field as expected, do not change hardware drivers or Windows scanner settings during business operations.
7. Close the browser tab when finished. All simulated cart data disappears automatically. Reopen the real Cashier.exe normally when appropriate.

## Strict boundaries

- Do **not** run the simulator simultaneously with an active cashier operation. HID keyboard-wedge devices send keystrokes to the focused window, and focus mistakes can affect live input.
- Do not copy or overwrite `Admin.exe`, `Cashier.exe` or `D:\DataBase`.
- Never point a test POS at the live SQL Server.
- Do not print a real receipt, submit a sale, charge a card, apply a price change, or manipulate stock as part of this demo.
- No global keyboard capture/keylogging or remote desktop takeover.
- A local HTML simulation can demonstrate basic scanner input and row merging but **cannot prove** the closed-source Cashier.exe can be safely modified.
- To test actual SQL sales/stock/receipt persistence, use a logically isolated test database with **verified independent credentials and no live write permissions**, even on the same machine. Otherwise remain in in-memory simulation only.

## Safety gates still missing

- Physical scanner focus and suffix measurements performed in an isolated test window
- Reliable mapping of actual `ItemNameID`, barcode aliases and unit types
- Native, supported integration strategy with the closed-source legacy binaries
- Nonproduction SQL checkout, cash totals, stock decrement, crash/rollback and printer parity

**Sorani:** تەنها یەک کۆمپیوتەر بەسە بۆ تاقیکردنەوەی سەرەتایی؛ داتابەیس و بەرنامەی فرۆشتنی ڕاستەقینە نەگۆڕدرێن.
