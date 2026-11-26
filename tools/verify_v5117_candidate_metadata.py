"""Fail-closed, offline check for the **unsigned** v5.1.17 GitHub artifact.

This script NEVER signs, uploads, promotes or uses a private key. It is run
before publishing a GitHub Actions CI artifact and is independent of
Supabase credentials. The expected source SHA comes from GITHUB_SHA after
the actual source checkout has been verified.
"""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re
import sys

PINNED_BASELINE = "46b208507293d677428a25b146599eb537c532b9"
TRUSTED_ED25519_PUB = "19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143"
HEX40 = re.compile(r"^[0-9a-f]{40}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")


def require(ok: bool, error: str) -> None:
    if not ok:
        raise ValueError(error)


def _read_json(filename: Path) -> dict:
    raw = json.loads(filename.read_text(encoding="utf-8-sig"))
    require(type(raw) is dict, "invalid_json_object")
    return raw


def verify_candidate(directory: Path, expected_source_commit: str) -> dict:
    """Returns only public provenance fields, never any signing material."""
    require(bool(HEX40.fullmatch(expected_source_commit)), "source_commit_invalid")
    root = directory.resolve(strict=True)
    require(root.is_dir(), "candidate_directory_missing")

    metadata = _read_json(root / "metadata.json")
    request = _read_json(root / "signing-request.json")
    executable = root / "ZhiroxAISupervisor.exe"
    require(executable.is_file() and not executable.is_symlink(), "executable_missing_or_linked")
    with executable.open("rb") as stream:
        actual_sha = hashlib.file_digest(stream, "sha256").hexdigest()
    actual_size = executable.stat().st_size
    require(actual_size > 1024 * 1024, "executable_unreasonably_small")

    for key, value in (
        ("version", "5.1.17"),
        ("from_version", "5.1.13"),
        ("source_relay_version", "5.1.15"),
        ("source_commit", expected_source_commit),
        ("inherited_signed_baseline_commit", PINNED_BASELINE),
        ("bounded_signer_action", "cashierynwe_sign_release"),
        ("required_agent_version", "5.1.17"),
        ("algorithm", "Ed25519"),
        ("signing_public_key_hex", TRUSTED_ED25519_PUB),
        ("file", "ZhiroxAISupervisor.exe"),
        ("sha256", actual_sha),
        ("size", actual_size),
    ):
        require(metadata.get(key) == value and
                type(metadata.get(key)) is type(value), f"metadata_{key}_invalid")

    for key, value in (
        ("cashierynwe_registered_version_read_only", True),
        ("publishable", False),
        ("signed_update_published", False),
        ("remote_input_enabled", False),
        ("unattended_access", False),
        ("attended_approval_required", True),
        ("pairing_code_returned", False),
    ):
        require(metadata.get(key) is value, f"unsafe_metadata_{key}")

    for forbidden in ("signature_b64", "private_key", "signing_seed",
                      "pfx", "password", "token", "secret"):
        require(forbidden not in metadata and forbidden not in request,
                "signing_secret_or_signature_leak")

    require(bool(HEX64.fullmatch(actual_sha)), "executable_hash_invalid")
    checksum = (root / "ZhiroxAISupervisor.exe.sha256").read_text(
        encoding="utf-8-sig").strip()
    require(checksum == actual_sha, "checksum_file_mismatch")

    request_expected = {
        "canonical": f"5.1.17|{actual_sha}|{actual_size}",
        "version": "5.1.17",
        "sha256": actual_sha,
        "size": actual_size,
        "signing_public_key_hex": TRUSTED_ED25519_PUB,
    }
    require(request == request_expected, "signing_request_canonical_mismatch")
    return {
        "status": "unsigned_candidate_verified",
        "source_commit": expected_source_commit,
        "signed_baseline_commit": PINNED_BASELINE,
        "version": "5.1.17",
        "sha256": actual_sha,
        "size_bytes": actual_size,
        "publishable": False,
    }


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: verify_v5117_candidate_metadata.py <candidate-dir> <full-source-commit>",
              file=sys.stderr)
        return 2
    try:
        result = verify_candidate(Path(sys.argv[1]), sys.argv[2])
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        # Only print the *category*; never dump the manifest itself.
        print(f"REJECTED: {type(exc).__name__}", file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
