# PC Relay 5.1.19 target — safe cashier logout, source-only phase 1

The user's goal is app-only logout with no remote screen viewing, input injection, Windows logoff, forced process termination or arbitrary commands. Cashierynwe 1.0.16 already has a Safe Logout hook and a separate update-install gate. The current Relay 5.1.18 only observes a bounded update journal; the journal is never proof that it is safe to close a session.

This PR adds the strict typed policy and test suite only. It does not register a command, bump the published binary, modify Supabase or deploy anything. All production behavior stays unchanged. The version number 5.1.19 is a target, not a released version.

Required next gates:
1. App-owned local authenticated channel, fixed target, no arbitrary execution. Fresh opaque random session nonce generated on each successful login. No reliance on the desktop title or stale journal. Relay must not accept safety claims from the caller.
2. Cloud action is mutation-only, owner-authenticated one-time exact-args approval, bound to the agent, market, PC, session and expiry. Allocate command ID transactionally before approval or revise protocol consistently. No approval creation or queue from a read-only path. Verify actual deployed connector compatibility.
3. App validates the request, atomically consumes durable replay record, blocks new business operations during the safe logout handoff, rechecks all pending sale/cart/receipt/printing/finance/queue gates, invokes its own existing safe logout, and reports actual outcome. Never erase carts or close a Windows session. Expired, duplicated, invalid, unavailable and busy states fail closed.
4. App-side version must support the channel first; 1.0.16 does not. Only then integrate the Relay dispatch and control-plane allowlist. No release until exact signed builds, offline behavior, real Windows idle/busy/replay tests and recovery checks pass. No automatically generated owner approval. No live market writes during this review.

The policy result eligible_for_app_recheck is not proof of owner approval or logout; it is only a precondition check. No raw session data, credentials or market business data in telemetry.
