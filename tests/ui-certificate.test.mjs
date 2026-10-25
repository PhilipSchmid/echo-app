import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("../internal/ui/assets/certificate.js", import.meta.url), "utf8");
const html = await readFile(new URL("../internal/ui/assets/index.html", import.meta.url), "utf8");
const connect = await readFile(new URL("../internal/ui/assets/connect.html", import.meta.url), "utf8");
const visit = await readFile(new URL("../internal/ui/assets/visit.js", import.meta.url), "utf8");
const script = source.replace(/^import .*;\n/gm, "").replace("export function ", "function ");

class Node {
  constructor(tagName, className = "", textContent = "") {
    Object.assign(this, { tagName: tagName.toUpperCase(), className, textContent, children: [], listeners: new Map(), hidden: false, open: false });
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  addEventListener(type, callback) { this.listeners.set(type, callback); }
  toggle(open) { this.open = open; return this.listeners.get("toggle")?.(); }
}

function render(fetch = () => { throw new Error("Unexpected certificate request"); }, hash = "", enabled = true) {
  const nodes = new Map(["certificate-help", "certificate-note", "certificate-status", "certificate-fields", "certificate-technical-details", "certificate-technical-fields"].map((id) => [id, new Node("div")]));
  if (!enabled) nodes.delete("certificate-help");
  const timers = new Map();
  const listeners = new Map();
  const window = {
    location: { hash },
    addEventListener(type, callback) { listeners.set(type, callback); },
    setTimeout(callback, delay) { const timer = {}; timers.set(timer, { callback, delay }); return timer; },
    clearTimeout(timer) { timers.delete(timer); }
  };
  const context = { $: (id) => nodes.get(id), element: (...args) => new Node(...args), URL, AbortController, fetch, window };
  vm.runInNewContext(script + '\ninitCertificateDetails(new URL("https://echo.test/prefix/ui/"));', context);
  return { nodes, timers, renderCertificate: context.renderCertificate, navigate(hash) { window.location.hash = hash; listeners.get("hashchange")?.(); } };
}

test("certificate insights belong to request connection security, without claiming HTTP used TLS", () => {
  const security = html.match(/<article id="connection-security"[\s\S]*?<\/article>/)?.[0];
  assert.ok(security);
  assert.match(security, /This request used HTTP\. Visit HTTPS to use TLS/);
  assert.match(security, /\{\{if \.Certificates\}\}/);
  assert.match(security, /id="certificate-help"/);
  assert.match(security, /Certificate from the app's TLS listeners\. Dates are UTC\./);
  assert.doesNotMatch(connect, /id="certificate-(?:help|fields|status)"/);
  assert.match(visit, /import \{ initCertificateDetails \} from "\.\/certificate\.js"/);
  assert.match(visit, /initCertificateDetails\(uiBase\)/);
});

test("certificate insights show certificate details without a client setup box", () => {
  const security = html.match(/<article id="connection-security"[\s\S]*?<\/article>/)?.[0];
  assert.ok(security);
  assert.match(security, /id="certificate-fields"/);
  assert.match(security, /<summary>More certificate details<\/summary>/);
  assert.match(security, /id="certificate-technical-fields"/);
  assert.doesNotMatch(security, /certificate-trust-help|Custom CA &amp; client certificates|--cacert|--ca-certificate|-cacert/);
});

test("certificate deep links open the request details and instances without TLS remain usable", () => {
  const linked = render(undefined, "#certificate-help");
  assert.equal(linked.nodes.get("certificate-help").open, true);
  const plain = render();
  assert.equal(plain.nodes.get("certificate-help").open, false);
  plain.navigate("#certificate-help");
  assert.equal(plain.nodes.get("certificate-help").open, true);
  assert.doesNotThrow(() => render(undefined, "#certificate-help", false));
});

const certificate = {
  subject: "CN=echo.test", issuer: "CN=Example CA", dns_names: ["echo.test", "www.echo.test"], ip_addresses: ["127.0.0.1", "::1"],
  not_before: "2026-10-01T09:30:00+02:00", not_after: "2027-01-01T07:30:00Z",
  sha256_fingerprint: "AA:BB:CC", serial_number: "123ABC", public_key_algorithm: "ECDSA P-256", signature_algorithm: "ECDSA-SHA256", self_signed: false
};

function fieldValues(nodes, id = "certificate-fields") {
  return Object.fromEntries(nodes.get(id).children.map((row) => row.children.map((node) => node.textContent)));
}

test("certificate details render actual public fields as text and normalize dates to UTC", () => {
  const { nodes, renderCertificate } = render();
  const subject = '<img src=x onerror="alert(1)">';
  renderCertificate({ mode: "file", certificate: { ...certificate, subject } });
  assert.deepEqual(fieldValues(nodes), {
    Subject: subject, Issuer: "CN=Example CA", "DNS names": "echo.test, www.echo.test", "IP addresses": "127.0.0.1, ::1",
    "Valid from": "2026-10-01 07:30:00 UTC", "Valid until": "2027-01-01 07:30:00 UTC", "SHA-256 fingerprint": "AA:BB:CC"
  });
  assert.deepEqual(fieldValues(nodes, "certificate-technical-fields"), { "Serial number": "123ABC", "Public key": "ECDSA P-256", Signature: "ECDSA-SHA256" });
  assert.equal(nodes.get("certificate-fields").hidden, false);
  assert.equal(nodes.get("certificate-technical-details").hidden, false);
  for (const row of nodes.get("certificate-fields").children) assert.equal(row.children[1].children.length, 0);
  renderCertificate({ mode: "file", certificate: { ...certificate, dns_names: null, ip_addresses: [], not_before: "invalid" } });
  assert.equal(fieldValues(nodes)["DNS names"], "Not specified");
  assert.equal(fieldValues(nodes)["IP addresses"], "Not specified");
  assert.equal(fieldValues(nodes)["Valid from"], "Unavailable");
  assert.match(html, /Certificate from the app's TLS listeners\. Dates are UTC\./);
  assert.match(html, /<details id="certificate-help" class="connection-help certificate-insights">/);
});

test("certificate notes follow mode and self-signature without implying file certificates are trusted", () => {
  for (const [mode, self_signed, note] of [
    ["demo", true, /self-signed/], ["file", true, /self-signed/],
    ["acme", false, /Let's Encrypt.*renewed automatically.*normally trust/],
    ["acme-staging", false, /do not trust this testing CA/],
    ["file", false, /trust depends on its issuer/]
  ]) {
    const { nodes, renderCertificate } = render();
    renderCertificate({ mode, certificate: { ...certificate, self_signed } });
    assert.match(nodes.get("certificate-note").textContent, note);
  }
});

test("certificate loading is lazy, bounded and limited to the same deployment", async () => {
  const requests = [];
  const { nodes, timers } = render( async (url, options) => {
    requests.push({ url, options });
    return { ok: true, json: async () => ({ mode: "acme", certificate }) };
  });
  assert.equal(requests.length, 0);
  assert.equal(nodes.get("certificate-help").open, false);
  const pending = nodes.get("certificate-help").toggle(true);
  assert.equal(timers.size, 1);
  assert.equal([...timers.values()][0].delay, 5000);
  await pending;
  assert.equal(timers.size, 0);
  assert.equal(requests[0].url.href, "https://echo.test/prefix/ui/certificate.json");
  assert.equal(requests[0].options.credentials, "same-origin");
  assert.equal(requests[0].options.cache, "no-store");
  assert.equal(requests[0].options.redirect, "error");
  assert.ok(requests[0].options.signal instanceof AbortSignal);
  assert.equal(fieldValues(nodes).Subject, certificate.subject);
  assert.equal(nodes.get("certificate-status").textContent, "Certificate loaded.");
});

test("reopening refreshes rotated certificates and stale requests cannot replace them", async () => {
  const requests = [];
  const { nodes } = render( (url, options) => new Promise((resolve) => requests.push({ resolve, signal: options.signal })));
  const help = nodes.get("certificate-help");
  const first = help.toggle(true);
  await help.toggle(false);
  assert.equal(requests[0].signal.aborted, true);
  const second = help.toggle(true);
  requests[1].resolve({ ok: true, json: async () => ({ mode: "file", certificate: { ...certificate, subject: "CN=renewed.test" } }) });
  await second;
  requests[0].resolve({ ok: true, json: async () => ({ mode: "file", certificate }) });
  await first;
  assert.equal(fieldValues(nodes).Subject, "CN=renewed.test");
});

test("pending certificates, malformed replies and fetch failures stay local to certificate help", async () => {
  for (const response of [
    { ok: false }, { ok: true, json: async () => { throw new Error("Invalid JSON"); } },
    { ok: true, json: async () => ({ mode: "file", certificate: { ...certificate, issuer: {} } }) },
    { ok: true, json: async () => ({ mode: "unexpected", certificate }) }
  ]) {
    const { nodes, timers } = render( async () => response);
    await nodes.get("certificate-help").toggle(true);
    assert.match(nodes.get("certificate-status").textContent, /unavailable.*Reopen/);
    assert.equal(nodes.get("certificate-fields").hidden, true);
    assert.equal(timers.size, 0);
  }
  const { nodes } = render( async () => ({ ok: true, json: async () => ({ mode: "acme", certificate: null }) }));
  await nodes.get("certificate-help").toggle(true);
  assert.equal(nodes.get("certificate-status").textContent, "No certificate is available yet. Reopen after a TLS connection loads it.");
  assert.equal(nodes.get("certificate-fields").hidden, true);
  assert.equal(nodes.get("certificate-technical-details").hidden, true);
});

test("certificate request timeout aborts and leaves a retryable status", async () => {
  const { nodes, timers } = render( (url, { signal }) => new Promise((resolve, reject) => {
    signal.addEventListener("abort", () => reject(new Error("Aborted")));
  }));
  const pending = nodes.get("certificate-help").toggle(true);
  [...timers.values()][0].callback();
  await pending;
  assert.match(nodes.get("certificate-status").textContent, /unavailable.*Reopen/);
  assert.equal(timers.size, 0);
});
