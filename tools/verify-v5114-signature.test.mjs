import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash, generateKeyPairSync, randomBytes, sign } from 'node:crypto';
import { readFileSync, writeFileSync, symlinkSync, unlinkSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { verifyCandidate, PIN } from './verify-v5114-signature.mjs';

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'zhirox-v5114-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const pub = publicKey.export({ type: 'spki', format: 'der' }).subarray(-32).toString('hex');
  const data = randomBytes(4096), sha = createHash('sha256').update(data).digest('hex');
  const pins = { ...PIN, size: data.length, sha256: sha, publicKey: pub };
  const manifest = join(root, 'manifest.json'), binary = join(root, 'ZhiroxAISupervisor.exe');
  const signature = join(root, 'signature.b64');
  const m = {
    version: pins.version, candidate_kind:'unsigned_windows_relay', status:'build_verified_signing_pending',
    publishable:false, live_update_channel_unchanged:true, source_commit:pins.source,
    inherited_signed_baseline_commit:pins.baseline, workflow_run_id:pins.run,
    workflow_artifact_id:pins.artifact, artifact_name:'Zhirox-PC-OS-v5.1.14-' + pins.source,
    filename:pins.name, sha256:sha, size_bytes:data.length, signing_algorithm:'Ed25519',
    pinned_update_public_key_hex:pub, canonical_signing_message:'5.1.14|' + sha + '|' + data.length,
    signature_b64:null, windows_ci_passed:true, candidate_provenance_verified:true,
    private_signing_key_exported:false, production_agent_expected_before_rollout:'5.1.13',
    rollout_allowed:false,
    blocked_reasons:['exact_artifact_ed25519_signature_not_created','durable_authenticated_artifact_transport_not_staged',
      'one_time_owner_approval_and_canary_rollback_not_verified','windows_user_currentuser_certificate_and_github_cli_context_not_verified'],
    note:'Not deployable',
  };
  const persist=()=>writeFileSync(manifest, JSON.stringify(m));
  persist(); writeFileSync(binary,data);
  writeFileSync(signature,sign(null,Buffer.from(m.canonical_signing_message),privateKey).toString('base64'));
  return { m, persist, pins, manifest,binary,signature };
}
test('ephemeral Ed25519 verification succeeds, without authorizing rollout',t=>{
 const f=fixture(t);const result=verifyCandidate(f,f.pins);
 assert.equal(result.status,'signature_verified_only');assert.equal(result.rollout_allowed,false);
});
test('real production key and digest pins cannot be replaced by a test key',t=>{
 const f=fixture(t);
 assert.throws(()=>verifyCandidate(f),/manifest_production_gate_failed/);
});
test('reject binary tampering',t=>{
 const f=fixture(t);const b=readFileSync(f.binary);b[0]^=1;writeFileSync(f.binary,b);
 assert.throws(()=>verifyCandidate(f,f.pins),/binary_sha256_mismatch/);
});
test('reject forged signature',t=>{
 const f=fixture(t);writeFileSync(f.signature,Buffer.alloc(64).toString('base64'));
 assert.throws(()=>verifyCandidate(f,f.pins),/ed25519_signature_invalid/);
});
test('reject changed rollout flag and injected secrets',t=>{
 const f=fixture(t);
 f.m.rollout_allowed=true;f.persist();assert.throws(()=>verifyCandidate(f,f.pins),/manifest_production_gate_failed/);
 f.m.rollout_allowed=false;f.m.private_key='not allowed';f.persist();
 assert.throws(()=>verifyCandidate(f,f.pins),/manifest_schema_modified/);
});
test('reject malformed signature and symlinks',t=>{
 const f=fixture(t);writeFileSync(f.signature,'%%%');
 assert.throws(()=>verifyCandidate(f,f.pins),/signature_base64_invalid/);
 unlinkSync(f.signature);symlinkSync(f.binary,f.signature);
 assert.throws(()=>verifyCandidate(f,f.pins),/file_not_regular_or_symlink/);
});
