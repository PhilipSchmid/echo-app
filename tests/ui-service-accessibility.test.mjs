import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url));
const pages = await Promise.all(["index.html", "workbench.html"].map((name) => readFile(new URL("../internal/ui/assets/" + name, import.meta.url), "utf8")));
const { checkService, initServiceStatus, initDisclosureHelp } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

function fixture(t) {
  const description = { textContent: "Checking" };
  let announcements = 0;
  const label = { value: "Checking", get textContent() { return this.value; }, set textContent(value) { this.value = value; announcements++; } };
  const handlers = new Map();
  const details = { open: false, hovered: false, matches: () => details.hovered, querySelector: () => description, contains: (target) => target === indicator || target === description,
    addEventListener: (type, handler) => handlers.set("details:" + type, handler) };
  const instance = { querySelector: () => label };
  const indicator = { dataset: {}, focusVisible: false, matches: () => indicator.focusVisible, closest: (selector) => selector === ".instance" ? instance : details,
    querySelector: () => null, addEventListener: (type, handler) => handlers.set(type, handler) };
  const address = {};
  const document = { visibilityState: "visible", activeElement: null, getElementById: (id) => ({ "service-status": indicator, "service-address": address })[id],
    addEventListener: (type, handler) => handlers.set("document:" + type, handler) };
  const window = { setTimeout, clearTimeout, setInterval: (handler) => handlers.set("interval", handler) };
  for (const [name, value] of Object.entries({ document, window })) {
    const old = Object.getOwnPropertyDescriptor(globalThis, name);
    Object.defineProperty(globalThis, name, { configurable: true, value });
    t.after(() => { if (old) Object.defineProperty(globalThis, name, old); else delete globalThis[name]; });
  }
  const settings = { http_port: "8080", max_request_size: 65536 };
  t.mock.method(globalThis, "fetch", async () => Response.json(settings));
  return { description, label, details, indicator, handlers, address, document, announcements: () => announcements,
    hover(inside, pointerType = "mouse") {
      details.hovered = inside;
      handlers.get("details:pointer" + (inside ? "enter" : "leave"))({ pointerType });
    },
    focus() { document.activeElement = indicator; indicator.focusVisible = true; handlers.get("focus")(); },
    click() {
      let prevented = false;
      handlers.get("click")({ preventDefault() { prevented = true; } });
      if (!prevented) details.open = !details.open;
    }
  };
}

test("service help uses the styled disclosure without a competing native title tooltip", () => {
  for (const page of pages) {
    const summary = page.match(/<summary\b[^>]*\bid="service-status"[^>]*>/)?.[0];
    assert.ok(summary);
    assert.doesNotMatch(summary, /\btitle=/);
    assert.match(summary, /aria-describedby="service-status-description"/);
    assert.doesNotMatch(page, /\stitle=/);
  }
  const badge = pages[0].match(/<span class="badge neutral ip-family"([^>]*)>/)?.[1];
  assert.notEqual(badge, undefined);
  assert.doesNotMatch(badge, /data-tooltip=|tabindex=/, "A visible IP-family label needs no extra hint or tab stop");
  for (const page of pages) {
    for (const button of page.matchAll(/<button\b[^>]*\bid="copy-(?:visit|curl|response)"[^>]*>/g)) {
      assert.doesNotMatch(button[0], /data-tooltip=/, "Visible Copy labels need no redundant tooltip");
    }
  }
  const destination = pages[1].match(/<details id="request-destination-help"[\s\S]*?<summary([^>]*)>/)?.[1];
  assert.ok(destination);
  assert.doesNotMatch(destination, /data-tooltip=/, "The destination uses its existing disclosure content");
});

test("service status updates the persistent sibling live region and visible disclosure quietly", async (t) => {
  const state = fixture(t);
  const base = new URL("https://echo.test/ui/");
  await initServiceStatus(base, new URL("https://echo.test/"));
  assert.equal(state.address.textContent, "https://echo.test/");
  assert.equal(state.label.textContent, "Responding");
  assert.equal(state.announcements(), 1);
  assert.match(state.description.textContent, /responded.*Other protocol listeners may differ/);
  assert.equal(Object.hasOwn(state.indicator, "title"), false);
  await checkService(base, state.indicator);
  assert.equal(state.announcements(), 1, "A successful periodic check must not repeat the same announcement");
  globalThis.fetch.mock.mockImplementation(async () => { throw new Error("offline"); });
  await assert.rejects(checkService(base, state.indicator), /offline/);
  assert.equal(state.label.textContent, "No response");
  assert.match(state.description.textContent, /check failed/);
  assert.equal(state.announcements(), 2);
});

test("mouse hover and keyboard focus share the panel while click pins it and touch stays native", async (t) => {
  const state = fixture(t);
  await initServiceStatus(new URL("https://echo.test/ui/"), new URL("https://echo.test/"));
  state.hover(true);
  assert.equal(state.details.open, true);
  state.handlers.get("details:focusout")({ relatedTarget: null });
  assert.equal(state.details.open, true, "Hover keeps the content available after focus leaves");
  state.hover(false);
  assert.equal(state.details.open, false);
  state.focus();
  assert.equal(state.details.open, true);
  state.hover(true);
  state.hover(false);
  assert.equal(state.details.open, true, "Keyboard focus keeps the content available after pointer leave");
  state.click();
  assert.equal(state.details.open, true, "First activation pins automatically opened help");
  state.hover(true);
  state.handlers.get("details:focusout")({ relatedTarget: null });
  assert.equal(state.details.open, true, "Clicking or selecting hovered help must not dismiss it");
  state.click();
  assert.equal(state.details.open, false, "Second activation closes normally");
  state.document.activeElement = null;
  state.indicator.focusVisible = false;
  state.hover(true, "touch");
  assert.equal(state.details.open, false, "Touch pointer entry must not open hover help");
  state.click();
  assert.equal(state.details.open, true, "Tap opens the native disclosure");
  state.click();
  assert.equal(state.details.open, false, "A second tap closes it");
});

test("the same native disclosure lifecycle works without service status configuration", (t) => {
  const state = fixture(t);
  initDisclosureHelp(state.indicator);
  state.hover(true);
  assert.equal(state.details.open, true);
  state.click();
  assert.equal(state.details.open, true);
  state.hover(false);
  assert.equal(state.details.open, true, "Explicitly pinned help stays open after hover leaves");
  state.handlers.get("document:pointerdown")({ target: {} });
  assert.equal(state.details.open, false);
  assert.equal(state.handlers.has("interval"), false, "Plain disclosure help must not start a service poll");
});

test("Escape dismisses hover help until the mouse leaves without blocking intentional activation", async (t) => {
  const state = fixture(t);
  await initServiceStatus(new URL("https://echo.test/ui/"), new URL("https://echo.test/"));
  state.hover(true);
  state.handlers.get("document:keydown")({ key: "Escape", preventDefault() {} });
  assert.equal(state.details.open, false);
  state.focus();
  state.hover(true);
  assert.equal(state.details.open, false, "Neither focus nor pointer movement reopens dismissed help");
  state.click();
  assert.equal(state.details.open, true, "Explicit activation still opens dismissed help");
  state.click();
  state.document.activeElement = null;
  state.hover(false);
  state.hover(true);
  assert.equal(state.details.open, true, "Leaving and re-entering starts a new hover interaction");
});

test("service disclosure dismisses with Escape, pointer-away and focus-away without trapping focus", async (t) => {
  const state = fixture(t);
  await initServiceStatus(new URL("https://echo.test/ui/"), new URL("https://echo.test/"));
  state.details.open = true;
  let prevented = false;
  state.handlers.get("document:keydown")({ key: "Escape", preventDefault() { prevented = true; } });
  assert.equal(state.details.open, false);
  assert.equal(prevented, true);
  for (const extra of [{ ctrlKey: true }, { isComposing: true }]) {
    state.details.open = true;
    state.handlers.get("document:keydown")({ key: "Escape", ...extra, preventDefault() { assert.fail("Preserve platform/IME keys"); } });
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
