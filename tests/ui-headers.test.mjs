import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const commonSource = await readFile(new URL("../internal/ui/assets/common.js", import.meta.url), "utf8");
const commonURL = "data:text/javascript;base64," + Buffer.from(commonSource).toString("base64");
const { headersFromRows, maxHeaderRows, maxHeaderCharacters, requestURL, curlCommand } = await import(commonURL);
const editorSource = (await readFile(new URL("../internal/ui/assets/headers.js", import.meta.url), "utf8")).replace('"./common.js"', JSON.stringify(commonURL));
const { createHeaderEditor } = await import("data:text/javascript;base64," + Buffer.from(editorSource).toString("base64"));
const appSource = await readFile(new URL("../internal/ui/assets/app.js", import.meta.url), "utf8");
const presetsSource = await readFile(new URL("../internal/ui/assets/presets.js", import.meta.url), "utf8");
const { getPreset } = await import("data:text/javascript;base64," + Buffer.from(presetsSource).toString("base64"));

function fixture(t) {
  const document = { activeElement: null, createElement: (tag) => new Node(tag) };
  class Node {
    constructor(tag) { Object.assign(this, { tagName: tag.toUpperCase(), children: [], attributes: new Map(), listeners: new Map(), value: "", disabled: false }); }
    append(...children) { for (const child of children) { child.parent = this; this.children.push(child); } }
    replaceChildren(...children) { this.children = []; this.append(...children); }
    remove() { this.parent.children = this.parent.children.filter((child) => child !== this); }
    setAttribute(name, value) { this.attributes.set(name, String(value)); }
    getAttribute(name) { return this.attributes.get(name) ?? null; }
    addEventListener(name, callback) { this.listeners.set(name, callback); }
    click() { if (!this.disabled) this.listeners.get("click")?.(); }
    focus() { document.activeElement = this; }
    closest(tag) { return this.tagName === tag.toUpperCase() ? this : this.parent?.closest(tag); }
  }
  const original = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", { configurable: true, value: document });
  t.after(() => { if (original) Object.defineProperty(globalThis, "document", original); else delete globalThis.document; });
  const nodes = new Map();
  const $ = (id) => { if (!nodes.has(id)) nodes.set(id, new Node("div")); return nodes.get(id); };
  const details = new Node("details");
  details.append($("request-headers"), $("add-header"));
  let changes = 0;
  const editor = createHeaderEditor($("request-headers"), $("add-header"), () => { changes++; });
  const rows = () => $("request-headers").children.map((node) => ({
    node, key: node.children[0].children[0], value: node.children[1].children[0], remove: node.children[2]
  }));
  return { $, editor, rows, details, document, changes: () => changes };
}

test("blank rows are skipped, empty values are retained, and duplicate names combine case-insensitively", () => {
  const headers = headersFromRows([
    { name: "", value: "" }, { name: "  ", value: "" },
    { name: " X-Sample ", value: "first" }, { name: "x-sample", value: "second" }, { name: "X-Empty", value: "" }
  ]);
  assert.equal(headers.get("x-sample"), "first, second");
  assert.equal(headers.get("x-empty"), "");
  assert.equal([...headers].length, 2);
  for (const [row, part] of [
    [{ name: "", value: "orphan" }, "name"], [{ name: " \t ", value: "orphan" }, "name"],
    [{ name: "Bad Name", value: "value" }, "name"], [{ name: "X-Test\r", value: "value" }, "name"],
    [{ name: "X-Test", value: "line\nnext" }, "value"], [{ name: "X-Test", value: "\r\n" }, "value"],
    [{ name: "X-Test", value: "\0" }, "value"], [{ name: "X-Test", value: 1 }, "value"]
  ]) {
    assert.throws(() => headersFromRows([{ name: "X-Valid", value: "first" }, row]), (error) => error.row === 1 && error.part === part);
  }
});

test("row count and aggregate input size remain bounded", () => {
  assert.throws(() => headersFromRows(Array.from({ length: maxHeaderRows + 1 }, () => ({ name: "", value: "" }))), /at most 64/);
  const name = "X-Test";
  assert.equal(headersFromRows([{ name, value: "a".repeat(maxHeaderCharacters - name.length) }]).get(name).length, maxHeaderCharacters - name.length);
  assert.throws(() => headersFromRows([{ name, value: "a".repeat(maxHeaderCharacters - name.length) }, { name: "X", value: "" }]), /65,536/);
});

test("adding and removing rows preserves labels, focus, preview notifications, and one empty row", (t) => {
  const f = fixture(t);
  assert.equal(f.rows().length, 1);
  assert.equal(f.rows()[0].key.getAttribute("aria-label"), "Header name 1");
  assert.equal(f.rows()[0].value.getAttribute("aria-label"), "Header value 1");
  for (const field of [f.rows()[0].key, f.rows()[0].value]) assert.equal(field.getAttribute("aria-describedby"), "headers-hint");
  assert.deepEqual([...f.editor.read()], []);
  f.$("add-header").click();
  assert.equal(f.rows().length, 2);
  assert.equal(f.document.activeElement, f.rows()[1].key);
  assert.equal(f.rows()[1].remove.type, "button");
  assert.equal(f.rows()[1].remove.getAttribute("aria-label"), "Remove header 2");
  assert.equal(f.rows()[1].remove.children[0].textContent, "×");
  assert.equal(f.rows()[1].remove.children[0].getAttribute("aria-hidden"), "true");
  f.rows()[0].remove.click();
  assert.equal(f.rows().length, 1);
  assert.equal(f.document.activeElement, f.rows()[0].key);
  assert.equal(f.rows()[0].key.getAttribute("aria-label"), "Header name 1");
  f.rows()[0].key.value = "X-Sample";
  f.rows()[0].value.value = "hello";
  f.rows()[0].remove.click();
  assert.equal(f.rows().length, 1);
  assert.equal(f.rows()[0].key.value, "");
  assert.equal(f.rows()[0].value.value, "");
  assert.equal(f.changes(), 3);
  assert.equal(f.document.activeElement, f.rows()[0].key);
});

test("the Add button stops at the row limit and becomes usable after removal", (t) => {
  const f = fixture(t);
  for (let index = 1; index < maxHeaderRows; index++) f.$("add-header").click();
  assert.equal(f.rows().length, maxHeaderRows);
  assert.equal(f.$("add-header").disabled, true);
  f.$("add-header").click();
  assert.equal(f.rows().length, maxHeaderRows);
  f.rows().at(-1).remove.click();
  assert.equal(f.$("add-header").disabled, false);
  assert.equal(f.rows().length, maxHeaderRows - 1);
});

test("invalid header errors identify the exact field and hostile values stay input text", (t) => {
  const f = fixture(t);
  f.rows()[0].value.value = "missing name";
  assert.throws(() => f.editor.read(), (error) => error.field === f.rows()[0].key);
  f.rows()[0].key.value = "X-Sample";
  f.rows()[0].value.value = "bad\nvalue";
  assert.throws(() => f.editor.read(), (error) => error.field === f.rows()[0].value);
  const value = '<img src=x onerror="alert(1)">';
  f.editor.set({ "X-Sample": value });
  assert.equal(f.rows()[0].value.value, value);
  assert.equal(f.editor.read().get("X-Sample"), value);
  assert.equal(f.rows()[0].node.children.length, 3);
  assert.equal(f.rows()[0].value.children.length, 0);
});

test("preset loading fills rows and request/curl construction uses the same edited headers", (t) => {
  const f = fixture(t);
  const buildSource = appSource.slice(appSource.indexOf("function buildRequest("), appSource.indexOf("\nfunction updateCommand("));
  const exampleSource = appSource.slice(appSource.indexOf("function applyExample("), appSource.indexOf("\ninitTheme();"));
  const context = {
    $: f.$, headerEditor: f.editor, getPreset, Headers, TextEncoder, URL,
    serviceBase: new URL("https://echo.test/"), config: { max_request_size: 10485760 },
    window: { location: { href: "https://echo.test/ui/workbench/?example=json-post" } },
    methodControl: { refresh() {} }, selectTab() {}, updateMethod() {}, updateCommand() {},
    requestURL, invalid(message, field) { const error = new Error(message); error.field = field; throw error; }
  };
  vm.runInNewContext(buildSource + "\n" + exampleSource + "\napplyExample(null);", context);
  assert.equal(f.rows().length, 1);
  assert.equal(f.rows()[0].key.value, "Content-Type");
  assert.equal(f.rows()[0].value.value, "application/json");
  assert.equal(f.details.open, true);
  assert.equal(f.$("method").value, "POST");
  f.$("add-header").click();
  f.rows()[1].key.value = "X-Sample";
  f.rows()[1].value.value = "O'Reilly";
  const request = context.buildRequest();
  assert.equal(request.headers.get("X-Sample"), "O'Reilly");
  assert.match(curlCommand(request), /--header 'x-sample: O'\\''Reilly'/);
  assert.match(curlCommand(request), /--header 'content-type: application\/json'/);
  f.rows()[1].value.value = "changed";
  assert.equal(context.buildRequest().headers.get("X-Sample"), "changed");
  context.window.location.href = "https://echo.test/ui/workbench/?example=inspect";
  context.applyExample(null);
  assert.equal(f.rows().length, 1);
  assert.deepEqual([...f.editor.read()], []);
  assert.equal(f.details.open, false);
});
