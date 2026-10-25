import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import test from "node:test";

// Load the embedded browser sources directly without introducing a Node package.
async function loadAsset(name) {
  const source = await readFile(new URL(`../internal/ui/assets/${name}`, import.meta.url));
  return import(`data:text/javascript;base64,${source.toString("base64")}`);
}

const { baseURLs, requestURL, headersFromRows, curlCommand, shellQuote, checkService, legacyWorkbenchURL } = await loadAsset("common.js");
const { presets, getPreset } = await loadAsset("presets.js");
const { nextOptionIndex } = await loadAsset("controls.js");

test("service indicator follows successful, failed and recovered listener checks", async (t) => {
  const settings = { http_port: "8080", max_request_size: 1048576 };
  const label = { textContent: "Checking" };
  const indicator = { dataset: {}, querySelector: () => label, title: "" };
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url.href, "https://echo.example/ui/config.json");
    assert.equal(options.cache, "no-store");
    return Response.json(settings);
  });
  const previousWindow = globalThis.window;
  globalThis.window = { setTimeout, clearTimeout };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
  });
  const base = new URL("https://echo.example/ui/");
  assert.deepEqual(await checkService(base, indicator), settings);
  assert.equal(indicator.dataset.state, "ready");
  assert.equal(label.textContent, "Responding");
  assert.match(indicator.title, /Other protocol listeners may differ/);
  globalThis.fetch.mock.mockImplementation(async () => { throw new Error("offline"); });
  await assert.rejects(checkService(base, indicator), /offline/);
  assert.equal(indicator.dataset.state, "unavailable");
  assert.equal(label.textContent, "No response");
  globalThis.fetch.mock.mockImplementation(async () => Response.json({}));
  await assert.rejects(checkService(base, indicator), /Invalid listener settings/);
  assert.equal(indicator.dataset.state, "unavailable");
  globalThis.fetch.mock.mockImplementation(async () => Response.json(settings));
  await checkService(base, indicator);
  assert.equal(indicator.dataset.state, "ready");
  assert.equal(label.textContent, "Responding");
});

test("all embedded browser modules parse as native ES modules", async () => {
  for (const name of ["app.js", "visit.js", "certificate.js", "common.js", "headers.js", "presets.js", "guide.js", "controls.js", "highlight.js", "terminal.js", "privacy.js"]) {
    const source = await readFile(new URL(`../internal/ui/assets/${name}`, import.meta.url));
    const result = spawnSync(process.execPath, ["--input-type=module", "--check"], { input: source, encoding: "utf8" });
    assert.equal(result.status, 0, `${name}: ${result.stderr}`);
  }
});

const methods = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"].map((label) => ({ label, disabled: false }));

test("custom selects support bounded arrow, Home, and End navigation", () => {
  assert.equal(nextOptionIndex(methods, 0, "ArrowDown"), 1);
  assert.equal(nextOptionIndex(methods, 3, "ArrowUp"), 2);
  assert.equal(nextOptionIndex(methods, 0, "ArrowUp"), 0);
  assert.equal(nextOptionIndex(methods, 6, "ArrowDown"), 6);
  assert.equal(nextOptionIndex(methods, 4, "Home"), 0);
  assert.equal(nextOptionIndex(methods, 2, "End"), 6);
});

test("custom selects search and cycle matching labels without changing the option data", () => {
  assert.equal(nextOptionIndex(methods, 0, "p", "p"), 1);
  assert.equal(nextOptionIndex(methods, 1, "p", "p"), 2);
  assert.equal(nextOptionIndex(methods, 2, "p", "p"), 3);
  assert.equal(nextOptionIndex(methods, 3, "p", "p"), 1);
  assert.equal(nextOptionIndex(methods, 1, "u", "pu"), 2);
  assert.equal(nextOptionIndex(methods, 1, "z", "z"), 1);
  assert.equal(methods[1].label, "POST");
});

test("custom selects skip disabled options and handle an empty selectable set", () => {
  const options = [{ label: "One", disabled: true }, { label: "Two", disabled: false }, { label: "Three", disabled: true }, { label: "Four", disabled: false }];
  assert.equal(nextOptionIndex(options, 1, "ArrowDown"), 3);
  assert.equal(nextOptionIndex(options, 3, "ArrowUp"), 1);
  assert.equal(nextOptionIndex(options, 3, "Home"), 1);
  assert.equal(nextOptionIndex(options, 1, "t", "three"), 1);
  assert.equal(nextOptionIndex([], -1, "Home"), -1);
  assert.equal(nextOptionIndex([{ label: "Off", disabled: true }], 0, "End"), -1);
});

function shellArguments(command) {
  const result = spawnSync("/bin/sh", ["-c", `curl() { printf '%s\\0' "$@"; }; ${command}`], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.split("\0").slice(0, -1);
}

test("module URLs preserve the deployment prefix on every UI page", () => {
  for (const module of ["app.js", "visit.js", "guide.js", "common.js"]) {
    const { uiBase, serviceBase } = baseURLs(`https://demo.example/echo/ui/${module}`);
    assert.equal(uiBase.href, "https://demo.example/echo/ui/");
    assert.equal(serviceBase.href, "https://demo.example/echo/");
  }
  assert.equal(baseURLs("http://localhost:8080/ui/app.js").serviceBase.href, "http://localhost:8080/");
});

test("legacy workbench links preserve query parameters within the same deployment", () => {
  for (const origin of ["https://demo.example", "http://localhost:18880", "http://[::1]:18880"]) {
    for (const prefix of ["/ui/", "/echo/ui/"]) {
      const base = new URL(prefix, origin);
      for (const suffix of ["#workbench", "?example=json-post", "?example=one&example=two#workbench", "?example=https%3A%2F%2Fother.example&return=%2Foutside#section"]) {
        const source = new URL(base.href + suffix);
        const target = legacyWorkbenchURL(source, base);
        assert.ok(target instanceof URL);
        assert.equal(target.origin, base.origin);
        assert.equal(target.pathname, prefix + "workbench/");
        assert.equal(target.search, source.search);
        assert.equal(target.hash, "");
      }
    }
  }
});

test("ordinary requests and other pages are not redirected to the workbench", () => {
  const base = new URL("https://demo.example/echo/ui/");
  for (const address of [
    base.href, base.href + "?visit=one", base.href + "#connection-stack", base.href + "#workbench-other",
    base.href + "examples/?example=json-post", base.href + "workbench/?example=json-post",
    "https://other.example/echo/ui/?example=json-post", "https://demo.example/ui/#workbench"
  ]) assert.equal(legacyWorkbenchURL(new URL(address), base), null, address);
});

test("request paths stay on this service and retain encoded data", () => {
  const base = new URL("https://demo.example/echo/");
  assert.equal(requestURL(" /inspect?item=42&label=a%23b ", base).href, "https://demo.example/echo/inspect?item=42&label=a%23b");
  assert.equal(requestURL("/", base).href, base.href);
  for (const path of ["https://other.example/", "//other.example/", "inspect", "/../outside", "/%2e%2e/outside", "/%2e%2e%2foutside", "/\\other", "/%5coutside", "/invalid%ZZ", "/inspect#fragment"]) {
    assert.throws(() => requestURL(path, base), Error, path);
  }
});

test("header rows validate names and string values", () => {
  assert.deepEqual([...headersFromRows([])], []);
  const headers = headersFromRows([{ name: "Content-Type", value: "application/json" }, { name: "X-Example", value: "synthetic" }]);
  assert.equal(headers.get("content-type"), "application/json");
  assert.equal(headers.get("x-example"), "synthetic");
  for (const row of [{ name: "X-Count", value: 1 }, { name: "Bad Header", value: "value" }, { name: "X-Test", value: "line\nnext" }]) {
    assert.throws(() => headersFromRows([row]), Error);
  }
});

test("copied curl commands preserve payload and header quoting", () => {
  const body = '{"message":"O\'Reilly; $(printf injected) `printf injected`"}';
  const request = {
    url: new URL("https://demo.example/echo/inspect?name=O%27Reilly&sample=true"),
    method: "POST",
    headers: headersFromRows([{ name: "Content-Type", value: "application/json" }, { name: "X-Example", value: "O'Reilly" }, { name: "X-Empty", value: "" }]),
    body
  };
  assert.deepEqual(shellArguments(curlCommand(request)), [
    "--globoff", "--silent", "--show-error", "--max-time", "15", "--request", "POST",
    "--header", "content-type: application/json", "--header", "x-empty;", "--header", "x-example: O'Reilly",
    "--data-raw", body, request.url.href
  ]);
  assert.equal(shellQuote("a'b"), "'a'\\''b'");
});

test("copied curl treats bracket and brace queries as a single literal URL", () => {
  const url = new URL("https://demo.example/echo/inspect?item={a,b}&filter[status]=ok");
  const args = shellArguments(curlCommand({ url, method: "GET", headers: new Headers() }));
  assert.ok(args.includes("--globoff"));
  assert.equal(args.at(-1), url.href);
});

test("readable multiline curl keeps exact shell arguments and payload newlines", () => {
  const body = "first line\nO'Reilly\t$(printf injected) `printf injected`";
  const url = new URL("https://demo.example/echo/inspect?item={a,b}&filter[status]=ok");
  const command = curlCommand({ url, method: "POST", headers: new Headers(), body });
  assert.ok(command.includes(" \\\n  "));
  assert.ok(command.startsWith("curl "));
  const args = shellArguments(command);
  assert.equal(args[args.indexOf("--data-raw") + 1], body);
  assert.equal(args[args.indexOf("--request") + 1], "POST");
  assert.equal(args.at(-1), url.href);
});

test("public HTTPS commands verify TLS and HEAD uses curl's head mode", () => {
  for (const method of ["GET", "HEAD"]) {
    const args = shellArguments(curlCommand({ url: new URL("https://demo.example/echo/"), method, headers: new Headers() }));
    assert.ok(!args.includes("--insecure") && !args.includes("-k"));
    assert.ok(!args.includes("--data-raw"));
    if (method === "HEAD") {
      assert.ok(args.includes("--head"));
      assert.ok(!args.includes("--request"));
    } else {
      assert.equal(args[args.indexOf("--request") + 1], "GET");
    }
    assert.equal(args.at(-1), "https://demo.example/echo/");
  }
});

test("example IDs form a fixed immutable allowlist", () => {
  assert.deepEqual(presets.map(({ id }) => id), ["inspect", "json-post", "forwarded", "websocket", "status-418", "delay-250ms", "size-1024"]);
  assert.ok(Object.isFrozen(presets));
  for (const preset of presets) {
    assert.ok(Object.isFrozen(preset));
    assert.ok(Object.isFrozen(preset.headers));
    if (preset.controls) assert.ok(Object.isFrozen(preset.controls));
  }
  for (const id of ["", "constructor", "__proto__", "https://other.example/"]) assert.equal(getPreset(id, {}), undefined);
  assert.equal(getPreset("inspect", {}).method, "GET");
  assert.deepEqual(JSON.parse(getPreset("json-post", {}).body), { message: "Hello from Echo Observatory", sample: true });
  assert.equal(getPreset("websocket", {}).kind, "websocket");
});

test("optional examples require an explicitly enabled instance feature", () => {
  for (const [id, flag] of [["forwarded", "print_headers"], ["status-418", "response_controls"], ["delay-250ms", "response_controls"], ["size-1024", "response_controls"]]) {
    for (const settings of [undefined, {}, { [flag]: false }, { [flag]: "true" }, Object.create({ [flag]: true })]) {
      assert.equal(getPreset(id, settings), undefined);
    }
    assert.equal(getPreset(id, { [flag]: true }).id, id);
  }
});
