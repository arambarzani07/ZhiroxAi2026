// Exact v5.1.14 Ed25519 detached-signature verifier. No signing key, DB access,
// remote calls, release mutation or automatic deployment. Node.js built-ins only.
import { createHash, createPublicKey, verify } from 'node:crypto';
import { readFileSync, lstatSync } from 'node:fs';
import { basename } from 'node:path';
import { pathToFileURL } from 'node:url';

export const PIN = Object.freeze({
  version: '5.1.14',
  sha256: '371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa',
  size: 20025856,
  publicKey: '19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143',
  source: '091c7e23ee50807a5c721a45c7b1c7c2267695c0',
  baseline: '46b208507293d677428a25b146599eb537c532b9',
  run: 43708666560,
  artifact: 14159935571,
  name: 'ZhiroxAISupervisor.exe',
});
const REQUIRED = [
  'version','candidate_kind','status','publishable','live_update_channel_unchanged',
  'source_commit','inherited_signed_baseline_commit','workflow_run_id',
  'workflow_artifact_id','artifact_name','filename','sha256','size_bytes',
  'signing_algorithm','pinned_update_public_key_hex','canonical_signing_message',
  'signature_b64','windows_ci_passed','candidate_provenance_verified',
  'private_signing_key_exported','production_agent_expected_before_rollout',
  'rollout_allowed','blocked_reasons','note',
].sort();
const BLOCKED = [
  'exact_artifact_ed25519_signature_not_created',
  'durable_authenticated_artifact_transport_not_staged',
  'one_time_owner_approval_and_canary_rollback_not_verified',
  'windows_user_currentuser_certificate_and_github_cli_context_not_verified',
];
function check(ok, message) { if (!ok) throw new Error(message); }
function regular(path, maxBytes) {
  const stat = lstatSync(path);
  check(stat.isFile() && !stat.isSymbolicLink(), 'file_not_regular_or_symlink');
  check(stat.size > 0 && stat.size <= maxBytes, 'file_size_out_of_bounds');
  return stat;
}
function exactManifest(file) {
  regular(file, 32 * 1024);
  const obj = JSON.parse(readFileSync(file, 'utf8'));
  check(obj && typeof obj === 'object' && !Array.isArray(obj), 'manifest_invalid');
  check(JSON.stringify(Object.keys(obj).sort()) === JSON.stringify(REQUIRED), 'manifest_schema_modified');
  return obj;
}
function decodeSignature(file) {
  regular(file, 512);
  const body = readFileSync(file, 'utf8').trim();
  check(/^[A-Za-z0-9+/]{86}==$/.test(body), 'signature_base64_invalid');
  const bytes = Buffer.from(body, 'base64');
  check(bytes.length === 64 && bytes.toString('base64') === body, 'signature_encoding_invalid');
  return bytes;
}

/** Test-only custom pins can validate ephemeral test signatures. The CLI uses immutable PIN. */
export function verifyCandidate({ manifest, binary, signature }, pins = PIN) {
  const m = exactManifest(manifest);
  const canonical = pins.version + '|' + pins.sha256 + '|' + pins.size;
  check(m.version === pins.version && m.candidate_kind === 'unsigned_windows_relay' &&
    m.status === 'build_verified_signing_pending' && m.publishable === false &&
    m.live_update_channel_unchanged === true && m.source_commit === pins.source &&
    m.inherited_signed_baseline_commit === pins.baseline &&
    m.workflow_run_id === pins.run && m.workflow_artifact_id === pins.artifact &&
    m.artifact_name === 'Zhirox-PC-OS-v5.1.14-' + pins.source &&
    m.filename === pins.name && m.sha256 === pins.sha256 &&
    m.size_bytes === pins.size && m.signing_algorithm === 'Ed25519' &&
    m.pinned_update_public_key_hex === pins.publicKey &&
    m.canonical_signing_message === canonical && m.signature_b64 === null &&
    m.windows_ci_passed === true && m.candidate_provenance_verified === true &&
    m.private_signing_key_exported === false &&
    m.production_agent_expected_before_rollout === '5.1.13' &&
    m.rollout_allowed === false && m.note &&
    JSON.stringify(m.blocked_reasons) === JSON.stringify(BLOCKED),
    'manifest_production_gate_failed');
  check(basename(binary) === pins.name, 'binary_filename_changed');
  const stat = regular(binary, 80 * 1024 * 1024);
  check(stat.size === pins.size, 'binary_size_mismatch');
  const sha = createHash('sha256').update(readFileSync(binary)).digest('hex');
  check(sha === pins.sha256, 'binary_sha256_mismatch');
  check(/^[0-9a-f]{64}$/.test(pins.publicKey), 'pinned_public_key_invalid');
  const spki = Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), Buffer.from(pins.publicKey,'hex')]);
  const publicKey = createPublicKey({ key: spki, format: 'der', type: 'spki' });
  const authentic = verify(null, Buffer.from(canonical, 'utf8'), publicKey, decodeSignature(signature));
  check(authentic, 'ed25519_signature_invalid');
  return Object.freeze({ status: 'signature_verified_only', version: pins.version,
    sha256: pins.sha256, rollout_allowed: false });
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const input = process.argv.slice(2);
  if (input.length !== 6 || input[0] !== '--manifest' || input[2] !== '--binary' || input[4] !== '--signature') {
    console.error('usage: node tools/verify-v5114-signature.mjs --manifest JSON --binary EXE --signature BASE64');
    process.exitCode = 2;
  } else {
    try {
      const result = verifyCandidate({ manifest: input[1], binary: input[3], signature: input[5] });
      console.log(JSON.stringify(result));
      console.log('Signature is authentic; this does NOT authorize release or PC deployment.');
    } catch {
      console.error('REJECTED: signed-release provenance, file, or Ed25519 verification failed');
      process.exitCode = 1;
    }
  }
}
