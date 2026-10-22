import { createClient } from "npm:@supabase/supabase-js@2";
const DB = createClient(Deno.env.get("SUPABASE_URL")!, Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")!, {
  auth: { persistSession: false, autoRefreshToken: false },
});
const BUCKET = "zhirox-csv-private";
const MAX_BYTES = 8388608;
const HEADER = "\uFEFFItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice\r\n";
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), {
  status, headers: { "content-type": "application/json", "cache-control": "no-store", "x-content-type-options": "nosniff" },
});
const bearer = (r: Request) => {
  const h = r.headers.get("authorization") || "";
  return h.toLowerCase().startsWith("bearer ") ? h.slice(7).trim() : "";
};
const sha = async (b: Uint8Array) => Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", b))).map(v => v.toString(16).padStart(2, "0")).join("");
async function agentAuth(req: Request, agentId: string) {
  const h = req.headers.get("x-agent-id") || "";
  const b = bearer(req);
  if (!h || h !== agentId || !b) return null;
  const { data, error } = await DB.from("pc_relay_agents").select("agent_id,secret_hash,version").eq("agent_id", agentId).maybeSingle();
  if (error || !data || !/^5\.(?:1\.(?:1[2-9]|[2-9][0-9]))/.test(data.version || "")) return null;
  const tokenHash = await sha(new TextEncoder().encode(b));
  return tokenHash === data.secret_hash ? data : null;
}
async function ownerConnectorAuth(req: Request) {
  const b = bearer(req);
  if (!b) return false;
  const tokenHash = await sha(new TextEncoder().encode(b));
  const { data, error } = await DB.from("pc_relay_connector_tokens").select("token_hash,revoked_at").eq("token_hash", tokenHash).is("revoked_at", null).maybeSingle();
  return !error && !!data;
}
async function readMax(req: Request): Promise<Uint8Array> {
  if (!req.body) throw new Error("empty_file");
  const reader = req.body.getReader();
  const chunks: Uint8Array[] = [];
  let count = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      count += value.byteLength;
      if (count > MAX_BYTES) { await reader.cancel(); throw new Error("file_oversize"); }
      chunks.push(value);
    }
  } finally { reader.releaseLock(); }
  const out = new Uint8Array(count);
  let pos = 0; for (const chunk of chunks) { out.set(chunk,pos); pos += chunk.length; }
  return out;
}
function validCSV(src: Uint8Array): boolean {
  if (src.length < 80) return false;
  let s: string;
  try { s = new TextDecoder("utf-8", { fatal: true }).decode(src); }
  catch { return false; }
  return s.startsWith(HEADER) && s.endsWith("\r\n") && !s.includes("\u0000");
}
Deno.serve(async (req: Request) => {
  const raw = new URL(req.url).pathname;
  const marker = "/zhirox-live-csv-sync";
  const path = raw.slice(raw.indexOf(marker) + marker.length);
  if (req.method === "GET" && path === "/health") return json({ok:true,private:true,max_bytes:MAX_BYTES});
  const upload = path.match(/^\/api\/agent\/([A-Za-z0-9_-]{6,100})\/upload$/);
  if (upload && req.method === "POST") {
    const agentId = upload[1];
    if (!(await agentAuth(req, agentId))) return json({ok:false,error:"unauthorized"},401);
    const claimedSha = req.headers.get("x-csv-sha256") || "";
    const rowCount = Number(req.headers.get("x-csv-rows") || "");
    const capture = req.headers.get("x-csv-captured-utc") || "";
    const capturedAt = new Date(capture);
    const contentType = (req.headers.get("content-type")||"").toLowerCase();
    if (!/^[a-f0-9]{64}$/.test(claimedSha) || !Number.isInteger(rowCount) || rowCount<1 || rowCount>150000 ||
        !Number.isFinite(capturedAt.getTime()) || capturedAt.getTime()>Date.now()+60000 || capturedAt.getTime()<Date.now()-86400000*14 ||
        !contentType.startsWith("text/csv")) return json({ok:false,error:"invalid_csv_metadata"},422);
    const len = Number(req.headers.get("content-length")||"0");
    if (len>MAX_BYTES) return json({ok:false,error:"file_oversize"},413);
    let file: Uint8Array;
    try { file = await readMax(req); }
    catch { return json({ok:false,error:"invalid_file_body"},413); }
    if (!validCSV(file)) return json({ok:false,error:"invalid_utf8_csv"},422);
    const actual = await sha(file);
    if (actual !== claimedSha) return json({ok:false,error:"hash_mismatch"},422);
    const { data: existing, error: existingError } = await DB.from("pc_live_csv_manifest").select("sha256,source_captured_utc").eq("agent_id",agentId).maybeSingle();
    if (existingError) return json({ok:false,error:"manifest_unavailable"},503);
    if (existing && new Date(existing.source_captured_utc).getTime()>capturedAt.getTime())
      return json({ok:false,error:"stale_snapshot"},409);
    if (existing?.sha256===actual) return json({ok:true,sha256:actual,unchanged:true,rows:rowCount});
    const pathKey = agentId + "/versions/" + actual + ".csv";
    const { error: uploadError } = await DB.storage.from(BUCKET).upload(pathKey, file, {
      contentType:"text/csv",cacheControl:"0",upsert:false,
    });
    if (uploadError && !/already exists|duplicate|resourcealreadyexists/i.test(uploadError.message))
      return json({ok:false,error:"storage_unavailable"},503);
    const { error: manifestError } = await DB.from("pc_live_csv_manifest").upsert({
      agent_id:agentId,sha256:actual,storage_bucket:BUCKET,storage_path:pathKey,
      bytes:file.length,row_count:rowCount,source_captured_utc:capturedAt.toISOString(),uploaded_at:new Date().toISOString()
    },{onConflict:"agent_id"});
    if (manifestError) return json({ok:false,error:"manifest_save_failed"},503);
    return json({ok:true,sha256:actual,rows:rowCount,bytes:file.length,private:true});
  }
  const download = path.match(/^\/api\/connector\/([A-Za-z0-9_-]{6,100})\/download$/);
  if (download && req.method==="GET") {
    if (!(await ownerConnectorAuth(req))) return json({ok:false,error:"unauthorized"},401);
    const agentId=download[1];
    const {data, error}=await DB.from("pc_live_csv_manifest").select("storage_path,sha256,row_count,bytes,uploaded_at").eq("agent_id",agentId).maybeSingle();
    if(error || !data) return json({ok:false,error:"csv_not_uploaded"},404);
    const {data:link,error:linkError}=await DB.storage.from(BUCKET).createSignedUrl(data.storage_path,120,{
      download:"SuperMarket_ItemName_latest.csv"
    });
    if(linkError||!link?.signedUrl)return json({ok:false,error:"download_unavailable"},503);
    return json({ok:true,url:link.signedUrl,sha256:data.sha256,rows:data.row_count,bytes:data.bytes,uploaded_at:data.uploaded_at,expires_in_seconds:120});
  }
  return json({ok:false,error:"not_found"},404);
});