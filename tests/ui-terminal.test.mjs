import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import test from "node:test";

let source = await readFile(new URL("../internal/ui/assets/terminal.js", import.meta.url), "utf8");
for (const name of ["common", "presets"]) {
  const dependency = await readFile(new URL(`../internal/ui/assets/${name}.js`, import.meta.url));
  source = source.replace(`"./${name}.js"`, `"data:text/javascript;base64,${dependency.toString("base64")}"`);
}
const { wgetCommand, httpExamples, directCommands } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

function argumentsFor(command, tool) {
  const result = spawnSync("/bin/sh", ["-c", `${tool}() { printf '%s\\0' "$@"; }; ${command}`], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.split("\0").slice(0, -1);
}

test("wget preserves quotes, JSON bodies, headers, and literal query strings", () => {
  const body = '{"message":"O\'Reilly; $(printf unsafe) `printf unsafe`"}';
  const url = new URL("https://demo.example/echo/inspect?filter[status]={ok}&quote=O%27Reilly");
  const command = wgetCommand({ url, method: "POST", headers: new Headers({ "Content-Type": "application/json", "X-Sample": "O'Reilly" }), body });
  const args = argumentsFor(command, "wget");
  assert.equal(args[args.indexOf("--body-data") + 1], body);
  assert.ok(args.includes("x-sample: O'Reilly"));
  assert.ok(args.includes("--content-on-error"));
  assert.ok(args.includes("--max-redirect=0"));
  assert.equal(args.at(-1), url.href);
  assert.ok(!args.includes("+"));
  assert.ok(!command.includes("--no-check-certificate"));
});

test("terminal recipes retain the deployment prefix and feature gates", () => {
  const base = new URL("https://demo.example/echo/");
  assert.deepEqual(httpExamples(base).map((example) => example.id), ["get", "post", "head", "ip-family"]);
  const examples = httpExamples(base, { print_headers: true, response_controls: true });
  assert.deepEqual(examples.map((example) => example.id), ["get", "post", "head", "ip-family", "forwarded", "status-418", "delay-250ms", "size-1024"]);
  for (const example of examples) for (const { label, command } of example.commands) {
    const args = argumentsFor(command, label.startsWith("curl") ? "curl" : "wget");
    assert.ok(!args.includes("+"));
    assert.ok(args.at(-1).startsWith(base.href));
    assert.equal(new URL(args.at(-1)).pathname, base.pathname, "basic recipes use the echo root, without implying a dedicated /inspect endpoint");
    assert.ok(!args.includes("--insecure") && !args.includes("--no-check-certificate"));
    if (example.id === "head") assert.ok(args.includes(label === "curl" ? "--head" : "--server-response"));
    if (example.id === "status-418") assert.equal(new URL(args.at(-1)).searchParams.get("status"), "418");
    if (example.id === "delay-250ms") assert.equal(new URL(args.at(-1)).searchParams.get("delay"), "250ms");
    if (example.id === "size-1024") {
      assert.equal(new URL(args.at(-1)).searchParams.get("size"), "1024");
      assert.ok(args.includes(label === "curl" ? "/dev/null" : "--output-document=/dev/null"));
    }
  }
  assert.deepEqual(httpExamples(base, { print_headers: "true", response_controls: "true" }).map((example) => example.id), ["get", "post", "head", "ip-family"]);
});

const listeners = { http_port: "18880", tls_port: "18843", tcp_port: "19990", grpc_port: "15051", quic_port: "18843", h2c: true, tls: true, tcp: true, grpc: true, quic: true };

test("native commands use the current hostname and each advertised listener port", () => {
  for (const hostname of ["localhost", "127.0.0.1"]) {
    const base = new URL(`http://${hostname}:18880/echo/`);
    for (const [name, scheme, port] of [["HTTP", "http", "18880"], ["HTTPS", "https", "18843"], ["HTTP/3", "https", "18843"]]) {
      for (const { label, command } of directCommands(name, listeners, base)) {
        const args = argumentsFor(command, label.startsWith("curl") ? "curl" : "wget");
        assert.equal(args.at(-1), `${scheme}://${hostname}:${port}/`);
        assert.doesNotMatch(command, /\bHOST\b|--insecure|--no-check-certificate/);
      }
    }
    const tcp = argumentsFor(directCommands("TCP", listeners, base)[0].command, "nc");
    assert.deepEqual(tcp, ["-w", "5", "--", hostname, "19990"]);
    const grpc = argumentsFor(directCommands("gRPC", listeners, base)[0].command, "grpcurl");
    assert.ok(grpc.includes("-plaintext"));
    assert.deepEqual(grpc.slice(-3), ["--", hostname + ":15051", "echo.EchoService/Echo"]);
  }
  const base = new URL("http://localhost:18880/");
  assert.ok(directCommands("HTTP", listeners, base).some(({ command }) => command.includes("--http2-prior-knowledge")));
  assert.ok(directCommands("HTTP/3", listeners, base)[0].command.includes("--http3-only"));
  assert.ok(!directCommands("gRPC", { ...listeners, grpc_tls: true }, base)[0].command.includes("-plaintext"));
});

test("native commands use configured public ports without copying URL credentials or prefixes", () => {
  const settings = { ...listeners, http_port: "80", tls_port: "443", tcp_port: "9090", grpc_port: "50051", quic_port: "443", grpc_tls: true };
  const base = new URL("https://user:secret@echo.container.education/echo/?token=secret#example");
  for (const name of ["HTTP", "HTTPS", "HTTP/3"]) for (const { label, command } of directCommands(name, settings, base)) {
    const args = argumentsFor(command, label.startsWith("curl") ? "curl" : "wget");
    assert.equal(args.at(-1), `${name === "HTTP" ? "http" : "https"}://echo.container.education/`);
    assert.doesNotMatch(command, /user|secret|token|example|8443|8080|4433/);
  }
  assert.deepEqual(argumentsFor(directCommands("TCP", settings, base)[0].command, "nc").slice(-2), ["echo.container.education", "9090"]);
  assert.deepEqual(argumentsFor(directCommands("gRPC", settings, base)[0].command, "grpcurl").slice(-2), ["echo.container.education:50051", "echo.EchoService/Echo"]);
});

test("native IPv6 commands bracket authorities but pass a bare address to netcat", () => {
  const base = new URL("http://[::1]:18880/echo/");
  for (const [name, scheme] of [["HTTP", "http"], ["HTTPS", "https"], ["HTTP/3", "https"]]) for (const { label, command } of directCommands(name, listeners, base)) {
    const args = argumentsFor(command, label.startsWith("curl") ? "curl" : "wget");
    assert.equal(args.at(-1), `${scheme}://[::1]:${name === "HTTP" ? "18880" : "18843"}/`);
  }
  assert.deepEqual(argumentsFor(directCommands("TCP", listeners, base)[0].command, "nc").slice(-2), ["::1", "19990"]);
  assert.deepEqual(argumentsFor(directCommands("gRPC", listeners, base)[0].command, "grpcurl").slice(-2), ["[::1]:15051", "echo.EchoService/Echo"]);
});

test("native commands hide disabled listeners, invalid addresses and ports, and private admin endpoints", () => {
  const base = new URL("http://localhost:18880/");
  for (const [name, flag] of [["HTTPS", "tls"], ["TCP", "tcp"], ["gRPC", "grpc"], ["HTTP/3", "quic"]]) {
    for (const value of [false, undefined, "true"]) assert.deepEqual(directCommands(name, { ...listeners, [flag]: value }, base), []);
  }
  for (const [name, key] of [["HTTP", "http_port"], ["HTTPS", "tls_port"], ["TCP", "tcp_port"], ["gRPC", "grpc_port"], ["HTTP/3", "quic_port"]]) {
    for (const value of [undefined, null, 8080, "0", "65536", "8080;printf unsafe", "443/path", "08080"]) assert.deepEqual(directCommands(name, { ...listeners, [key]: value }, base), []);
    for (const address of [undefined, "not a URL", "file:///tmp/service", "javascript:alert(1)"]) assert.deepEqual(directCommands(name, listeners, address), []);
  }
  for (const name of ["Liveness", "Readiness", "Prometheus metrics"]) assert.deepEqual(directCommands(name, { ...listeners, metrics: true, metrics_port: "3000" }, base), []);
});

test("native hostname operands stay shell-quoted and cannot become client options", () => {
  for (const hostname of ["o'reilly.example", "--help", "$(printf-unsafe).example"]) {
    const base = new URL("https://" + hostname + "/");
    for (const [name, tool] of [["TCP", "nc"], ["gRPC", "grpcurl"]]) {
      const command = directCommands(name, listeners, base)[0].command;
      const args = argumentsFor(command, tool);
      assert.equal(args[args.indexOf("--") + 1], name === "TCP" ? hostname : hostname + ":15051");
    }
  }
});

test("address-family examples preserve IPv6 literals and do not promise a second family for them", () => {
  for (const literal of ["https://[::1]:8443/echo/", "http://127.0.0.1:8080/"]) assert.ok(!httpExamples(new URL(literal)).some((example) => example.id === "ip-family"));
  const example = httpExamples(new URL("https://echo.example/echo/")).find((example) => example.id === "ip-family");
  assert.equal(example.commands.length,2);
  for (const [index,{command}] of example.commands.entries()) {
    const args = argumentsFor(command,"curl");
    assert.ok(args.includes(index === 0 ? "-4" : "-6"));
    assert.equal(args.at(-1),"https://echo.example/echo/");
  }
  for (const {command,label} of httpExamples(new URL("http://[::1]:8080/echo/")).find((example)=>example.id==="get").commands) assert.equal(argumentsFor(command,label).at(-1),"http://[::1]:8080/echo/");
});
