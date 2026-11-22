// Pure fail-closed policy for a single pre-approved signed app release.
// This module does not fetch, sign, install, or mutate any device.
export const FIXED_INSTALLER = Object.freeze({
  app_id: "cashierynwe",
  version: "1.0.10",
  asset_id: 779202065,
  filename: "Cashierynwe-Setup-1.0.10-x64.exe",
  sha256: "e7c45af426fb7702a3b4d21167fc63051ace8c01f77d6ad0913fe1ed3e189b8b",
  size: 158128464,
  agent_id: "Server-c24b5c0f",
  min_relay_version: "5.1.16",
});
export const APPROVAL_MAX_AGE_MS = 15 * 60 * 1000;

type Json = Record<string, unknown>;
type ApprovedCmd = {
  id: string;
  agent_id: string;
  action: string;
  args: unknown;
  status: string;
  created_at: string;
  delivered_at?: string | null;
};
type Approval = {
  id: string;
  agent_id: string;
  action: string;
  args: unknown;
  command_id: string | null;
  approved_at: string;
  expires_at: string;
  used_at: string | null;
  approved_by: string | null;
};

const uuidRe = /^[a-f0-9]{8}-[a-f0-9]{4}-[1-5][a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/;
function exactArgs(value: unknown, approvalId: string): value is Json {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const a = value as Json;
  return Object.keys(a).sort().join(",") === "app_id,approval_id,version" &&
    a.app_id === FIXED_INSTALLER.app_id &&
    a.version === FIXED_INSTALLER.version &&
    a.approval_id === approvalId;
}
function inWindow(value: unknown, now: number, maxAge: number): boolean {
  if (typeof value !== "string") return false;
  const t = Date.parse(value);
  return Number.isFinite(t) && t <= now && t >= now - maxAge;
}
export function approvedSignedDownload(
  agentId: string,
  approvalId: string,
  approval: Approval | null,
  command: ApprovedCmd | null,
  nowMs: number,
): boolean {
  if (agentId !== FIXED_INSTALLER.agent_id || !uuidRe.test(approvalId)) return false;
  if (!approval || !command || !Number.isFinite(nowMs)) return false;
  if (approval.id !== approvalId || command.id !== approval.command_id) return false;
  if (approval.agent_id !== agentId || command.agent_id !== agentId) return false;
  if (approval.action !== "signed_app_install" || command.action !== "signed_app_install") return false;
  if (!exactArgs(approval.args, approvalId) || !exactArgs(command.args, approvalId)) return false;
  if (typeof approval.approved_by !== "string" || !approval.approved_by.trim()) return false;
  if (command.status !== "delivered" && command.status !== "completed") return false;
  if (!inWindow(command.created_at, nowMs, APPROVAL_MAX_AGE_MS) ||
      !inWindow(command.delivered_at, nowMs, APPROVAL_MAX_AGE_MS) ||
      !inWindow(approval.approved_at, nowMs, APPROVAL_MAX_AGE_MS) ||
      !inWindow(approval.used_at, nowMs, APPROVAL_MAX_AGE_MS)) return false;
  const exp = Date.parse(approval.expires_at);
  const used = Date.parse(approval.used_at!);
  const issued = Date.parse(approval.approved_at);
  if (!Number.isFinite(exp) || exp < used || used < issued) return false;
  // An approval may be expired *now* because consumption was atomic in time,
  // but it must have been valid at the moment it was consumed.
  return true;
}

const HOSTS = new Set([
  "release-assets.githubusercontent.com",
  "objects.githubusercontent.com",
]);
export function approvedGitHubArtifactRedirect(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length < 20 || raw.length > 8192) return null;
  try {
    const u = new URL(raw);
    if (u.protocol !== "https:" || !HOSTS.has(u.hostname) ||
        u.port || u.username || u.password || u.hash ||
        !u.pathname.startsWith("/") || u.pathname.includes("/../")) return null;
    // The location is a short-lived, bearer-style CDN URL. It must not be
    // logged, cached, returned to browsers or persisted on the relay device.
    return u.toString();
  } catch {
    return null;
  }
}
