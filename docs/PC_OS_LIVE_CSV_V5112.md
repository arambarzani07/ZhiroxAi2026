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
