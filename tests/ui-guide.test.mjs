import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("../internal/ui/assets/guide.js", import.meta.url), "utf8");
const html = await readFile(new URL("../internal/ui/assets/connect.html", import.meta.url), "utf8");
const renderSource = source.slice(source.indexOf("function renderListeners("), source.indexOf('\nif (page === "connect")'));

class Node {
  constructor(tagName, className = "", textContent = "") {
    Object.assign(this, { tagName: tagName.toUpperCase(), className, textContent, children: [], listeners: new Map(), hidden: false, open: false });
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  addEventListener(type, callback) { this.listeners.set(type, callback); }
  cloneNode() { return Object.assign(new Node(this.tagName, this.className, this.textContent), { templateID: this.templateID }); }
  querySelector(selector) { return descendants(this).find((node) => node.className.split(" ").includes(selector.slice(1))) ?? null; }
}

function descendants(node) { return node.children.flatMap((child) => [child, ...descendants(child)]); }

function render(settings) {
  const nodes = new Map([["listener-cards", new Node("div")]]);
  const calls = [];
  for (const [, id, content] of html.matchAll(/<template\b[^>]*\bid="([^"]+-hint-template)"[^>]*>([\s\S]*?)<\/template>/g)) {
    const classes = content.match(/<aside\b[^>]*class="([^"]+)"/)[1];
    assert.ok(classes.split(" ").includes("protocol-hint"));
    const hint = new Node("aside", classes, content);
    hint.templateID = id;
    nodes.set(id, { content: { firstElementChild: hint } });
  }
  const context = {
    settings, $: (id) => nodes.get(id),
    element: (...args) => new Node(...args),
    directCommands: (name, settings, base) => { calls.push({ name, settings, base }); return []; }, commandPicker: () => new Node("div", "command-picker"),
    URL, uiBase: new URL("https://echo.test/prefix/ui/"), serviceBase: new URL("https://echo.test/prefix/")
  };
  vm.runInNewContext(renderSource + "\nrenderListeners(settings);", context);
  const cards = nodes.get("listener-cards").children;
  return { nodes, calls, card: (name) => cards.find((card) => card.querySelector(".listener-name").textContent === name) };
}

const ports = { http_port: "8080", tls_port: "8443", tcp_port: "9090", grpc_port: "50051", quic_port: "4433" };

test("protocol hints stay inside the matching expanded listener examples", () => {
  const { card } = render({ ...ports, h2c: true, tls: true, tcp: true, grpc: true, grpc_tls: true, quic: true });
  for (const [name, templateID] of [["HTTP", "h2c-hint-template"], ["HTTPS", "http2-hint-template"], ["HTTP/3", "http3-hint-template"]]) {
    const listener = card(name);
    assert.equal(listener.tagName, "DETAILS");
    assert.equal(listener.open, false);
    assert.equal(listener.querySelector(".listener-body").querySelector(".protocol-hint").templateID, templateID);
  }
  for (const name of ["TCP", "gRPC"]) assert.equal(card(name).querySelector(".protocol-hint"), null);
});

test("disabled protocols and HTTP without h2c have no irrelevant hints or certificate links", () => {
  const { card } = render({ ...ports, grpc: true });
  for (const name of ["HTTP", "HTTPS", "TCP", "gRPC", "HTTP/3"]) {
    assert.equal(card(name).querySelector(".protocol-hint"), null, name);
    assert.equal(card(name).querySelector(".certificate-link"), null, name);
  }
  for (const name of ["HTTPS", "TCP", "HTTP/3"]) {
    assert.equal(card(name).tagName, "ARTICLE");
    assert.equal(card(name).querySelector(".listener-body"), null);
  }
});

test("public Connect hides private admin endpoints and configuration", () => {
  const { nodes } = render({ ...ports, tls: true, tcp: true, grpc: true, quic: true });
  assert.equal(nodes.get("listener-cards").children.length, 5);
  assert.doesNotMatch(html, /id="health-metrics"|admin-cards|admin-status|\/(?:health|ready|metrics)\b/);
  assert.doesNotMatch(source, /renderAdministration|metrics_port|admin-status|admin-cards/);
});

test("TLS examples link to certificate insights on Your request", () => {
  for (const [name, enabled] of [["HTTPS", { tls: true }], ["HTTP/3", { quic: true }], ["gRPC", { grpc: true, grpc_tls: true }]]) {
    const { card } = render({ ...ports, ...enabled });
    const link = card(name).querySelector(".certificate-link");
    assert.equal(link.textContent, "Certificate details →");
    assert.equal(link.href, "https://echo.test/prefix/ui/#certificate-help");
  }
  assert.doesNotMatch(html, /id="certificate-(?:help|fields|technical-fields)"/);
});

test("listener cards and commands use the same advertised ports and page address", () => {
  const settings = { http_port: "80", tls_port: "443", tcp_port: "9090", grpc_port: "50051", quic_port: "443", tls: true, tcp: true, grpc: true, quic: true };
  const { card, calls } = render(settings);
  for (const [name, port, transport] of [["HTTP", "80", "TCP"], ["HTTPS", "443", "TCP"], ["TCP", "9090", "TCP"], ["gRPC", "50051", "TCP"], ["HTTP/3", "443", "UDP"]]) {
    assert.equal(card(name).querySelector(".listener-port").textContent, port + " / " + transport);
    const call = calls.find((entry) => entry.name === name);
    assert.equal(call.settings, settings);
    assert.equal(call.base.href, "https://echo.test/prefix/");
  }
});

test("the IPv4-only provider hint is opt-in and scoped to the enabled HTTP/3 example", () => {
  const settings = { ...ports, tls: true, quic: true, http3_ipv4_only: true };
  const { card } = render(settings);
  const hint = card("HTTP/3").querySelector(".http3-ipv4-hint");
  assert.equal(hint.templateID, "http3-ipv4-hint-template");
  assert.match(hint.textContent, /<h3>IPv4 only<\/h3>/);
  assert.match(hint.textContent, /Our hosting provider does not yet support IPv6 for UDP\./);
  assert.match(hint.textContent, /class="http3-client-check"/);
  assert.match(hint.textContent, /<code>curl --version<\/code>/);
  assert.equal(descendants(card("HTTP/3")).filter((node) => node.className.split(" ").includes("protocol-hint")).length, 1);
  for (const name of ["HTTP", "HTTPS", "TCP", "gRPC"]) assert.equal(card(name).querySelector(".http3-ipv4-hint"), null);
  for (const override of [{ http3_ipv4_only: undefined }, { http3_ipv4_only: false }, { http3_ipv4_only: "true" }, { quic: false }]) {
    assert.equal(render({ ...settings, ...override }).card("HTTP/3").querySelector(".http3-ipv4-hint"), null);
  }
});
