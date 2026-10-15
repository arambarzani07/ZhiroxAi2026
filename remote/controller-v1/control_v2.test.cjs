const assert = require('node:assert/strict');
const { createControlEncoder, PROTOCOL_VERSION, MAX_EVENTS } = require('./control_v2.js');

const enc = createControlEncoder();
const move = enc.pointerMove(0.25, 0.75);
const down = enc.pointerButton('left', true);
const key = enc.key(65, true);
const msg = enc.message([move, down, key]);
assert.equal(msg.v, PROTOCOL_VERSION);
assert.equal(msg.type, 'input');
assert.equal(msg.events.length, 3);
assert.ok(msg.events[0].sequence < msg.events[1].sequence);
assert.ok(msg.events[1].sequence < msg.events[2].sequence);

assert.throws(() => enc.pointerMove(-0.1, 0.5), /coordinate/);
assert.throws(() => enc.pointerButton('x1', true), /unsupported/);
assert.throws(() => enc.pointerWheel(1201), /wheel/);
assert.throws(() => enc.key(0x5b, true), /approved range/);
assert.throws(() => enc.key(0x5d, true), /approved range/);
assert.throws(() => enc.message([]), /1\.\.32/);
assert.throws(() => enc.message(Array.from({ length: MAX_EVENTS + 1 }, (_, i) => ({ sequence: i + 1, kind: 'pointer_move', x: 0, y: 0 }))), /1\.\.32/);
assert.throws(() => enc.message([{ sequence: 4, kind: 'key', key_code: 65 }, { sequence: 3, kind: 'key', key_code: 66 }]), /strictly ordered/);

console.log('remote controller v2 encoder tests passed');
