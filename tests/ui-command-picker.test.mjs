import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

let source = await readFile(new URL("../internal/ui/assets/terminal.js", import.meta.url), "utf8");
for (const name of ["common", "presets"]) {
  const dependency = await readFile(new URL(`../internal/ui/assets/${name}.js`, import.meta.url));
  source = source.replace(`"./${name}.js"`, `"data:text/javascript;base64,${dependency.toString("base64")}"`);
}
const { createCommandPicker } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
const storageKey = "echo-observatory-terminal-client";

function fixture(t, saved = null) {
  const state = { blocked: false, values: new Map([[storageKey, saved]]), writes: [], rendered: [] };
  const document = { activeElement: null, createElement: (tag) => new Node(tag) };
  class Node {
    constructor(tag) {
      Object.assign(this, { tagName: tag.toUpperCase(), children: [], attributes: new Map(), listeners: new Map(), hidden: false, tabIndex: 0 });
    }
    append(...children) { this.children.push(...children); }
    setAttribute(name, value) {
      this.attributes.set(name, String(value));
      if (name === "id") this.id = String(value);
      if (name === "tabindex") this.tabIndex = Number(value);
    }
    getAttribute(name) { return name === "id" ? this.id ?? null : this.attributes.get(name) ?? null; }
    addEventListener(name, callback) { this.listeners.set(name, callback); }
    focus() { document.activeElement = this; }
    click() { this.listeners.get("click")?.({ currentTarget: this }); }
    press(key, modifiers = {}) {
      const event = { key, ...modifiers, currentTarget: this, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; } };
      this.listeners.get("keydown")?.(event);
      return event;
    }
  }
  const replacements = {
    document,
    sessionStorage: {
      getItem: (key) => { if (state.blocked) throw new Error("blocked"); return state.values.get(key) ?? null; },
      setItem: (key, value) => {
        if (state.blocked) throw new Error("blocked");
        state.values.set(key, value);
        state.writes.push([key, value]);
      }
    }
  };
  for (const [key, value] of Object.entries(replacements)) {
    const original = Object.getOwnPropertyDescriptor(globalThis, key);
    Object.defineProperty(globalThis, key, { configurable: true, value });
    t.after(() => { if (original) Object.defineProperty(globalThis, key, original); else delete globalThis[key]; });
  }
  state.renderBlock = (label, command, context) => {
    state.rendered.push({ label, command, context });
    const block = new Node("div");
    block.textContent = command;
    return block;
  };
  state.document = document;
  return state;
}

function byRole(root, role) {
  return [root, ...root.children.flatMap((child) => byRole(child, role))].filter((node) => node.getAttribute("role") === role);
}

function selected(root) {
  return byRole(root, "tab").find((tab) => tab.getAttribute("aria-selected") === "true");
}

const commands = [
  { label: "curl", command: "curl 'https://echo.example/'" },
  { label: "wget", command: "wget 'https://echo.example/'" },
  { label: "curl · h2c", command: "curl --http2-prior-knowledge 'http://echo.example/'" }
];

test("command client choice stays synchronized across public and listener examples, including later groups", (t) => {
  const state = fixture(t);
  const picker = createCommandPicker(state.renderBlock);
  const publicExample = picker(commands.slice(0, 2), "Public address");
  const http = picker(commands, "HTTP");
  const https = picker(commands.slice(0, 2), "HTTPS");
  assert.deepEqual(state.writes, [], "Rendering examples must not save a preference");
  byRole(http, "tab")[1].click();
  for (const group of [publicExample, http, https]) {
    assert.equal(selected(group).textContent, "wget");
    assert.equal(byRole(group, "tabpanel").filter((panel) => !panel.hidden).length, 1);
  }
  const health = picker(commands.slice(0, 2), "Health & metrics");
  assert.equal(selected(health).textContent, "wget");
  assert.deepEqual(state.writes.at(-1), [storageKey, "wget"]);
  assert.equal(selected(createCommandPicker(state.renderBlock)(commands, "Reloaded page")).textContent, "wget");
  byRole(publicExample, "tab")[0].click();
  for (const group of [publicExample, http, https, health]) assert.equal(selected(group).textContent, "curl");
  assert.deepEqual(state.rendered.slice(0, 2), commands.slice(0, 2).map((command) => ({ ...command, context: "Public address" })));
});

test("command tabs expose linked panels and support wrapped arrow, Home and End navigation without moving another group’s focus", (t) => {
  const state = fixture(t);
  const picker = createCommandPicker(state.renderBlock);
  const http = picker(commands, "HTTP");
  const https = picker(commands.slice(0, 2), "HTTPS");
  const tabs = byRole(http, "tab");
  const panels = byRole(http, "tabpanel");
  const ids = [...tabs, ...byRole(https, "tab")].map((tab) => tab.id);
  assert.ok(ids.every(Boolean));
  assert.equal(new Set(ids).size, ids.length);
  assert.equal(byRole(http, "tablist").length, 1);
  assert.ok(byRole(http, "tablist")[0].getAttribute("aria-label").includes("HTTP"));
  for (const [index, tab] of tabs.entries()) {
    assert.equal(tab.getAttribute("aria-controls"), panels[index].id);
    assert.equal(panels[index].getAttribute("aria-labelledby"), tab.id);
  }
  for (const [from, key, to] of [[0, "ArrowLeft", 2], [2, "ArrowRight", 0], [0, "End", 2], [2, "Home", 0], [0, "ArrowRight", 1]]) {
    assert.equal(tabs[from].press(key).defaultPrevented, true);
    assert.equal(state.document.activeElement, tabs[to]);
    assert.equal(selected(http), tabs[to]);
    for (const [index, tab] of tabs.entries()) {
      assert.equal(tab.tabIndex, index === to ? 0 : -1);
      assert.equal(tab.getAttribute("aria-selected"), String(index === to));
      assert.equal(panels[index].hidden, index !== to);
    }
  }
  assert.equal(selected(https).textContent, "wget");
  assert.equal(tabs[1].press("Tab").defaultPrevented, false);
  assert.equal(tabs[1].press("Home", { ctrlKey: true }).defaultPrevented, false);
  assert.equal(selected(http), tabs[1]);
  for (const modifiers of [{shiftKey:true},{isComposing:true},{altKey:true},{metaKey:true}]) {
    assert.equal(tabs[1].press("ArrowRight",modifiers).defaultPrevented,false);
    assert.equal(selected(http),tabs[1]);
  }
});

test("saved client preference survives a fresh picker and invalid or blocked storage stays usable", (t) => {
  const state = fixture(t, "wget");
  assert.equal(selected(createCommandPicker(state.renderBlock)(commands, "HTTP")).textContent, "wget");
  state.values.set(storageKey, "<script>invalid</script>");
  assert.equal(selected(createCommandPicker(state.renderBlock)(commands, "HTTP")).textContent, "curl");
  state.blocked = true;
  const picker = createCommandPicker(state.renderBlock);
  const http = picker(commands, "HTTP");
  assert.equal(selected(http).textContent, "curl");
  assert.doesNotThrow(() => byRole(http, "tab")[1].click());
  assert.equal(selected(picker(commands, "HTTPS")).textContent, "wget");
});

test("curl-only variants and single-client protocols remain available without inventing wget support or altering command bytes", (t) => {
  const state = fixture(t);
  const picker = createCommandPicker(state.renderBlock);
  const http = picker(commands, "HTTP");
  const family = picker([{ label: "curl · IPv4", command: "curl -4 HOST" }, { label: "curl · IPv6", command: "curl -6 HOST" }], "Address family");
  const literal = "curl --http3-only 'https://HOST/'\n# <script> & $(printf sample)";
  const h3 = picker([{ label: "curl · HTTP/3", command: literal }], "HTTP/3");
  const tcp = picker([{ label: "netcat", command: "nc HOST 9090" }], "TCP");
  const grpc = picker([{ label: "grpcurl", command: "grpcurl HOST:50051 echo.EchoService/Echo" }], "gRPC");
  byRole(http, "tab")[1].click();
  assert.equal(state.values.get(storageKey), "wget");
  assert.ok(selected(family).textContent.startsWith("curl"));
  assert.equal(byRole(family, "tabpanel").filter((panel) => !panel.hidden).length, 1);
  for (const group of [h3, tcp, grpc]) {
    assert.equal(byRole(group, "tablist").length, 0);
    assert.equal(group.hidden, false);
  }
  assert.equal(h3.textContent, literal);
  assert.ok(state.rendered.some((entry) => entry.label === "curl · HTTP/3" && entry.command === literal && entry.context === "HTTP/3"));
  byRole(family, "tab")[1].click();
  assert.equal(selected(http).textContent, "curl");
  assert.deepEqual(state.writes.at(-1), [storageKey, "curl"]);
});
