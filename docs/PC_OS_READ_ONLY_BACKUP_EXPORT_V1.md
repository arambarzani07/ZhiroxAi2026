# PC OS Read-Only Backup Export v1

## Purpose

Prepare a verified local copy of an existing SQL `.bak` backup for an attended export workflow without modifying the source backup and without enabling background file exfiltration.

## Security contract

- Source scope is restricted to the approved backup root (`D:\Backup` in production).
- Only regular `.bak` files are eligible.
- Symlink escapes are rejected after path resolution.
- Maximum source size is 512 MiB.
- An exact approval phrase is required by the in-agent command boundary: `STAGE READ-ONLY BACKUP EXPORT`.
- The relay action `pc_os_export_prepare_latest` must be treated as a mutation and pass the existing one-time exact-args approval gate.
- The source file is opened read-only and never modified.
- The staged copy is written only under the agent-managed `exports/<random-id>/` directory.
- SHA-256 is calculated while copying, then the staged file is re-hashed; a mismatch fails closed.
- The staged file and manifest are marked read-only where supported.
- The manifest explicitly records `transfer_enabled=false` and `network_upload=false`.
- No HTTP upload, WebRTC file transfer, clipboard transfer, shell execution, PowerShell, or command interpreter is implemented by this feature.

## Latest-backup selection

`pcOSFindLatestSuperMarketBackup` scans only the direct approved backup directory and selects the newest regular file whose name begins with `SuperMarket_backup_` and ends with `.bak`. Directories and symlinks are ignored.

## Staging result

A successful staging operation returns metadata containing:

- export ID
- original source path
- staged local path
- manifest path
- byte size
- SHA-256
- source modification time
- creation time
- read-only / approval / transfer-disabled flags

The staging operation alone does **not** transmit the file off the PC. A separate, explicitly attended transfer mechanism would be required for any future movement of the staged file.

## Production status

This feature branch is development-only. It does not modify the currently deployed v5.1.7 production agent or the existing remote-control canary.