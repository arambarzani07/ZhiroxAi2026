import {
  FIXED_INSTALLER, APPROVAL_MAX_AGE_MS,
  approvedSignedDownload, approvedGitHubArtifactRedirect,
} from "./installer_transport_policy_v23.ts";

function assert(v: unknown, hint: string) { if (!v) throw Error(hint); }
const agent = FIXED_INSTALLER.agent_id;
const uuid = "12345678-1234-4234-8234-1234567890ab";
const now = Date.parse("2026-11-22T15:00:00Z");
const when = (ms: number) => new Date(ms).toISOString();
const args = { app_id: "cashierynwe", version: "1.0.10", approval_id: uuid };
const cmd = {
  id: "abcdefab-1234-4234-8234-1234567890ab", agent_id: agent,
  action: "signed_app_install", args, status: "delivered",
  created_at: when(now - 2000), delivered_at: when(now - 1000),
};
const approval = {
  id: uuid, agent_id: agent, action: "signed_app_install", args,
  command_id: cmd.id, approved_at: when(now - 5000),
  expires_at: when(now + 30000), used_at: when(now - 1500),
  approved_by: "authenticated-owner",
};
Deno.test("only consumed exact owner approval enables fixed asset", () => {
  assert(approvedSignedDownload(agent,uuid,approval,cmd,now), "exact approved action rejected");
  for (const [badA,badC] of [
    [{ ...approval, used_at: null }, cmd],
    [{ ...approval, command_id: "fake" }, cmd],
    [{ ...approval, approved_by: null }, cmd],
    [{ ...approval, args: { ...args, url: "https://evil.invalid" } }, cmd],
    [{ ...approval, expires_at: when(now - 2000) }, cmd],
    [{ ...approval, used_at: when(now + 10000) }, cmd],
    [approval, { ...cmd, status: "queued" }],
    [approval, { ...cmd, status: "failed" }],
    [approval, { ...cmd, agent_id: "other" }],
    [approval, { ...cmd, args: { ...args, filename: "cmd.exe" } }],
    [approval, { ...cmd, created_at: when(now - APPROVAL_MAX_AGE_MS - 1) }],
  ] as const) assert(!approvedSignedDownload(agent,uuid,badA,badC,now), "invalid approval accepted");
  assert(!approvedSignedDownload("other",uuid,approval,cmd,now), "unapproved agent");
  assert(!approvedSignedDownload(agent,"../../secret",approval,cmd,now), "invalid UUID");
});
Deno.test("only GitHub's exact CDN hostnames accepted", () => {
  assert(!!approvedGitHubArtifactRedirect("https://release-assets.githubusercontent.com/a?token=x"), "valid redirect rejected");
  assert(!!approvedGitHubArtifactRedirect("https://objects.githubusercontent.com/a?sig=x"), "valid second host rejected");
  for(const raw of [
    "http://release-assets.githubusercontent.com/file",
    "https://release-assets.githubusercontent.com.evil.example/file",
    "https://evil.example/file",
    "https://127.0.0.1/file",
    "https://user:pass@release-assets.githubusercontent.com/a",
    "https://release-assets.githubusercontent.com:8443/a",
    "https://release-assets.githubusercontent.com/a#fragment",
    "file:///C:/Windows/system32/cmd.exe",
  ]) assert(approvedGitHubArtifactRedirect(raw)===null,"unsafe redirect accepted: "+raw);
});
