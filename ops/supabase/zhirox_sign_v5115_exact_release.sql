-- Source-only, fixed-release signing capability.
-- The function MAY sign only the verified v5.1.15 Windows artifact, and does
-- NOT set publishable=true, change an update channel, or execute on a PC.
-- This SQL references an existing Supabase Vault secret by NAME only.
-- It never reveals, returns, exports, or logs the Ed25519 private seed.
CREATE OR REPLACE FUNCTION public.zhirox_sign_v5115_exact_release(
  p_release_id uuid,
  p_dry_run boolean DEFAULT true
) RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path TO pg_catalog, public, vault, pgsodium
AS $function$
DECLARE
  r public.pc_update_releases%ROWTYPE;
  seed text;
  kp pgsodium.crypto_sign_keypair;
  sig bytea;
  msg bytea;
BEGIN
  IF p_release_id IS NULL OR p_dry_run IS NULL THEN
    RETURN jsonb_build_object('ok', false, 'error', 'explicit_arguments_required');
  END IF;
  SELECT * INTO r FROM public.pc_update_releases
   WHERE id = p_release_id FOR UPDATE;
  IF NOT FOUND THEN
    RETURN jsonb_build_object('ok', false, 'error', 'release_not_staged');
  END IF;

  IF r.version IS DISTINCT FROM '5.1.15'
    OR r.source_commit IS DISTINCT FROM '7185cd9625511eb48cb2974c5ed9920662d7150a'
    OR r.sha256 IS DISTINCT FROM '74cffc3eafab23cdead7b7d25df65cdb8d950ca0774a6b55f7d4c6a3dc21048b'
    OR r.size_bytes IS DISTINCT FROM 20027904
    OR r.status IS DISTINCT FROM 'staged'
    OR r.publishable IS DISTINCT FROM false
    OR r.signature_required IS DISTINCT FROM true
    OR r.signature_b64 IS NOT NULL
    OR r.pinned_trust_anchor IS DISTINCT FROM true
    OR r.rollback_updater_embedded IS DISTINCT FROM true
    OR r.artifact_encoding IS DISTINCT FROM 'identity'
    OR r.artifact_bucket IS DISTINCT FROM 'github-release'
    OR r.artifact_path IS DISTINCT FROM 'https://github.com/arambarzani07/ZhiroxAi2026/releases/download/v5.1.15-candidate-7185cd96/ZhiroxAISupervisor.exe'
    OR r.build_provenance ->> 'windows_build_passed' IS DISTINCT FROM 'true'
    OR r.build_provenance ->> 'artifact_readback_verified' IS DISTINCT FROM 'true'
    OR r.build_provenance ->> 'source_run_id' IS DISTINCT FROM '43721853659'
    OR r.build_provenance ->> 'artifact_id' IS DISTINCT FROM '14165021306'
    OR r.build_provenance ->> 'github_release_readback_run_id' IS DISTINCT FROM '43723286774'
    OR r.build_provenance ->> 'rollout_allowed' IS DISTINCT FROM 'false'
    OR r.build_provenance ->> 'remote_input_enabled' IS DISTINCT FROM 'false'
  THEN
    RETURN jsonb_build_object('ok', false, 'error', 'exact_release_gate_rejected');
  END IF;

  SELECT decrypted_secret INTO seed
    FROM vault.decrypted_secrets
   WHERE name = 'ZHIROX_V511_NEXT_ED25519_SEED_B64'
   ORDER BY created_at DESC LIMIT 1;
  IF seed IS NULL THEN
    RETURN jsonb_build_object('ok', false, 'error', 'trusted_key_missing');
  END IF;
  kp := pgsodium.crypto_sign_seed_new_keypair(decode(seed, 'base64'));
  IF encode(kp.public, 'hex') <> '19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143' THEN
    RETURN jsonb_build_object('ok', false, 'error', 'pinned_trust_anchor_mismatch');
  END IF;

  msg := convert_to(r.version || '|' || r.sha256 || '|' || r.size_bytes::text, 'UTF8');
  sig := pgsodium.crypto_sign_detached(msg, kp.secret);
  IF NOT pgsodium.crypto_sign_verify_detached(sig, msg, kp.public) THEN
    RAISE EXCEPTION 'signature_self_verification_failed';
  END IF;

  IF NOT p_dry_run THEN
    UPDATE public.pc_update_releases
       SET signature_b64 = encode(sig, 'base64'),
           signature_algorithm = 'Ed25519',
           public_key_hex = encode(kp.public, 'hex')
     WHERE id = r.id AND status = 'staged' AND publishable = false AND signature_b64 IS NULL;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'signature_write_conflict';
    END IF;
  END IF;

  RETURN jsonb_build_object(
    'ok', true,
    'version', '5.1.15',
    'release_id', r.id,
    'dry_run', p_dry_run,
    'signature_verified', true,
    'publishable', false,
    'promotion_performed', false,
    'key_exported', false
  );
END
$function$;

-- Security-definer functions must NOT remain callable via PostgREST
-- anonymous/authenticated PUBLIC privileges (Postgres default).
REVOKE ALL ON FUNCTION public.zhirox_sign_v5115_exact_release(uuid,boolean)
  FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.zhirox_sign_v5115_exact_release(uuid,boolean)
  TO service_role;
