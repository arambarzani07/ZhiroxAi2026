const assert = require('node:assert/strict');
const { test } = require('node:test');
const { createAttendedInputController } = require('./attended_input_v1.js');

function fixture() {
  const sent = [];
  const status = [];
  const controller = createAttendedInputController({
    send: (payload) => sent.push(JSON.parse(payload)),
    status: (message) => status.push(message),
  });
  return { controller, sent, status };
}

test('attended input is fail-closed until channel availability and explicit arm', () => {
  const { controller, sent } = fixture();
  assert.throws(() => controller.arm(), /unavailable/);
  controller.setAvailable(true);
  assert.throws(() => controller.pointerMove(0.5, 0.5), /not armed/);
  controller.arm();
  controller.pointerMove(0.5, 0.25);
  assert.equal(sent.length, 1);
  assert.equal(sent[0].v, 2);
  assert.equal(sent[0].events[0].kind, 'pointer_move');
});

test('disarm releases pressed key and pointer button before becoming inactive', () => {
  const { controller, sent } = fixture();
  controller.setAvailable(true);
  controller.arm();
  controller.key(65, true, false);
  controller.pointerButton('left', true);
  controller.disarm('test');

  const events = sent.flatMap((message) => message.events);
  assert.deepEqual(events.map((event) => [event.kind, event.down]), [
    ['key', true],
    ['pointer_button', true],
    ['key', false],
    ['pointer_button', false],
  ]);
  assert.equal(controller.snapshot().armed, false);
  assert.equal(controller.snapshot().pressedKeys, 0);
  assert.equal(controller.snapshot().pressedButtons, 0);
});

test('channel loss clears local pressed state and leaves controller disarmed', () => {
  const { controller } = fixture();
  controller.setAvailable(true);
  controller.arm();
  controller.key(66, true, false);
  controller.pointerButton('right', true);
  controller.setAvailable(false);
  const snapshot = controller.snapshot();
  assert.equal(snapshot.available, false);
  assert.equal(snapshot.armed, false);
  assert.equal(snapshot.pressedKeys, 0);
  assert.equal(snapshot.pressedButtons, 0);
});

test('key repeat is suppressed and meta/os key remains blocked by protocol encoder', () => {
  const { controller, sent } = fixture();
  controller.setAvailable(true);
  controller.arm();
  assert.equal(controller.key(67, true, true), false);
  assert.equal(sent.length, 0);
  assert.throws(() => controller.key(0x5b, true, false), /approved range/);
  assert.equal(sent.length, 0);
});

test('wheel is bounded and pointer buttons are allowlisted', () => {
  const { controller, sent } = fixture();
  controller.setAvailable(true);
  controller.arm();
  controller.wheel(99999);
  assert.equal(sent[0].events[0].wheel, 1200);
  assert.throws(() => controller.pointerButton(9, true), /unsupported/);
});

test('sequence remains strictly increasing across mixed events and releases', () => {
  const { controller, sent } = fixture();
  controller.setAvailable(true);
  controller.arm();
  controller.pointerMove(0.1, 0.2);
  controller.pointerButton('left', true);
  controller.key(68, true, false);
  controller.releaseAll();
  const sequence = sent.flatMap((message) => message.events).map((event) => event.sequence);
  assert.deepEqual(sequence, [...sequence].sort((a, b) => a - b));
  assert.equal(new Set(sequence).size, sequence.length);
});
