import hashlib
import json
from pathlib import Path
from tempfile import TemporaryDirectory
import unittest

from verify_v5115_candidate_metadata import (
    PINNED_BASELINE,
    TRUSTED_ED25519_PUB,
    verify_candidate,
)

COMMIT = "a" * 40


class ReleaseCandidateProvenanceTests(unittest.TestCase):
    def setUp(self):
        self.temp = TemporaryDirectory(prefix="zhirox-sign-test-")
        self.addCleanup(self.temp.cleanup)
        self.path = Path(self.temp.name)
        self.executable = self.path / "ZhiroxAISupervisor.exe"
        self.executable.write_bytes(b"x" * (1024 * 1024 + 256))
        self.sha = hashlib.sha256(self.executable.read_bytes()).hexdigest()
        self.size = self.executable.stat().st_size
        self.metadata = {
            "version": "5.1.15",
            "from_version": "5.1.13",
            "file": "ZhiroxAISupervisor.exe",
            "sha256": self.sha,
            "size": self.size,
            "algorithm": "Ed25519",
            "signing_public_key_hex": TRUSTED_ED25519_PUB,
            "source_commit": COMMIT,
            "inherited_signed_baseline_commit": PINNED_BASELINE,
            "bounded_signer_action": "cashierynwe_sign_release",
            "required_agent_version": "5.1.15",
            "signed_update_published": False,
            "publishable": False,
            "remote_input_enabled": False,
            "unattended_access": False,
            "attended_approval_required": True,
            "pairing_code_returned": False,
        }
        self.request = {
            "canonical": f"5.1.15|{self.sha}|{self.size}",
            "version": "5.1.15",
            "sha256": self.sha,
            "size": self.size,
            "signing_public_key_hex": TRUSTED_ED25519_PUB,
        }
        (self.path / "ZhiroxAISupervisor.exe.sha256").write_text(self.sha + "\n")
        self.flush()

    def flush(self):
        (self.path / "metadata.json").write_text(json.dumps(self.metadata))
        (self.path / "signing-request.json").write_text(json.dumps(self.request))

    def verify(self):
        return verify_candidate(self.path, COMMIT)

    def test_genuine_unsigned_candidate_passes_without_signing(self):
        result = self.verify()
        self.assertEqual(result["status"], "unsigned_candidate_verified")
        self.assertEqual(result["sha256"], self.sha)
        self.assertIs(result["publishable"], False)

    def test_prevent_modified_executable(self):
        with self.executable.open("ab") as f:
            f.write(b"tampered")
        with self.assertRaisesRegex(ValueError, "metadata_.*_invalid"):
            self.verify()

    def test_prevent_legacy_or_unpinned_source_and_trust_key(self):
        for k, value in (
            ("version", "5.1.13"),
            ("from_version", "5.1.6"),
            ("source_commit", "b" * 40),
            ("inherited_signed_baseline_commit", "b" * 40),
            ("signing_public_key_hex", "b" * 64),
        ):
            prior = self.metadata[k]
            try:
                self.metadata[k] = value
                self.flush()
                with self.assertRaises(ValueError):
                    self.verify()
            finally:
                self.metadata[k] = prior
                self.flush()

    def test_forged_signed_or_publishable_metadata_rejected(self):
        for k, value in (
            ("publishable", True),
            ("signed_update_published", True),
            ("remote_input_enabled", True),
            ("unattended_access", True),
            ("attended_approval_required", False),
            ("pairing_code_returned", True),
        ):
            prior = self.metadata[k]
            try:
                self.metadata[k] = value
                self.flush()
                with self.assertRaisesRegex(ValueError, "unsafe_metadata"):
                    self.verify()
            finally:
                self.metadata[k] = prior
                self.flush()

    def test_reject_signing_secrets_and_early_signature(self):
        for k in ("signature_b64", "private_key", "pfx", "token"):
            self.metadata[k] = "unsafe"
            self.flush()
            with self.assertRaisesRegex(ValueError, "signing_secret_or_signature_leak"):
                self.verify()
            self.metadata.pop(k)
            self.flush()

    def test_canonical_signature_request_must_equal_exact_exe(self):
        self.request["canonical"] = f"5.1.15|{self.sha}|{self.size - 1}"
        self.flush()
        with self.assertRaisesRegex(ValueError, "signing_request_canonical_mismatch"):
            self.verify()

    def test_main_commit_parameter_is_enforced(self):
        with self.assertRaisesRegex(ValueError, "metadata_source_commit_invalid"):
            verify_candidate(self.path, "b" * 40)
        with self.assertRaisesRegex(ValueError, "source_commit_invalid"):
            verify_candidate(self.path, "main")


if __name__ == "__main__":
    unittest.main()
