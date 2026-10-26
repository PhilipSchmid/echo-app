import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url));
const { initTooltips } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

function fixture(t) {
  const handlers = new Map(), windowHandlers = new Map(), timers = new Map();
  let timerID = 0;
  class Node {
    constructor(tag) {
      Object.assign(this, { tag, attributes: new Map(), children: [], style: {}, isConnected: true, keyboard: false, hidden: false, textContent: "", rect: { left: 40, top: 30, width: 44, height: 44, bottom: 74 } });
    }
    append(child) { this.children.push(child); child.parentElement = this; }
    setAttribute(name, value) { this.attributes.set(name, String(value)); }
    getAttribute(name) { return this.attributes.get(name) ?? null; }
    removeAttribute(name) { this.attributes.delete(name); }
    contains(node) { return node === this || this.children.some((child) => child.contains(node)); }
    closest() { return this.attributes.has("data-tooltip") ? this : this.parentElement?.closest() ?? null; }
    matches(selector) { assert.equal(selector, ":focus-visible"); return this.keyboard; }
    getBoundingClientRect() { return this.rect; }
    set innerHTML(_) { assert.fail("Tooltip text must never become HTML"); }
  }
  const body = new Node("body");
  const document = {
    body, activeElement: null,
    getElementById: (id) => body.children.find((node) => node.id === id),
    createElement: (tag) => { const node = new Node(tag); node.rect = { width: 160, height: 48 }; return node; },
    addEventListener: (name, callback) => handlers.set(name, callback)
  };
  const window = {
    innerWidth: 400, innerHeight: 300,
    addEventListener: (name, callback) => windowHandlers.set(name, callback),
    setTimeout(callback) { timers.set(++timerID, callback); return timerID; },
    clearTimeout(id) { timers.delete(id); }
  };
  for (const [name, value] of Object.entries({ document, window })) {
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, name);
    Object.defineProperty(globalThis, name, { configurable: true, value });
    t.after(() => { if (descriptor) Object.defineProperty(globalThis, name, descriptor); else delete globalThis[name]; });
  }
  const emit = (name, target, extra = {}) => {
    const event = { target, relatedTarget: null, pointerType: "mouse", prevented: false, preventDefault() { this.prevented = true; }, ...extra };
    handlers.get(name)?.(event);
    return event;
  };
  initTooltips();
  return {
    body, document, window, windowHandlers, timers, emit, popup: document.getElementById("ui-tooltip"),
    owner(text = "Copy this command") { const node = new Node("button"); node.setAttribute("data-tooltip", text); body.append(node); return node; },
    child(owner) { const node = new Node("svg"); owner.append(node); return node; },
    flush() { for (const [id, callback] of [...timers]) { timers.delete(id); callback(); } }
  };
}

test("tooltips delegate to dynamic owners and render hostile text in one reusable popup", (t) => {
  const f = fixture(t), owner = f.owner('<img src=x onerror="alert(1)">');
  f.emit("pointerover", f.child(owner));
  assert.equal(f.popup.hidden, false);
  assert.equal(f.popup.getAttribute("role"), "tooltip");
  assert.equal(f.popup.textContent, owner.getAttribute("data-tooltip"));
  assert.equal(f.popup.children.length, 0);
  const next = f.owner("A newer label");
  f.emit("pointerover", next);
  assert.equal(f.popup.textContent, "A newer label");
  assert.equal(owner.getAttribute("aria-describedby"), null);
  initTooltips();
  assert.equal(f.body.children.filter((node) => node.id === "ui-tooltip").length, 1);
  next.setAttribute("data-tooltip", "");
  f.emit("pointerover", next);
  assert.equal(f.popup.hidden, true);
});

test("tooltip descriptions retain existing and newly added ARIA references", (t) => {
  const f = fixture(t), owner = f.owner();
  owner.setAttribute("aria-describedby", "field-hint");
  f.emit("pointerover", owner);
  assert.equal(owner.getAttribute("aria-describedby"), "field-hint ui-tooltip");
  owner.setAttribute("aria-describedby", "field-hint ui-tooltip field-error");
  f.emit("click", owner);
  assert.equal(owner.getAttribute("aria-describedby"), "field-hint field-error");
});

test("keyboard focus opens help and Escape stays dismissed until leave or refocus", (t) => {
  const f = fixture(t), owner = f.owner();
  owner.keyboard = true; f.document.activeElement = owner;
  f.emit("focusin", owner);
  assert.equal(f.popup.hidden, false);
  f.emit("pointerover", owner);
  assert.equal(f.emit("keydown", owner, { key: "Escape" }).prevented, true);
  assert.equal(f.popup.hidden, true);
  f.emit("pointerover", f.child(owner));
  assert.equal(f.popup.hidden, true);
  f.emit("pointerout", owner, { relatedTarget: f.body });
  f.emit("pointerover", owner);
  assert.equal(f.popup.hidden, false);
  f.emit("keydown", owner, { key: "Escape" });
  f.emit("focusout", owner);
  f.emit("focusin", owner);
  assert.equal(f.popup.hidden, false);
  for (const extra of [{ ctrlKey: true }, { shiftKey: true }, { isComposing: true }]) assert.equal(f.emit("keydown", owner, { key: "Escape", ...extra }).prevented, false);
});

test("touch and pointer focus do not open hover tooltips", (t) => {
  const f = fixture(t), owner = f.owner();
  f.emit("pointerover", owner, { pointerType: "touch" });
  f.emit("focusin", owner);
  assert.equal(f.popup.hidden, true);
});

test("popup remains hoverable across its gap and closes after leaving both areas", (t) => {
  const f = fixture(t), owner = f.owner();
  f.emit("pointerover", owner);
  f.emit("pointerout", owner, { relatedTarget: f.body });
  assert.equal(f.timers.size, 1);
  assert.equal(f.popup.hidden, false);
  f.emit("pointerover", f.popup);
  f.flush();
  assert.equal(f.popup.hidden, false);
  f.emit("pointerout", f.popup, { relatedTarget: f.body });
  f.flush();
  assert.equal(f.popup.hidden, true);
});

test("losing keyboard focus preserves help while the trigger or popup remains hovered", (t) => {
  const f = fixture(t), owner = f.owner();
  owner.keyboard = true; f.document.activeElement = owner;
  f.emit("focusin", owner);
  f.emit("pointerover", owner);
  f.document.activeElement = null;
  f.emit("focusout", owner, { relatedTarget: f.body });
  f.flush();
  assert.equal(f.popup.hidden, false, "hovered trigger keeps its help visible");
  f.emit("pointerout", owner, { relatedTarget: f.popup });
  f.emit("pointerover", f.popup);
  f.emit("focusout", owner, { relatedTarget: f.body });
  f.flush();
  assert.equal(f.popup.hidden, false, "hovered popup keeps its help visible");
  f.emit("pointerout", f.popup, { relatedTarget: f.body });
  f.flush();
  assert.equal(f.popup.hidden, true);
});

test("scrolling tooltip content keeps it open while outside nested or page scrolling dismisses it", (t) => {
  const f = fixture(t), owner = f.owner();
  f.emit("pointerover", owner);
  f.windowHandlers.get("scroll")({ target: f.child(f.popup) });
  assert.equal(f.popup.hidden, false);
  f.windowHandlers.get("scroll")({ target: f.popup });
  assert.equal(f.popup.hidden, false);
  f.windowHandlers.get("scroll")({ target: f.body });
  assert.equal(f.popup.hidden, true);
  f.emit("pointerout", owner, { relatedTarget: f.body });
  f.emit("pointerover", owner);
  f.windowHandlers.get("scroll")({ target: f.window });
  assert.equal(f.popup.hidden, true);
});

test("tooltips clamp to viewport edges and prefer above when below is full", (t) => {
  const f = fixture(t), owner = f.owner();
  f.window.innerWidth = 250; f.window.innerHeight = 200;
  owner.rect = { left: 220, top: 170, width: 24, height: 24, bottom: 194 };
  f.emit("pointerover", owner);
  assert.equal(f.popup.style.left, "82px");
  assert.equal(f.popup.style.top, "114px");
  owner.rect = { left: 0, top: 0, width: 24, height: 24, bottom: 24 };
  f.emit("pointerover", owner);
  assert.equal(f.popup.style.left, "8px");
  assert.equal(f.popup.style.top, "32px");
});

test("click, pointerdown, scroll, resize and blur dismiss stale help until a new interaction", (t) => {
  const f = fixture(t), owner = f.owner();
  for (const event of ["click", "pointerdown", "scroll", "resize", "blur"]) {
    f.emit("pointerout", owner, { relatedTarget: f.body });
    f.emit("pointerover", owner);
    assert.equal(f.popup.hidden, false);
    if (f.windowHandlers.has(event)) f.windowHandlers.get(event)({ target: f.body }); else f.emit(event, f.body);
    assert.equal(f.popup.hidden, true, event);
    owner.setAttribute("data-tooltip", "Fresh label after " + event);
    f.emit("pointerover", owner);
    assert.equal(f.popup.hidden, true, "dismissal suppresses stale redisplay");
  }
  f.emit("pointerout", owner, { relatedTarget: f.body });
  f.emit("pointerover", owner);
  assert.equal(f.popup.textContent, "Fresh label after blur");
});
