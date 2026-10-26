import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url));
const { initTheme } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

function themeFixture(t, { saved = null, dark = true, blocked = false } = {}) {
  const attributes = new Map();
  const listeners = new Map();
  const writes = [];
  const root = { dataset: {} };
  const toggle = {
    hidden: true,
    dataset: {},
    setAttribute: (key, value) => attributes.set(key, value),
    getAttribute: (key) => attributes.get(key),
    addEventListener: (name, callback) => listeners.set(name, callback)
  };
  let onSystemChange;
  const system = { matches: dark, addEventListener: (_, callback) => { onSystemChange = callback; } };
  const replacements = {
    document: { documentElement: root, getElementById: (id) => { assert.equal(id, "theme-toggle"); return toggle; } },
    window: { matchMedia: (query) => { assert.equal(query, "(prefers-color-scheme: dark)"); return system; } },
    sessionStorage: {
      getItem: () => { if (blocked) throw new Error("blocked"); return saved; },
      setItem: (key, value) => { if (blocked) throw new Error("blocked"); writes.push([key, value]); }
    }
  };
  for (const [key, value] of Object.entries(replacements)) {
    const original = Object.getOwnPropertyDescriptor(globalThis, key);
    Object.defineProperty(globalThis, key, { configurable: true, value });
    t.after(() => { if (original) Object.defineProperty(globalThis, key, original); else delete globalThis[key]; });
  }
  initTheme();
  return { root, toggle, attributes, writes, click: () => listeners.get("click")(), changeSystem(value) { system.matches = value; onSystemChange(); } };
}

test("theme toggle follows the system until the visitor chooses a theme", (t) => {
  const fixture = themeFixture(t);
  assert.equal(fixture.root.dataset.theme, "system");
  assert.equal(fixture.attributes.get("aria-checked"), "true");
  assert.equal(fixture.toggle.dataset.tooltip, "Switch to light theme");
  assert.equal(Object.hasOwn(fixture.toggle, "title"), false);
  assert.equal(fixture.toggle.hidden, false);
  assert.deepEqual(fixture.writes, [], "Opening the page must not save a preference");
  fixture.changeSystem(false);
  assert.equal(fixture.attributes.get("aria-checked"), "false");
  fixture.click();
  assert.equal(fixture.root.dataset.theme, "dark");
  assert.equal(fixture.attributes.get("aria-checked"), "true");
  assert.deepEqual(fixture.writes, [["echo-observatory-theme", "dark"]]);
  fixture.changeSystem(false);
  assert.equal(fixture.attributes.get("aria-checked"), "true");
  fixture.click();
  assert.equal(fixture.root.dataset.theme, "light");
  assert.equal(fixture.toggle.dataset.tooltip, "Switch to dark theme");
});

test("saved theme survives navigation and takes precedence over system preference", (t) => {
  const fixture = themeFixture(t, { saved: "light", dark: true });
  assert.equal(fixture.root.dataset.theme, "light");
  fixture.changeSystem(true);
  assert.equal(fixture.attributes.get("aria-checked"), "false");
  fixture.click();
  assert.deepEqual(fixture.writes, [["echo-observatory-theme", "dark"]]);
});

test("theme toggle works when browser storage is blocked", (t) => {
  const fixture = themeFixture(t, { blocked: true });
  assert.doesNotThrow(() => fixture.click());
  assert.equal(fixture.root.dataset.theme, "light");
  assert.equal(fixture.attributes.get("aria-checked"), "false");
});
