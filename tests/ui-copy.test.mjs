import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url));
const { copy, setCopyLabel } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

// Only browser boundaries are doubled; tests execute the embedded copy helper.
function clipboardFixture(t, { writeText, secure = true, legacy = () => true } = {}) {
  const timers = new Map();
  let timerID = 0;
  const originalRange = { cloneRange() { return this; } };
  const selection = {
    ranges: [originalRange],
    get rangeCount() { return this.ranges.length; },
    getRangeAt(index) { return this.ranges[index]; },
    removeAllRanges() { this.ranges = []; },
    addRange(range) { this.ranges.push(range); }
  };
  const fields = [];
  const legacyTexts = [];
  const status = { textContent: "", dataset: {}, hidden: false };
  const label = { textContent: "Copy" };
  const icon = {};
  const attributes = new Map([["aria-label", "Copy curl command"]]);
  const document = {
    activeElement: null,
    getSelection: () => selection,
    body: { append(field) { fields.push(field); } },
    createElement(tag) {
      assert.equal(tag, "textarea");
      const field = {
        value: "", style: {}, setAttribute() {},
        focus() { document.activeElement = this; },
        select() { selection.ranges = [{}]; },
        remove() { fields.splice(fields.indexOf(this), 1); }
      };
      return field;
    },
    execCommand(command) {
      assert.equal(command, "copy");
      assert.equal(this.activeElement, fields.at(-1), "legacy copying must focus the temporary textarea");
      legacyTexts.push(fields.at(-1).value);
      return legacy();
    }
  };
  const button = {
    ownerDocument: document, dataset: {}, disabled: false, isConnected: true, icon,
    querySelector: () => label,
    closest: () => ({ querySelector: () => status }),
    getAttribute: (name) => attributes.get(name) ?? null,
    setAttribute: (name, value) => attributes.set(name, String(value)),
    removeAttribute: (name) => attributes.delete(name),
    focus() { document.activeElement = this; },
    get textContent() { return label.textContent; },
    set textContent(_) { throw new Error("Replacing the button contents destroys its icon"); }
  };
  document.activeElement = button;
  const replacements = {
    document,
    navigator: { clipboard: writeText ? { writeText } : undefined },
    window: {
      isSecureContext: secure,
      getSelection: () => selection,
      setTimeout(callback) { timers.set(++timerID, callback); return timerID; },
      clearTimeout(id) { timers.delete(id); }
    }
  };
  const descriptors = new Map(Object.keys(replacements).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
  for (const [key, value] of Object.entries(replacements)) Object.defineProperty(globalThis, key, { value, configurable: true });
  t.after(() => {
    for (const [key, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
  setCopyLabel(button, "Copy", "Copy curl command");
  return { button, label, icon, status, timers, document, fields, selection, originalRange, legacyTexts };
}

test("clipboard success copies exact text and resets feedback without replacing the icon", async (t) => {
  const writes = [];
  const f = clipboardFixture(t, { writeText: async (text) => { writes.push(text); } });
  const text = "curl \\\n  'https://demo.example/echo/?item={a,b}'";
  assert.equal(await copy(text, f.button), true);
  assert.deepEqual(writes, [text]);
  assert.equal(f.button.icon, f.icon);
  assert.equal(f.label.textContent, "Copied");
  assert.equal(f.button.getAttribute("aria-label"), "Copied curl command");
  assert.equal(f.button.dataset.copyState, "copied");
  assert.match(f.status.textContent, /copied/i);
  for (const callback of [...f.timers.values()]) callback();
  assert.equal(f.label.textContent, "Copy");
  assert.equal(f.button.getAttribute("aria-label"), "Copy curl command");
});

test("denied Clipboard API falls back and restores focus, caret, and document selection", async (t) => {
  const f = clipboardFixture(t, { writeText: async () => { throw new Error("Denied"); } });
  const input = {
    selectionStart: 2, selectionEnd: 5, selectionDirection: "backward", isConnected: true,
    focus() { f.document.activeElement = this; },
    setSelectionRange(...args) { this.restored = args; }
  };
  f.document.activeElement = input;
  assert.equal(await copy("first\nsecond", f.button), true);
  assert.deepEqual(f.legacyTexts, ["first\nsecond"]);
  assert.equal(f.fields.length, 0);
  assert.equal(f.document.activeElement, input);
  assert.deepEqual(input.restored, [2, 5, "backward"]);
  assert.deepEqual(f.selection.ranges, [f.originalRange]);
});

test("plain HTTP uses the fallback without attempting the secure Clipboard API", async (t) => {
  let modernCalls = 0;
  const f = clipboardFixture(t, { secure: false, writeText: async () => { modernCalls++; } });
  assert.equal(await copy("synthetic command", f.button), true);
  assert.equal(modernCalls, 0);
  assert.deepEqual(f.legacyTexts, ["synthetic command"]);
  assert.equal(f.fields.length, 0);
});

test("an unavailable Clipboard API still permits the legacy copy path", async (t) => {
  const f = clipboardFixture(t);
  assert.equal(await copy("synthetic command", f.button), true);
  assert.deepEqual(f.legacyTexts, ["synthetic command"]);
  assert.equal(f.fields.length, 0);
  assert.equal(f.button.dataset.copyState, "copied");
});

for (const [name, legacy] of [["returns false", () => false], ["throws", () => { throw new Error("Blocked"); }]]) {
  test(`failed legacy clipboard ${name}: show manual guidance and clean up`, async (t) => {
    const f = clipboardFixture(t, { secure: false, legacy });
    assert.equal(await copy("synthetic command", f.button), false);
    assert.equal(f.fields.length, 0);
    assert.equal(f.document.activeElement, f.button);
    assert.equal(f.button.dataset.copyState, "error");
    assert.equal(f.label.textContent, "Retry");
    assert.equal(f.button.getAttribute("aria-label"), "Retry copying curl command");
    assert.match(f.status.textContent, /select|manually/i);
    assert.doesNotMatch(f.status.textContent, /copied to clipboard/i);
  });
}

test("a pending copy cannot overwrite feedback for a changed response", async (t) => {
  let finish;
  const f = clipboardFixture(t, { writeText: () => new Promise((resolve) => { finish = resolve; }) });
  const pending = copy("old response", f.button);
  assert.equal(f.button.dataset.copyState, "copying");
  assert.equal(f.button.getAttribute("aria-label"), "Copying curl command");
  setCopyLabel(f.button, "Copy", "Copy response preview");
  f.button.disabled = true;
  finish();
  assert.equal(await pending, false);
  assert.equal(f.label.textContent, "Copy");
  assert.equal(f.status.textContent, "");
  assert.equal(f.button.getAttribute("aria-label"), "Copy response preview");
  assert.equal(f.button.disabled, true);
});

test("an obsolete rejection cannot run fallback or replace a newer copy's state", async (t) => {
  let rejectOld;
  let calls = 0;
  const f = clipboardFixture(t, {
    writeText: () => ++calls === 1 ? new Promise((_, reject) => { rejectOld = reject; }) : Promise.resolve()
  });
  const old = copy("old response", f.button);
  setCopyLabel(f.button, "Copy", "Copy response preview");
  assert.equal(await copy("new response", f.button), true);
  rejectOld(new Error("Old permission denial"));
  assert.equal(await old, false);
  assert.deepEqual(f.legacyTexts, []);
  assert.equal(f.label.textContent, "Copied");
  assert.equal(f.button.dataset.copyState, "copied");
});

test("queued feedback reset cannot restore a stale response label", async (t) => {
  const f = clipboardFixture(t, { writeText: async () => {} });
  await copy("old response", f.button);
  const queued = [...f.timers.values()];
  setCopyLabel(f.button, "Copy", "Copy response preview");
  for (const callback of queued) callback();
  assert.equal(f.label.textContent, "Copy");
  assert.equal(f.button.getAttribute("aria-label"), "Copy response preview");
  assert.equal(f.button.dataset.copyState, "idle");
});

test("a rejected copy from another block cannot overwrite the latest clipboard action", async (t) => {
  let rejectOld;
  let calls = 0;
  const f = clipboardFixture(t, {
    writeText: () => ++calls === 1 ? new Promise((_, reject) => { rejectOld = reject; }) : Promise.resolve()
  });
  const label = { textContent: "Copy" };
  const status = { textContent: "" };
  const attributes = new Map([["aria-label", "Copy response"]]);
  const otherButton = {
    ...f.button, dataset: {},
    querySelector: () => label,
    closest: () => ({ querySelector: () => status }),
    getAttribute: (name) => attributes.get(name) ?? null,
    setAttribute: (name, value) => attributes.set(name, String(value))
  };
  const old = copy("old command", f.button);
  setCopyLabel(otherButton, "Copy", "Copy response");
  assert.equal(await copy("new response", otherButton), true);
  rejectOld(new Error("Old permission denial"));
  assert.equal(await old, false);
  assert.deepEqual(f.legacyTexts, []);
  assert.equal(f.button.dataset.copyState, "idle");
  assert.equal(label.textContent, "Copied");
});
