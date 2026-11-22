import { createClient } from "npm:@supabase/supabase-js@2";

const supabase = createClient(
  Deno.env.get("SUPABASE_URL")!,
  Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!,
  { auth: { persistSession: false, autoRefreshToken: false } },
);

const FUNCTION_SLUG = "zhirox-pc-relay";
const cors = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, x-agent-id, content-type",
  "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  "Content-Type": "application/json",
};
const encoder = new TextEncoder();
const RESULT_MAX_JSON_BYTES = 1500000;
const RESULT_MAX_STRING_CHARS = 524288;

async function sha256(s: string) {
  const d = await crypto.subtle.digest("SHA-256", encoder.encode(s));
  return Array.from(new Uint8Array(d)).map((b) => b.toString(16).padStart(2, "0")).join("");
}
function json(data: unknown, status = 200, extraHeaders: Record<string, string> = {}) {
  return new Response(JSON.stringify(data), { status, headers: { ...cors, ...extraHeaders } });
}
function bearer(req: Request) {
  const h = req.headers.get("authorization") || "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
}
function normalizedPath(req: Request) {
  const p = new URL(req.url).pathname;
  const marker = `/${FUNCTION_SLUG}`;
  const i = p.indexOf(marker);
  return i >= 0 ? (p.slice(i + marker.length) || "/") : p;
}
function normalizeSeverity(v: unknown) {
  const s = String(v || "info").toLowerCase();
  return s === "warning" || s === "critical" ? s : "info";
}
function parseVersion(v: string) {
  const m = String(v || "").trim().replace(/^v/i, "").match(/^(\d+)\.(\d+)\.(\d+)/);
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : [0, 0, 0];
}
function versionAtLeast(v: string, min: string) {
  const a = parseVersion(v), b = parseVersion(min);
  for (let i = 0; i < 3; i++) {
    if (a[i] > b[i]) return true;
    if (a[i] < b[i]) return false;
  }
  return true;
}
function sanitizeForPostgres(value: unknown, depth = 0): unknown {
  if (depth > 20) return "[max_depth]";
  if (typeof value === "string") {
    const clean = value.replace(/\u0000/g, "\uFFFD");
    return clean.length > RESULT_MAX_STRING_CHARS ? clean.slice(0, RESULT_MAX_STRING_CHARS) + "\n[truncated_string]" : clean;
  }
  if (Array.isArray(value)) return value.map((v) => sanitizeForPostgres(v, depth + 1));
  if (value && typeof value === "object") {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      const safeKey = String(k).replace(/\u0000/g, "\uFFFD").slice(0, 1024);
      out[safeKey] = sanitizeForPostgres(v, depth + 1);
    }
    return out;
  }
  return value;
}
function boundedResult(value: unknown): unknown {
  const safe = sanitizeForPostgres(value);
  let encoded = "";
  try { encoded = JSON.stringify(safe); } catch { return { truncated: true, reason: "json_encode_failed" }; }
  const bytes = encoder.encode(encoded).byteLength;
  if (bytes <= RESULT_MAX_JSON_BYTES) return safe;
  return { truncated: true, reason: "result_payload_limit", original_json_bytes: bytes, preview: encoded.slice(0, RESULT_MAX_STRING_CHARS) };
}
function uuidLike(v: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}
function flattenStrings(value: unknown, depth = 0): string[] {
  if (depth > 8) return [];
  if (typeof value === "string") return [value];
  if (Array.isArray(value)) return value.flatMap((v) => flattenStrings(v, depth + 1));
  if (value && typeof value === "object") return Object.entries(value as Record<string, unknown>).flatMap(([k, v]) => [k, ...flattenStrings(v, depth + 1)]);
  return [];
}
function binaryReadRefused(action: string, args: unknown) {
  if (action !== "read_text") return false;
  const s = flattenStrings(args).join(" ").toLowerCase().replace(/\//g, "\\");
  return s.includes("d:\\database\\") || /\.(exe|dll|sys|bin|dat|mdf|ndf|ldf)(\s|$)/i.test(s);
}

async function authAgent(req: Request, expectedId: string) {
  const headerId = (req.headers.get("x-agent-id") || "").trim();
  const secret = bearer(req);
  if (!headerId || !secret || headerId !== expectedId) return null;
  const { data } = await supabase.from("pc_relay_agents").select("agent_id,secret_hash,version").eq("agent_id", expectedId).maybeSingle();
  if (!data) return null;
  return (await sha256(secret)) === data.secret_hash ? data : null;
}
async function authConnector(req: Request) {
  const token = bearer(req);
  if (!token) return false;
  const tokenHash = await sha256(token);
  const { data } = await supabase.from("pc_relay_connector_tokens").select("token_hash,revoked_at").eq("token_hash", tokenHash).is("revoked_at", null).maybeSingle();
  return !!data;
}

const READ_ACTIONS = new Set([
  "metrics", "processes", "services", "connections", "event_errors", "hardware", "disks",
  "network_adapters", "startup_items", "scheduled_tasks", "drivers", "security_status",
  "windows_update_status", "full_snapshot", "find_system", "capabilities", "app_status",
  "file_hash", "read_text", "search_files", "list_files", "storage_scan", "env_get",
  "firewall_rules", "printers", "registry_get", "timezone_info", "network",
  "remote_actions_info", "binary_phrase_probe", "cashierynwe_sign_status", "signed_app_install_status",
]);
const PC_OS_READ_ACTIONS = new Set([
  "pc_os_overview", "pc_os_modules", "pc_os_guardian", "pc_os_backup", "pc_os_backup_readiness",
  "pc_os_backup_verify", "pc_os_continuity", "pc_os_update_chain", "pc_os_verify_update", "pc_os_disk",
  "pc_os_network", "pc_os_security", "pc_os_files", "pc_os_inventory", "pc_os_ai", "pc_os_zero_ui",
  "pc_os_disaster_recovery",
]);
const MUTATION_ACTIONS = new Set([
  "env_set", "env_delete", "firewall_rule_add", "firewall_rule_delete", "firewall_rule_set",
  "dns_set", "dns_reset", "network_adapter_set", "network_ipv4_set", "driver_install_inf",
  "msi_install", "msi_uninstall", "windows_feature_set", "printer_set_default", "printer_queue_purge",
  "registry_set", "registry_delete", "service_control", "restart_service", "service_start_type_set",
  "archive_create", "archive_extract", "directory_copy", "file_delete", "timezone_set", "computer_rename",
  "defender_scan", "defender_update", "power_plan_set", "reboot", "owner_batch", "pc_os_backup_execute",
]);

// Production v20 control-plane; update channel deliberately remains pinned to known-good 5.0.8.
const FALLBACK_BINARY = "https://raw.githubusercontent.com/arambarzani07/ZhiroxAi2026/main/channel/ZhiroxAISupervisor.exe";
const UPDATE_BINARY_ENDPOINT = `https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/${FUNCTION_SLUG}/api/update/binary`;
const UPDATE_MANIFEST = {
  version: "5.0.8",
  url: UPDATE_BINARY_ENDPOINT,
  sha256: "c8af730b1ec4f12763378ff3abb67eadfacfa63adfee7d3503c13cafa6450547",
  size: 6328320,
  notes: "Production v20 control-plane; update remains pinned to known-good 5.0.8 pending separate v5.1 promotion",
};

Deno.serve(async (req: Request) => {
  if (req.method === "OPTIONS") return new Response(null, { status: 204, headers: cors });
  const path = normalizedPath(req);
  try {
    if (req.method === "GET" && path === "/health") {
      const [{ data: agent, error: agentErr }, { data: release, error: releaseErr }] = await Promise.all([
        supabase.from("pc_relay_agents").select("agent_id,version,last_seen_at,last_health_score").eq("agent_id", "Server-c24b5c0f").maybeSingle(),
        supabase.from("pc_update_releases").select("id,version,status,publishable,sha256,size_bytes,pinned_trust_anchor,rollback_updater_embedded").eq("version", "5.1.0").order("created_at", { ascending: false }).limit(1).maybeSingle(),
      ]);
      return json({
        ok: !agentErr && !releaseErr,
        service: "zhirox-pc-relay",
        version: "2.0.0-production",
        runtime_strategy: "direct_npm_import",
        module_resolution: "passed",
        control_plane: "full-v17-equivalent",
        result_sanitizer: true,
        structured_universal: true,
        mutation_approval_gate: "one_time_exact_args",
        pc_os_min_agent: "5.1.0",
        update_channel: UPDATE_MANIFEST.version,
        production_agent: agent ? { version: agent.version, health: agent.last_health_score, last_seen_at: agent.last_seen_at } : null,
        staged_v510: release ?? null,
        error: agentErr?.message ?? releaseErr?.message ?? null,
      }, !agentErr && !releaseErr ? 200 : 503, { "Cache-Control": "no-store, max-age=0" });
    }

    if (req.method === "POST" && path === "/api/agent/pair") {
      const body = await req.json().catch(() => ({}));
      const agent_id = String(body.agent_id || "").trim();
      const secret = String(body.secret || "");
      const pairing_code = String(body.pairing_code || "").trim();
      const hostname = String(body.hostname || "").trim();
      const version = String(body.version || "").trim();
      if (!agent_id || !secret || !pairing_code) return json({ ok: false, error: "missing_fields" }, 400);
      const tokenHash = await sha256(pairing_code), now = new Date().toISOString();
      const { data: token } = await supabase.from("pc_relay_pairing_tokens").select("token_hash,expires_at,used_at").eq("token_hash", tokenHash).is("used_at", null).gt("expires_at", now).maybeSingle();
      if (!token) return json({ ok: false, error: "invalid_or_expired_pairing_code" }, 401);
      const secret_hash = await sha256(secret);
      const { error: upsertError } = await supabase.from("pc_relay_agents").upsert({ agent_id, secret_hash, hostname, version, paired_at: now, updated_at: now }, { onConflict: "agent_id" });
      if (upsertError) throw upsertError;
      await supabase.from("pc_relay_pairing_tokens").update({ used_at: now }).eq("token_hash", tokenHash);
      return json({ ok: true, agent_id });
    }

    const hb = path.match(/^\/api\/agent\/([^/]+)\/heartbeat$/);
    if (hb && req.method === "POST") {
      const agentId = decodeURIComponent(hb[1]);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const body = await req.json().catch(() => ({}));
      const captured = new Date().toISOString();
      const version = String(body.version || ""), hostname = String(body.hostname || "");
      const health_score = Number.isFinite(Number(body.health_score)) ? Number(body.health_score) : null;
      const metrics = sanitizeForPostgres(body.metrics && typeof body.metrics === "object" ? body.metrics : {});
      const alerts = sanitizeForPostgres(Array.isArray(body.alerts) ? body.alerts : []);
      const { error: insErr } = await supabase.from("pc_relay_heartbeats").insert({ agent_id: agentId, captured_at: captured, version, hostname, health_score, metrics, alerts });
      if (insErr) throw insErr;
      const { error: updErr } = await supabase.from("pc_relay_agents").update({ hostname, version, last_seen_at: captured, last_health_score: health_score, last_metrics: metrics, last_alerts: alerts, updated_at: captured }).eq("agent_id", agentId);
      if (updErr) throw updErr;
      return json({ ok: true });
    }

    const snapshot = path.match(/^\/api\/agent\/([^/]+)\/pc-os\/snapshot$/);
    if (snapshot && req.method === "POST") {
      const agentId = decodeURIComponent(snapshot[1]), agent = await authAgent(req, agentId);
      if (!agent) return json({ ok: false, error: "unauthorized" }, 401);
      if (!versionAtLeast(String(agent.version || ""), "5.1.0")) return json({ ok: false, error: "agent_version_not_pc_os_capable" }, 409);
      const body = await req.json().catch(() => ({}));
      const module = String(body.module || "").trim().slice(0, 80);
      if (!module) return json({ ok: false, error: "module_required" }, 400);
      const payload = sanitizeForPostgres(body.payload && typeof body.payload === "object" ? body.payload : {});
      const { error } = await supabase.from("pc_os_snapshots").insert({ agent_id: agentId, module, severity: normalizeSeverity(body.severity), payload });
      if (error) throw error;
      return json({ ok: true }, 202);
    }

    const event = path.match(/^\/api\/agent\/([^/]+)\/pc-os\/event$/);
    if (event && req.method === "POST") {
      const agentId = decodeURIComponent(event[1]), agent = await authAgent(req, agentId);
      if (!agent) return json({ ok: false, error: "unauthorized" }, 401);
      if (!versionAtLeast(String(agent.version || ""), "5.1.0")) return json({ ok: false, error: "agent_version_not_pc_os_capable" }, 409);
      const body = await req.json().catch(() => ({}));
      const event_type = String(body.event_type || "").trim().replace(/\u0000/g, "\uFFFD").slice(0, 120);
      if (!event_type) return json({ ok: false, error: "event_type_required" }, 400);
      const payload = sanitizeForPostgres(body.payload && typeof body.payload === "object" ? body.payload : {});
      if (event_type === "itemname_delta") {
        const reject = (reason: string) => json({ok: false, error: reason}, 422);
        if (!payload || typeof payload !== "object" || Array.isArray(payload)) return reject("invalid_itemname_payload");
        if (typeof payload.event_key !== "string" || !/^[a-f0-9]{64}$/.test(payload.event_key)) return reject("invalid_event_key");
        if (payload.database !== "SuperMarket" || payload.read_only !== true) return reject("invalid_itemname_scope");
        if (!Array.isArray(payload.changes) || payload.changes.length < 1 || payload.changes.length > 25) return reject("invalid_change_batch");
        for (const change of payload.changes) {
          if (!change || typeof change !== "object" || Array.isArray(change)) return reject("invalid_change_item");
          if (typeof change.itemNameID !== "string" || !change.itemNameID.trim() || change.itemNameID.length > 50) return reject("invalid_item_id");
          if (change.kind !== "added" && change.kind !== "price_changed") return reject("invalid_change_kind");
          if (change.kind === "price_changed" && change.priceField !== "SalePrice" && change.priceField !== "WholeSalePrice") return reject("invalid_price_field");
        }
      }
      const { error } = await supabase.from("pc_os_events").insert({ agent_id: agentId, event_type, severity: normalizeSeverity(body.severity), payload });
      if (error) {
        if (event_type === "itemname_delta" && error.code === "23505" && String(error.message || "").includes("pc_os_events_agent_event_key_unique")) {
          return json({ ok: true, duplicate: true }, 202);
        }
        throw error;
      }
      return json({ ok: true }, 202);
    }

    const next = path.match(/^\/api\/agent\/([^/]+)\/next$/);
    if (next && req.method === "GET") {
      const agentId = decodeURIComponent(next[1]);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const { data: cmd, error } = await supabase.from("pc_relay_commands").select("id,action,args").eq("agent_id", agentId).eq("status", "queued").order("created_at", { ascending: true }).limit(1).maybeSingle();
      if (error) throw error;
      if (!cmd) return new Response(null, { status: 204, headers: cors });
      await supabase.from("pc_relay_commands").update({ status: "delivered", delivered_at: new Date().toISOString() }).eq("id", cmd.id);
      return json({ id: cmd.id, action: cmd.action, args: cmd.args || {} });
    }

    const result = path.match(/^\/api\/agent\/([^/]+)\/result$/);
    if (result && req.method === "POST") {
      const agentId = decodeURIComponent(result[1]);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const body = await req.json().catch(() => ({}));
      const id = String(body.id || "").replace(/\u0000/g, "");
      if (!id) return json({ ok: false, error: "missing_command_id" }, 400);
      const ok = !!body.ok;
      const safeResult = boundedResult(body.result ?? null);
      const safeError = body.error ? String(body.error).replace(/\u0000/g, "\uFFFD").slice(0, 32768) : null;
      const { error } = await supabase.from("pc_relay_commands").update({ status: ok ? "completed" : "failed", completed_at: new Date().toISOString(), result: safeResult, error: safeError }).eq("id", id).eq("agent_id", agentId);
      if (error) throw error;
      return json({ ok: true, sanitized: true });
    }

    if (req.method === "GET" && path === "/api/update/manifest") {
      const agentId = (req.headers.get("x-agent-id") || "").trim();
      if (!agentId || !(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      return json(UPDATE_MANIFEST, 200, { "Cache-Control": "no-store, max-age=0" });
    }
    if (req.method === "GET" && path === "/api/update/binary") {
      const agentId = (req.headers.get("x-agent-id") || "").trim();
      if (!agentId || !(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const upstream = await fetch(FALLBACK_BINARY, { method: "GET", redirect: "follow", headers: { "User-Agent": "Zhirox-PC-Relay/2.0.0" } });
      if (!upstream.ok || !upstream.body) return json({ ok: false, error: "update_upstream_failed", status: upstream.status }, 502);
      const headers = new Headers({ "Content-Type": "application/octet-stream", "Cache-Control": "no-store, max-age=0", "Content-Disposition": "attachment; filename=ZhiroxAISupervisor.exe", "X-Content-Type-Options": "nosniff" });
      const contentLength = upstream.headers.get("content-length");
      if (contentLength) headers.set("Content-Length", contentLength);
      return new Response(upstream.body, { status: 200, headers });
    }

    if (path.startsWith("/api/connector/")) {
      if (!(await authConnector(req))) return json({ ok: false, error: "connector_unauthorized" }, 401);

      if (req.method === "GET" && path === "/api/connector/agents") {
        const { data, error } = await supabase.from("pc_relay_agents").select("agent_id,hostname,version,paired_at,last_seen_at,last_health_score,last_metrics,last_alerts").order("last_seen_at", { ascending: false, nullsFirst: false });
        if (error) throw error;
        return json({ agents: data || [] });
      }

      const latest = path.match(/^\/api\/connector\/agents\/([^/]+)\/latest$/);
      if (latest && req.method === "GET") {
        const agentId = decodeURIComponent(latest[1]);
        const { data, error } = await supabase.from("pc_relay_agents").select("agent_id,hostname,version,paired_at,last_seen_at,last_health_score,last_metrics,last_alerts").eq("agent_id", agentId).maybeSingle();
        if (error) throw error;
        if (!data) return json({ ok: false, error: "not_found" }, 404);
        return json(data);
      }

      const pcLatest = path.match(/^\/api\/connector\/agents\/([^/]+)\/pc-os\/latest$/);
      if (pcLatest && req.method === "GET") {
        const agentId = decodeURIComponent(pcLatest[1]);
        const [{ data: snapshots, error: sErr }, { data: events, error: eErr }, { data: updates, error: uErr }] = await Promise.all([
          supabase.from("pc_os_snapshots").select("captured_at,module,severity,payload").eq("agent_id", agentId).order("captured_at", { ascending: false }).limit(25),
          supabase.from("pc_os_events").select("created_at,event_type,severity,payload").eq("agent_id", agentId).order("created_at", { ascending: false }).limit(25),
          supabase.from("pc_os_update_audit").select("created_at,version,artifact_sha256,signature_ok,staged_ok,health_gate_ok,rollback_performed,metadata").eq("agent_id", agentId).order("created_at", { ascending: false }).limit(10),
        ]);
        if (sErr) throw sErr;
        if (eErr) throw eErr;
        if (uErr) throw uErr;
        return json({ agent_id: agentId, snapshots: snapshots || [], events: events || [], updates: updates || [] });
      }

      const command = path.match(/^\/api\/connector\/agents\/([^/]+)\/command$/);
      if (command && req.method === "POST") {
        const agentId = decodeURIComponent(command[1]);
        const body = await req.json().catch(() => ({}));
        const action = String(body.action || "").trim();
        const args = body.args && typeof body.args === "object" ? sanitizeForPostgres(body.args) : {};

        if (binaryReadRefused(action, args)) return json({ ok: false, error: "binary_read_refused", use: "file_hash/app_status/binary_phrase_probe" }, 422);

        if (action === "signed_app_install_status") {
          if (agentId !== "Server-c24b5c0f" ||
              !args || Array.isArray(args) || Object.keys(args).length !== 0) {
            return json({ ok: false, error: "signed_installer_status_args_rejected" }, 422);
          }
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents")
            .select("version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (!agent || !versionAtLeast(String(agent.version || ""), "5.1.16")) {
            return json({ ok: false, error: "signed_installer_agent_not_ready", required: "5.1.16" }, 409);
          }
        }

        if (action === "cashierynwe_sign_status") {
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents")
            .select("version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (agentId !== "Server-c24b5c0f" ||
              !agent || !versionAtLeast(String(agent.version || ""), "5.1.14")) {
            return json({ ok: false, error: "relay_agent_signer_not_installed", required: "5.1.14" }, 409);
          }
          if (Object.keys(args || {}).length) {
            return json({ ok: false, error: "sign_status_takes_no_args" }, 422);
          }
        }

        if (READ_ACTIONS.has(action)) {
          const { data, error } = await supabase.from("pc_relay_commands").insert({ agent_id: agentId, action, args, status: "queued" }).select("id,agent_id,action,args,status,created_at").single();
          if (error) throw error;
          return json(data, 202);
        }

        if (PC_OS_READ_ACTIONS.has(action)) {
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents").select("agent_id,version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (!agent) return json({ ok: false, error: "agent_not_found" }, 404);
          if (!versionAtLeast(String(agent.version || ""), "5.1.0")) return json({ ok: false, error: "agent_version_not_pc_os_capable", required: "5.1.0", current: agent.version || null }, 409);
          const { data, error } = await supabase.from("pc_relay_commands").insert({ agent_id: agentId, action, args, status: "queued" }).select("id,agent_id,action,args,status,created_at").single();
          if (error) throw error;
          return json(data, 202);
        }

        // Strictly typed, single-app fresh installer. This is NOT generic remote
        // process execution. A separate authenticated owner approval must match
        // this exact action+args, expire, and be consumed atomically by the RPC.
        // The local Windows agent independently verifies the pinned SHA256,
        // exact publisher Authenticode identity and interactive-user safety.
        if (action === "signed_app_install") {
          if (agentId !== "Server-c24b5c0f") {
            return json({ ok: false, error: "signed_installer_target_not_allowed" }, 403);
          }
          if (!args || Array.isArray(args) || typeof args !== "object" ||
              Object.keys(args).sort().join(",") !== "app_id,approval_id,version" ||
              args.app_id !== "cashierynwe" || args.version !== "1.0.10" ||
              typeof args.approval_id !== "string" || !uuidLike(args.approval_id)) {
            return json({ ok: false, error: "signed_installer_args_rejected" }, 422);
          }
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents")
            .select("version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (!agent || !versionAtLeast(String(agent.version || ""), "5.1.16")) {
            return json({ ok: false, error: "signed_installer_agent_not_ready", required: "5.1.16" }, 409);
          }
          const approvalId = String(body.approval_id || "").trim();
          if (!uuidLike(approvalId) || approvalId.toLowerCase() !== args.approval_id.toLowerCase()) {
            return json({ ok: false, error: "one_time_exact_args_approval_required" }, 403);
          }
          const { data, error } = await supabase.rpc("enqueue_approved_pc_relay_command", {
            p_approval_id: approvalId, p_agent_id: agentId, p_action: action, p_args: args,
          });
          if (error) return json({ ok: false, error: "approval_rejected" }, 403);
          return json(data, 202);
        }

        // One-time, fail-closed Windows code-signing capability.
        // No shell text, paths, URLs, certificate, password or token is accepted as an argument.
        // Agent < 5.1.14 never receives the command; old production remains read-only.
        if (action === "cashierynwe_sign_release") {
          if (agentId !== "Server-c24b5c0f") {
            return json({ ok: false, error: "cashierynwe_agent_not_allowed" }, 403);
          }
          if (!args || Array.isArray(args) || typeof args !== "object" ||
              Object.keys(args).sort().join(",") !== "approval_id,commit_sha,tag" ||
              args.tag !== "v1.0.1" ||
              typeof args.commit_sha !== "string" ||
              !/^[a-f0-9]{40}$/.test(args.commit_sha) ||
              typeof args.approval_id !== "string" ||
              !uuidLike(args.approval_id)) {
            return json({ ok: false, error: "cashierynwe_sign_args_rejected" }, 422);
          }
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents")
            .select("version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (!agent || !versionAtLeast(String(agent.version || ""), "5.1.14")) {
            return json({ ok: false, error: "relay_agent_signer_not_installed", required: "5.1.14" }, 409);
          }
          const approvalId = String(body.approval_id || "").trim();
          // The owner-approved RPC consumes exactly these action arguments.
          // Bind the UUID in the command payload to the separately authenticated
          // one-time approval record; a phrase is NEVER owner authorization.
          if (!uuidLike(approvalId) || approvalId.toLowerCase() !== args.approval_id.toLowerCase()) {
            return json({ ok: false, error: "one_time_exact_args_approval_required" }, 403);
          }
          const { data, error } = await supabase.rpc("enqueue_approved_pc_relay_command", {
            p_approval_id: approvalId, p_agent_id: agentId, p_action: action, p_args: args,
          });
          if (error) return json({ ok: false, error: "approval_rejected" }, 403);
          return json(data, 202);
        }

        if (MUTATION_ACTIONS.has(action)) {
          const approvalId = String(body.approval_id || "").trim();
          if (!approvalId) return json({ ok: false, error: "approval_required", policy: "one_time_exact_args" }, 403);
          if (!uuidLike(approvalId)) return json({ ok: false, error: "invalid_approval_id" }, 400);
          const { data, error } = await supabase.rpc("enqueue_approved_pc_relay_command", { p_approval_id: approvalId, p_agent_id: agentId, p_action: action, p_args: args });
          if (error) return json({ ok: false, error: "approval_rejected", detail: String(error.message || "approval mismatch/expired/used") }, 403);
          return json(data, 202);
        }

        return json({ ok: false, error: "action_not_allowed" }, 403);
      }

      const commandStatus = path.match(/^\/api\/connector\/commands\/([^/]+)$/);
      if (commandStatus && req.method === "GET") {
        const id = decodeURIComponent(commandStatus[1]);
        const { data, error } = await supabase.from("pc_relay_commands").select("id,agent_id,action,args,status,created_at,delivered_at,completed_at,result,error").eq("id", id).maybeSingle();
        if (error) throw error;
        if (!data) return json({ ok: false, error: "not_found" }, 404);
        return json(data);
      }
    }

    return json({ ok: false, error: "not_found", path }, 404);
  } catch (e) {
    console.error(e);
    return json({ ok: false, error: "server_error", detail: String((e as any)?.message || e) }, 500);
  }
});
