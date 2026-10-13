import { createClient } from "npm:@supabase/supabase-js@2";

const db = createClient(
  Deno.env.get("SUPABASE_URL")!,
  Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!,
  { auth: { persistSession: false, autoRefreshToken: false } },
);

const SERVICE = "zhirox-pc-remote-signaling-v1";
const MAX_SIGNAL_BYTES = 256 * 1024;
const APPROVAL_TTL_MS = 2 * 60 * 1000;
const SESSION_TTL_MS = 30 * 60 * 1000;
const encoder = new TextEncoder();
const headers = {
  "content-type": "application/json",
  "cache-control": "no-store, max-age=0",
  "x-content-type-options": "nosniff",
};

function pathOf(req: Request) {
  const p = new URL(req.url).pathname;
  const marker = `/${SERVICE}`;
  const i = p.indexOf(marker);
  return i >= 0 ? (p.slice(i + marker.length) || "/") : p;
}

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers });
}

function bearer(req: Request) {
  const h = req.headers.get("authorization") || "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
}

async function sha256Text(s: string) {
  const d = await crypto.subtle.digest("SHA-256", encoder.encode(s));
  return Array.from(new Uint8Array(d)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

function uuidLike(v: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}

function cleanLabel(v: unknown) {
  return String(v || "").replace(/[\u0000-\u001f\u007f]/g, " ").trim().slice(0, 120);
}

function approvedCapabilities(v: unknown) {
  const x = v && typeof v === "object" ? v as Record<string, unknown> : {};
  // Phase 1 intentionally carries only screen streaming. Input control remains
  // disabled until a separate reviewed release enables it.
  return {
    screen_stream: x.screen_stream !== false,
    mouse: false,
    keyboard: false,
    clipboard: false,
    file_transfer: false,
    audio: false,
  };
}

function signalKind(v: unknown) {
  const k = String(v || "").trim().toLowerCase();
  return ["offer", "answer", "ice_candidate", "hangup"].includes(k) ? k : "";
}

function payloadSize(v: unknown) {
  try { return encoder.encode(JSON.stringify(v)).byteLength; } catch { return MAX_SIGNAL_BYTES + 1; }
}

async function authController(req: Request) {
  const token = bearer(req);
  if (!token) return null;
  const tokenHash = await sha256Text(token);
  const { data, error } = await db.from("pc_relay_connector_tokens")
    .select("token_hash,revoked_at")
    .eq("token_hash", tokenHash)
    .is("revoked_at", null)
    .maybeSingle();
  return error || !data ? null : tokenHash;
}

async function authAgent(req: Request, expectedAgent: string) {
  const agentId = (req.headers.get("x-agent-id") || "").trim();
  const secret = bearer(req);
  if (!agentId || !secret || agentId !== expectedAgent) return false;
  const { data, error } = await db.from("pc_relay_agents")
    .select("agent_id,secret_hash")
    .eq("agent_id", expectedAgent)
    .maybeSingle();
  if (error || !data) return false;
  return await sha256Text(secret) === data.secret_hash;
}

async function readSession(id: string) {
  const { data, error } = await db.from("pc_remote_sessions")
    .select("id,agent_id,state,controller_token_hash,requested_at,approved_at,expires_at,last_activity_at,requested_capabilities,approved_capabilities")
    .eq("id", id)
    .maybeSingle();
  if (error) throw error;
  return data;
}

async function audit(sessionId: string | null, agentId: string, eventType: string, actor: "agent" | "controller" | "system", metadata: Record<string, unknown> = {}) {
  const { error } = await db.from("pc_remote_audit").insert({
    session_id: sessionId,
    agent_id: agentId,
    event_type: eventType,
    actor,
    metadata,
  });
  if (error) throw error;
}

async function endSession(session: any, actor: "agent" | "controller" | "system", reason: string) {
  const now = new Date().toISOString();
  const { error } = await db.from("pc_remote_sessions")
    .update({ state: "ended", ended_at: now, last_activity_at: now, reason: reason.slice(0, 200) })
    .eq("id", session.id)
    .in("state", ["requested", "approved", "connecting", "active"]);
  if (error) throw error;
  await audit(session.id, session.agent_id, "session_ended", actor, { reason });
}

async function pushSignal(session: any, sender: "agent" | "controller", kind: string, payload: unknown) {
  if (!["approved", "connecting", "active"].includes(session.state)) throw new Error("session_not_signalable");
  if (Date.parse(session.expires_at) <= Date.now()) throw new Error("session_expired");
  if (!kind) throw new Error("signal_kind_refused");
  if (payloadSize(payload) > MAX_SIGNAL_BYTES) throw new Error("signal_payload_too_large");
  const now = new Date().toISOString();
  const { data, error } = await db.from("pc_remote_signals")
    .insert({ session_id: session.id, sender, kind, payload: payload ?? {} })
    .select("id,session_id,sender,kind,created_at")
    .single();
  if (error) throw error;
  const nextState = session.state === "approved" ? "connecting" : session.state;
  const { error: touchError } = await db.from("pc_remote_sessions")
    .update({ state: nextState, last_activity_at: now })
    .eq("id", session.id);
  if (touchError) throw touchError;
  if (kind === "hangup") await endSession(session, sender, "peer_hangup");
  return data;
}

Deno.serve(async (req: Request) => {
  const path = pathOf(req);
  try {
    if (req.method === "GET" && path === "/health") {
      return json({
        ok: true,
        service: SERVICE,
        protocol_version: 1,
        signaling_only: true,
        media_proxy: false,
        attended_approval_required: true,
        unattended_access: false,
        input_control_enabled: false,
        max_signal_bytes: MAX_SIGNAL_BYTES,
      });
    }

    if (req.method === "POST" && path === "/api/sessions") {
      const tokenHash = await authController(req);
      if (!tokenHash) return json({ ok: false, error: "controller_unauthorized" }, 401);
      const body = await req.json().catch(() => ({}));
      const agentId = String(body.agent_id || "").trim();
      if (!agentId) return json({ ok: false, error: "agent_id_required" }, 400);
      const { data: agent, error: agentErr } = await db.from("pc_relay_agents")
        .select("agent_id,version,last_seen_at,last_health_score")
        .eq("agent_id", agentId)
        .maybeSingle();
      if (agentErr) throw agentErr;
      if (!agent) return json({ ok: false, error: "agent_not_found" }, 404);
      const now = new Date();
      const expires = new Date(now.getTime() + SESSION_TTL_MS).toISOString();
      const caps = approvedCapabilities(body.capabilities);
      const { data, error } = await db.from("pc_remote_sessions").insert({
        agent_id: agentId,
        controller_token_hash: tokenHash,
        state: "requested",
        requested_capabilities: caps,
        approved_capabilities: {},
        expires_at: expires,
        controller_label: cleanLabel(body.controller_label),
        controller_fingerprint: cleanLabel(body.controller_fingerprint),
      }).select("id,agent_id,state,requested_at,expires_at,requested_capabilities").single();
      if (error) throw error;
      await audit(data.id, agentId, "session_requested", "controller", { capabilities: caps });
      return json(data, 202);
    }

    const approve = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/approve$/);
    if (approve && req.method === "POST") {
      const agentId = decodeURIComponent(approve[1]), id = approve[2];
      if (!uuidLike(id)) return json({ ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "agent_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json({ ok: false, error: "session_not_found" }, 404);
      if (session.state !== "requested") return json({ ok: false, error: "session_not_requestable" }, 409);
      if (Date.now() - Date.parse(session.requested_at) > APPROVAL_TTL_MS) {
        await db.from("pc_remote_sessions").update({ state: "expired", ended_at: new Date().toISOString() }).eq("id", id);
        await audit(id, agentId, "approval_expired", "system");
        return json({ ok: false, error: "approval_window_expired" }, 410);
      }
      const caps = approvedCapabilities(session.requested_capabilities);
      const now = new Date().toISOString();
      const { error } = await db.from("pc_remote_sessions")
        .update({ state: "approved", approved_at: now, last_activity_at: now, approved_capabilities: caps })
        .eq("id", id).eq("state", "requested");
      if (error) throw error;
      await audit(id, agentId, "session_approved", "agent", { capabilities: caps });
      return json({ ok: true, id, state: "approved", approved_capabilities: caps });
    }

    const reject = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/reject$/);
    if (reject && req.method === "POST") {
      const agentId = decodeURIComponent(reject[1]), id = reject[2];
      if (!uuidLike(id)) return json({ ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "agent_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json({ ok: false, error: "session_not_found" }, 404);
      const { error } = await db.from("pc_remote_sessions")
        .update({ state: "rejected", ended_at: new Date().toISOString(), reason: "user_rejected" })
        .eq("id", id).eq("state", "requested");
      if (error) throw error;
      await audit(id, agentId, "session_rejected", "agent");
      return json({ ok: true, id, state: "rejected" });
    }

    const controllerSignal = path.match(/^\/api\/sessions\/([^/]+)\/signals$/);
    if (controllerSignal) {
      const id = controllerSignal[1];
      if (!uuidLike(id)) return json({ ok: false, error: "invalid_session_id" }, 400);
      const tokenHash = await authController(req);
      if (!tokenHash) return json({ ok: false, error: "controller_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.controller_token_hash !== tokenHash) return json({ ok: false, error: "session_not_found" }, 404);
      if (req.method === "POST") {
        const body = await req.json().catch(() => ({}));
        const data = await pushSignal(session, "controller", signalKind(body.kind), body.payload ?? {});
        return json(data, 202);
      }
      if (req.method === "GET") {
        const after = Number(new URL(req.url).searchParams.get("after") || "0");
        const { data, error } = await db.from("pc_remote_signals")
          .select("id,sender,kind,payload,created_at")
          .eq("session_id", id).eq("sender", "agent").gt("id", Number.isFinite(after) ? after : 0)
          .order("id", { ascending: true }).limit(100);
        if (error) throw error;
        return json({ signals: data || [] });
      }
    }

    const agentSignal = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/signals$/);
    if (agentSignal) {
      const agentId = decodeURIComponent(agentSignal[1]), id = agentSignal[2];
      if (!uuidLike(id)) return json({ ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json({ ok: false, error: "agent_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json({ ok: false, error: "session_not_found" }, 404);
      if (req.method === "POST") {
        const body = await req.json().catch(() => ({}));
        const data = await pushSignal(session, "agent", signalKind(body.kind), body.payload ?? {});
        return json(data, 202);
      }
      if (req.method === "GET") {
        const after = Number(new URL(req.url).searchParams.get("after") || "0");
        const { data, error } = await db.from("pc_remote_signals")
          .select("id,sender,kind,payload,created_at")
          .eq("session_id", id).eq("sender", "controller").gt("id", Number.isFinite(after) ? after : 0)
          .order("id", { ascending: true }).limit(100);
        if (error) throw error;
        return json({ signals: data || [] });
      }
    }

    return json({ ok: false, error: "not_found" }, 404);
  } catch (e) {
    console.error(e);
    const message = String((e as any)?.message || e);
    const status = message.includes("expired") ? 410 : message.includes("refused") || message.includes("too_large") ? 422 : 500;
    return json({ ok: false, error: "remote_signaling_error", detail: message }, status);
  }
});
