const $ = (id) => document.getElementById(id);
const endpointEl = $("endpoint");
const tokenEl = $("token");
const agentEl = $("agent");
const labelEl = $("label");
const iceEl = $("ice");
const connectBtn = $("connect");
const disconnectBtn = $("disconnect");
const videoEl = $("screen");
const statusEl = $("status");

let pc = null;
let sessionId = "";
let signalCursor = 0;
let polling = false;
let controllerToken = "";
let baseURL = "";

function status(message) {
  statusEl.textContent = `${new Date().toISOString()}  ${message}\n${statusEl.textContent}`.slice(0, 12000);
}

function authHeaders(json = false) {
  const h = { Authorization: `Bearer ${controllerToken}` };
  if (json) h["Content-Type"] = "application/json";
  return h;
}

async function api(path, init = {}) {
  const res = await fetch(`${baseURL}${path}`, init);
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) throw new Error(`${res.status} ${data?.error || res.statusText}${data?.detail ? `: ${data.detail}` : ""}`);
  return data;
}

function readICEServers() {
  const raw = iceEl.value.trim();
  if (!raw) return [];
  const parsed = JSON.parse(raw);
  if (!Array.isArray(parsed)) throw new Error("ICE servers must be a JSON array");
  return parsed;
}

async function requestSession() {
  const endpoint = endpointEl.value.trim().replace(/\/+$/, "");
  const token = tokenEl.value.trim();
  const agent = agentEl.value.trim();
  if (!endpoint || !token || !agent) throw new Error("endpoint, controller token and agent id are required");
  baseURL = endpoint;
  controllerToken = token; // memory only; never persisted to local/session storage.

  const data = await api("/api/sessions", {
    method: "POST",
    headers: authHeaders(true),
    body: JSON.stringify({
      agent_id: agent,
      controller_label: labelEl.value.trim().slice(0, 120),
      controller_fingerprint: `${navigator.platform || "browser"}|${navigator.userAgent.slice(0, 80)}`,
      capabilities: { screen_stream: true },
    }),
  });
  sessionId = data.id;
  status(`session requested: ${sessionId}; waiting for local approval on target PC`);
}

async function waitForApproval() {
  while (sessionId) {
    const s = await api(`/api/sessions/${encodeURIComponent(sessionId)}`, {
      headers: authHeaders(),
      cache: "no-store",
    });
    if (s.state === "approved" || s.state === "connecting" || s.state === "active") return s;
    if (["rejected", "expired", "ended"].includes(s.state)) throw new Error(`session ${s.state}`);
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error("session cancelled");
}

function preferH264(transceiver) {
  if (!transceiver?.setCodecPreferences || !window.RTCRtpReceiver?.getCapabilities) return;
  const caps = RTCRtpReceiver.getCapabilities("video");
  if (!caps?.codecs) return;
  const h264 = caps.codecs.filter((c) => c.mimeType?.toLowerCase() === "video/h264");
  const other = caps.codecs.filter((c) => c.mimeType?.toLowerCase() !== "video/h264");
  if (h264.length) transceiver.setCodecPreferences([...h264, ...other]);
}

async function postSignal(kind, payload) {
  return api(`/api/sessions/${encodeURIComponent(sessionId)}/signals`, {
    method: "POST",
    headers: authHeaders(true),
    body: JSON.stringify({ kind, payload }),
  });
}

async function setupPeer() {
  pc = new RTCPeerConnection({ iceServers: readICEServers(), bundlePolicy: "max-bundle" });
  const transceiver = pc.addTransceiver("video", { direction: "recvonly" });
  preferH264(transceiver);

  pc.ontrack = (event) => {
    const stream = event.streams?.[0] || new MediaStream([event.track]);
    videoEl.srcObject = stream;
    status(`screen track received: ${event.track.kind}`);
  };
  pc.onicecandidate = (event) => {
    if (!event.candidate || !sessionId) return;
    postSignal("ice_candidate", event.candidate.toJSON()).catch((err) => status(`ICE send failed: ${err.message}`));
  };
  pc.onconnectionstatechange = () => {
    status(`peer state: ${pc.connectionState}`);
    if (["failed", "closed"].includes(pc.connectionState)) disconnect(false).catch(() => {});
  };
  pc.ondatachannel = (event) => {
    // Phase 1/2 deliberately does not send keyboard/mouse events.
    status(`control channel received (${event.channel.label}); input remains disabled`);
    event.channel.onmessage = () => {};
  };

  const offer = await pc.createOffer({ offerToReceiveVideo: true });
  await pc.setLocalDescription(offer);
  await postSignal("offer", { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
  status("WebRTC offer sent");
}

async function consumeAgentSignals() {
  polling = true;
  while (polling && sessionId && pc) {
    const data = await api(`/api/sessions/${encodeURIComponent(sessionId)}/signals?after=${signalCursor}`, {
      headers: authHeaders(),
      cache: "no-store",
    });
    for (const signal of data.signals || []) {
      signalCursor = Math.max(signalCursor, Number(signal.id) || 0);
      if (signal.kind === "answer") {
        if (!pc.currentRemoteDescription) {
          await pc.setRemoteDescription(new RTCSessionDescription(signal.payload));
          status("WebRTC answer applied");
        }
      } else if (signal.kind === "ice_candidate") {
        await pc.addIceCandidate(new RTCIceCandidate(signal.payload));
      } else if (signal.kind === "hangup") {
        status("target ended the session");
        await disconnect(false);
        return;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 350));
  }
}

async function connect() {
  connectBtn.disabled = true;
  disconnectBtn.disabled = false;
  try {
    await requestSession();
    await waitForApproval();
    status("target approved session");
    await setupPeer();
    consumeAgentSignals().catch((err) => {
      status(`signal polling failed: ${err.message}`);
      disconnect(false).catch(() => {});
    });
  } catch (err) {
    status(`connect failed: ${err.message}`);
    await disconnect(false);
  }
}

async function disconnect(sendHangup = true) {
  polling = false;
  const oldSession = sessionId;
  if (sendHangup && oldSession && baseURL && controllerToken) {
    try { await postSignal("hangup", { reason: "controller_disconnect" }); } catch (_) {}
  }
  sessionId = "";
  signalCursor = 0;
  if (pc) {
    try { pc.close(); } catch (_) {}
    pc = null;
  }
  if (videoEl.srcObject) {
    for (const track of videoEl.srcObject.getTracks()) track.stop();
    videoEl.srcObject = null;
  }
  controllerToken = "";
  tokenEl.value = "";
  connectBtn.disabled = false;
  disconnectBtn.disabled = true;
  status("disconnected; controller token cleared from memory/input");
}

connectBtn.addEventListener("click", () => connect());
disconnectBtn.addEventListener("click", () => disconnect(true));
window.addEventListener("beforeunload", () => {
  polling = false;
  if (pc) pc.close();
});
