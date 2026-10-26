import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = (await readFile(new URL("../internal/ui/assets/app.js", import.meta.url), "utf8"))
  .replace(/^import .*;\n/gm, "")
  .replaceAll("import.meta.url", '"http://echo.test/ui/app.js"');
const previewLimit = 1048576;
const html = await readFile(new URL("../internal/ui/assets/workbench.html", import.meta.url), "utf8");

class Node {
  constructor(tagName = "div", className = "", text = "") {
    Object.assign(this, { tagName, className, textContent: text, value: "", children: [], listeners: new Map(), attributes: new Map() });
    this.classList = { toggle: (name, enabled) => {
      const classes = new Set(this.className.split(" ").filter(Boolean));
      if (enabled) classes.add(name); else classes.delete(name);
      this.className = [...classes].join(" ");
    } };
  }
  append(...nodes) { for (const node of nodes) { node.parent = this; this.children.push(node); } }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  remove() { this.parent.children = this.parent.children.filter((node) => node !== this); }
  get firstElementChild() { return this.children[0]; }
  get scrollHeight() { return this.children.length; }
  setAttribute(name, value) { this.attributes.set(name, value); }
  getAttribute(name) { return this.attributes.get(name); }
  removeAttribute(name) { this.attributes.delete(name); }
  addEventListener(name, callback) {
    if (!this.listeners.has(name)) this.listeners.set(name, []);
    this.listeners.get(name).push(callback);
  }
  emit(name, event = {}) { for (const callback of this.listeners.get(name) || []) callback(event); }
  querySelector() { return new Node(); }
  focus() { this.focused = true; }
  closest() { return this.details ?? null; }
  requestSubmit() { this.submissions = (this.submissions || 0) + 1; this.emit("submit", { preventDefault() {} }); }
}

function fixture(settings) {
  const nodes = new Map();
  const $ = (id) => { if (!nodes.has(id)) nodes.set(id, new Node()); return nodes.get(id); };
  $("method").value = "GET";
  $("request-path").value = "/";
  const sockets = [];
  class Socket extends Node {
    static OPEN = 1;
    static CLOSING = 2;
    constructor() { super(); Object.assign(this, { readyState: 0, bufferedAmount: 0, sent: [] }); sockets.push(this); }
    send(text) { this.sent.push(text); }
    close() { this.readyState = 3; }
  }
  const context = {
    $, previewLimit, URL, Headers, TextEncoder, WebSocket: Socket,
    baseURLs: () => ({ uiBase: new URL("http://echo.test/ui/"), serviceBase: new URL("http://echo.test/") }),
    element: (tag, classes, text) => new Node(tag, classes, text),
    renderCode: (node, text) => { node.textContent = text; },
    requestURL: (path, base) => new URL(path, base), createHeaderEditor: () => ({ read: () => new Headers(), set() {} }), curlCommand: () => "curl",
    initTheme() {}, initTooltips() {}, initDisclosureHelp() {}, enhanceSelect() {}, copy() {}, setCopyLabel() {}, getPreset() {},
    initServiceStatus: () => settings ? Promise.resolve(settings) : new Promise(() => {}),
    document: { addEventListener() {} },
    window: { location: { href: "http://echo.test/ui/workbench/" }, addEventListener() {}, setTimeout() {}, clearTimeout() {} }
  };
  vm.runInNewContext(source, context);
  return {
    context, $, rows: () => $("websocket-log").children,
    connect() { $("connect-websocket").emit("click"); const socket = sockets.at(-1); socket.readyState = Socket.OPEN; socket.emit("open"); return socket; },
    key(options = {}) {
      const event = { key: "Enter", keyCode: 13, repeat: false, isComposing: false, shiftKey: false, ctrlKey: false, altKey: false, metaKey: false,
        prevented: false, preventDefault() { this.prevented = true; }, ...options };
      $("websocket-message").emit("keydown", event);
      return event;
    }
  };
}

function find(node, className) {
  if (node.className.split(" ").includes(className)) return node;
  for (const child of node.children) { const result = find(child, className); if (result) return result; }
}

test("Enter sends through the form and keeps the message available for resending", () => {
  const page = fixture();
  const socket = page.connect();
  const text = 'Hello observatory!\n{"echo":true}';
  page.$("websocket-message").value = text;
  assert.equal(page.key().prevented, true);
  assert.equal(page.$("websocket-form").submissions, 1);
  assert.deepEqual(socket.sent, [text]);
  assert.equal(page.$("websocket-message").value, text);
  assert.equal(page.key({ repeat: true }).prevented, true);
  assert.equal(page.$("websocket-form").submissions, 1);
  assert.deepEqual(socket.sent, [text]);
});

test("HTTP sending stays focusable, suppresses duplicate submits and resets after success or failure", async () => {
  for (const fails of [false, true]) {
    const page = fixture();
    let complete;
    let calls = 0;
    let timers = 0;
    let cleared = 0;
    page.context.AbortController = AbortController;
    page.context.performance = { now: () => 0 };
    page.context.window.setTimeout = () => { timers++; return 1; };
    page.context.window.clearTimeout = () => { cleared++; };
    page.context.fetch = () => {
      calls++;
      return new Promise((resolve, reject) => { complete = () => fails ? reject(new Error("Offline")) : resolve({ status: 200, headers: new Headers(), body: null }); });
    };
    const button = page.$("send-request");
    button.disabled = false;
    const submit = page.$("request-form").listeners.get("submit")[0];
    const first = submit({ preventDefault() {} });
    assert.equal(button.disabled, false, "sending does not remove the focused button from keyboard navigation");
    assert.equal(button.getAttribute("aria-disabled"), "true");
    assert.equal(page.$("send-label").textContent, "Sending…");
    await submit({ preventDefault() {} });
    assert.equal(calls, 1);
    assert.equal(timers, 1, "ignored submits do not start another timeout");
    complete();
    await first;
    assert.equal(button.getAttribute("aria-disabled"), "false");
    assert.equal(button.disabled, false);
    assert.equal(page.$("send-label").textContent, "Send request");
    assert.equal(page.$("response-panel").getAttribute("aria-busy"), "false");
    assert.equal(cleared, 1);
    const next = submit({ preventDefault() {} });
    assert.equal(calls, 2, "the completed request can be retried");
    complete();
    await next;
  }
});

test("modified Enter and IME input retain their native textarea behavior", () => {
  const page = fixture();
  const socket = page.connect();
  for (const options of [{ shiftKey: true }, { ctrlKey: true }, { altKey: true }, { metaKey: true }, { isComposing: true }, { keyCode: 229 }, { key: "Escape" }]) {
    assert.equal(page.key(options).prevented, false, JSON.stringify(options));
  }
  assert.equal(page.$("websocket-form").submissions, undefined);
  assert.deepEqual(socket.sent, []);
});

test("keyboard sending preserves connection, UTF-8 size, and backpressure guards", () => {
  const page = fixture();
  page.key();
  assert.equal(page.$("websocket-error").textContent, "Connect before sending a message.");
  const socket = page.connect();
  page.$("websocket-message").value = "🌌".repeat(previewLimit / 4 + 1);
  page.key();
  assert.match(page.$("websocket-error").textContent, /message exceeds/);
  page.$("websocket-message").value = "hello";
  socket.bufferedAmount = previewLimit + 1;
  page.key();
  assert.match(page.$("websocket-error").textContent, /still sending/);
  assert.deepEqual(socket.sent, []);
  assert.equal(page.rows().length, 1);
});

test("activity distinguishes sent, received, and lifecycle events without interpreting payload markup", () => {
  const page = fixture();
  const socket = page.connect();
  const payload = '</pre><img src="x" onerror="alert(1)">';
  page.$("websocket-message").value = payload;
  page.key();
  socket.emit("message", { data: payload });
  const [connected, sent, received] = page.rows();
  assert.equal(connected.className, "socket-entry socket-event");
  assert.equal(find(connected, "log-direction"), undefined);
  assert.equal(find(connected, "log-label").textContent, "Connected");
  for (const [row, kind, label, direction, arrow] of [[sent, "sent", "Sent", "You → Server", "↑"], [received, "received", "Received", "Server → You", "↓"]]) {
    assert.equal(row.className, "socket-entry socket-" + kind);
    assert.equal(find(row, "log-label").textContent, label);
    assert.equal(find(row, "log-direction").textContent, direction);
    assert.equal(find(row, "log-icon").textContent, arrow);
    assert.equal(find(row, "log-icon").getAttribute("aria-hidden"), "true");
    assert.equal(row.children[1].tagName, "pre");
    assert.equal(row.children[1].textContent, payload);
    assert.equal(row.children[1].children.length, 0);
    assert.match(row.children[0].children[1].dateTime, /^\d{4}-\d\d-/);
  }
  socket.emit("close", { code: 1000 });
  assert.equal(page.rows().at(-1).className, "socket-entry socket-event");
  assert.equal(find(page.rows().at(-1), "log-label").textContent, "Disconnected");
});

test("activity retains bounded previews and history, and clears to its empty state", () => {
  const page = fixture();
  const socket = page.connect();
  socket.emit("message", { data: "x".repeat(2001) });
  assert.equal(page.rows().at(-1).children[1].textContent, "x".repeat(2000) + "\n[Preview shortened]");
  for (let index = 0; index < 21; index++) socket.emit("message", { data: "message " + index });
  assert.equal(page.rows().length, 20);
  assert.equal(page.rows()[0].children[1].textContent, "message 1");
  assert.equal(page.$("websocket-empty").hidden, true);
  assert.equal(page.$("clear-websocket").disabled, false);
  page.$("clear-websocket").emit("click");
  assert.equal(page.rows().length, 0);
  assert.equal(page.$("websocket-empty").hidden, false);
  assert.equal(page.$("clear-websocket").disabled, true);
});

test("workbench size controls and WebSocket messages follow configured caps", async () => {
  const page = fixture({ max_request_size: 65536, max_response_size: 16384, response_controls: true });
  await Promise.resolve();
  assert.equal(page.$("control-size").max, "16384");
  assert.match(page.$("response-limits").textContent, /16,384 bytes/);
  assert.equal(page.$("request-body").maxLength, 65536);
  assert.equal(page.$("websocket-message").maxLength, 16384);
  assert.match(page.$("websocket-limit").textContent, /16,384 bytes/);
  const socket = page.connect();
  page.$("websocket-message").value = "🌌".repeat(4097);
  page.key();
  assert.deepEqual(socket.sent, []);
  assert.match(page.$("websocket-error").textContent, /16,384 byte limit/);
  page.$("websocket-message").value = "small sample";
  page.key();
  assert.deepEqual(socket.sent, ["small sample"]);
});

test("legacy zero response cap uses the protocol's 1 MiB ceiling", async () => {
  const page = fixture({ max_request_size: 2097152, max_response_size: 0, response_controls: true });
  await Promise.resolve();
  assert.equal(page.$("control-size").max, "1048576");
  assert.equal(page.$("websocket-message").maxLength, 1048576);
  assert.match(page.$("response-limits").textContent, /1,048,576 bytes/);
});

test("messages use a conservative 64 KiB cap while settings are unavailable", () => {
  const page = fixture();
  const socket = page.connect();
  page.$("websocket-message").value = "x".repeat(65537);
  page.key();
  assert.deepEqual(socket.sent, []);
  assert.match(page.$("websocket-error").textContent, /65,536 byte limit/);
});


test("request validation identifies the field, opens its section and clears stale errors after editing", () => {
  const page = fixture();
  const field = page.$("request-path");
  field.details = { open: false };
  field.setAttribute("aria-describedby", "path-hint");
  page.context.feedback("request-error", "Keep the path in this service.");
  page.context.focusError({ field });
  assert.equal(field.getAttribute("aria-invalid"), "true");
  assert.equal(field.details.open, true);
  assert.equal(field.focused, true);
  assert.equal(field.getAttribute("aria-describedby"), "path-hint request-error");
  page.$("request-form").emit("input");
  assert.equal(field.getAttribute("aria-invalid"), "false");
  assert.equal(field.getAttribute("aria-describedby"), "path-hint");
  assert.equal(page.$("request-error").hidden, true);
  assert.match(html, /id="request-path"[^>]*aria-describedby="path-hint"/);
  assert.match(html, /id="request-body"[^>]*aria-describedby="body-hint"/);
});

test("native invalid events expose the first invalid control instead of trapping focus in a closed section", () => {
  const page = fixture();
  const field = page.$("control-status");
  field.details = { open: false };
  field.validationMessage = "Value must be at least 200.";
  const event = { target: field, prevented: false, preventDefault() { this.prevented = true; } };
  page.$("request-form").emit("invalid", event);
  assert.equal(event.prevented, true);
  assert.equal(page.$("request-error").textContent, field.validationMessage);
  assert.equal(field.getAttribute("aria-invalid"), "true");
  assert.equal(field.focused, true);
  assert.equal(field.details.open, true);
  const second = page.$("control-size");
  page.$("request-form").emit("invalid", { target: second, preventDefault() {} });
  assert.notEqual(second.focused, true);
  assert.equal(page.$("request-error").textContent, field.validationMessage);
});

test("responses and socket messages announce concise results without reading payloads", () => {
  const page = fixture();
  page.context.showResponse({ status: 200, statusText: "OK", headers: new Headers({ "Content-Type": "application/json" }) },
    { text: '{"sample":"private-payload-marker"}', bytes: 35, truncated: false }, 12, { method: "GET" }, true);
  assert.equal(page.$("request-status").textContent, "Response received: HTTP 200, 35 bytes, 12 milliseconds.");
  assert.doesNotMatch(page.$("request-status").textContent, /private-payload-marker/);
  assert.match(html, /class="instance" role="group" aria-label="Current service"/);
  assert.match(html, /id="request-status"[^>]*role="status"[^>]*aria-atomic="true"/);
  assert.doesNotMatch(html, /id="response-badges"[^>]*aria-live/);
  assert.doesNotMatch(html, /id="websocket-log"[^>]*aria-live/);
  const socket = page.connect();
  const payload = "sensitive-message-marker";
  page.$("websocket-message").value = payload;
  page.key();
  assert.equal(page.$("websocket-announcement").textContent, "Sent message 1, 24 bytes.");
  socket.emit("message", { data: payload });
  assert.equal(page.$("websocket-announcement").textContent, "Received message 1, 24 bytes.");
  socket.emit("message", { data: payload });
  assert.equal(page.$("websocket-announcement").textContent, "Received message 2, 24 bytes.");
  assert.doesNotMatch(page.$("websocket-announcement").textContent, /sensitive-message-marker/);
  assert.equal(page.$("websocket-log").tabIndex, 0);
  page.$("clear-websocket").emit("click");
  assert.equal(page.$("websocket-announcement").textContent, "Connection activity cleared.");
  assert.equal(page.$("websocket-log").tabIndex, -1);
  assert.equal(page.$("socket-log-title").focused, true);
});

test("response metadata distinguishes trusted client addresses from the socket hop", () => {
  for (const forwarded of [false, true]) {
    const page = fixture();
    const client = "2001:db8a:1234:5678:9abc:def0:1234:5678";
    const data = { source_ip: client, ip_version: "IPv6", http_host: "[" + client + "]:8443", remote_addr: "[" + client + "]:40500", local_addr: "[2001:db8::43]:443" };
    if (forwarded) Object.assign(data, { socket_peer_ip: "172.16.0.2", socket_peer_ip_version: "IPv4", socket_peer_addr: "172.16.0.2:50000", socket_local_addr: "172.16.0.3:8443", socket_local_ip_version: "IPv4" });
    page.context.showResponse({ status: 200, statusText: "OK", headers: new Headers({ "Content-Type": "application/json" }) },
      { text: JSON.stringify(data), bytes: 100, truncated: false }, 12, { method: "GET" }, true);
    const rows = (id) => Object.fromEntries(page.$(id).children.map((row) => row.children.map((node) => node.textContent)));
    const summary = rows("response-metadata");
    assert.equal(summary[forwarded ? "Client IP" : "Socket peer IP"], data.source_ip);
    assert.equal(summary[forwarded ? "Client IP family" : "IP family on this hop"], "IPv6");
    const details = rows("connection-metadata");
    for (const id of ["response-metadata", "connection-metadata"]) {
      for (const row of page.$(id).children) {
        const [label, value] = row.children;
        const address = /^(Client IP|Socket peer IP|Host|Client address|Destination address|Remote socket|Local socket)$/.test(label.textContent);
        assert.equal(row.className === "metadata-address", address, label.textContent);
        assert.equal(value.className === "address-value", address, label.textContent);
      }
    }
    assert.equal(details.Host, data.http_host);
    if (forwarded) {
      assert.equal(details["Client address"], data.remote_addr);
      assert.equal(details["Destination address"], data.local_addr);
      assert.equal(details["Socket peer"], undefined);
      assert.equal(details["Socket peer IP family"], undefined);
      assert.equal(details["App socket"], undefined);
      assert.match(page.$("response-body").textContent, /"socket_peer_addr": "172\.16\.0\.2:50000"/);
    } else {
      assert.equal(details["Remote socket"], data.remote_addr);
      assert.equal(details["Local socket"], data.local_addr);
      assert.equal(details["Socket peer"], undefined);
    }
  }
});

test("workbench tab shortcuts preserve modifier and composition keys", () => {
  const page = fixture();
  const current = page.$("request-tab");
  for (const options of [{ altKey: true }, { ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { isComposing: true }]) {
    let prevented = false;
    current.emit("keydown", { key: "ArrowRight", preventDefault() { prevented = true; }, ...options });
    assert.equal(prevented, false);
    assert.notEqual(page.$("websocket-tab").focused, true);
  }
  page.$("request-tab").getAttribute = (name) => name === "aria-controls" ? "request-panel" : undefined;
  page.$("websocket-tab").getAttribute = (name) => name === "aria-controls" ? "websocket-panel" : undefined;
  let prevented = false;
  current.emit("keydown", { key: "ArrowRight", preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  assert.equal(page.$("websocket-tab").attributes.get("aria-selected"), "true");
  assert.equal(page.$("request-tab").tabIndex, -1);
  assert.equal(page.$("websocket-tab").tabIndex, 0);
  assert.equal(page.$("request-panel").hidden, true);
  assert.equal(page.$("websocket-panel").hidden, false);
  assert.equal(page.$("websocket-tab").focused, true);
});
