export const $ = (id) => document.getElementById(id);
export const previewLimit = 1048576;
const copyStates = new WeakMap();
let latestCopy = 0;

export function baseURLs(moduleURL) {
  const uiBase = new URL("./", moduleURL);
  return { uiBase, serviceBase: new URL("../", uiBase) };
}

export function legacyWorkbenchURL(pageURL, uiBase) {
  const page = new URL(pageURL);
  const home = new URL(uiBase);
  if (page.origin !== home.origin || page.pathname !== home.pathname) return null;
  if (!page.searchParams.has("example") && page.hash !== "#workbench") return null;
  const target = new URL("workbench/", uiBase);
  target.search = page.search;
  return target;
}

export function requestURL(path, serviceBase) {
  path = path.trim();
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) throw new Error("Enter a path beginning with one slash, such as /?item=42.");
  const url = new URL("." + path, serviceBase);
  let decoded;
  try { decoded = decodeURIComponent(url.pathname); } catch { throw new Error("The path contains invalid percent encoding."); }
  if (url.origin !== serviceBase.origin || !url.pathname.startsWith(serviceBase.pathname) || decoded.includes("\\") || decoded.split("/").some((part) => part === "." || part === "..") || url.hash) throw new Error("Keep the path inside this service. Encode a literal # as %23.");
  return url;
}

export const maxHeaderRows = 64;
export const maxHeaderCharacters = 65536;

export function headersFromRows(rows) {
  const headers = new Headers();
  let characters = 0;
  for (const [index, row] of rows.entries()) {
    const invalid = (message, part) => {
      const error = new Error(message);
      error.row = index;
      error.part = part;
      throw error;
    };
    if (index >= maxHeaderRows) invalid("Use at most " + maxHeaderRows + " header rows.", "name");
    if (typeof row.name !== "string" || typeof row.value !== "string") invalid("Header names and values must be text.", typeof row.name !== "string" ? "name" : "value");
    characters += row.name.length + row.value.length;
    if (characters > maxHeaderCharacters) invalid("Headers exceed the 65,536 character limit.", "value");
    if (/[\r\n]/.test(row.name)) invalid("Header names must not contain line breaks.", "name");
    if (/[\r\n]/.test(row.value)) invalid("Header values must not contain line breaks.", "value");
    const name = row.name.trim();
    if (!name && !row.value) continue;
    if (!name) invalid("Enter a header name for this value.", "name");
    try { headers.append(name, row.value); }
    catch (error) {
      let part = "value";
      try { new Headers([[name, ""]]); } catch { part = "name"; }
      invalid("Invalid header " + part + ": " + error.message, part);
    }
  }
  return headers;
}

export function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

export function shellQuote(value) {
  return "'" + String(value).replaceAll("'", "'\\''") + "'";
}

function copyState(button) {
  if (!copyStates.has(button)) copyStates.set(button, {
    version: 0, timer: null,
    label: button.querySelector("[data-copy-label]").textContent || "Copy",
    ariaLabel: button.getAttribute("aria-label") || "Copy"
  });
  return copyStates.get(button);
}

function showCopyState(button, state, phase, message = "") {
  button.dataset.copyState = phase;
  button.querySelector("[data-copy-label]").textContent = { idle: state.label, copying: "Copying", copied: "Copied", error: "Retry" }[phase];
  const context = state.ariaLabel.replace(/^Copy\s*/i, "");
  const ariaLabel = phase === "idle" ? state.ariaLabel : { copying: "Copying ", copied: "Copied ", error: "Retry copying " }[phase] + context;
  button.setAttribute("aria-label", ariaLabel);
  button.setAttribute("title", ariaLabel);
  const status = button.closest("[data-code-block]")?.querySelector("[data-copy-status]");
  if (status) status.textContent = message;
}

export function setCopyLabel(button, label, ariaLabel) {
  const state = copyState(button);
  state.version++;
  window.clearTimeout(state.timer);
  state.label = label;
  if (ariaLabel !== undefined) state.ariaLabel = ariaLabel;
  showCopyState(button, state, "idle");
}

function legacyCopy(text, doc) {
  const focused = doc.activeElement;
  const selection = doc.getSelection?.();
  const ranges = [];
  for (let index = 0; index < (selection?.rangeCount || 0); index++) ranges.push(selection.getRangeAt(index).cloneRange());
  const inputSelection = focused && typeof focused.selectionStart === "number" ? [focused.selectionStart, focused.selectionEnd, focused.selectionDirection] : null;
  const field = doc.createElement("textarea");
  field.className = "copy-buffer";
  field.value = text;
  field.readOnly = true;
  field.tabIndex = -1;
  field.setAttribute("aria-hidden", "true");
  try {
    doc.body.append(field);
    field.focus({ preventScroll: true });
    field.select();
    return doc.execCommand("copy") === true;
  } catch {
    return false;
  } finally {
    field.remove();
    try {
      focused?.focus({ preventScroll: true });
      if (selection) {
        selection.removeAllRanges();
        for (const range of ranges) selection.addRange(range);
      }
      if (inputSelection) focused.setSelectionRange(...inputSelection);
    } catch { /* The original focused content may have been replaced. */ }
  }
}

export async function copy(text, button) {
  const state = copyState(button);
  const version = ++state.version;
  const sequence = ++latestCopy;
  const current = () => state.version === version;
  window.clearTimeout(state.timer);
  showCopyState(button, state, "copying", "Copying to clipboard…");
  let copied = false;
  if (window.isSecureContext && typeof globalThis.navigator?.clipboard?.writeText === "function") {
    try { await navigator.clipboard.writeText(text); copied = true; }
    catch { /* Try the user-initiated textarea fallback after rejection. */ }
  }
  if (!current()) return false;
  if (!copied && sequence !== latestCopy) {
    showCopyState(button, state, "idle");
    return false;
  }
  if (!copied) copied = legacyCopy(text, button.ownerDocument || document);
  if (!current()) return false;
  if (!copied) {
    showCopyState(button, state, "error", "Copy is unavailable. Select the displayed code and copy it manually, or try again.");
    return false;
  }
  showCopyState(button, state, "copied", "Copied to clipboard.");
  state.timer = window.setTimeout(() => { if (current()) showCopyState(button, state, "idle"); }, 2000);
  return true;
}

export function curlCommand(request) {
  const groups = ["curl --globoff --silent --show-error --max-time 15"];
  if (request.method === "HEAD") groups.push("--head");
  else groups.push("--request " + shellQuote(request.method));
  for (const [name, value] of request.headers) groups.push("--header " + shellQuote(value === "" ? name + ";" : name + ": " + value));
  if (request.body !== undefined) groups.push("--data-raw " + shellQuote(request.body));
  groups.push(shellQuote(request.url.href));
  return groups.join(" \\\n  ");
}

export function initTheme() {
  const toggle = $("theme-toggle");
  const system = window.matchMedia("(prefers-color-scheme: dark)");
  let choice = "system";
  try {
    const saved = sessionStorage.getItem("echo-observatory-theme");
    if (["light", "dark"].includes(saved)) choice = saved;
  } catch { /* Follow the system when storage is unavailable. */ }
  function render() {
    const dark = choice === "dark" || (choice === "system" && system.matches);
    document.documentElement.dataset.theme = choice;
    toggle.setAttribute("aria-checked", String(dark));
    toggle.title = dark ? "Switch to light theme" : "Switch to dark theme";
    toggle.hidden = false;
  }
  render();
  toggle.addEventListener("click", () => {
    choice = toggle.getAttribute("aria-checked") === "true" ? "light" : "dark";
    render();
    try { sessionStorage.setItem("echo-observatory-theme", choice); } catch { /* The toggle still works without storage. */ }
  });
  system.addEventListener("change", render);
}

export async function loadConfig(uiBase) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(new URL("config.json", uiBase), { credentials: "same-origin", cache: "no-store", redirect: "error", signal: controller.signal });
    if (!response.ok) throw new Error("Listener settings unavailable");
    const settings = await response.json();
    if (!settings || typeof settings.http_port !== "string" || !Number.isFinite(settings.max_request_size) || settings.max_request_size <= 0) throw new Error("Invalid listener settings");
    return settings;
  } finally { window.clearTimeout(timer); }
}

export async function checkService(uiBase, indicator) {
  const label = indicator.closest?.(".instance")?.querySelector("[data-service-status-label]") || indicator.querySelector("[data-service-status-label]");
  const describe = (text) => {
    indicator.title = text;
    const description = indicator.closest?.(".service-info")?.querySelector("[data-service-description]");
    if (description) description.textContent = text;
  };
  try {
    const settings = await loadConfig(uiBase);
    indicator.dataset.state = "ready";
    if (label.textContent !== "Responding") label.textContent = "Responding";
    describe("This web listener responded at " + new Date().toLocaleTimeString() + ". Checked every 30 seconds while this page is visible. Other protocol listeners may differ.");
    return settings;
  } catch (error) {
    indicator.dataset.state = "unavailable";
    if (label.textContent !== "No response") label.textContent = "No response";
    describe("The latest web listener check failed. The service or your connection may be unavailable.");
    throw error;
  }
}

export function initServiceStatus(uiBase, serviceBase) {
  $("service-address").textContent = serviceBase.href;
  const indicator = $("service-status");
  const details = indicator.closest(".service-info");
  indicator.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !event.altKey && !event.ctrlKey && !event.metaKey && !event.isComposing && details.open) {
      event.preventDefault();
      details.open = false;
    }
  });
  details.addEventListener("focusout", (event) => {
    if (!details.contains(event.relatedTarget)) details.open = false;
  });
  document.addEventListener("pointerdown", (event) => {
    if (!details.contains(event.target)) details.open = false;
  });
  const refresh = () => { if (document.visibilityState === "visible") checkService(uiBase, indicator).catch(() => {}); };
  window.setInterval(refresh, 30000);
  document.addEventListener("visibilitychange", refresh);
  return checkService(uiBase, indicator);
}
