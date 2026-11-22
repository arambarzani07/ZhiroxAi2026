"""Static CI guardrails for the fail-closed Relay v5.1.16 signed installer."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent.parent

class TestSignedInstallerGate(unittest.TestCase):
    def test_exact_release_digest_and_restricted_fields(self):
        policy = (ROOT / "src/pc_os_signed_app_install_policy.go").read_text(encoding="utf-8")
        self.assertIn("e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b", policy)
        self.assertIn("158128464", policy)
        self.assertIn("Cashierynwe-Setup-1.0.10-x64.exe", policy)
        self.assertIn("len(args) != 3", policy)
        self.assertIn('signedAppInstallMinAgent = "5.1.16"', policy)
        self.assertNotIn('args["url"]', policy)
        self.assertNotIn('args["exe"]', policy)
        self.assertNotIn('args["flags"]', policy)

    def test_windows_runtime_refuses_service_session_and_untrusted_payload(self):
        text = (ROOT / "src/pc_os_signed_app_install_windows.go").read_text(encoding="utf-8")
        for critical in (
            "ProcessIdToSessionId", "sessionID == 0",
            "interactive_user_broker_required", "sha256_mismatch",
            "Get-AuthenticodeSignature", "PublisherSubject",
            "windows_authenticode_identity_or_trust_failed", "privatePath, \"/S\"",
            "existing_install_use_in_app_updater",
        ):
            self.assertIn(critical, text)
        self.assertNotIn("cmd.exe", text)
        self.assertNotIn("WinExec", text)
        self.assertNotIn("PowerShell -ExecutionPolicy Bypass", text)
        self.assertIn("installer_exited_zero_needs_app_verification", text)

    def test_backend_never_allows_generic_installs_or_self_approval(self):
        edge = (ROOT / "ops/supabase/zhirox-pc-relay-v22-signed-installer.ts").read_text(encoding="utf-8")
        for critical in (
            '"signed_app_install_status"', '"signed_app_install"', "Server-c24b5c0f",
            '"5.1.16"', '"app_id,approval_id,version"', 'args.version !== "1.0.10"',
            'enqueue_approved_pc_relay_command',
            'approvalId.toLowerCase() !== args.approval_id.toLowerCase()',
        ):
            self.assertIn(critical, edge)
        self.assertNotIn('owner_exec_enabled: true', edge)

    def test_candidate_cannot_auto_publish(self):
        workflow = (ROOT / ".github/workflows/build-v5116-signed-app-installer.yml").read_text(encoding="utf-8")
        self.assertIn("publishable = $false", workflow)
        self.assertIn("signed_update_published = $false", workflow)
        self.assertIn("patch_v5115_cashierynwe_signer.py", workflow)
        self.assertIn("patch_v5116_installer.py", workflow)
        self.assertIn("pc_os_signed_app_install_windows.go", workflow)
        self.assertIn("Unsigned", workflow.replace("UNSIGNED", "Unsigned"))
        self.assertNotIn("gh release create", workflow)
        self.assertNotIn("git push --force", workflow)

if __name__ == "__main__":
    unittest.main()
