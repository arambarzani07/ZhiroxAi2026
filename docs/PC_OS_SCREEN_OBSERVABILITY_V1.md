# Zhirox PC OS — Screen Observability v1

## Purpose

Add two strictly read-only, structured remote observations to the Zhirox PC Supervisor:

- `pc_os_active_window` — returns foreground window title, PID, process name/path, and capture timestamp.
- `pc_os_screenshot` — captures the primary interactive display on demand and returns a bounded JPEG payload.

## Privacy and safety boundary

- No continuous capture.
- No background screenshot timer.
- No keyboard capture or key logging.
- No mouse capture or input injection.
- No clipboard capture.
- No microphone or camera access.
- No shell, cmd.exe, or PowerShell execution.
- No remote desktop control.
- No mutation of Admin/Cashier, SQL Server, `D:\DataBase`, Hikvision/DVR, or other business data.
- Screenshot capture is primary-display-only in v1.
- JPEG payload is bounded below the relay result limit; current hard maximum is 900,000 encoded-image bytes before base64 transport.
- Capture is transient; the helper returns the observation over an inherited pipe and does not persist screenshots to disk.

## Windows service/session model

The Supervisor normally runs as a Windows service and therefore may be isolated from the interactive user desktop. For an explicit observation command only, the service launches the same signed Supervisor binary into the active console session with a fixed helper flag and a fixed mode allowlist. The helper communicates back through an inherited anonymous pipe, exits immediately after the single observation, and cannot accept arbitrary executable paths or shell commands.

## Screenshot parameters

Optional command args:

- `max_width`: 640–1920, default 1280.
- `quality`: 25–70, default 45.

If a JPEG still exceeds the approved payload size, the implementation automatically retries at smaller dimensions/lower quality and fails closed if it cannot meet the limit.

## Release boundary

This feature branch does not replace the immutable production v5.1.2 binary. It must pass policy tests, safety invariants, Windows compile checks, and then be incorporated into a new signed release before production enablement. The relay allowlist should only be extended together with that signed agent rollout.
