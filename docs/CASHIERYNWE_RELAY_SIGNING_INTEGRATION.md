# Cashierynwe signing contract — relay compatibility gate

## Source status (NOT DEPLOYED)

The public ZhiroxAi2026 source contains a strictly allowlisted
`cashierynwe_sign_release` action. The private Cashierynwe repository
contains the real, certificate-store signing PowerShell script and a
reference fail-closed integration adapter.

**Correct one-time command arguments**, enforced by the server and Go
policy (reject additional fields):

- `commit_sha`: exactly 40 lowercase hexadecimal characters, matching
  the approved HEAD of private `arambarzani07/Cashierynwe` `main`
- `tag`: exactly `v1.0.1` for this one-time release
- `approval_id`: a canonical UUID bound to an actual separately approved,
  unexpired, single-use owner approval and the command ID

The connector request must include a top-level `approval_id` exactly equal
to `args.approval_id`. The service-role approval RPC must atomically consume
an owner approval whose `args` JSONB is **exactly** the command `args`
(including the UUID), and insert that command in the same transaction.
A UUID alone or a phrase such as `SIGN_CASHIERYNWE_V1_0_1_ON_OWNER_PC`
is not authorization; only an approval issued through an authenticated owner
approval mechanism qualifies. Never allow an untrusted connector/client to
mint its own approvals with the service-role credentials.

After queue delivery to a compatible, trusted agent, the typed Go handler
passes three pinned PowerShell parameters:
`-ExpectedCommit`, `-ExpectedTag`, and `-RelayApprovalId`, plus the
hardcoded `-UseWindowsCertificateStore` switch. No private key is exported.

## Hard production blockers

1. The observed installed agent is **v5.1.13**, while the server explicitly
   requires **v5.1.14 or newer** for this action. The current v5.1.6 source
   build in GitHub Actions is only a compile test and is **not** a candidate
   to replace the newer installed agent.
2. The signed agent self-update chain, trusted signing identity and rollback
   must be validated for a v5.1.14+ build. Never publish an unsigned build,
   lower the version gate or overwrite the existing update channel.
3. The running Windows agent must have access to the logged-in user's
   certificate in `Cert:\CurrentUser\My`, local checkout, Node and authenticated
   GitHub CLI. Services running as SYSTEM will normally have a different
   `CurrentUser` certificate store and profile. Do **not** impersonate a user
   or export their PFX to bypass this restriction.
4. A full real Windows signed build plus a signed-update A→B test is
   necessary before claiming that production Auto-Update is active.

This repo change updates **source only**. It does not deploy the Supabase
function, mutate database records, activate remote command execution, install
a new PC agent, publish a Windows release, upload private keys or change
market/DVR services.
