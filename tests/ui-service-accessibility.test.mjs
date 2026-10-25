import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url));
const { checkService, initServiceStatus } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

function fixture(t) {
  const description = { textContent: "Checking" };
  let announcements = 0;
  const label = { value: "Checking", get textContent() { return this.value; }, set textContent(value) { this.value = value; announcements++; } };
  const handlers = new Map();
  const details = { open: false, querySelector: () => description, contains: (target) => target === indicator || target === description,
    addEventListener: (type, handler) => handlers.set("details:" + type, handler) };
  const instance = { querySelector: () => label };
  const indicator = { dataset: {}, title: "", closest: (selector) => selector === ".instance" ? instance : details,
    querySelector: () => null, addEventListener: (type, handler) => handlers.set(type, handler) };
  const address = {};
  const document = { visibilityState: "visible", getElementById: (id) => ({ "service-status": indicator, "service-address": address })[id],
    addEventListener: (type, handler) => handlers.set("document:" + type, handler) };
  const window = { setTimeout, clearTimeout, setInterval: (handler) => handlers.set("interval", handler) };
  for (const [name, value] of Object.entries({ document, window })) {
    const old = Object.getOwnPropertyDescriptor(globalThis, name);
    Object.defineProperty(globalThis, name, { configurable: true, value });
    t.after(() => { if (old) Object.defineProperty(globalThis, name, old); else delete globalThis[name]; });
  }
  const settings = { http_port: "8080", max_request_size: 65536 };
  t.mock.method(globalThis, "fetch", async () => Response.json(settings));
  return { description, label, details, indicator, handlers, address, announcements: () => announcements };
}

test("service status updates the persistent sibling live region and visible disclosure quietly", async (t) => {
  const state = fixture(t);
  const base = new URL("https://echo.test/ui/");
  await initServiceStatus(base, new URL("https://echo.test/"));
  assert.equal(state.address.textContent, "https://echo.test/");
  assert.equal(state.label.textContent, "Responding");
  assert.equal(state.announcements(), 1);
  assert.equal(state.description.textContent, state.indicator.title);
  assert.match(state.description.textContent, /responded.*Other protocol listeners may differ/);
  await checkService(base, state.indicator);
  assert.equal(state.announcements(), 1, "A successful periodic check must not repeat the same announcement");
  globalThis.fetch.mock.mockImplementation(async () => { throw new Error("offline"); });
  await assert.rejects(checkService(base, state.indicator), /offline/);
  assert.equal(state.label.textContent, "No response");
  assert.equal(state.description.textContent, state.indicator.title);
  assert.match(state.description.textContent, /check failed/);
  assert.equal(state.announcements(), 2);
});

test("service disclosure dismisses with Escape, pointer-away and focus-away without trapping focus", async (t) => {
  const state = fixture(t);
  await initServiceStatus(new URL("https://echo.test/ui/"), new URL("https://echo.test/"));
  state.details.open = true;
  let prevented = false;
  state.handlers.get("keydown")({ key: "Escape", preventDefault() { prevented = true; } });
  assert.equal(state.details.open, false);
  assert.equal(prevented, true);
  for (const extra of [{ ctrlKey: true }, { isComposing: true }]) {
    state.details.open = true;
    state.handlers.get("keydown")({ key: "Escape", ...extra, preventDefault() { assert.fail("Preserve platform/IME keys"); } });
    assert.equal(state.details.open, true);
  }
  state.handlers.get("details:focusout")({ relatedTarget: state.description });
  assert.equal(state.details.open, true);
  state.handlers.get("details:focusout")({ relatedTarget: null });
  assert.equal(state.details.open, false);
  state.details.open = true;
  state.handlers.get("document:pointerdown")({ target: state.indicator });
  assert.equal(state.details.open, true);
  state.handlers.get("document:pointerdown")({ target: {} });
  assert.equal(state.details.open, false);
});
