
const enc=new TextEncoder();
const key=Deno.env.get("SUPABASE_SERVICE_ROLE_KEY")||"";
const base=Deno.env.get("SUPABASE_URL")||"";
const headers={"apikey":key,"Authorization":"Bearer "+key,"Content-Type":"application/json"};
function reply(x:unknown,status=200){return Response.json(x,{status,headers:{"Cache-Control":"no-store","X-Content-Type-Options":"nosniff"}})}
async function rpc(name:string,body:unknown){
 const r=await fetch(base+"/rest/v1/rpc/"+name,{method:"POST",headers,body:JSON.stringify(body),redirect:"error"});
 if(!r.ok)throw new Error("internal_rpc_failed");
 return await r.json();
}
async function authorized(req:Request){
 const bearer=req.headers.get("Authorization")||"";
 if(!key||!bearer.startsWith("Bearer "))return false;
 const a=new Uint8Array(await crypto.subtle.digest("SHA-256",enc.encode(bearer.slice(7))));
 const b=new Uint8Array(await crypto.subtle.digest("SHA-256",enc.encode(key)));
 let d=0;for(let i=0;i<a.length;i++)d|=a[i]^b[i];return d===0;
}
Deno.serve(async req=>{
 try{
 const path=new URL(req.url).pathname;
 if(req.method==="GET"&&path.endsWith("/health")){
  const s=await rpc("zhirox_release_signer_status",{});
  return reply({active:true,...s},s.ready?200:503);
 }
 if(!await authorized(req))return reply({ok:false,error:"unauthorized"},401);
 if(req.method!=="POST"||!path.endsWith("/sign"))return reply({ok:false,error:"not_found"},404);
 const body=await req.json().catch(()=>null);
 if(!body||typeof body.release_id!=="string"||! /^[0-9a-f-]{36}$/i.test(body.release_id)||typeof body.dry_run!=="boolean")
  return reply({ok:false,error:"release_id_and_explicit_dry_run_required"},400);
 const r=await fetch(base+"/rest/v1/pc_update_releases?id=eq."+encodeURIComponent(body.release_id)+"&select=id,version,source_commit,status,publishable,sha256,size_bytes,artifact_bucket,artifact_path",{headers,redirect:"error"});
 if(!r.ok)throw new Error("release_lookup_failed");
 const rows=await r.json();const release=rows[0];
 if(!release)return reply({ok:false,error:"release_not_staged"},409);
 if(release.version!=="5.1.8"||release.source_commit!=="af8394e8583c8cca72ee389d8f5af1a2446d63d8"||release.status!=="staged"||release.publishable!==false)
  return reply({ok:false,error:"release_gate_rejected"},409);
 if(!Number.isSafeInteger(release.size_bytes)||release.size_bytes<1||release.size_bytes>50000000||!release.artifact_bucket||!release.artifact_path)
  return reply({ok:false,error:"artifact_gate_rejected"},409);
 const objectPath=[release.artifact_bucket,...release.artifact_path.split("/")].map(encodeURIComponent).join("/");
 const binary=await fetch(base+"/storage/v1/object/"+objectPath,{headers,redirect:"error"});
 if(!binary.ok)return reply({ok:false,error:"artifact_unavailable"},409);
 const bytes=await binary.arrayBuffer();
 if(bytes.byteLength!==release.size_bytes)return reply({ok:false,error:"artifact_size_mismatch"},409);
 const hash=Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256",bytes))).map(x=>x.toString(16).padStart(2,"0")).join("");
 if(hash!==release.sha256)return reply({ok:false,error:"artifact_hash_mismatch"},409);
 const result=await rpc("zhirox_sign_v518_release",{p_release_id:body.release_id,p_dry_run:body.dry_run});
 return reply(result,result.ok?200:409);
 }catch{return reply({ok:false,error:"signer_internal_error"},500)}
});
