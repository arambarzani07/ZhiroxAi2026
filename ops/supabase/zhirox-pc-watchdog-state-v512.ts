import { createClient } from "npm:@supabase/supabase-js@2";

const db = createClient(
  Deno.env.get("SUPABASE_URL")!,
  Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!,
  { auth: { persistSession: false, autoRefreshToken: false } },
);

const FUNCTION_SLUG = "zhirox-pc-watchdog-state-v512";
const encoder = new TextEncoder();
const headers = {
  "content-type": "application/json",
  "cache-control": "no-store, max-age=0",
  "x-content-type-options": "nosniff",
};

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers });
}

function normalizedPath(req: Request) {
  const p = new URL(req.url).pathname;
  const marker = `/${FUNCTION_SLUG}`;
  const i = p.indexOf(marker);
  return i >= 0 ? (p.slice(i + marker.length) || "/") : p;
}

function bearer(req: Request) {
  const h = req.headers.get("authorization") || "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
}

async function sha256(s: string) {
  const digest = await crypto.subtle.digest("SHA-256", encoder.encode(s));
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

async function authAgent(req: Request, expectedId: string) {
  const headerId = (req.headers.get("x-agent-id") || "").trim();
  const secret = bearer(req);
  if (!headerId || !secret || headerId !== expectedId) return null;

  const { data, error } = await db
    .from("pc_relay_agents")
    .select("agent_id,secret_hash,version,last_seen_at,last_health_score")
    .eq("agent_id", expectedId)
    .maybeSingle();
  if (error || !data) return null;
  if ((await sha256(secret)) !== data.secret_hash) return null;
  return data;
}

Deno.serve(async (req: Request) => {
  const path = normalizedPath(req);

  if (req.method === "GET" && path === "/health") {
    return json({
      ok: true,
      service: FUNCTION_SLUG,
      read_only: true,
      mutates_agent_state: false,
      heartbeat_write: false,
      purpose: "server-observed heartbeat age for bounded zero-touch recovery",
    });
  }

  const state = path.match(/^\/api\/agent\/([^/]+)\/state$/);
  if (!state || req.method !== "GET") {
    return json({ ok: false, error: "not_found" }, 404);
  }

  try {
    const agentId = decodeURIComponent(state[1]);
    const agent = await authAgent(req, agentId);
    if (!agent) return json({ ok: false, error: "unauthorized" }, 401);

    const serverNow = Date.now();
    const parsedLastSeen = agent.last_seen_at ? Date.parse(String(agent.last_seen_at)) : Number.NaN;
    const heartbeatAgeSeconds = Number.isFinite(parsedLastSeen)
      ? Math.max(0, Math.floor((serverNow - parsedLastSeen) / 1000))
      : null;

    return json({
      ok: true,
      agent_id: agent.agent_id,
      version: agent.version ?? null,
      last_seen_at: agent.last_seen_at ?? null,
      heartbeat_age_seconds: heartbeatAgeSeconds,
      last_health_score: agent.last_health_score ?? null,
      server_time: new Date(serverNow).toISOString(),
      heartbeat_fresh: heartbeatAgeSeconds !== null && heartbeatAgeSeconds < 120,
      stale_after_seconds: 120,
      read_only: true,
    });
  } catch (e) {
    console.error(e);
    return json({ ok: false, error: "server_error" }, 500);
  }
});
