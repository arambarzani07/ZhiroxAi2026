const SERVICE="zhirox-pc-update-v510-production",TARGET="Server-c24b5c0f";
const PREVIOUS={version:"5.1.15",sha:"74cffc3eafab23cdead7b7d25df65cdb8d950ca0774a6b55f7d4c6a3dc21048b",size:20027904,sig:"8TSQG/IGmjPAKac4VHezC94E7f7+TJ00Em4ZgV2VznSzKNjuvkGMWMZoV8Y/Ur0BbPpYDeAA8evcFYhXBYFTBw==",commit:"7185cd9625511eb48cb2974c5ed9920662d7150a",url:"https://github.com/arambarzani07/ZhiroxAi2026/releases/download/v5.1.15-candidate-7185cd96/ZhiroxAISupervisor.exe"};
const NEXT={version:"5.1.17",sha:"8e5c6e3c97e7362aaee4fda6afa331f0bc38d8e792f3bd98072bfb815c58e063",size:20037120,sig:"AoQYY6Ufxrmy4NZOTsUj2YW7D9CTXJ5g7mnyg4ms/UcupL21M8IyzNYNWGcCDzztBUItCMOFRh3egUIS3x8GCw==",commit:"5c82e8b55dca561f934dec4bd927c849bcaf1cd6",url:"https://github.com/arambarzani07/ZhiroxAi2026/releases/download/v5.1.17-candidate-5c82e8b5/ZhiroxAISupervisor.exe"};
const CANDIDATE={version:"5.1.18",sha:"07a006867d6763d67fa866e33c5cca2eab5d49634881c4312979015f3ae5a469",size:20055552,sig:"fN0qFQLakQBx3DT4l3O7r5NVM4ZgW8wDN/C35NkV24lGjF31aLe4H4EwxzaMNMyP+S0/it3YeSUARn7UMzTxCQ==",commit:"6a79813f36bb0f16b152544bdafd99d4dba72296",url:"https://github.com/arambarzani07/ZhiroxAi2026/releases/download/v5.1.18-candidate-6a79813f/ZhiroxAISupervisor.exe"};
const PUB="19b97bd1183b741f2dce3a3b8495aba13d5b0d160504c69c92bdc37b0d941143";
const enc=new TextEncoder(),BASE=Deno.env.get("SUPABASE_URL")||"",SECRET=Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")||"";
const JH={"content-type":"application/json","cache-control":"no-store, max-age=0","x-content-type-options":"nosniff"};
function json(x:unknown,s=200){return new Response(JSON.stringify(x),{status:s,headers:JH})}
function pathOf(req:Request){const p=new URL(req.url).pathname,m=`/${SERVICE}`,i=p.indexOf(m);return i>=0?(p.slice(i+m.length)||"/"):p}
function bearer(req:Request){const h=req.headers.get("authorization")||"";return h.toLowerCase().startsWith("bearer ")?h.slice(7).trim():""}
// Both callers supply a concrete ArrayBuffer-backed view (TextEncoder or response.arrayBuffer).
async function shaBytes(b:Uint8Array<ArrayBuffer>){const d=await crypto.subtle.digest("SHA-256",b);return Array.from(new Uint8Array(d)).map(x=>x.toString(16).padStart(2,"0")).join("")}
async function shaText(s:string){return shaBytes(enc.encode(s))}
function hex(s:string){return new Uint8Array((s.match(/.{2}/g)||[]).map(x=>parseInt(x,16)))}
function b64(s:string){return Uint8Array.from(atob(s),c=>c.charCodeAt(0))}
async function rest(path:string){const r=await fetch(`${BASE}/rest/v1/${path}`,{headers:{apikey:SECRET,authorization:`Bearer ${SECRET}`,accept:"application/json"}});if(!r.ok)throw new Error(`rest_${r.status}`);return r.json()}
type Release=typeof NEXT;
const CURRENT=NEXT;
async function verifySig(c:Release){const k=await crypto.subtle.importKey("raw",hex(PUB),{name:"Ed25519"},false,["verify"]);if(!await crypto.subtle.verify({name:"Ed25519"},k,b64(c.sig),enc.encode(`${c.version}|${c.sha}|${c.size}`)))throw new Error("signature_mismatch")}
async function releaseRow(c:Release){const rows=await rest(`pc_update_releases?select=version,status,publishable,blocked_reason,sha256,size_bytes,signature_required,signature_b64,signature_algorithm,public_key_hex,pinned_trust_anchor,rollback_updater_embedded,source_commit,artifact_path,build_provenance,promoted_at&version=eq.${c.version}&limit=2`);if(rows?.length!==1)throw new Error("release_not_unique");const r=rows[0];if(r.version!==c.version||r.signature_required!==true||r.pinned_trust_anchor!==true||r.rollback_updater_embedded!==true||r.signature_algorithm!=="Ed25519"||r.sha256!==c.sha||Number(r.size_bytes)!==c.size||String(r.signature_b64||"").replace(/\s+/g,"")!==c.sig||r.public_key_hex!==PUB||r.source_commit!==c.commit||r.artifact_path!==c.url)throw new Error("release_integrity_gate_failed");await verifySig(c);return r}
function promoted(r:any){return r.status==="promoted"&&r.publishable===true&&!r.blocked_reason}
async function authAgent(req:Request){const id=(req.headers.get("x-agent-id")||"").trim(),secret=bearer(req);if(!id||!secret)return null;const rows=await rest(`pc_relay_agents?select=agent_id,secret_hash,version,last_seen_at,last_health_score&agent_id=eq.${encodeURIComponent(id)}&limit=1`),r=rows?.[0];if(!r)return null;return await shaText(secret)===r.secret_hash?r:null}
async function legacyRelease(agent:any):Promise<Release>{
 const prev=await releaseRow(PREVIOUS);if(!promoted(prev))throw new Error("fallback_not_promoted");
 if(agent.agent_id!==TARGET)return PREVIOUS;
 const next=await releaseRow(NEXT),p=next.build_provenance||{};
 if(!promoted(next)||p.rollout_allowed!==true||p.target_agent_id!==TARGET||p.owner_rollout_requested!==true||p.artifact_readback_verified!==true||p.signature_verified!==true)return PREVIOUS;
 if(agent.version===NEXT.version||agent.version===CANDIDATE.version)return NEXT;
 if(agent.version!==PREVIOUS.version||agent.last_health_score!==100||Date.now()-Date.parse(agent.last_seen_at)>120000)return PREVIOUS;
 const started=Date.parse(next.promoted_at||"");
 if(!Number.isFinite(started)||Date.now()-started>20*60*1000||Date.now()<started)return PREVIOUS;
 const seen=await rest(`pc_relay_heartbeats?select=id&agent_id=eq.${encodeURIComponent(TARGET)}&version=eq.${NEXT.version}&captured_at=gte.${encodeURIComponent(new Date(started).toISOString())}&limit=1`);
 if(seen.length>0)return PREVIOUS;
 return NEXT;
}
function canOfferCandidate(agent:any,r:any,now:number){const p=r.build_provenance||{},started=Date.parse(r.promoted_at||""),seen=Date.parse(agent.last_seen_at||"");
 return agent.agent_id===TARGET&&promoted(r)&&p.rollout_allowed===true&&p.target_agent_id===TARGET&&p.owner_rollout_requested===true&&p.artifact_readback_verified===true&&p.signature_verified===true&&agent.version===CURRENT.version&&agent.last_health_score===100&&Number.isFinite(seen)&&seen<=now&&now-seen<=120000&&Number.isFinite(started)&&now>=started&&now-started<=20*60*1000&&!p.binary_claimed_at;
}
async function selectRelease(agent:any):Promise<Release>{
 const prior=await legacyRelease(agent);
 if(agent.agent_id!==TARGET)return prior;
 const candidate=await releaseRow(CANDIDATE),p=candidate.build_provenance||{};
 if(agent.version===CANDIDATE.version&&promoted(candidate)&&p.rollout_allowed===true&&p.target_agent_id===TARGET&&p.owner_rollout_requested===true&&p.artifact_readback_verified===true&&p.signature_verified===true)return CANDIDATE;
 if(prior.version!==CURRENT.version||!canOfferCandidate(agent,candidate,Date.now()))return prior;
 const started=Date.parse(candidate.promoted_at);
 const seen=await rest(`pc_relay_heartbeats?select=id&agent_id=eq.${encodeURIComponent(TARGET)}&version=eq.${CANDIDATE.version}&captured_at=gte.${encodeURIComponent(new Date(started).toISOString())}&limit=1`);
 return seen.length===0?CANDIDATE:prior;
}
// Atomically consume the single scoped binary delivery attempt only after verifying the bytes.
// The manifest and older releases remain read-only; lost delivery requires explicit recovery.
async function claimCandidate(agent:any){
 const r=await releaseRow(CANDIDATE);
 if(!canOfferCandidate(agent,r,Date.now()))return false;
 const at=new Date().toISOString(),p=r.build_provenance||{};
 const filters=new URLSearchParams({version:"eq."+CANDIDATE.version,status:"eq.promoted",publishable:"eq.true",sha256:"eq."+CANDIDATE.sha,promoted_at:"eq."+r.promoted_at,"build_provenance->>target_agent_id":"eq."+TARGET,"build_provenance->>rollout_allowed":"eq.true","build_provenance->>owner_rollout_requested":"eq.true","build_provenance->>binary_claimed_at":"is.null",select:"version"});
 const response=await fetch(`${BASE}/rest/v1/pc_update_releases?${filters}`,{method:"PATCH",headers:{apikey:SECRET,authorization:`Bearer ${SECRET}`,"content-type":"application/json",prefer:"return=representation"},body:JSON.stringify({build_provenance:{...p,binary_claimed_at:at,rollout_health_status:"binary_delivery_claimed"}})});
 if(!response.ok)throw new Error("candidate_claim_failed");
 const rows=await response.json();return Array.isArray(rows)&&rows.length===1&&rows[0].version===CANDIDATE.version;
}
async function verifiedExe(c:Release){const rr=await fetch(c.url,{redirect:"follow",headers:{"user-agent":"Zhirox-PC-Update-v5.1.18"}});if(!rr.ok)throw new Error(`artifact_fetch_${rr.status}`);const exe=new Uint8Array(await rr.arrayBuffer());if(exe.byteLength!==c.size||await shaBytes(exe)!==c.sha)throw new Error("exe_integrity_mismatch");await verifySig(c);return exe}
Deno.serve(async(req:Request)=>{const path=pathOf(req);try{
 if(req.method==="GET"&&path==="/health"){const prev=await releaseRow(CURRENT),next=await releaseRow(CANDIDATE);return json({ok:true,service:SERVICE,version:promoted(next)?CANDIDATE.version:CURRENT.version,previous_version:CURRENT.version,previous_promoted:promoted(prev),candidate_version:CANDIDATE.version,candidate_status:next.status,candidate_publishable:next.publishable,signature:"ed25519_verified",target_scoped:true,artifact_binary_fetched:false,attended_screen_only:true,remote_input:false,unattended_access:false})}
 if(req.method==="GET"&&(path==="/api/update/manifest"||path==="/api/update/binary")){const agent=await authAgent(req);if(!agent)return json({ok:false,error:"unauthorized"},401);const c=await selectRelease(agent);
 if(path==="/api/update/manifest")return json({version:c.version,url:`${BASE}/functions/v1/${SERVICE}/api/update/binary`,sha256:c.sha,size:c.size,signature:c.sig,algorithm:"Ed25519",notes:"Signed scoped Relay update; existing Zero UI and rollback gate"});
 const exe=await verifiedExe(c);const latest=await authAgent(req);if(!latest||(await selectRelease(latest)).version!==c.version)return json({ok:false,error:"release_changed"},409);
 if(c.version===CANDIDATE.version&&latest.version!==CANDIDATE.version&&!await claimCandidate(latest))return json({ok:false,error:"candidate_attempt_not_available"},409);
 return new Response(exe,{status:200,headers:{"content-type":"application/octet-stream","content-length":String(exe.byteLength),"content-disposition":"attachment; filename=ZhiroxAISupervisor.exe","cache-control":"no-store, max-age=0","x-content-type-options":"nosniff","x-zhirox-sha256":c.sha,"x-zhirox-signature":"ed25519-verified"}})}
 return json({ok:false,error:"not_found"},404)
 }catch(e){console.error(e);return json({ok:false,error:"release_gate_error"},503)}});
