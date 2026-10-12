# Zhirox PC OS v5.1.1 — Next Trust Key Initialization Runbook

This runbook is for an authorized production operator. It exists because the ChatGPT/Supabase connector safety boundary intentionally blocks reading decrypted Vault secrets or handling raw private-key material.

## Preconditions

- Production project: `vllwundrtrgdqmkqbduy`.
- `pgsodium` and `pgcrypto` are installed.
- Vault contains exactly one secret named `ZHIROX_V511_NEXT_ED25519_SEED_B64`.
- Do not print, copy, export, log, or paste the decrypted seed anywhere.
- Do not place the v5.1.1 next private seed in GitHub Actions or repository files.
- The only value allowed to leave the database is the 32-byte Ed25519 **public** key encoded as 64 lowercase hex characters.

## Derive the public key in Supabase SQL Editor

Run this only from an authenticated administrator session in Supabase Dashboard SQL Editor. The query returns only the public key; the seed remains inside PostgreSQL execution memory.

```sql
with seed_row as (
  select decode(decrypted_secret, 'base64') as seed
  from vault.decrypted_secrets
  where name = 'ZHIROX_V511_NEXT_ED25519_SEED_B64'
),
validated as (
  select seed
  from seed_row
  where octet_length(seed) = 32
),
kp as (
  select pgsodium.crypto_sign_seed_new_keypair(seed) as pair
  from validated
)
select lower(encode((pair).public, 'hex')) as public_key_hex
from kp;
```

Fail closed unless the result is exactly one row matching `^[0-9a-f]{64}$`.

## Record only the public key

Create/update the repository file:

`release-gates/v5.1.1-next-public-key.hex`

The file must contain exactly the 64-character lowercase public key plus an optional trailing newline. It is public metadata, not a secret.

Never commit the seed, the 64-byte Ed25519 private key, Vault decrypted output, SQL result containing private material, or any temporary export.

## Production build contract

On `main`, `.github/workflows/build-v511.yml` must:

- require `release-gates/v5.1.1-next-public-key.hex`;
- reject malformed public keys;
- reject any supplied `ZHIROX_V511_NEXT_ED25519_SEED_B64`;
- require the v5.1.0 transition signer to derive to the deployed production trust key `3cdf94418d5a43734c49238268ed7512481c97faa691a0cd053eb9f74fbe10bd`;
- require transition signer and next trust key to be different;
- verify SHA-256, artifact size, Ed25519 transition signature, and trust-transition metadata before staging.

## Stop conditions

Stop immediately and do not build or roll out if any of these occur:

- the Vault secret is missing or duplicated;
- decoded seed length is not 32 bytes;
- public key output is not exactly one 64-hex value;
- next public key equals the v5.1.0 production trust key;
- any private material appears in logs, terminal history, repository files, artifacts, or CI environment variables;
- required CI, Zero-UI, Windows rollback, or health-gate checks fail.

## After the public key is recorded

1. Keep PR #6 in Draft until CI on the public-key commit passes.
2. Merge only after all required safety checks are green.
3. Build/sign/stage the v5.1.1 transition candidate through the protected candidate PR path.
4. Do not change `channel/*` or begin agent rollout during staging.
5. Production rollout requires separate explicit authorization, a fresh live preflight, known-good v5.1.0 rollback, and the 90-second health/hash/heartbeat gate.
