-- Zhirox PC OS v5.1.1 next-trust Ed25519 KMS bootstrap.
--
-- Purpose:
--   * Generate the next Ed25519 keypair entirely inside PostgreSQL/pgsodium.
--   * Persist ONLY the private signing material through pgsodium's encrypted
--     raw-key path (parent managed AEAD key + nonce).
--   * Persist the public key separately as non-secret metadata.
--   * Expose only service-role functions that return the public key.
--
-- Non-goals:
--   * This migration does NOT create a general signing oracle.
--   * It does NOT read Supabase Vault decrypted secrets.
--   * It does NOT promote/release/roll out any PC artifact.

create extension if not exists pgsodium;

create schema if not exists zhirox_kms;

revoke all on schema zhirox_kms from public;
revoke all on schema zhirox_kms from anon;
revoke all on schema zhirox_kms from authenticated;
grant usage on schema zhirox_kms to service_role;

create table if not exists zhirox_kms.ed25519_key_metadata (
    key_name text primary key,
    pgsodium_key_id uuid not null unique,
    public_key_hex text not null unique,
    purpose text not null,
    created_at timestamptz not null default now(),
    constraint ed25519_public_key_hex_ck
        check (public_key_hex ~ '^[0-9a-f]{64}$')
);

revoke all on table zhirox_kms.ed25519_key_metadata from public;
revoke all on table zhirox_kms.ed25519_key_metadata from anon;
revoke all on table zhirox_kms.ed25519_key_metadata from authenticated;
revoke all on table zhirox_kms.ed25519_key_metadata from service_role;

create or replace function zhirox_kms.init_v511_next_signing_key()
returns text
language plpgsql
security definer
set search_path = pg_catalog, pgsodium, zhirox_kms
as $$
declare
    existing_public text;
    kp pgsodium.crypto_sign_keypair;
    stored_key pgsodium.valid_key;
    public_hex text;
begin
    select m.public_key_hex
      into existing_public
      from zhirox_kms.ed25519_key_metadata as m
     where m.key_name = 'v5.1.1-next-ed25519';

    if existing_public is not null then
        return existing_public;
    end if;

    -- libsodium/pgsodium produces a 32-byte Ed25519 public key and a 64-byte
    -- secret key. The secret value exists only in this transaction's memory.
    kp := pgsodium.crypto_sign_new_keypair();
    public_hex := lower(encode((kp).public, 'hex'));

    if octet_length((kp).public) <> 32 or octet_length((kp).secret) <> 64 then
        raise exception 'unexpected Ed25519 keypair size';
    end if;

    -- pgsodium.create_key(raw_key => ...) encrypts raw_key through its
    -- parent managed AEAD key before persistence. No plaintext private key is
    -- stored in our metadata table or returned by this function.
    stored_key := pgsodium.create_key(
        key_type => 'aead-det'::pgsodium.key_type,
        name => 'zhirox-v511-next-ed25519-secret',
        raw_key => (kp).secret,
        associated_data => 'zhirox:v5.1.1:next-ed25519'
    );

    insert into zhirox_kms.ed25519_key_metadata(
        key_name,
        pgsodium_key_id,
        public_key_hex,
        purpose
    ) values (
        'v5.1.1-next-ed25519',
        stored_key.id,
        public_hex,
        'Next trust anchor after the v5.1.0 -> v5.1.1 transition; private key encrypted in pgsodium key store'
    );

    return public_hex;
end;
$$;

revoke all on function zhirox_kms.init_v511_next_signing_key() from public;
revoke all on function zhirox_kms.init_v511_next_signing_key() from anon;
revoke all on function zhirox_kms.init_v511_next_signing_key() from authenticated;
grant execute on function zhirox_kms.init_v511_next_signing_key() to service_role;

create or replace function zhirox_kms.get_v511_next_public_key()
returns text
language sql
stable
security definer
set search_path = pg_catalog, zhirox_kms
as $$
    select m.public_key_hex
      from zhirox_kms.ed25519_key_metadata as m
     where m.key_name = 'v5.1.1-next-ed25519'
     limit 1
$$;

revoke all on function zhirox_kms.get_v511_next_public_key() from public;
revoke all on function zhirox_kms.get_v511_next_public_key() from anon;
revoke all on function zhirox_kms.get_v511_next_public_key() from authenticated;
grant execute on function zhirox_kms.get_v511_next_public_key() to service_role;

comment on function zhirox_kms.init_v511_next_signing_key() is
'Idempotently creates the v5.1.1 next-trust Ed25519 key. Returns only the public key; private key is persisted only through pgsodium encrypted raw-key storage.';

comment on function zhirox_kms.get_v511_next_public_key() is
'Returns only the public half of the v5.1.1 next-trust Ed25519 key.';
