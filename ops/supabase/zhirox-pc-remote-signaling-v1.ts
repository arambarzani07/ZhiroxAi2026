import { createClient } from "npm:@supabase/supabase-js@2.117.3";

function requiredEnv(name: string) {
  const value = (Deno.env.get(name) || "").trim();
  if (!value) throw new Error(`missing_required_env:${name}`);
  return value;
}

const db = createClient(
  requiredEnv("SUPABASE_URL"),
  requiredEnv("SUPABASE_SERVICE_ROLE_KEY"),
  { auth: { persistSession: false, autoRefreshToken: false } },
);

const SERVICE = "zhirox-pc-remote-signaling-v1";
const MAX_SIGNAL_BYTES = 256 * 1024;
const MAX_SESSION_REQUEST_BYTES = 16 * 1024;
const MAX_SIGNAL_REQUEST_BYTES = MAX_SIGNAL_BYTES + 16 * 1024;
const APPROVAL_TTL_MS = 2 * 60 * 1000;
const SESSION_TTL_MS = 30 * 60 * 1000;
const AGENT_ONLINE_WINDOW_MS = 2 * 60 * 1000;
const encoder = new TextEncoder();
const terminalStates = new Set(["ended", "rejected", "expired"]);
const signalableStates = new Set(["approved", "connecting", "active"]);
const allowedCapabilityKeys = new Set([
  "screen_stream",
  "mouse",
  "keyboard",
  "clipboard",
  "file_transfer",
  "audio",
]);
const allowedOrigins = new Set(
  (Deno.env.get("ZHIROX_REMOTE_ALLOWED_ORIGINS") || "")
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean),
);

class ApiError extends Error {
  code: string;
  status: number;

  constructor(code: string, status: number) {
    super(code);
    this.code = code;
    this.status = status;
  }
}

function pathOf(req: Request) {
  const p = new URL(req.url).pathname;
  const marker = `/${SERVICE}`;
  const i = p.indexOf(marker);
  return i >= 0 ? (p.slice(i + marker.length) || "/") : p;
}

function originAllowed(req: Request) {
  const origin = req.headers.get("origin");
  return !origin || allowedOrigins.has(origin);
}

function responseHeaders(req: Request) {
  const h: Record<string, string> = {
    "content-type": "application/json",
    "cache-control": "no-store, max-age=0",
    "x-content-type-options": "nosniff",
    "referrer-policy": "no-referrer",
    "vary": "Origin",
  };
  const origin = req.headers.get("origin");
  if (origin && allowedOrigins.has(origin)) {
    h["access-control-allow-origin"] = origin;
    h["access-control-allow-methods"] = "GET, POST, OPTIONS";
    h["access-control-allow-headers"] = "authorization, content-type, x-agent-id";
    h["access-control-max-age"] = "600";
  }
  return h;
}

function json(req: Request, data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: responseHeaders(req) });
}

function bearer(req: Request) {
  const h = req.headers.get("authorization") || "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
}

async function readJSON(req: Request, maxBytes: number) {
  const declared = Number(req.headers.get("content-length") || "0");
  if (Number.isFinite(declared) && declared > maxBytes) {
    throw new ApiError("request_too_large", 413);
  }
  const text = await req.text();
  if (encoder.encode(text).byteLength > maxBytes) {
    throw new ApiError("request_too_large", 413);
  }
  if (!text.trim()) return {} as Record<string, unknown>;
  try {
    const value = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      throw new ApiError("invalid_json_object", 400);
    }
    return value as Record<string, unknown>;
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError("invalid_json", 400);
  }
}

async function sha256Text(s: string) {
  const d = await crypto.subtle.digest("SHA-256", encoder.encode(s));
  return Array.from(new Uint8Array(d)).map((b) => b.toString(16).padStart(2, "0")).join("");
}

function safeEqual(a: string, b: string) {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

function uuidLike(v: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(v);
}

function cleanLabel(v: unknown) {
  return String(v || "").replace(/[\u0000-\u001f\u007f]/g, " ").trim().slice(0, 120);
}

function screenOnlyCapabilities(v: unknown) {
  const x = v && typeof v === "object" && !Array.isArray(v) ? v as Record<string, unknown> : {};
  for (const key of Object.keys(x)) {
    if (!allowedCapabilityKeys.has(key)) throw new ApiError("capability_refused", 422);
  }
  if (x.screen_stream === false) throw new ApiError("screen_stream_required", 422);
  for (const key of ["mouse", "keyboard", "clipboard", "file_transfer", "audio"]) {
    if (x[key] !== undefined && x[key] !== false) throw new ApiError("capability_refused", 422);
  }
  return {
    screen_stream: true,
    mouse: false,
    keyboard: false,
    clipboard: false,
    file_transfer: false,
    audio: false,
  };
}

function signalKind(v: unknown) {
  const k = String(v || "").trim().toLowerCase();
  if (!["offer", "answer", "ice_candidate", "hangup"].includes(k)) {
    throw new ApiError("signal_kind_refused", 422);
  }
  return k;
}

function payloadSize(v: unknown) {
  try {
    return encoder.encode(JSON.stringify(v)).byteLength;
  } catch {
    return MAX_SIGNAL_BYTES + 1;
  }
}

function signalCursor(req: Request) {
  const raw = new URL(req.url).searchParams.get("after") || "0";
  if (!/^\d+$/.test(raw)) throw new ApiError("invalid_signal_cursor", 400);
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value < 0) throw new ApiError("invalid_signal_cursor", 400);
  return value;
}

type ControllerAuth = { tokenHash: string; agentId: string };

async function authController(req: Request): Promise<ControllerAuth | null> {
  const token = bearer(req);
  if (!token) return null;
  const tokenHash = await sha256Text(token);
  const { data, error } = await db.from("pc_remote_controller_tokens")
    .select("token_hash,agent_id,allow_screen_stream,expires_at,revoked_at")
    .eq("token_hash", tokenHash)
    .maybeSingle();
  if (error) throw error;
  if (!data || data.revoked_at || !data.allow_screen_stream) return null;
  if (Date.parse(data.expires_at) <= Date.now()) return null;
  return { tokenHash, agentId: data.agent_id };
}

async function touchControllerToken(tokenHash: string) {
  const { error } = await db.from("pc_remote_controller_tokens")
    .update({ last_used_at: new Date().toISOString() })
    .eq("token_hash", tokenHash)
    .is("revoked_at", null);
  if (error) throw error;
}

async function authAgent(req: Request, expectedAgent: string) {
  const agentId = (req.headers.get("x-agent-id") || "").trim();
  const secret = bearer(req);
  if (!agentId || !secret || agentId !== expectedAgent) return false;
  const { data, error } = await db.from("pc_relay_agents")
    .select("agent_id,secret_hash")
    .eq("agent_id", expectedAgent)
    .maybeSingle();
  if (error) throw error;
  if (!data) return false;
  return safeEqual(await sha256Text(secret), String(data.secret_hash || ""));
}

async function readSession(id: string) {
  const { data, error } = await db.from("pc_remote_sessions")
    .select("id,agent_id,state,controller_token_hash,requested_at,approved_at,connected_at,ended_at,expires_at,last_activity_at,requested_capabilities,approved_capabilities,controller_label,controller_fingerprint,reason")
    .eq("id", id)
    .maybeSingle();
  if (error) throw error;
  return data;
}

function publicSession(session: any) {
  return {
    id: session.id,
    agent_id: session.agent_id,
    state: session.state,
    requested_at: session.requested_at,
    approved_at: session.approved_at,
    connected_at: session.connected_at,
    ended_at: session.ended_at,
    expires_at: session.expires_at,
    last_activity_at: session.last_activity_at,
    requested_capabilities: session.requested_capabilities,
    approved_capabilities: session.approved_capabilities,
    reason: session.reason,
  };
}

async function audit(
  sessionId: string | null,
  agentId: string,
  eventType: string,
  actor: "agent" | "controller" | "system",
  metadata: Record<string, unknown> = {},
) {
  const { error } = await db.from("pc_remote_audit").insert({
    session_id: sessionId,
    agent_id: agentId,
    event_type: eventType,
    actor,
    metadata,
  });
  if (error) throw error;
}

async function expireRows(agentId: string) {
  const now = new Date();
  const nowISO = now.toISOString();
  const approvalCutoff = new Date(now.getTime() - APPROVAL_TTL_MS).toISOString();

  const { data: staleRequested, error: requestedError } = await db.from("pc_remote_sessions")
    .update({ state: "expired", ended_at: nowISO, last_activity_at: nowISO, reason: "approval_expired" })
    .eq("agent_id", agentId)
    .eq("state", "requested")
    .lt("requested_at", approvalCutoff)
    .select("id,agent_id");
  if (requestedError) throw requestedError;
  for (const row of staleRequested || []) await audit(row.id, row.agent_id, "approval_expired", "system");

  const { data: staleSessions, error: expiryError } = await db.from("pc_remote_sessions")
    .update({ state: "expired", ended_at: nowISO, last_activity_at: nowISO, reason: "session_expired" })
    .eq("agent_id", agentId)
    .in("state", ["requested", "approved", "connecting", "active"])
    .lte("expires_at", nowISO)
    .select("id,agent_id");
  if (expiryError) throw expiryError;
  for (const row of staleSessions || []) await audit(row.id, row.agent_id, "session_expired", "system");
}

async function expireSessionIfNeeded(session: any) {
  if (!session || terminalStates.has(session.state)) return session;
  const now = Date.now();
  let reason = "";
  if (session.state === "requested" && now - Date.parse(session.requested_at) > APPROVAL_TTL_MS) {
    reason = "approval_expired";
  } else if (Date.parse(session.expires_at) <= now) {
    reason = "session_expired";
  }
  if (!reason) return session;

  const nowISO = new Date(now).toISOString();
  const { data, error } = await db.from("pc_remote_sessions")
    .update({ state: "expired", ended_at: nowISO, last_activity_at: nowISO, reason })
    .eq("id", session.id)
    .in("state", ["requested", "approved", "connecting", "active"])
    .select("id,agent_id")
    .maybeSingle();
  if (error) throw error;
  if (data) await audit(data.id, data.agent_id, reason, "system");
  return await readSession(session.id);
}

async function endSession(session: any, actor: "agent" | "controller" | "system", reason: string) {
  const now = new Date().toISOString();
  const { data, error } = await db.from("pc_remote_sessions")
    .update({ state: "ended", ended_at: now, last_activity_at: now, reason: reason.slice(0, 200) })
    .eq("id", session.id)
    .in("state", ["requested", "approved", "connecting", "active"])
    .select("id,agent_id")
    .maybeSingle();
  if (error) throw error;
  if (data) await audit(data.id, data.agent_id, "session_ended", actor, { reason });
}

async function pushSignal(session: any, sender: "agent" | "controller", kind: string, payload: unknown) {
  session = await expireSessionIfNeeded(session);
  if (!session || !signalableStates.has(session.state)) throw new ApiError("session_not_signalable", 409);
  if (payloadSize(payload) > MAX_SIGNAL_BYTES) throw new ApiError("signal_payload_too_large", 413);

  const { data: signal, error: insertError } = await db.from("pc_remote_signals")
    .insert({ session_id: session.id, sender, kind, payload: payload ?? {} })
    .select("id,session_id,sender,kind,created_at")
    .single();
  if (insertError) throw insertError;

  const now = new Date().toISOString();
  const nextState = session.state === "approved" ? "connecting" : session.state;
  const { data: touched, error: touchError } = await db.from("pc_remote_sessions")
    .update({ state: nextState, last_activity_at: now })
    .eq("id", session.id)
    .eq("state", session.state)
    .select("id")
    .maybeSingle();
  if (touchError) throw touchError;
  if (!touched) {
    await db.from("pc_remote_signals").delete().eq("id", signal.id);
    throw new ApiError("session_state_changed", 409);
  }

  if (kind === "hangup") await endSession(session, sender, "peer_hangup");
  return signal;
}

Deno.serve(async (req: Request) => {
  const requestId = crypto.randomUUID();
  const path = pathOf(req);
  try {
    if (!originAllowed(req)) return json(req, { ok: false, error: "origin_refused" }, 403);

    if (req.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: responseHeaders(req) });
    }

    if (req.method === "GET" && path === "/health") {
      return json(req, {
        ok: true,
        service: SERVICE,
        protocol_version: 1,
        signaling_only: true,
        media_proxy: false,
        attended_approval_required: true,
        unattended_access: false,
        input_control_enabled: false,
        idle_timeout_seconds: 0,
        max_signal_bytes: MAX_SIGNAL_BYTES,
        controller_auth: "dedicated_agent_scoped_token",
        browser_origin_allowlist_configured: allowedOrigins.size > 0,
      });
    }

    if (req.method === "POST" && path === "/api/sessions") {
      const controller = await authController(req);
      if (!controller) return json(req, { ok: false, error: "controller_unauthorized" }, 401);
      const body = await readJSON(req, MAX_SESSION_REQUEST_BYTES);
      const agentId = String(body.agent_id || controller.agentId).trim();
      if (!agentId) return json(req, { ok: false, error: "agent_id_required" }, 400);
      if (agentId !== controller.agentId) return json(req, { ok: false, error: "agent_scope_refused" }, 403);
      const caps = screenOnlyCapabilities(body.capabilities);

      const { data: agent, error: agentErr } = await db.from("pc_relay_agents")
        .select("agent_id,version,last_seen_at,last_health_score")
        .eq("agent_id", agentId)
        .maybeSingle();
      if (agentErr) throw agentErr;
      if (!agent) return json(req, { ok: false, error: "agent_not_found" }, 404);
      const lastSeen = Date.parse(agent.last_seen_at || "");
      if (!Number.isFinite(lastSeen) || Date.now() - lastSeen > AGENT_ONLINE_WINDOW_MS) {
        return json(req, { ok: false, error: "agent_offline" }, 409);
      }
      if (Number(agent.last_health_score ?? 0) < 80) {
        return json(req, { ok: false, error: "agent_unhealthy" }, 409);
      }

      await expireRows(agentId);
      const now = new Date();
      const expires = new Date(now.getTime() + SESSION_TTL_MS).toISOString();
      const { data, error } = await db.from("pc_remote_sessions").insert({
        agent_id: agentId,
        controller_token_hash: controller.tokenHash,
        state: "requested",
        requested_capabilities: caps,
        approved_capabilities: {},
        expires_at: expires,
        controller_label: cleanLabel(body.controller_label),
        controller_fingerprint: cleanLabel(body.controller_fingerprint),
      }).select("id,agent_id,state,requested_at,expires_at,requested_capabilities").single();
      if (error) {
        if ((error as any)?.code === "23505") throw new ApiError("session_already_live", 409);
        throw error;
      }
      await touchControllerToken(controller.tokenHash);
      await audit(data.id, agentId, "session_requested", "controller", { capabilities: caps });
      return json(req, data, 202);
    }

    const controllerStatus = path.match(/^\/api\/sessions\/([^/]+)$/);
    if (controllerStatus && req.method === "GET") {
      const id = controllerStatus[1];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      const controller = await authController(req);
      if (!controller) return json(req, { ok: false, error: "controller_unauthorized" }, 401);
      let session = await readSession(id);
      if (!session || session.controller_token_hash !== controller.tokenHash || session.agent_id !== controller.agentId) {
        return json(req, { ok: false, error: "session_not_found" }, 404);
      }
      session = await expireSessionIfNeeded(session);
      return json(req, publicSession(session));
    }

    const agentPending = path.match(/^\/api\/agent\/([^/]+)\/sessions\/pending$/);
    if (agentPending && req.method === "GET") {
      const agentId = decodeURIComponent(agentPending[1]);
      if (!(await authAgent(req, agentId))) return json(req, { ok: false, error: "agent_unauthorized" }, 401);
      await expireRows(agentId);
      const cutoff = new Date(Date.now() - APPROVAL_TTL_MS).toISOString();
      const now = new Date().toISOString();
      const { data, error } = await db.from("pc_remote_sessions")
        .select("id,state,requested_at,expires_at,requested_capabilities,controller_label,controller_fingerprint")
        .eq("agent_id", agentId)
        .eq("state", "requested")
        .gte("requested_at", cutoff)
        .gt("expires_at", now)
        .order("requested_at", { ascending: true })
        .limit(5);
      if (error) throw error;
      return json(req, { sessions: data || [] });
    }

    const approve = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/approve$/);
    if (approve && req.method === "POST") {
      const agentId = decodeURIComponent(approve[1]), id = approve[2];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json(req, { ok: false, error: "agent_unauthorized" }, 401);
      let session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json(req, { ok: false, error: "session_not_found" }, 404);
      session = await expireSessionIfNeeded(session);
      if (!session || session.state === "expired") return json(req, { ok: false, error: "approval_window_expired" }, 410);
      if (session.state !== "requested") return json(req, { ok: false, error: "session_not_requestable" }, 409);
      const caps = screenOnlyCapabilities(session.requested_capabilities);
      const now = new Date().toISOString();
      const { data, error } = await db.from("pc_remote_sessions")
        .update({ state: "approved", approved_at: now, last_activity_at: now, approved_capabilities: caps })
        .eq("id", id)
        .eq("state", "requested")
        .select("id")
        .maybeSingle();
      if (error) throw error;
      if (!data) return json(req, { ok: false, error: "session_state_changed" }, 409);
      await audit(id, agentId, "session_approved", "agent", { capabilities: caps });
      return json(req, { ok: true, id, state: "approved", approved_capabilities: caps });
    }

    const reject = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/reject$/);
    if (reject && req.method === "POST") {
      const agentId = decodeURIComponent(reject[1]), id = reject[2];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json(req, { ok: false, error: "agent_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json(req, { ok: false, error: "session_not_found" }, 404);
      const now = new Date().toISOString();
      const { data, error } = await db.from("pc_remote_sessions")
        .update({ state: "rejected", ended_at: now, last_activity_at: now, reason: "user_rejected" })
        .eq("id", id)
        .eq("state", "requested")
        .select("id")
        .maybeSingle();
      if (error) throw error;
      if (!data) return json(req, { ok: false, error: "session_state_changed" }, 409);
      await audit(id, agentId, "session_rejected", "agent");
      return json(req, { ok: true, id, state: "rejected" });
    }

    const active = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/active$/);
    if (active && req.method === "POST") {
      const agentId = decodeURIComponent(active[1]), id = active[2];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json(req, { ok: false, error: "agent_unauthorized" }, 401);
      let session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json(req, { ok: false, error: "session_not_found" }, 404);
      session = await expireSessionIfNeeded(session);
      if (!session || !["approved", "connecting"].includes(session.state)) {
        return json(req, { ok: false, error: "session_not_connecting" }, 409);
      }
      const now = new Date().toISOString();
      const { data, error } = await db.from("pc_remote_sessions")
        .update({ state: "active", connected_at: now, last_activity_at: now })
        .eq("id", id)
        .in("state", ["approved", "connecting"])
        .select("id")
        .maybeSingle();
      if (error) throw error;
      if (!data) return json(req, { ok: false, error: "session_state_changed" }, 409);
      await audit(id, agentId, "session_active", "agent");
      return json(req, { ok: true, id, state: "active", connected_at: now });
    }

    const controllerSignal = path.match(/^\/api\/sessions\/([^/]+)\/signals$/);
    if (controllerSignal) {
      const id = controllerSignal[1];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      const controller = await authController(req);
      if (!controller) return json(req, { ok: false, error: "controller_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.controller_token_hash !== controller.tokenHash || session.agent_id !== controller.agentId) {
        return json(req, { ok: false, error: "session_not_found" }, 404);
      }
      if (req.method === "POST") {
        const body = await readJSON(req, MAX_SIGNAL_REQUEST_BYTES);
        const data = await pushSignal(session, "controller", signalKind(body.kind), body.payload ?? {});
        return json(req, data, 202);
      }
      if (req.method === "GET") {
        const after = signalCursor(req);
        const { data, error } = await db.from("pc_remote_signals")
          .select("id,sender,kind,payload,created_at")
          .eq("session_id", id)
          .eq("sender", "agent")
          .gt("id", after)
          .order("id", { ascending: true })
          .limit(100);
        if (error) throw error;
        return json(req, { signals: data || [] });
      }
    }

    const agentSignal = path.match(/^\/api\/agent\/([^/]+)\/sessions\/([^/]+)\/signals$/);
    if (agentSignal) {
      const agentId = decodeURIComponent(agentSignal[1]), id = agentSignal[2];
      if (!uuidLike(id)) return json(req, { ok: false, error: "invalid_session_id" }, 400);
      if (!(await authAgent(req, agentId))) return json(req, { ok: false, error: "agent_unauthorized" }, 401);
      const session = await readSession(id);
      if (!session || session.agent_id !== agentId) return json(req, { ok: false, error: "session_not_found" }, 404);
      if (req.method === "POST") {
        const body = await readJSON(req, MAX_SIGNAL_REQUEST_BYTES);
        const data = await pushSignal(session, "agent", signalKind(body.kind), body.payload ?? {});
        return json(req, data, 202);
      }
      if (req.method === "GET") {
        const after = signalCursor(req);
        const { data, error } = await db.from("pc_remote_signals")
          .select("id,sender,kind,payload,created_at")
          .eq("session_id", id)
          .eq("sender", "controller")
          .gt("id", after)
          .order("id", { ascending: true })
          .limit(100);
        if (error) throw error;
        return json(req, { signals: data || [] });
      }
    }

    return json(req, { ok: false, error: "not_found" }, 404);
  } catch (e) {
    if (e instanceof ApiError) return json(req, { ok: false, error: e.code }, e.status);
    console.error(JSON.stringify({ request_id: requestId, error: String((e as any)?.message || e) }));
    return json(req, { ok: false, error: "remote_signaling_error", request_id: requestId }, 500);
  }
});
