# Zhirox PC OS v5.1.2 — Zero-Touch Recovery

## Objective

Recover automatically from the failure mode where the Windows `ZhiroxAICore` service still reports **Running** while the Supervisor's relay/heartbeat path has stopped making progress.

The design is intentionally bounded and fail-closed. It does not grant arbitrary shell access, does not reboot Windows, and does not modify `Admin.exe`, `Cashier.exe`, SQL data, or `D:\DataBase`.

## Architecture

- The normal signed `ZhiroxAISupervisor.exe` launches a detached hidden companion instance of itself with `--zhirox-zero-touch-watchdog`.
- The companion is singleton-protected by a Windows named mutex.
- The watchdog authenticates to a dedicated read-only Supabase endpoint using the existing Agent ID and Agent secret.
- The endpoint reports the **server-observed** age of `pc_relay_agents.last_seen_at`; reading the endpoint never refreshes that heartbeat timestamp.
- Only after a stale heartbeat is confirmed repeatedly does the companion restart the fixed Windows service `ZhiroxAICore` through Windows SCM APIs.
- The normal Supervisor also ensures bounded SCM crash/non-crash recovery remains configured as `10s -> 30s -> 60s -> NONE`.

## Recovery policy

A Core restart is allowed only when all of these are true:

1. the watchdog endpoint is reachable;
2. Agent authentication succeeds;
3. the server-observed heartbeat age is at least 120 seconds;
4. three consecutive watchdog observations confirm the stale state;
5. at least 10 minutes have elapsed since the prior watchdog restart attempt;
6. fewer than three restart attempts occurred in the previous hour.

The watchdog checks every 20 seconds. A fresh heartbeat resets the consecutive-stale counter immediately.

## Fail-closed behavior

The watchdog **does not restart** the service when:

- the Internet or watchdog endpoint cannot be reached;
- authentication fails;
- the heartbeat age is invalid;
- the heartbeat is fresh;
- the restart cooldown is active;
- the hourly restart budget is exhausted.

If Windows cannot stop the Core service within the bounded timeout, the watchdog does not force-kill the process. It records the failure and remains bounded by the cooldown/restart budget.

## Zero-UI and protection invariants

- Windows GUI subsystem remains required.
- The companion is launched hidden/background-only.
- No `cmd.exe` or PowerShell recovery path is required.
- Windows reboot is not allowed.
- External recovery commands are not configured in SCM.
- The only service target is `ZhiroxAICore`.
- `Admin.exe` and `Cashier.exe` are not stopped, restarted, patched, or modified.
- SQL/database mutation is not part of this recovery module.
- `D:\DataBase` remains protected.

## Observability

The watchdog writes a Zhirox-owned status file:

`zero-touch-watchdog-status.json`

The read-only structured command `pc_os_zero_touch_status` reports watchdog policy/state and SCM recovery configuration.

## Trust and rollout boundary

This feature belongs to **v5.1.2** and must not be merged into `main` while the already-signed v5.1.1 trust-transition artifact is still awaiting production transition. Any source change to the v5.1.1 build path would produce a different binary and invalidate the existing transition signature.

Required release order:

1. complete and verify the signed v5.1.1 transition;
2. confirm v5.1.1 health/rollback gates in production;
3. retire/freeze the v5.1.1 transition build path;
4. merge and build v5.1.2;
5. sign v5.1.2 only with the new post-transition Ed25519 signing key;
6. deploy the dedicated read-only watchdog-state endpoint;
7. perform a controlled v5.1.2 rollout and a real Windows hang-recovery test.

Until those gates pass, this branch is development-only and does not change the live PC.
