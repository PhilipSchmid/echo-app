import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../internal/ui/assets/highlight.js", import.meta.url));
const { codeTokens, renderCode } = await import(`data:text/javascript;base64,${source.toString("base64")}`);

test("JSON highlighting distinguishes keys, escaped strings, numbers, literals, and punctuation", () => {
  const text = '{\r\n  "key\\\"": "a\\\\b 🛰", "count": -1.25e+3, "ok": true, "empty": null\r\n}';
  const tokens = codeTokens(text, "json");
  assert.equal(tokens.map((token) => token.text).join(""), text);
  for (const kind of ["key", "string", "number", "literal", "punctuation"]) assert.ok(tokens.some((token) => token.kind === kind), kind);
  assert.equal(tokens.find((token) => token.kind === "key").text, '"key\\\""');
});

test("shell highlighting preserves multiline commands and quoted payloads exactly", () => {
  const text = "curl --silent --max-time 15 \\\n  --data-raw '{\"sample\":\"O'\\''Reilly <script>\"}' \\\n  \"$HOST/path#fragment\" | cat # comment\n";
  const tokens = codeTokens(text, "bash");
  assert.equal(tokens.map((token) => token.text).join(""), text);
  assert.deepEqual(tokens.filter((token) => token.kind === "command").map((token) => token.text), ["curl", "cat"]);
  assert.ok(tokens.some((token) => token.kind === "option" && token.text === "--max-time"));
  assert.ok(tokens.some((token) => token.kind === "number" && token.text === "15"));
  assert.ok(tokens.some((token) => token.kind === "comment" && token.text === "# comment"));
});

test("auto detection highlights JSON only and unsupported or malformed content stays plain", () => {
  assert.ok(codeTokens('{"ok":true}').some((token) => token.kind === "key"));
  for (const [text, language] of [["plain text", "auto"], ['{"unfinished":', "json"], ["<html>sample</html>", "html"], ["", "auto"]]) {
    assert.deepEqual(codeTokens(text, language), [{ text }]);
  }
});

test("large and token-dense previews retain the complete text without excessive DOM nodes", () => {
  for (const text of [JSON.stringify("a".repeat(65536)), JSON.stringify(Array(5000).fill(1))]) assert.deepEqual(codeTokens(text, "json"), [{ text }]);
});

test("highlighted untrusted content creates only text nodes and fixed-class spans", (t) => {
  const original = Object.getOwnPropertyDescriptor(globalThis, "document");
  const node = (tag = "#text", text = "") => ({ tag, textContent: text, children: [], append(child) { this.children.push(child); } });
  Object.defineProperty(globalThis, "document", { configurable: true, value: {
    createDocumentFragment: () => node("#fragment"),
    createTextNode: (text) => node("#text", text),
    createElement: (tag) => { assert.equal(tag, "span"); return node(tag); }
  } });
  t.after(() => { if (original) Object.defineProperty(globalThis, "document", original); else delete globalThis.document; });
  const text = JSON.stringify({ payload: '</pre><img src=x onerror=alert(1)>& "quoted"' }, null, 2);
  let children;
  const target = { replaceChildren(fragment) { children = fragment.children; } };
  renderCode(target, text, "json");
  assert.equal(children.map((child) => child.textContent).join(""), text);
  assert.ok(children.some((child) => child.className === "syntax-key"));
  assert.ok(children.every((child) => child.tag === "#text" || /^syntax-(key|string|number|literal|punctuation)$/.test(child.className)));
  renderCode(target, "plain replacement", "text");
  assert.equal(children.length, 1);
  assert.equal(children[0].textContent, "plain replacement");
});
