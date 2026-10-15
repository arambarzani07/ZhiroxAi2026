# Remote Input v2 Contract

## Scope

Remote Input v2 is an attended-control security boundary for screen + mouse + keyboard sessions. The production Windows release remains screen-only until a separately reviewed execution phase is explicitly approved, built, tested, signed, and rolled out.

## Zero-touch maintenance goal

Installation, Windows startup, signed updates, recovery, rollback, health checks, and reconnect behavior are designed to become zero-touch after the initial trusted setup. This operational automation is separate from active mouse/keyboard control.

Actual remote input remains user-visible while active: the attended runtime requires a live visible session indicator and the authorization chain fails closed if that runtime disappears. Trusted enrollment may reduce repetitive setup work, but it does not create a hidden-control path.

## Security invariants

- A trusted controller alone is not sufficient to execute input.
- Mouse/keyboard authorization requires a fresh local interactive approval bound to the exact controller, agent, session, fingerprint, and requested capabilities.
- Local approval is short-lived and the shorter upstream expiry always wins.
- The visible active-control indicator must remain live for the attended runtime.
- Every WebRTC control message is revalidated against the current attended authorization gate.
- Revocation, expiry, emergency stop, session replacement, capability drift, indicator exit, or runtime close fail closed.
- Screen-only sessions cannot obtain mouse/keyboard permission.
- Clipboard, file transfer, audio, arbitrary shell execution, UAC/secure-desktop bypass, and hidden unattended control are outside this phase.

## Phase 2.3 activation boundary

Immediately before any future reviewed input-execution implementation, the live attended runtime can mint an `AttendedActivationReadiness` proof with a maximum TTL of 15 seconds. Each readiness proof has a cryptographically random `ReadinessID` and is bound to the same approval, controller, agent, session, fingerprint, capabilities, and arbiter generation.

A process-local `AttendedActivationConsumer` can consume a valid readiness proof exactly once. Concurrent replay attempts are serialized; only one consumption can succeed. Successful consumption yields an immutable `AttendedActivationGrant` that remains non-executing and must continue to validate against the live attended runtime until expiry.

The following events invalidate readiness/grants or prevent further use:

- local approval expiry
- trusted permit expiry or revocation
- emergency stop
- controller/session replacement
- capability mismatch or drift
- visible indicator exit
- runtime close/cancellation
- readiness expiry
- replay of an already consumed readiness proof

## Explicit non-execution boundary

Phase 2.3 does **not**:

- call `ControlDispatcher.Activate()` from production code
- implement Windows `SendInput`, `mouse_event`, or `keybd_event`
- inject mouse or keyboard events into the operating system
- enable unattended or zero-awareness remote control
- modify the live screen-only production release

The activation grant exists only as a reviewable policy object for a future separately gated execution implementation.