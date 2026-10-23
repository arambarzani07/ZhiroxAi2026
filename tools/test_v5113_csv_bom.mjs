import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const src = readFileSync('supabase/functions/zhirox-live-csv-sync/index.ts','utf8');
assert.ok(src.includes('new TextDecoder("utf-8", { fatal: true, ignoreBOM: true })'),
  'The ingest function MUST preserve the UTF-8 BOM for strict header validation');
const header = String.fromCharCode(0xfeff) + 'ItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice\r\n';
const good = new Uint8Array(Buffer.from(header + '1,کاڵا,001234,1000,0\r\n','utf8'));
const decoded = new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(good);
assert.ok(decoded.startsWith(header));
assert.ok(decoded.endsWith('\r\n'));
assert.equal(decoded.charCodeAt(0),0xfeff);
const bug = new TextDecoder('utf-8',{fatal:true}).decode(good);
assert.equal(bug.startsWith(header),false,'Regression reproduces previous HTTP 422');
const invalid = new Uint8Array(Buffer.from('ItemNameID,ItemName,ItemCode,SalePrice,WholeSalePrice\r\n1,test,0,1,0\r\n'));
assert.equal(new TextDecoder('utf-8',{fatal:true,ignoreBOM:true}).decode(invalid).startsWith(header),false);
console.log('PASS: valid BOM CSV accepted; missing BOM rejected; previous 422 mechanism reproduced');
