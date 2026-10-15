(function (root) {
  'use strict';

  const PROTOCOL_VERSION = 2;
  const MAX_EVENTS = 32;
  const BLOCKED_KEYS = new Set([0x5b, 0x5c, 0x5d]);

  function clamp01(value) {
    const n = Number(value);
    if (!Number.isFinite(n) || n < 0 || n > 1) throw new Error('pointer coordinate outside 0..1');
    return n;
  }

  function validateKeyCode(value) {
    const code = Number(value);
    if (!Number.isInteger(code) || code < 1 || code > 255 || BLOCKED_KEYS.has(code)) {
      throw new Error('key code outside approved range');
    }
    return code;
  }

  function validateButton(value) {
    const button = String(value || '').toLowerCase();
    if (!['left', 'right', 'middle'].includes(button)) throw new Error('unsupported pointer button');
    return button;
  }

  function createControlEncoder() {
    let sequence = 0;
    const next = () => ++sequence;

    return {
      pointerMove(x, y) {
        return { sequence: next(), kind: 'pointer_move', x: clamp01(x), y: clamp01(y) };
      },
      pointerButton(button, down) {
        return { sequence: next(), kind: 'pointer_button', button: validateButton(button), down: Boolean(down) };
      },
      pointerWheel(delta) {
        const wheel = Math.trunc(Number(delta));
        if (!Number.isFinite(wheel) || wheel < -1200 || wheel > 1200) throw new Error('wheel delta outside approved bounds');
        return { sequence: next(), kind: 'pointer_wheel', wheel };
      },
      key(keyCode, down) {
        return { sequence: next(), kind: 'key', key_code: validateKeyCode(keyCode), down: Boolean(down) };
      },
      message(events) {
        if (!Array.isArray(events) || events.length < 1 || events.length > MAX_EVENTS) {
          throw new Error(`control message must contain 1..${MAX_EVENTS} events`);
        }
        for (let i = 1; i < events.length; i += 1) {
          if (!(events[i].sequence > events[i - 1].sequence)) throw new Error('control events are not strictly ordered');
        }
        return { v: PROTOCOL_VERSION, type: 'input', events };
      },
      currentSequence() {
        return sequence;
      },
    };
  }

  const api = { createControlEncoder, PROTOCOL_VERSION, MAX_EVENTS };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.ZhiroxControlV2 = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);
