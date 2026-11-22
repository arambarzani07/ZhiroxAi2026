# Immutable Cashierynwe installer stager — isolated library (source only)

## State

No production installer launch, no Windows PC change, no Supabase Edge deploy.
The current store service is still PC Relay 5.1.15. This library is NOT
wired into any live relay command, and it cannot execute an EXE.

## Exact artifact

- App: Cashierynwe POS 1.0.10
- Asset filename: `Cashierynwe-Setup-1.0.10-x64.exe`
- SHA256: `e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b`
- Size: 158128464 bytes
- Agent: `Server-c24b5c0f`
- Control-plane endpoint: the existing Supabase project, exact agent-bound
  `/signed-app-download` route defined in un-deployed v23 source

## Protocol

1. The service receives an owner-approved command; the approved command must
   have been consumed by the authenticated one-shot approval RPC.
2. With its **existing Relay secret**, it requests only the fixed Supabase
   HTTPS route, using the exact UUID in `approval_id`.
3. The function verifies command+owner approval+agent version and returns the
   limited, ephemeral location of the fixed private GitHub asset.
4. The stager checks exact JSON fields, maximum 4 KiB control response, and
   the fixed GitHub CDN HTTPS host. Any redirect or unknown field fails.
5. The Relay secret is **never** sent to the GitHub CDN.
6. The stager streams at most 158128465 bytes to a temporary file in the
   trusted staging directory, hashes SHA-256 while streaming, and rejects
   wrong size, hash, HTTP status, compression or redirects.
7. It can only atomically publish a single exact filename into the staging
   directory, with no overwrite of existing artifacts.
8. Execution is **not** part of this module.

## Required preflight / never bypass

The directory supplied by a prospective service caller MUST be
`C:\ProgramData\ZhiroxAI\ApprovedInstallers`, created and permissioned
by an administrator / SYSTEM. This is a **precondition**, not guaranteed by
the independent module. Before wiring in the service, inspect the NTFS DACL
and ownership: ordinary Users, Authenticated Users and Everyone must not
have effective Write/Modify/Delete/DeleteChild/WriteDAC access to the
directory, request tickets or staged EXE; reject reparse-point parents.
Do not trust Unix mode bits as an equivalent to a Windows ACL audit.

Next, introduce a protected service-side request-ticket emitter and a
local-user broker installer handoff with an independently authenticated
one-time approval, status handshake, Windows owner consent and rollback.
The separate Windows Broker remains unsigned and disabled and must not
be enabled merely because its source compiles.

## Release gates

- Full Windows and Linux Go tests and `go vet` for this module.
- Signed Relay v5.1.16 proof from owner-only GitHub workflow, using the
  existing pinned production Ed25519 trust anchor.
- Controlled Supabase v23 deployment, correct scoped GitHub read-only token.
- Approved installer folder ACL tests, staging operation smoke tests.
- Separate signed Windows broker and actual user-session canary.
- No interruption or alteration of legacy `D:\DataBase\Cashier.exe` and
  `D:\DataBase\Admin.exe` or market database.

Do not describe the Windows PC as updated or Cashierynwe as installed
until physical machine telemetry proves the exact version and app launch.
