const $ = (id) => document.getElementById(id);
const endpointEl = $("endpoint");
const tokenEl = $("token");
const agentEl = $("agent");
const labelEl = $("label");
const connectBtn = $("connect");
const disconnectBtn = $("disconnect");
const armInputBtn = $("arm-input");
const disarmInputBtn = $("disarm-input");
const inputStateEl = $("input-state");
const videoEl = $("screen");
const statusEl = $("status");

const API_TIMEOUT_MS = 12_000;
const SIGNALING_PATH = "/functions/v1/zhirox-pc-remote-signaling-v1";
const CONTROL_CHANNEL = "zhirox-control";

let pc = null;
let sessionId = "";
let signalCursor = 0;
let polling = false;
let controllerToken = "";
let baseURL = "";
let controlChannel = null;
let inputController = null;
let unbindInput = null;

function status(message) {
  statusEl.textContent = `${new Date().toISOString()}  ${message}\n${statusEl.textContent}`.slice(0, 12000);
}

function setInputState(message) {
  inputStateEl.textContent = message;
}

function authHeaders(json = false) {
  const h = { Authorization: `Bearer ${controllerToken}` };
  if (json) h["Content-Type"] = "application/json";
  return h;
}

async function api(path, init = {}) {
  const aborter = new AbortController();
  const timer = setTimeout(() => aborter.abort(), API_TIMEOUT_MS);
  try {
    const res = await fetch(`${baseURL}${path}`, {
      ...init,
      signal: aborter.signal,
      credentials: "omit",
      referrerPolicy: "no-referrer",
    });
    const text = await res.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch { data = null; }
    if (!res.ok) throw new Error(`${res.status} ${data?.error || res.statusText}`);
    return data;
  } catch (err) {
    if (err?.name === "AbortError") throw new Error("signaling request timed out");
    throw err;
  } finally {
    clearTimeout(timer);
  }
}

function secureEndpoint(raw) {
  const value = raw.trim().replace(/\/+$/, "");
  let u;
  try { u = new URL(value); } catch { throw new Error("signaling endpoint must be an absolute URL"); }
  const local = u.hostname === "localhost" || u.hostname === "127.0.0.1" || u.hostname === "[::1]" || u.hostname === "::1";
  if (u.protocol !== "https:" && !(local && u.protocol === "http:")) {
    throw new Error("signaling endpoint must use HTTPS (HTTP is allowed only for localhost development)");
  }
  if (u.username || u.password) throw new Error("signaling endpoint must not contain URL credentials");
  if (u.search || u.hash) throw new Error("signaling endpoint must not contain query or fragment data");
  if (u.pathname.replace(/\/+$/, "") !== SIGNALING_PATH) {
    throw new Error("signaling endpoint path is not the approved Zhirox remote-signaling function");
  }
  return value;
}

function randomFingerprint() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

function cleanLabel(value) {
  return String(value || "").replace(/[\u0000-\u001f\u007f]/g, " ").trim().slice(0, 120);
}

async function requestSession() {
  const endpoint = secureEndpoint(endpointEl.value);
  const token = tokenEl.value.trim();
  const agent = agentEl.value.trim();
  if (token.length < 32 || token.length > 512) throw new Error("controller token length is invalid");
  if (!agent || agent.length > 160 || !/^[A-Za-z0-9._:-]+$/.test(agent)) throw new Error("agent id is invalid");
  baseURL = endpoint;
  controllerToken = token; // Memory only; never persisted to local/session storage.

  const data = await api("/api/sessions", {
    method: "POST",
    headers: authHeaders(true),
    body: JSON.stringify({
      agent_id: agent,
      controller_label: cleanLabel(labelEl.value),
      controller_fingerprint: randomFingerprint(),
      capabilities: {
        screen_stream: true,
        mouse: true,
        keyboard: true,
        clipboard: false,
        file_transfer: false,
        audio: false,
      },
    }),
  });
  sessionId = data.id;
  status(`attended session requested: ${sessionId}; waiting for explicit local approval on target PC`);
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

function teardownInput(reason) {
  try { if (inputController) inputController.disarm(reason || "session teardown"); } catch (_) {}
  try { if (unbindInput) unbindInput(); } catch (_) {}
  unbindInput = null;
  inputController = null;
  controlChannel = null;
  armInputBtn.disabled = true;
  disarmInputBtn.disabled = true;
  setInputState("inactive");
}

function attachControlChannel(channel) {
  if (!channel || channel.label !== CONTROL_CHANNEL) {
    status(`unexpected data channel refused: ${channel?.label || "unknown"}`);
    try { channel?.close(); } catch (_) {}
    return;
  }
  if (!window.ZhiroxAttendedInputV1) {
    status("attended input module unavailable; closing control channel");
    try { channel.close(); } catch (_) {}
    return;
  }

  controlChannel = channel;
  inputController = window.ZhiroxAttendedInputV1.createAttendedInputController({
    send: (payload) => {
      if (!controlChannel || controlChannel.readyState !== "open") throw new Error("control channel is not open");
      controlChannel.send(payload);
    },
    status: (message) => {
      setInputState(message);
      status(message);
    },
  });
  unbindInput = window.ZhiroxAttendedInputV1.bindBrowserInput(videoEl, inputController, window);

  channel.onopen = () => {
    if (channel !== controlChannel || !inputController) return;
    inputController.setAvailable(true);
    armInputBtn.disabled = false;
    disarmInputBtn.disabled = true;
    setInputState("ready — click Arm input after target-side approval/indicator is visible");
    status("attended control channel ready; input remains disarmed until explicit browser arm");
  };
  channel.onclose = () => {
    if (channel !== controlChannel) return;
    teardownInput("control channel closed");
    status("control channel closed; browser input fail-closed");
  };
  channel.onerror = () => {
    if (channel !== controlChannel) return;
    teardownInput("control channel error");
    status("control channel error; browser input fail-closed");
  };
}

async function setupPeer() {
  pc = new RTCPeerConnection({ iceServers: [], iceTransportPolicy: "all", bundlePolicy: "max-bundle" });
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
    const state = pc?.connectionState || "closed";
    status(`peer state: ${state}`);
    if (["disconnected", "failed", "closed"].includes(state)) {
      try { if (inputController) inputController.disarm(`peer ${state}`); } catch (_) {}
      armInputBtn.disabled = true;
      disarmInputBtn.disabled = true;
    }
    if (["failed", "closed"].includes(state)) disconnect(false).catch(() => {});
  };
  pc.ondatachannel = (event) => attachControlChannel(event.channel);

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
    status("target approved attended session");
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
  teardownInput("disconnect");
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
armInputBtn.addEventListener("click", () => {
  try {
    if (!inputController) throw new Error("control channel is not ready");
    inputController.arm();
    armInputBtn.disabled = true;
    disarmInputBtn.disabled = false;
    videoEl.focus({ preventScroll: true });
  } catch (err) {
    status(`input arm failed: ${err.message}`);
  }
});
disarmInputBtn.addEventListener("click", () => {
  try { if (inputController) inputController.disarm("manual browser disarm"); } catch (_) {}
  armInputBtn.disabled = !controlChannel || controlChannel.readyState !== "open";
  disarmInputBtn.disabled = true;
});
window.addEventListener("beforeunload", () => {
  polling = false;
  try { if (inputController) inputController.disarm("page unload"); } catch (_) {}
  controllerToken = "";
  tokenEl.value = "";
  if (pc) pc.close();
});
