# Smart Basket Merge — Phase 3 Compatibility Gate

**Mode:** development-only, no live POS integration. Branch: `feature/legacy-smart-basket-merge-safe-prototype`, Draft PR #25.

## Read-only Windows evidence

- PC Relay Agent `Server-c24b5c0f` reported version 5.1.13 and a fresh heartbeat on 2026-11-12.
- Fresh read-only driver enumeration completed 2026-11-12 18:03:30 UTC (21:03:30 local Iraq) and included `POS HID Barcode scanner` (`hidscanner.inf`) and two `HID Keyboard Device` records. Previous evidence from 2026-10-12 was consistent.
- A driver listing proves Windows has registered the driver/device entry at the moment checked. It does **not** prove the physical scanner is currently plugged in or working, nor its model, suffix configuration, focus behavior, real key intervals or application-level compatibility.
- Historical `app_status` showed legacy `D:\DataBase\Cashier.exe` present/running in October. This is not proof of current UI automation compatibility.
- No device serials, keypress traces, screenshots, credentials, personal customer data or live business SQL rows were captured for this feature.

## New Phase 3 offline test harness

`internal/basketmerge/replay.go` accepts **synthetic** barcode traces, a bounded test catalog and up to five separate test tickets:

1. Simulate digit-by-digit scanner keyboard input with Enter or no-Enter idle completion.
2. Reject unknown barcodes, mixed keyboard input, slow/unverified synthetic scan timing, invalid catalog prices/units, and prefix-27 scale labels.
3. Resolve **only** against the supplied, synthetic trusted catalog (no SQL connection).
4. Pass unique physical scan IDs into the deterministic Smart Basket Merge engine.
5. Preserve leading zeros and block duplicate event replay across tickets.
6. Derive an in-memory receipt preview, checking line totals and ticket parity.
7. Reject ambiguous/error scenarios without returning a partially accepted result.

The test fixture timing limit (80 ms between keys) and idle delay (250 ms) are provisional **testing constants**, not measured scanner properties. Do not deploy them as production device settings without on-device validation. HID keyboard-wedge input cannot reliably distinguish a scanner from rapid human keyboard typing unless the host application controls focus/provenance.

## Real Windows staging checklist (not yet executed)

| Verification | Status |
| --- | --- |
| Current Windows POS HID barcode scanner driver listed | Evidenced on 2026-11-12; physical scan still unverified |
| Enter/no-Enter synthetic traces | Automated tests |
| Multiple held tickets and retry protection | Automated tests |
| Synthetic basket/receipt arithmetic | Automated tests |
| Physical barcode reader brand, HID mode and suffix | **Not verified** |
| Real on-device timing under Sorani/English layouts | **Not verified** |
| Isolation of scanner focus from keyboard entry | **Not verified** |
| Live legacy `Cashier.exe` input and UI compatibility | **Not verified** |
| Real legacy receipt rendering/printing | **Not verified** |
| Test SQL stock, sales transaction and rollback | **Not verified** |

## Non-negotiable safeguards

- Keep original `Admin.exe`, `Cashier.exe`, `D:\DataBase`, SQL Server and DVR unchanged.
- No production driver reinstall, USB reconfiguration, keylogging, global keyboard hook or interactive control of the live cashier.
- No repository code may assume its synthetic scanner replay is validated on the deployed physical device.
- Before any production promotion, use an isolated test computer/database, verify native integration strategy, pass sale and inventory end-to-end tests, verify backups and obtain explicit release approval.

**کوردی:** ئەمە تاقیکردنەوەی سەربەخۆیە؛ هێشتا بارکۆدخوێنی ڕاستەقینە و Cashier.exe ـی مارکێت بە یەکەوە تاقی نەکراونەتەوە.

## Phase 4: ticket-focus/revision guard (development-only)

`internal/basketmerge/scan_gate.go` adds an in-memory application-level `ScanGate`. It makes these testable rules explicit:

- The scanner field must have application-managed focus before accepting a scan. The scanner's identity is **not** authenticated by this alone.
- Starting a scan captures the selected ticket, focus epoch and basket revision.
- Switching tickets, even away and back to the same ticket, clears scanner focus and invalidates all pending scan leases.
- Losing/regaining scanner-field focus invalidates pending scan leases.
- An intervening basket update invalidates pending scan leases; stale scans must be consciously retried or rescanned, never silently charged.
- A repeated physical scan event ID cannot add extra quantity.
- State is deep-copied and invalid scans do not mutate the original session.
- The test harness pairs Enter and no-Enter synthetic traces with the scan gate and in-memory receipt-parity checks.

### Important limitations

This is **not** connected to the legacy Cashier.exe. Physical HID scanning, Windows foreground/focus transitions, basket UI hooks, persistent transaction ID generation, multi-terminal synchronization and crash replay are **not yet verified**. A real POS adapter would have to own authoritative session state, serialize writes and authenticate scanner provenance before allowing a single sale. Never install a global keyboard logger or interact with the live cashier via remote input capture.

### Phase 4 pass/fail definition

Automated tests must pass `go test -race -count=1 -v .` and `go vet .` in `internal/basketmerge`. Real hardware, real SQL or production release cannot be marked successful based on these in-memory tests.
