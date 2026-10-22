# ItemName Delta Relay — Staged Integration Gate

**State: SOURCE ONLY / NOT DEPLOYED.**
No SQL Server, Admin.exe, Cashier.exe, SQL Server TLS settings, PC update channel, or active Supabase Edge Function is modified by this proposal.

## Confirmed production gap (read-only audit)
The ACTIVE `zhirox-pc-relay` function accepts agent-authenticated `POST /api/agent/:id/pc-os/event` and inserts directly into `pc_os_events`, returning HTTP 202. There is no event-key uniqueness check or automatic owner-facing delivery in that handler. A repeated attempt can therefore create duplicate rows. This is not proof that an ItemName notification reached an owner.
The database contained no `itemname_delta` rows at the time of the read-only audit.

## Candidate files
- `itemname-event-guard.mjs`: validates database, read-only flag, 64-hex event key, 1..25 changes, permitted change kinds/fields; classifies a duplicate-key database error.
- `itemname-event-guard.test.mjs`: 14 isolated checks, runnable with Node 22.
- `../migrations/20261022_01_itemname_event_idempotency.sql`: nullable event key + partial unique index (for the connected relay's Postgres database, *not* legacy SQL Server).

## Required handler integration (AFTER migration on staging only)
Keep `authAgent(req, agentId)` and the minimum agent version gate as currently implemented. Within the existing /pc-os/event authenticated POST handler, after parsing `body` and sanitizing payload:

```ts
import { validateItemNameDeltaEvent, classifyItemNameInsertResult }
  from "./itemname-event-guard.mjs";

// ... current authentication and version checks must precede this code ...
const verdict = validateItemNameDeltaEvent(event_type, payload);
if (!verdict.ok) return json({ok:false,error:verdict.error}, 422);
const insert = {
  agent_id: agentId, event_type,
  severity: normalizeSeverity(body.severity), payload,
  ...(event_type === "itemname_delta" ? {event_key:verdict.eventKey} : {}),
};
const {error} = await supabase.from("pc_os_events").insert(insert);
const classification = event_type === "itemname_delta"
  ? classifyItemNameInsertResult(error)
  : error ? "error" : "inserted";
if (classification === "duplicate") return json({ok:true,duplicate:true},202);
if (classification === "error") throw error;
return json({ok:true,duplicate:false},202);
```

The `23505` duplicate classification is valid ONLY when the error identifies `pc_os_events_agent_event_key_unique`; unrelated uniqueness violations must remain errors. No `upsert`, blanket ignore, or bypass of agent authentication.

## Mandatory E2E tests before production
1. In a nonproduction test context, pair or use a disposable test agent with its own secret and no link to the market PC.
2. POST a valid ItemName delta, verify HTTP 202, verify exactly one DB row and the expected event key.
3. Retry the EXACT same payload, verify HTTP 202 `duplicate:true` and DB row count still one.
4. POST an event with a different key, verify a second row.
5. Try missing or invalid auth, bad database name, changes over 25, missing read_only: verify rejection and zero new DB rows.
6. Test an unrelated event type to confirm backwards compatibility.
7. Confirm the owner's intended notification channel separately; storing a DB row is NOT an owner notification.
8. Remove test agent and test events after test completion.
9. Run full regression/security checks and approve production migration + Edge Function deployment deliberately; leave v5.1.8 PC update STAGED until SQL TLS and real SELECT probes pass.

## SQL TLS blocker
A real `MSSQLSERVER` SQL Server 2008 R2 certificate (SHA-1 thumbprint 35F0CEE5D8F4520E680BAE46E01B32EEB033EA09) was located by read-only SQL ERRORLOG and registry checks. Its DER was decoded and its self-signature verified: Subject=Issuer=`CN=192.168.1.10,CN=server`, no SAN, SHA-1 signature, valid 2025-11-20 to 2035-11-20. Trust and hostname validation on ODBC Driver 18 remain NOT VERIFIED. Using `-C` / TrustServerCertificate bypasses validation and is not an approved production fix.

## Read-only boundary
No `SELECT` from the live `SuperMarket.dbo.ItemName` has been demonstrated; do not represent service readiness or historical backup logs as a successful live ItemName SELECT.
