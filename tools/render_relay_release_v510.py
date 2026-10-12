#!/usr/bin/env python3
"""Render a v5.1.0 production relay source from a verified signed candidate.

This tool never signs anything and never deploys anything. It consumes the
already-signed release-candidates/v5.1.0/metadata.json and injects the exact
signed manifest fields into the hardened relay-v15 source. The resulting file
can then be reviewed and deployed as the final, explicit rollout gate.
"""

from __future__ import annotations

import base64
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CANDIDATE = ROOT / "release-candidates" / "v5.1.0"
BASE_RELAY = ROOT / "ops" / "supabase" / "zhirox-pc-relay-v15.ts"
OUTPUT = ROOT / "ops" / "supabase" / "zhirox-pc-relay-v510-release.ts"
UPDATE_BINARY_ENDPOINT = (
    "https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/"
    "zhirox-pc-relay/api/update/binary"
)


def fail(message: str) -> None:
    raise SystemExit(message)


def load_candidate() -> dict:
    required = [
        CANDIDATE / "ZhiroxAISupervisor.exe",
        CANDIDATE / "ZhiroxAISupervisor.exe.sha256",
        CANDIDATE / "metadata.json",
        CANDIDATE / "update-public-key.hex",
        CANDIDATE / "provenance.json",
    ]
    missing = [str(p.relative_to(ROOT)) for p in required if not p.is_file()]
    if missing:
        fail("signed candidate is incomplete: " + ", ".join(missing))

    meta = json.loads((CANDIDATE / "metadata.json").read_text())
    provenance = json.loads((CANDIDATE / "provenance.json").read_text())

    if meta.get("version") != "5.1.0":
        fail("candidate version is not 5.1.0")
    if meta.get("file") != "ZhiroxAISupervisor.exe":
        fail("candidate file identity mismatch")
    if meta.get("algorithm") != "Ed25519":
        fail("candidate algorithm is not Ed25519")
    sha = str(meta.get("sha256", "")).lower().strip()
    if not re.fullmatch(r"[0-9a-f]{64}", sha):
        fail("candidate sha256 is invalid")
    size = meta.get("size")
    if not isinstance(size, int) or size <= 0:
        fail("candidate size is invalid")
    signature = str(meta.get("signature", "")).strip()
    try:
        sig_raw = base64.b64decode(signature, validate=True)
    except Exception as exc:
        fail(f"candidate signature is not strict base64: {exc}")
    if len(sig_raw) != 64:
        fail("candidate Ed25519 signature must decode to 64 bytes")

    if provenance.get("signed") is not True:
        fail("candidate provenance is not marked signed")
    if provenance.get("promotion_requires_explicit_cutover") is not True:
        fail("candidate provenance is missing explicit-cutover invariant")

    return meta


def ts_string(value: str) -> str:
    # JSON strings are valid TypeScript string literals and avoid manual escape bugs.
    return json.dumps(value, ensure_ascii=False)


def main() -> None:
    meta = load_candidate()
    source = BASE_RELAY.read_text()

    replacement = "const UPDATE_MANIFEST={\n" + "\n".join(
        [
            f"  version:{ts_string(meta['version'])},",
            f"  file:{ts_string(meta['file'])},",
            f"  url:{ts_string(UPDATE_BINARY_ENDPOINT)},",
            f"  sha256:{ts_string(str(meta['sha256']).lower())},",
            f"  size:{int(meta['size'])},",
            f"  signature:{ts_string(meta['signature'])},",
            f"  algorithm:{ts_string(meta['algorithm'])},",
            f"  notes:{ts_string(str(meta.get('notes', '')))},",
        ]
    ) + "\n};"

    pattern = re.compile(r"const UPDATE_MANIFEST=\{.*?\n\};", re.S)
    rendered, count = pattern.subn(replacement, source, count=1)
    if count != 1:
        fail("could not locate exactly one UPDATE_MANIFEST block in relay source")

    # The binary endpoint intentionally continues to proxy main/channel. The
    # two-phase cutover workflow stages the verified signed candidate there
    # before this rendered manifest is ever deployed.
    if 'const TRUSTED_UPDATE_BINARY="https://raw.githubusercontent.com/arambarzani07/ZhiroxAi2026/main/channel/ZhiroxAISupervisor.exe";' not in rendered:
        fail("trusted production binary URL invariant is missing")
    if 'pc_os_min_agent:"5.1.0"' not in rendered:
        fail("PC OS minimum-version invariant is missing")
    if 'mutation_approval_gate:"one_time_exact_args"' not in rendered:
        fail("mutation approval invariant is missing")

    OUTPUT.write_text(rendered)
    print(f"rendered {OUTPUT.relative_to(ROOT)} for signed v5.1.0 candidate")


if __name__ == "__main__":
    main()
