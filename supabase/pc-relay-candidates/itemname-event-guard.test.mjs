import assert from "node:assert/strict";
import { validateItemNameDeltaEvent, classifyItemNameInsertResult } from "./itemname-event-guard.mjs";

const sample={
  event_key:"a".repeat(64),database:"SuperMarket",read_only:true,
  changes:[{kind:"added",itemNameID:"123",itemName:"کاڵا",itemCode:"2700002029709",unitPrice:"100.25",cartonPrice:"1200"}],
};
const tests = [
  ["valid", () => assert.deepEqual(validateItemNameDeltaEvent("itemname_delta",sample),{ok:true,eventKey:sample.event_key})],
  ["wrong database", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,database:"OtherShop"}).ok,false)],
  ["missing read-only", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,read_only:false}).ok,false)],
  ["invalid hash", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,event_key:"bad"}).ok,false)],
  ["empty changes", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,changes:[]}).ok,false)],
  ["26 changes", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,changes:Array(26).fill(sample.changes[0])}).ok,false)],
  ["delete ignored", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,changes:[{kind:"removed",itemNameID:"123"}]}).ok,false)],
  ["invalid ID", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,changes:[{kind:"added",itemNameID:""}]}).ok,false)],
  ["disallowed field", () => assert.equal(validateItemNameDeltaEvent("itemname_delta",{...sample,changes:[{kind:"price_changed",itemNameID:"123",priceField:"Quantity"}]}).ok,false)],
  ["other event unchanged", () => assert.deepEqual(validateItemNameDeltaEvent("agent_health",{}),{ok:true,eventKey:null})],
  ["new event", () => assert.equal(classifyItemNameInsertResult(null),"inserted")],
  ["same key collision", () => assert.equal(classifyItemNameInsertResult({code:"23505",message:"duplicate key violates unique constraint pc_os_events_agent_event_key_unique"}),"duplicate")],
  ["different constraint", () => assert.equal(classifyItemNameInsertResult({code:"23505",message:"other_unique"}),"error")],
  ["database failure", () => assert.equal(classifyItemNameInsertResult({code:"42P01",message:"missing table"}),"error")],
];
let passed=0;
for(const [name,run] of tests) {
  run();passed++;console.log("PASS",name);
}
console.log(`PASS ${passed}/${tests.length} isolated ItemName guard checks`);
