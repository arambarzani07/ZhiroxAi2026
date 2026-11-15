"""Static security regression checks for the ONE pinned Vault signer.

Does not connect to Supabase, fetch secret keys, or execute SQL.
"""
from pathlib import Path
import re
import unittest

SQL = (Path(__file__).resolve().parent.parent /
       "ops/supabase/zhirox_sign_v5114_exact_release.sql").read_text()


class ExactReleaseSignerPolicy(unittest.TestCase):
    def test_source_identity_and_binary_are_exact(self):
        for pin in (
            "r.version IS DISTINCT FROM '5.1.14'",
            "r.source_commit IS DISTINCT FROM '091c7e23ee50807a5c721a45c7b1c7c2267695c0'",
            "r.sha256 IS DISTINCT FROM '371394e3e31351929c122eea95ad926aec8a5d8316b347f25ec47f0b3d80f7fa'",
            "r.size_bytes IS DISTINCT FROM 20025856",
            "'https://github.com/arambarzani07/ZhiroxAi2026/releases/download/v5.1.14-candidate-091c7e23/ZhiroxAISupervisor.exe'",
            "'43708666560'",
            "'14159935571'",
        ):
            self.assertIn(pin, SQL)

    def test_staged_only_never_promotes(self):
        for pin in (
            "r.status IS DISTINCT FROM 'staged'",
            "r.publishable IS DISTINCT FROM false",
            "r.signature_b64 IS NOT NULL",
            "r.pinned_trust_anchor IS DISTINCT FROM true",
            "r.rollback_updater_embedded IS DISTINCT FROM true",
            "r.artifact_encoding IS DISTINCT FROM 'identity'",
            "IF NOT p_dry_run THEN",
            "signature_b64 = encode(sig, 'base64')",
        ):
            self.assertIn(pin, SQL)
        self.assertNotRegex(SQL, r"SET\s+publishable\s*=\s*true")
        self.assertNotRegex(SQL, r"SET\s+status\s*=\s*'promoted'")
        self.assertNotRegex(SQL, r"SET\s+promoted_at\s*=")

    def test_pinned_existing_vault_key_never_exported(self):
        self.assertIn("vault.decrypted_secrets", SQL)
        self.assertIn("ZHIROX_V511_NEXT_ED25519_SEED_B64", SQL)
        self.assertIn("crypto_sign_verify_detached", SQL)
        self.assertIn("'19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143'", SQL)
        self.assertIn("'key_exported', false", SQL)
        self.assertNotRegex(SQL, r"RETURN\s+seed\b")
        self.assertNotRegex(SQL, r"RETURN\s+kp\.secret\b")

    def test_privilege_grants_not_public(self):
        self.assertIn("SECURITY DEFINER", SQL)
        self.assertRegex(SQL, r"REVOKE ALL ON FUNCTION public\.zhirox_sign_v5114_exact_release\(uuid,boolean\)\s+FROM PUBLIC, anon, authenticated")
        self.assertRegex(SQL, r"GRANT EXECUTE ON FUNCTION public\.zhirox_sign_v5114_exact_release\(uuid,boolean\)\s+TO service_role")
        self.assertIn("'promotion_performed', false", SQL)


if __name__ == "__main__":
    unittest.main()
