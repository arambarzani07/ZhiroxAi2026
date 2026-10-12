import { createClient } from "npm:@supabase/supabase-js@2";

const supabase = createClient(
  Deno.env.get("SUPABASE_URL")!,
  Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!,
  { auth: { persistSession: false, autoRefreshToken: false } }
);

const cors = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Headers": "authorization, x-agent-id, content-type",
  "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  "Content-Type": "application/json",
};

const encoder = new TextEncoder();
async function sha256(s: string) {
  const d = await crypto.subtle.digest("SHA-256", encoder.encode(s));
  return Array.from(new Uint8Array(d)).map(b => b.toString(16).padStart(2, "0")).join("");
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
  const marker = "/zhirox-pc-relay";
  const i = p.indexOf(marker);
  return i >= 0 ? (p.slice(i + marker.length) || "/") : p;
}
function normalizeSeverity(v: unknown) {
  const s = String(v || "info").toLowerCase();
  return s === "warning" || s === "critical" ? s : "info";
}
function parseVersion(v: string) {
  const m = String(v || "").trim().replace(/^v/i, "").match(/^(\d+)\.(\d+)\.(\d+)/);
  if (!m) return [0, 0, 0];
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}
function versionAtLeast(v: string, min: string) {
  const a = parseVersion(v), b = parseVersion(min);
  for (let i = 0; i < 3; i++) {
    if (a[i] > b[i]) return true;
    if (a[i] < b[i]) return false;
  }
  return true;
}

async function authAgent(req: Request, expectedId: string) {
  const headerId = (req.headers.get("x-agent-id") || "").trim();
  const secret = bearer(req);
  if (!headerId || !secret || headerId !== expectedId) return null;
  const { data } = await supabase.from("pc_relay_agents")
    .select("agent_id,secret_hash,version")
    .eq("agent_id", expectedId)
    .maybeSingle();
  if (!data) return null;
  return (await sha256(secret)) === data.secret_hash ? data : null;
}

async function authConnector(req: Request) {
  const token = bearer(req);
  if (!token) return false;
  const hash = await sha256(token);
  const { data } = await supabase.from("pc_relay_connector_tokens")
    .select("token_hash,revoked_at")
    .eq("token_hash", hash)
    .is("revoked_at", null)
    .maybeSingle();
  return !!data;
}

const READ_ACTIONS = new Set([
  "metrics","processes","services","connections","event_errors",
  "hardware","disks","network_adapters","startup_items","scheduled_tasks",
  "drivers","security_status","windows_update_status","full_snapshot","find_system"
]);

const PC_OS_READ_ACTIONS = new Set([
  "pc_os_overview","pc_os_modules","pc_os_guardian","pc_os_backup",
  "pc_os_continuity","pc_os_update_chain","pc_os_verify_update","pc_os_disk",
  "pc_os_network","pc_os_security","pc_os_files","pc_os_inventory","pc_os_ai",
  "pc_os_zero_ui","pc_os_disaster_recovery"
]);

const TRUSTED_UPDATE_BINARY = "https://raw.githubusercontent.com/arambarzani07/ZhiroxAi2026/main/channel/ZhiroxAISupervisor.exe";
const UPDATE_BINARY_ENDPOINT = "https://vllwundrtrgdqmkqbduy.supabase.co/functions/v1/zhirox-pc-relay/api/update/binary";
// Keep production channel on the currently deployed known-good build until the
// v5.1.0 artifact is signed and explicitly promoted.
const UPDATE_MANIFEST = {
  version: "5.0.8",
  url: UPDATE_BINARY_ENDPOINT,
  sha256: "c8af730b1ec4f12763378ff3abb67eadfacfa63adfee7d3503c13cafa6450547",
  size: 6328320,
  notes: "Known-good production channel; PC OS v5.1.0 remains gated until signed promotion",
};

Deno.serve(async (req: Request) => {
  if (req.method === "OPTIONS") return new Response(null, { status: 204, headers: cors });
  const path = normalizedPath(req);

  try {
    if (req.method === "GET" && path === "/health") {
      return json({
        ok: true,
        service: "zhirox-pc-relay",
        version: "1.4.0",
        update_channel: UPDATE_MANIFEST.version,
        pc_os_min_agent: "5.1.0",
        pc_os_control_plane: true,
      });
    }

    if (req.method === "POST" && path === "/api/agent/pair") {
      const body = await req.json().catch(() => ({}));
      const agent_id = String(body.agent_id || "").trim();
      const secret = String(body.secret || "");
      const pairing_code = String(body.pairing_code || "").trim();
      const hostname = String(body.hostname || "").trim();
      const version = String(body.version || "").trim();
      if (!agent_id || !secret || !pairing_code) return json({ ok: false, error: "missing_fields" }, 400);

      const tokenHash = await sha256(pairing_code);
      const now = new Date().toISOString();
      const { data: token } = await supabase.from("pc_relay_pairing_tokens")
        .select("token_hash,expires_at,used_at")
        .eq("token_hash", tokenHash)
        .is("used_at", null)
        .gt("expires_at", now)
        .maybeSingle();
      if (!token) return json({ ok: false, error: "invalid_or_expired_pairing_code" }, 401);

      const secret_hash = await sha256(secret);
      const { error: upsertError } = await supabase.from("pc_relay_agents").upsert({
        agent_id, secret_hash, hostname, version, paired_at: now, updated_at: now
      }, { onConflict: "agent_id" });
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
      const version = String(body.version || "");
      const hostname = String(body.hostname || "");
      const health_score = Number.isFinite(Number(body.health_score)) ? Number(body.health_score) : null;
      const metrics = body.metrics && typeof body.metrics === "object" ? body.metrics : {};
      const alerts = Array.isArray(body.alerts) ? body.alerts : [];

      const { error: insErr } = await supabase.from("pc_relay_heartbeats").insert({
        agent_id: agentId, captured_at: captured, version, hostname, health_score, metrics, alerts
      });
      if (insErr) throw insErr;
      const { error: updErr } = await supabase.from("pc_relay_agents").update({
        hostname, version, last_seen_at: captured, last_health_score: health_score,
        last_metrics: metrics, last_alerts: alerts, updated_at: captured
      }).eq("agent_id", agentId);
      if (updErr) throw updErr;
      return json({ ok: true });
    }

    const snapshot = path.match(/^\/api\/agent\/([^/]+)\/pc-os\/snapshot$/);
    if (snapshot && req.method === "POST") {
      const agentId = decodeURIComponent(snapshot[1]);
      const agent = await authAgent(req, agentId);
      if (!agent) return json({ ok: false, error: "unauthorized" }, 401);
      if (!versionAtLeast(String(agent.version || ""), "5.1.0")) {
        return json({ ok: false, error: "agent_version_not_pc_os_capable" }, 409);
      }
      const body = await req.json().catch(() => ({}));
      const module = String(body.module || "").trim().slice(0, 80);
      if (!module) return json({ ok: false, error: "module_required" }, 400);
      const payload = body.payload && typeof body.payload === "object" ? body.payload : {};
      const { error } = await supabase.from("pc_os_snapshots").insert({
        agent_id: agentId,
        module,
        severity: normalizeSeverity(body.severity),
        payload,
      });
      if (error) throw error;
      return json({ ok: true }, 202);
    }

    const event = path.match(/^\/api\/agent\/([^/]+)\/pc-os\/event$/);
    if (event && req.method === "POST") {
      const agentId = decodeURIComponent(event[1]);
      const agent = await authAgent(req, agentId);
      if (!agent) return json({ ok: false, error: "unauthorized" }, 401);
      if (!versionAtLeast(String(agent.version || ""), "5.1.0")) {
        return json({ ok: false, error: "agent_version_not_pc_os_capable" }, 409);
      }
      const body = await req.json().catch(() => ({}));
      const event_type = String(body.event_type || "").trim().slice(0, 120);
      if (!event_type) return json({ ok: false, error: "event_type_required" }, 400);
      const payload = body.payload && typeof body.payload === "object" ? body.payload : {};
      const { error } = await supabase.from("pc_os_events").insert({
        agent_id: agentId,
        event_type,
        severity: normalizeSeverity(body.severity),
        payload,
      });
      if (error) throw error;
      return json({ ok: true }, 202);
    }

    const next = path.match(/^\/api\/agent\/([^/]+)\/next$/);
    if (next && req.method === "GET") {
      const agentId = decodeURIComponent(next[1]);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const { data: cmd, error } = await supabase.from("pc_relay_commands")
        .select("id,action,args")
        .eq("agent_id", agentId).eq("status", "queued")
        .order("created_at", { ascending: true }).limit(1).maybeSingle();
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
      const id = String(body.id || "");
      if (!id) return json({ ok: false, error: "missing_command_id" }, 400);
      const ok = !!body.ok;
      const { error } = await supabase.from("pc_relay_commands").update({
        status: ok ? "completed" : "failed",
        completed_at: new Date().toISOString(),
        result: body.result ?? null,
        error: body.error ? String(body.error) : null
      }).eq("id", id).eq("agent_id", agentId);
      if (error) throw error;
      return json({ ok: true });
    }

    if (req.method === "GET" && path === "/api/update/manifest") {
      const agentId = (req.headers.get("x-agent-id") || "").trim();
      if (!agentId || !(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      return json(UPDATE_MANIFEST, 200, { "Cache-Control": "no-store, max-age=0" });
    }

    if (req.method === "GET" && path === "/api/update/binary") {
      const agentId = (req.headers.get("x-agent-id") || "").trim();
      if (!agentId || !(await authAgent(req, agentId))) return json({ ok: false, error: "unauthorized" }, 401);
      const upstream = await fetch(TRUSTED_UPDATE_BINARY, {
        method: "GET", redirect: "follow", headers: { "User-Agent": "Zhirox-PC-Relay/1.4" },
      });
      if (!upstream.ok || !upstream.body) {
        return json({ ok: false, error: "update_upstream_failed", status: upstream.status }, 502);
      }
      const headers = new Headers({
        "Content-Type": "application/octet-stream",
        "Cache-Control": "no-store, max-age=0",
        "Content-Disposition": "attachment; filename=ZhiroxAISupervisor.exe",
        "X-Content-Type-Options": "nosniff",
      });
      const contentLength = upstream.headers.get("content-length");
      if (contentLength) headers.set("Content-Length", contentLength);
      return new Response(upstream.body, { status: 200, headers });
    }

    if (path.startsWith("/api/connector/")) {
      if (!(await authConnector(req))) return json({ ok: false, error: "connector_unauthorized" }, 401);

      if (req.method === "GET" && path === "/api/connector/agents") {
        const { data, error } = await supabase.from("pc_relay_agents")
          .select("agent_id,hostname,version,paired_at,last_seen_at,last_health_score,last_metrics,last_alerts")
          .order("last_seen_at", { ascending: false, nullsFirst: false });
        if (error) throw error;
        return json({ agents: data || [] });
      }

      const latest = path.match(/^\/api\/connector\/agents\/([^/]+)\/latest$/);
      if (latest && req.method === "GET") {
        const agentId = decodeURIComponent(latest[1]);
        const { data, error } = await supabase.from("pc_relay_agents")
          .select("agent_id,hostname,version,paired_at,last_seen_at,last_health_score,last_metrics,last_alerts")
          .eq("agent_id", agentId).maybeSingle();
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
        const args = body.args && typeof body.args === "object" ? body.args : {};

        if (READ_ACTIONS.has(action)) {
          // Existing safe read-only action; backward compatible.
        } else if (PC_OS_READ_ACTIONS.has(action)) {
          const { data: agent, error: agentErr } = await supabase.from("pc_relay_agents")
            .select("agent_id,version").eq("agent_id", agentId).maybeSingle();
          if (agentErr) throw agentErr;
          if (!agent) return json({ ok: false, error: "agent_not_found" }, 404);
          if (!versionAtLeast(String(agent.version || ""), "5.1.0")) {
            return json({ ok: false, error: "agent_version_not_pc_os_capable", required: "5.1.0", current: agent.version || null }, 409);
          }
        } else {
          return json({ ok: false, error: "action_not_allowed" }, 403);
        }

        const { data, error } = await supabase.from("pc_relay_commands")
          .insert({ agent_id: agentId, action, args, status: "queued" })
          .select("id,agent_id,action,args,status,created_at").single();
        if (error) throw error;
        return json(data, 202);
      }

      const commandStatus = path.match(/^\/api\/connector\/commands\/([^/]+)$/);
      if (commandStatus && req.method === "GET") {
        const id = decodeURIComponent(commandStatus[1]);
        const { data, error } = await supabase.from("pc_relay_commands")
          .select("id,agent_id,action,args,status,created_at,delivered_at,completed_at,result,error")
          .eq("id", id).maybeSingle();
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
