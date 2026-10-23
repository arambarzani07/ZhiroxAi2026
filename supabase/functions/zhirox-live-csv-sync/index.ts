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
  try { s = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true }).decode(src); }
  catch { return false; }
  return s.startsWith(HEADER) && s.endsWith("\r\n") && !s.includes("\u0000");
}

const KURDISH_LATEST = "kurdish/Server-c24b5c0f/SuperMarket_ItemName_latest.csv";
function soraniNormalize(name: string) {
  return name.normalize("NFC")
    .replace(/ي/g,"ی").replace(/ى/g,"ی").replace(/ك/g,"ک")
    .replace(/ة/g,"ە").replace(/ۀ/g,"ە")
    .replace(/\u0640/g,"").replace(/[ \t]+/g," ").trim();
}
function parseCSV(csv: string): string[][] {
  const rows:string[][]=[];let row:string[]=[],value="",quoted=false;
  for(let i=0;i<csv.length;i++){
    const ch=csv[i];
    if(quoted){
      if(ch==='"' && csv[i+1]==='"'){value+='"';i++;}
      else if(ch==='"')quoted=false;else value+=ch;
    }else if(ch==='"'){quoted=true;}
    else if(ch===','){row.push(value);value="";}
    else if(ch==='\n'){row.push(value.replace(/\r$/,""));rows.push(row);row=[];value="";}
    else value+=ch;
  }
  if(quoted)throw new Error("unclosed_csv_quote");
  if(value!==""||row.length){row.push(value);rows.push(row);}
  return rows;
}
function csvEscape(v:string){return /[",\r\n]/.test(v)?'"'+v.replace(/"/g,'""')+'"':v}
async function persistKurdishCSV(agentId:string,file:Uint8Array,srcHash:string,expectedRows:number){
  const header=["ItemNameID","ItemName","ItemCode","SalePrice","WholeSalePrice"];
  const text=new TextDecoder("utf-8",{fatal:true,ignoreBOM:true}).decode(file);
  const records=parseCSV(text);
  if(JSON.stringify(records[0])!==JSON.stringify(["\uFEFF"+header[0],...header.slice(1)])||records.length!==expectedRows+1)
    throw new Error("kurdish_csv_structure_mismatch");
  const overrides=new Map<string,string>();
  for(let from=0;;from+=1000){
    const {data,error}=await DB.from("pc_csv_kurdish_names").select("item_name_id,kurdish_name,status")
      .eq("agent_id",agentId).eq("status","verified").range(from,from+999);
    if(error)throw new Error("kurdish_override_read_failed");
    for(const r of data||[])overrides.set(String(r.item_name_id),r.kurdish_name);
    if((data||[]).length<1000)break;
  }
  let verified=0,normalized=0,review=0;
  const outRows:string[]=["\uFEFF"+header.join(",")];
  for(const r of records.slice(1)){
    if(r.length!==5||!r[0])throw new Error("kurdish_csv_invalid_row");
    const confirmed=overrides.get(r[0]);
    const adjusted=confirmed??soraniNormalize(r[1]);
    if(confirmed!==undefined)verified++; else if(/[اأإءآ]/.test(adjusted))review++;else normalized++;
    outRows.push([r[0],adjusted,r[2],r[3],r[4]].map(csvEscape).join(","));
  }
  const out=new TextEncoder().encode(outRows.join("\r\n")+"\r\n");
  if(out.byteLength>MAX_BYTES)throw new Error("kurdish_csv_oversize");
  const digest=await sha(out);
  const immutable=agentId+"/kurdish/versions/"+digest+".csv";
  for(const [key,upsert] of [[immutable,false],[KURDISH_LATEST,true]] as const){
    const {error}=await DB.storage.from(BUCKET).upload(key,out,{contentType:"text/csv",upsert,cacheControl:"0"});
    if(error && !(key===immutable && /already exists|duplicate|resourcealreadyexists/i.test(error.message)))
      throw new Error("kurdish_csv_storage_failed");
  }
  const {error}=await DB.from("pc_csv_kurdish_manifest").upsert({
    agent_id:agentId,source_sha256:srcHash,output_sha256:digest,
    storage_path:KURDISH_LATEST,rows:expectedRows,bytes:out.length,
    names_verified:verified,names_normalized:normalized,names_needing_review:review,
    updated_at:new Date().toISOString()
  },{onConflict:"agent_id"});
  if(error)throw new Error("kurdish_manifest_write_failed");
  return {ready:true,sha256:digest,rows:expectedRows,verified,normalized,review};
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
    if (existing?.sha256===actual) {
      try {const k=await persistKurdishCSV(agentId,file,actual,rowCount);return json({ok:true,sha256:actual,unchanged:true,rows:rowCount,kurdish:k});}
      catch(e){console.error("kurdish_derivation_failed",e);return json({ok:true,sha256:actual,unchanged:true,rows:rowCount,kurdish_ready:false});}
    }
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
    try{const k=await persistKurdishCSV(agentId,file,actual,rowCount);return json({ok:true,sha256:actual,rows:rowCount,bytes:file.length,private:true,kurdish:k});}
    catch(e){console.error("kurdish_derivation_failed",e);return json({ok:true,sha256:actual,rows:rowCount,bytes:file.length,private:true,kurdish_ready:false});}
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