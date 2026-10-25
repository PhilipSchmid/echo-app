import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/controls.js", import.meta.url), "utf8");
const { enhanceSelect } = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));

function fixture(t, labelled = true) {
  const doc = { listeners: new Map(), activeElement: null };
  class Node {
    constructor(tag) {
      Object.assign(this, { tagName: tag, ownerDocument: doc, attributes: new Map(), children: [], listeners: new Map(), classes: new Set(), dataset: {}, style: {}, hidden: false, disabled: false });
      this.classList = { contains: (name) => this.classes.has(name), add: (...names) => names.forEach((name) => this.classes.add(name)), remove: (name) => this.classes.delete(name) };
    }
    append(...children) { for (const child of children) { child.parentElement = this; this.children.push(child); } }
    setAttribute(name, value) { this.attributes.set(name, String(value)); }
    getAttribute(name) { return this.attributes.get(name) ?? null; }
    hasAttribute(name) { return this.attributes.has(name); }
    removeAttribute(name) { this.attributes.delete(name); }
    addEventListener(name, callback) { this.listeners.set(name, callback); }
    emit(type, options = {}) {
      const event = { type, target: this, prevented: false, preventDefault() { this.prevented = true; }, ...options };
      this.listeners.get(type)?.(event);
      return event;
    }
    dispatchEvent(event) { this.listeners.get(event.type)?.(event); return true; }
    matches(selector) { return selector === ":disabled" && this.disabled; }
    contains(node) { return node === this || this.children.some((child) => child.contains(node)); }
    getBoundingClientRect() { return { top: 40, bottom: 84, left: 20, width: 120 }; }
    get scrollHeight() { return this.children.length * 44; }
    get offsetHeight() { return this.scrollHeight; }
    scrollIntoView() { this.scrolled = true; }
    focus() { doc.activeElement = this; }
  }
  doc.createElement = (tag) => new Node(tag);
  doc.body = new Node("body");
  doc.addEventListener = (type, callback) => doc.listeners.set(type, callback);
  const previousWindow = globalThis.window;
  globalThis.window = { innerWidth: 1024, innerHeight: 768, addEventListener() {} };
  t.after(() => { if (previousWindow === undefined) delete globalThis.window; else globalThis.window = previousWindow; });
  const wrapper = new Node("div");
  wrapper.classList.add("select-control");
  const main = new Node("main");
  main.append(wrapper);
  doc.body.append(main);
  const select = new Node("select");
  Object.assign(select, { id: "method", name: "method", selectedIndex: 0, labels: labelled ? [{ htmlFor: "method" }] : [] });
  if (!labelled) select.setAttribute("aria-label", "Request method");
  select.setAttribute("aria-describedby", "method-hint");
  select.options = ["GET", "POST", "PUT"].map((label, index) => ({ label, parentElement: select, disabled: index === 1, hidden: false }));
  wrapper.append(select);
  const control = enhanceSelect(select);
  return { doc, main, wrapper, select, control, trigger: wrapper.children[1], menu: wrapper.children[2] };
}

test("enhanced select exposes its label, selected option, popup and invalid state", (t) => {
  const f = fixture(t);
  assert.equal(f.select.hidden, true);
  assert.equal(f.select.labels[0].htmlFor, f.trigger.id);
  assert.equal(f.trigger.getAttribute("role"), "combobox");
  assert.equal(f.trigger.getAttribute("aria-labelledby"), "method-label");
  assert.equal(f.trigger.getAttribute("aria-describedby"), "method-hint");
  assert.equal(f.trigger.getAttribute("aria-controls"), null);
  assert.equal(f.menu.getAttribute("role"), "listbox");
  assert.equal(f.menu.getAttribute("aria-labelledby"), "method-label", "the popup shares the actual field label, rather than its changing selected value");
  assert.equal(f.wrapper.contains(f.menu), true, "the listbox stays beside its trigger in the form");
  assert.equal(f.main.contains(f.menu), true, "the popup remains inside the main landmark");
  assert.equal(f.menu.children[0].getAttribute("aria-selected"), "true");
  assert.equal(f.menu.children[1].getAttribute("aria-disabled"), "true");
  f.select.setAttribute("aria-invalid", "true");
  f.control.refresh();
  assert.equal(f.trigger.getAttribute("aria-invalid"), "true");
  f.select.disabled = true;
  f.control.refresh();
  assert.equal(f.trigger.disabled, true);
  assert.equal(f.trigger.getAttribute("aria-disabled"), "true");
});

test("combobox navigation explores enabled options without committing until Tab or Enter", (t) => {
  const f = fixture(t);
  f.trigger.emit("keydown", { key: "ArrowDown" });
  assert.equal(f.trigger.getAttribute("aria-expanded"), "true");
  assert.equal(f.trigger.getAttribute("aria-controls"), f.menu.id);
  assert.equal(f.menu.getAttribute("aria-labelledby"), f.trigger.getAttribute("aria-labelledby"));
  assert.equal(f.menu.hidden, false);
  assert.equal(f.trigger.getAttribute("aria-activedescendant"), "method-option-0");
  assert.equal(f.doc.activeElement, f.trigger);
  f.trigger.emit("keydown", { key: "ArrowDown" });
  assert.equal(f.trigger.getAttribute("aria-activedescendant"), "method-option-2");
  assert.equal(f.select.selectedIndex, 0);
  const tab = f.trigger.emit("keydown", { key: "Tab" });
  assert.equal(tab.prevented, false, "Tab keeps native focus navigation");
  assert.equal(f.select.selectedIndex, 2);
  assert.equal(f.trigger.getAttribute("aria-expanded"), "false");
  assert.equal(f.trigger.getAttribute("aria-controls"), null);
  assert.equal(f.trigger.getAttribute("aria-activedescendant"), null);
  assert.equal(f.menu.hidden, true);
  f.trigger.emit("keydown", { key: "Home" });
  f.trigger.emit("keydown", { key: "Enter" });
  assert.equal(f.select.selectedIndex, 0);
});

test("combobox and popup retain the explicit accessible name when no visible label exists", (t) => {
  const f = fixture(t, false);
  assert.equal(f.trigger.getAttribute("aria-label"), "Request method");
  assert.equal(f.menu.getAttribute("aria-label"), "Request method");
  assert.equal(f.menu.getAttribute("aria-labelledby"), null);
});

test("Escape cancels the tentative value and retains focus on the combobox", (t) => {
  const f = fixture(t);
  f.trigger.emit("keydown", { key: "End" });
  assert.equal(f.trigger.getAttribute("aria-activedescendant"), "method-option-2");
  const escape = f.trigger.emit("keydown", { key: "Escape" });
  assert.equal(escape.prevented, true);
  assert.equal(f.select.selectedIndex, 0);
  assert.equal(f.menu.hidden, true);
  assert.equal(f.doc.activeElement, f.trigger);
});

test("combobox typing preserves IME and platform modifier shortcuts", (t) => {
  const f = fixture(t);
  for (const options of [{ isComposing: true }, { keyCode: 229 }, { ctrlKey: true }, { metaKey: true }, { altKey: true }]) {
    const event = f.trigger.emit("keydown", { key: "p", ...options });
    assert.equal(event.prevented, false);
    assert.equal(f.menu.hidden, true);
  }
  const event = f.trigger.emit("keydown", { key: "p" });
  assert.equal(event.prevented, true);
  assert.equal(f.menu.hidden, false);
  assert.equal(f.trigger.getAttribute("aria-activedescendant"), "method-option-2");
  assert.equal(f.select.selectedIndex, 0);
});
