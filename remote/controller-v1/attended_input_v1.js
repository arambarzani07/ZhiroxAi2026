(function (root) {
  'use strict';

  const controlApi = typeof module !== 'undefined' && module.exports
    ? require('./control_v2.js')
    : root.ZhiroxControlV2;

  if (!controlApi || typeof controlApi.createControlEncoder !== 'function') {
    throw new Error('ZhiroxControlV2 is required');
  }

  const BUTTONS = new Map([[0, 'left'], [1, 'middle'], [2, 'right']]);
  const MAX_WHEEL = 1200;

  function createAttendedInputController(options) {
    const opts = options || {};
    if (typeof opts.send !== 'function') throw new Error('attended input send callback is required');

    const encoder = controlApi.createControlEncoder();
    const pressedKeys = new Set();
    const pressedButtons = new Set();
    let armed = false;
    let available = false;

    function report(message) {
      if (typeof opts.status === 'function') opts.status(String(message));
    }

    function sendEvent(event) {
      if (!available) throw new Error('attended input channel is unavailable');
      const payload = JSON.stringify(encoder.message([event]));
      if (payload.length > 16 * 1024) throw new Error('attended input payload exceeds transport bound');
      opts.send(payload);
    }

    function requireArmed() {
      if (!armed) throw new Error('attended input is not armed');
      if (!available) throw new Error('attended input channel is unavailable');
    }

    function setAvailable(value) {
      available = Boolean(value);
      if (!available && armed) {
        disarm('channel unavailable');
      }
      return available;
    }

    function arm() {
      if (!available) throw new Error('attended input channel is unavailable');
      armed = true;
      report('attended input armed; target-side local approval and visible awareness are still required');
      return true;
    }

    function releaseAll() {
      if (!available) {
        pressedKeys.clear();
        pressedButtons.clear();
        return;
      }
      for (const keyCode of Array.from(pressedKeys)) {
        try { sendEvent(encoder.key(keyCode, false)); } catch (_) {}
      }
      for (const button of Array.from(pressedButtons)) {
        try { sendEvent(encoder.pointerButton(button, false)); } catch (_) {}
      }
      pressedKeys.clear();
      pressedButtons.clear();
    }

    function disarm(reason) {
      releaseAll();
      armed = false;
      if (reason) report(`attended input disarmed: ${reason}`);
      return true;
    }

    function pointerMove(x, y) {
      requireArmed();
      sendEvent(encoder.pointerMove(x, y));
    }

    function pointerButton(button, down) {
      requireArmed();
      const name = typeof button === 'number' ? BUTTONS.get(button) : String(button || '').toLowerCase();
      if (!name) throw new Error('unsupported browser pointer button');
      if (down) pressedButtons.add(name);
      else pressedButtons.delete(name);
      sendEvent(encoder.pointerButton(name, down));
    }

    function wheel(delta) {
      requireArmed();
      const bounded = Math.max(-MAX_WHEEL, Math.min(MAX_WHEEL, Math.trunc(Number(delta) || 0)));
      sendEvent(encoder.pointerWheel(bounded));
    }

    function key(keyCode, down, repeat) {
      requireArmed();
      const code = Number(keyCode);
      if (repeat && down) return false;
      if (down) pressedKeys.add(code);
      else pressedKeys.delete(code);
      sendEvent(encoder.key(code, down));
      return true;
    }

    function snapshot() {
      return {
        armed,
        available,
        pressedKeys: pressedKeys.size,
        pressedButtons: pressedButtons.size,
        sequence: encoder.currentSequence(),
      };
    }

    return {
      setAvailable,
      arm,
      disarm,
      releaseAll,
      pointerMove,
      pointerButton,
      wheel,
      key,
      snapshot,
    };
  }

  function bindBrowserInput(surface, controller, env) {
    if (!surface || typeof surface.addEventListener !== 'function') throw new Error('input surface is required');
    if (!controller) throw new Error('attended input controller is required');
    const browser = env || root;
    const documentRef = browser.document;

    function normalizedPointer(event) {
      const rect = surface.getBoundingClientRect();
      if (!rect || rect.width <= 0 || rect.height <= 0) throw new Error('input surface bounds are invalid');
      const x = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width));
      const y = Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height));
      return { x, y };
    }

    const onMove = (event) => {
      if (!controller.snapshot().armed) return;
      try {
        const p = normalizedPointer(event);
        controller.pointerMove(p.x, p.y);
      } catch (_) {}
    };
    const onDown = (event) => {
      if (!controller.snapshot().armed) return;
      try {
        event.preventDefault();
        const p = normalizedPointer(event);
        controller.pointerMove(p.x, p.y);
        controller.pointerButton(event.button, true);
        if (typeof surface.focus === 'function') surface.focus({ preventScroll: true });
      } catch (_) {}
    };
    const onUp = (event) => {
      if (!controller.snapshot().armed) return;
      try { event.preventDefault(); controller.pointerButton(event.button, false); } catch (_) {}
    };
    const onWheel = (event) => {
      if (!controller.snapshot().armed) return;
      try { event.preventDefault(); controller.wheel(event.deltaY); } catch (_) {}
    };
    const onContext = (event) => { if (controller.snapshot().armed) event.preventDefault(); };
    const onKeyDown = (event) => {
      if (!controller.snapshot().armed) return;
      if (event.metaKey || event.key === 'Meta') return;
      try { event.preventDefault(); controller.key(event.keyCode || event.which, true, event.repeat); } catch (_) {}
    };
    const onKeyUp = (event) => {
      if (!controller.snapshot().armed) return;
      if (event.metaKey || event.key === 'Meta') return;
      try { event.preventDefault(); controller.key(event.keyCode || event.which, false, false); } catch (_) {}
    };
    const onBlur = () => { if (controller.snapshot().armed) controller.disarm('browser focus lost'); };
    const onVisibility = () => {
      if (documentRef && documentRef.visibilityState === 'hidden' && controller.snapshot().armed) {
        controller.disarm('page hidden');
      }
    };

    surface.addEventListener('pointermove', onMove);
    surface.addEventListener('pointerdown', onDown);
    surface.addEventListener('pointerup', onUp);
    surface.addEventListener('pointercancel', onUp);
    surface.addEventListener('wheel', onWheel, { passive: false });
    surface.addEventListener('contextmenu', onContext);
    surface.addEventListener('keydown', onKeyDown);
    surface.addEventListener('keyup', onKeyUp);
    if (browser && typeof browser.addEventListener === 'function') browser.addEventListener('blur', onBlur);
    if (documentRef && typeof documentRef.addEventListener === 'function') documentRef.addEventListener('visibilitychange', onVisibility);

    return function unbind() {
      controller.disarm('input surface unbound');
      surface.removeEventListener('pointermove', onMove);
      surface.removeEventListener('pointerdown', onDown);
      surface.removeEventListener('pointerup', onUp);
      surface.removeEventListener('pointercancel', onUp);
      surface.removeEventListener('wheel', onWheel);
      surface.removeEventListener('contextmenu', onContext);
      surface.removeEventListener('keydown', onKeyDown);
      surface.removeEventListener('keyup', onKeyUp);
      if (browser && typeof browser.removeEventListener === 'function') browser.removeEventListener('blur', onBlur);
      if (documentRef && typeof documentRef.removeEventListener === 'function') documentRef.removeEventListener('visibilitychange', onVisibility);
    };
  }

  const api = { createAttendedInputController, bindBrowserInput };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.ZhiroxAttendedInputV1 = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);
