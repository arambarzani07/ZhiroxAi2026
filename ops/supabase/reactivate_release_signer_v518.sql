
CREATE OR REPLACE FUNCTION public.zhirox_release_signer_status()
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public, vault, pgsodium
AS $$
DECLARE seed text; kp pgsodium.crypto_sign_keypair; sig bytea; msg bytea := convert_to('zhirox-release-signer-self-test-v1','UTF8');
BEGIN
 SELECT decrypted_secret INTO seed FROM vault.decrypted_secrets WHERE name='ZHIROX_V511_NEXT_ED25519_SEED_B64' ORDER BY created_at DESC LIMIT 1;
 IF seed IS NULL THEN RETURN jsonb_build_object('ready',false,'error','existing_key_missing'); END IF;
 kp := pgsodium.crypto_sign_seed_new_keypair(decode(seed,'base64'));
 IF encode(kp.public,'hex') <> '19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143' THEN
 RETURN jsonb_build_object('ready',false,'error','trust_anchor_mismatch'); END IF;
 sig := pgsodium.crypto_sign_detached(msg,kp.secret);
 RETURN jsonb_build_object('ready',pgsodium.crypto_sign_verify_detached(sig,msg,kp.public),'public_key_hex',encode(kp.public,'hex'),'allowed_version','5.1.8','automatic_promotion',false);
END $$;
REVOKE ALL ON FUNCTION public.zhirox_release_signer_status() FROM PUBLIC,anon,authenticated;
GRANT EXECUTE ON FUNCTION public.zhirox_release_signer_status() TO service_role;

CREATE OR REPLACE FUNCTION public.zhirox_sign_v518_release(p_release_id uuid,p_dry_run boolean DEFAULT true)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER
SET search_path = pg_catalog, public, vault, pgsodium
AS $$
DECLARE r public.pc_update_releases%ROWTYPE; seed text; kp pgsodium.crypto_sign_keypair; sig bytea; msg bytea;
BEGIN
 SELECT * INTO r FROM public.pc_update_releases WHERE id=p_release_id FOR UPDATE;
 IF NOT FOUND THEN RETURN jsonb_build_object('ok',false,'error','release_not_staged'); END IF;
 IF r.version IS DISTINCT FROM '5.1.8'
 OR r.source_commit IS DISTINCT FROM 'af8394e8583c8cca72ee389d8f5af1a2446d63d8'
 OR r.status IS DISTINCT FROM 'staged'
 OR r.publishable IS DISTINCT FROM false
 OR r.signature_required IS DISTINCT FROM true
 OR r.sha256 IS NULL OR r.sha256 !~ '^[0-9a-f]{64}$'
 OR r.size_bytes IS NULL OR r.size_bytes <= 0
 OR coalesce(r.artifact_bucket,'')='' OR coalesce(r.artifact_path,'')=''
 THEN RETURN jsonb_build_object('ok',false,'error','release_gate_rejected'); END IF;
 SELECT decrypted_secret INTO seed FROM vault.decrypted_secrets WHERE name='ZHIROX_V511_NEXT_ED25519_SEED_B64' ORDER BY created_at DESC LIMIT 1;
 IF seed IS NULL THEN RETURN jsonb_build_object('ok',false,'error','existing_key_missing'); END IF;
 kp := pgsodium.crypto_sign_seed_new_keypair(decode(seed,'base64'));
 IF encode(kp.public,'hex') <> '19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143' THEN
 RETURN jsonb_build_object('ok',false,'error','trust_anchor_mismatch'); END IF;
 msg := convert_to(r.version||'|'||r.sha256||'|'||r.size_bytes::text,'UTF8');
 sig := pgsodium.crypto_sign_detached(msg,kp.secret);
 IF NOT pgsodium.crypto_sign_verify_detached(sig,msg,kp.public) THEN RAISE EXCEPTION 'signature_self_verification_failed'; END IF;
 IF NOT p_dry_run THEN
 UPDATE public.pc_update_releases SET signature_b64=encode(sig,'base64'),signature_algorithm='Ed25519',public_key_hex=encode(kp.public,'hex')
 WHERE id=r.id;
 END IF;
 RETURN jsonb_build_object('ok',true,'dry_run',p_dry_run,'signature_verified',true,'release_id',r.id,'version',r.version,'publishable',false);
END $$;
REVOKE ALL ON FUNCTION public.zhirox_sign_v518_release(uuid,boolean) FROM PUBLIC,anon,authenticated;
GRANT EXECUTE ON FUNCTION public.zhirox_sign_v518_release(uuid,boolean) TO service_role;
