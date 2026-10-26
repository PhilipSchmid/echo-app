import { $, baseURLs, previewLimit, element, copy, setCopyLabel, initTheme, initTooltips, initDisclosureHelp, initServiceStatus, requestURL, curlCommand } from "./common.js";
import { getPreset } from "./presets.js";
import { enhanceSelect } from "./controls.js";
import { renderCode } from "./highlight.js";
import { createHeaderEditor } from "./headers.js";

const { uiBase, serviceBase } = baseURLs(import.meta.url);
const destinationHelp = $("request-destination-help");
$("request-destination-address").textContent = serviceBase.href;
initDisclosureHelp(destinationHelp.querySelector("summary"));
let config = null;
let responseCopy = "";
let socket = null;
let socketTimer = null;
let socketClosing = false;
let requestInvalidField = null;
let sentMessages = 0;
let receivedMessages = 0;

function feedback(id, message) {
  if (!message) {
    const field = id === "request-error" ? requestInvalidField : id === "websocket-error" ? $("websocket-message") : null;
    if (field) {
      field.setAttribute("aria-invalid", "false");
      const descriptions = (field.getAttribute("aria-describedby") || "").split(/\s+/).filter((value) => value && value !== id).join(" ");
      if (descriptions) field.setAttribute("aria-describedby", descriptions);
      else field.removeAttribute("aria-describedby");
    }
    if (id === "request-error") requestInvalidField = null;
  }
  $(id).textContent = message;
  $(id).hidden = !message;
}
function invalid(message, field) {
  const error = new Error(message);
  error.field = field;
  throw error;
}
function identifyError(field, id) {
  field.setAttribute("aria-invalid", "true");
  const descriptions = new Set((field.getAttribute("aria-describedby") || "").split(/\s+/).filter(Boolean));
  descriptions.add(id);
  field.setAttribute("aria-describedby", [...descriptions].join(" "));
}
function focusError(error) {
  if (!error.field) return;
  identifyError(error.field, "request-error");
  requestInvalidField = error.field;
  const details = error.field.closest("details");
  if (details) details.open = true;
  error.field.focus();
}
function buildRequest() {
  let url;
  try { url = requestURL($("request-path").value, serviceBase); }
  catch (error) { invalid(error.message, $("request-path")); }
  if (config && config.response_controls) {
    for (const key of ["status", "delay", "size"]) {
      const value = $("control-" + key).value.trim();
      if (value) url.searchParams.set(key, value);
    }
  }
  let headers;
  try { headers = headerEditor.read(); }
  catch (error) { invalid(error.message, error.field); }
  const method = $("method").value;
  const body = method === "GET" || method === "HEAD" ? undefined : $("request-body").value || undefined;
  if (body !== undefined && !headers.has("Content-Type")) headers.set("Content-Type", "text/plain;charset=UTF-8");
  const max = config ? config.max_request_size : 65536;
  if (body && new TextEncoder().encode(body).length > max) invalid("The body exceeds the server's " + max.toLocaleString() + " byte limit.", $("request-body"));
  return { url, method, headers, body };
}

function updateCommand() {
  let command;
  let language = "bash";
  try { command = curlCommand(buildRequest()); }
  catch (error) { command = error.message; language = "text"; }
  if ($("curl-command").textContent !== command) setCopyLabel($("copy-curl"), "Copy", "Copy curl command");
  renderCode($("curl-command"), command, language);
}
function requestEdited() {
  feedback("request-error", "");
  updateCommand();
}
function updateMethod() {
  const noBody = $("method").value === "GET" || $("method").value === "HEAD";
  $("request-body").disabled = noBody;
  $("body-hint").textContent = noBody ? "Choose POST, PUT, PATCH, DELETE, or OPTIONS to send a body." : "The server checks body size and returns request metadata; it does not echo the payload.";
}
async function readPreview(response, textual) {
  if (!response.body) return { text: "", bytes: 0, truncated: false };
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let bytes = 0;
  let text = "";
  let truncated = false;
  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      const count = Math.min(value.length, previewLimit - bytes);
      if (textual) text += decoder.decode(value.subarray(0, count), { stream: true });
      bytes += count;
      if (count < value.length) { truncated = true; await reader.cancel(); break; }
    }
    if (textual) text += decoder.decode();
  } finally { reader.releaseLock(); }
  return { text, bytes, truncated };
}
function addMetadata(list, label, value, key) {
  if (value === undefined || value === null || value === "") return;
  const address = ["source_ip", "http_host", "remote_addr", "local_addr"].includes(key);
  const row = element("div", address ? "metadata-address" : "");
  row.append(element("dt", "", label), element("dd", address ? "address-value" : "", String(value).slice(0, 512)));
  list.append(row);
}
function showResponse(response, result, elapsed, request, textual) {
  let data = null;
  let formatted = result.text;
  if (textual && result.text && !result.truncated) {
    try { data = JSON.parse(result.text); formatted = JSON.stringify(data, null, 2); } catch { /* Non-JSON responses remain plain text. */ }
  }
  const clipped = formatted.length > previewLimit;
  if (clipped) formatted = formatted.slice(0, previewLimit);
  responseCopy = textual ? formatted : "";
  $("copy-response").disabled = !responseCopy;
  setCopyLabel($("copy-response"), "Copy", result.truncated || clipped ? "Copy response preview" : "Copy response");
  $("response-empty").hidden = true;
  $("response-content").hidden = false;
  $("response-badges").hidden = false;
  $("response-status").textContent = response.status + " " + (response.statusText || "HTTP response");
  $("response-status").classList.toggle("error", response.status >= 400);
  $("response-duration").textContent = Math.round(elapsed) + " ms";
  $("response-protocol").textContent = data && data.http_version ? data.http_version : "Protocol not reported";
  $("response-content-type").textContent = response.headers.get("Content-Type")?.split(";")[0] || "No content type";
  $("response-metadata").replaceChildren();
  $("connection-metadata").replaceChildren();
  if (data && typeof data === "object" && !Array.isArray(data)) {
    for (const [label, key] of [["Server hostname", "hostname"], ["Listener", "listener"], [data.socket_peer_ip ? "Client IP" : "Socket peer IP", "source_ip"], [data.socket_peer_ip ? "Client IP family" : "IP family on this hop", "ip_version"], ["Node", "node"]]) addMetadata($("response-metadata"), label, data[key], key);
    for (const [label, key] of [["Host", "http_host"], ["Original URI", "http_request_uri"], [data.socket_peer_ip ? "Client address" : "Remote socket", "remote_addr"], [data.socket_peer_ip ? "Destination address" : "Local socket", "local_addr"], [data.socket_peer_ip ? "Destination IP family" : "Local IP family", "local_ip_version"], ["TLS version", "tls_version"], ["Cipher suite", "tls_cipher_suite"], ["Server name", "tls_server_name"], ["ALPN", "tls_negotiated_protocol"]]) addMetadata($("connection-metadata"), label, data[key], key);
  }
  $("response-metadata").hidden = !$("response-metadata").children.length;
  $("connection-details").hidden = !$("connection-metadata").children.length;
  let note = result.bytes.toLocaleString() + " bytes received";
  if (request.method === "HEAD") note = "HEAD response: headers received, with no response body.";
  else if (!result.bytes) note = "The server returned no response body.";
  if (result.truncated || clipped) note += " Preview limited to 1 MiB; copying includes only the displayed preview.";
  $("response-note").textContent = note;
  renderCode($("response-body"), !textual ? "Binary response · " + result.bytes.toLocaleString() + " bytes" : formatted || "No response body.");
  $("request-status").textContent = "Response received: HTTP " + response.status + ", " + result.bytes.toLocaleString() + " bytes, " + Math.round(elapsed) + " milliseconds.";
}
$("request-form").addEventListener("invalid", (event) => {
  event.preventDefault();
  if (requestInvalidField && requestInvalidField !== event.target) return;
  feedback("request-error", "");
  feedback("request-error", event.target.validationMessage);
  focusError({ field: event.target });
}, true);
$("request-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  if ($("send-request").getAttribute("aria-disabled") === "true") return;
  feedback("request-error", "");
  let request;
  try { request = buildRequest(); } catch (error) { feedback("request-error", error.message); focusError(error); return; }
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 15000);
  $("send-request").setAttribute("aria-disabled", "true");
  $("send-label").textContent = "Sending…";
  $("request-status").textContent = "Sending request.";
  $("response-panel").setAttribute("aria-busy", "true");
  responseCopy = "";
  $("copy-response").disabled = true;
  setCopyLabel($("copy-response"), "Copy", "Copy response");
  $("response-content").hidden = true;
  $("response-badges").hidden = true;
  $("response-empty").hidden = false;
  $("response-empty").querySelector("h3").textContent = "Waiting for a response…";
  $("response-empty").querySelector("p").textContent = "The request will stop after 15 seconds.";
  const start = performance.now();
  try {
    const response = await fetch(request.url, { method: request.method, headers: request.headers, body: request.body, signal: controller.signal, credentials: "same-origin", redirect: "error", cache: "no-store" });
    const contentType = response.headers.get("Content-Type") || "";
    const textual = !contentType.includes("application/octet-stream");
    const result = await readPreview(response, textual);
    showResponse(response, result, performance.now() - start, request, textual);
  } catch (error) {
    $("request-status").textContent = "";
    $("response-empty").querySelector("h3").textContent = "The request did not complete";
    $("response-empty").querySelector("p").textContent = "Check the request error, then try again.";
    feedback("request-error", error.name === "AbortError" ? "The request timed out after 15 seconds. Check the service and try again." : "The request could not complete. Check that this service is reachable and the path does not redirect.");
  } finally {
    window.clearTimeout(timer);
    $("send-request").setAttribute("aria-disabled", "false");
    $("send-label").textContent = "Send request";
    $("response-panel").setAttribute("aria-busy", "false");
  }
});
$("request-form").addEventListener("input", requestEdited);
$("method").addEventListener("change", () => { updateMethod(); requestEdited(); });
$("copy-curl").addEventListener("click", () => {
  try { copy(curlCommand(buildRequest()), $("copy-curl")); }
  catch (error) { feedback("request-error", error.message); focusError(error); }
});
$("copy-response").addEventListener("click", () => copy(responseCopy, $("copy-response")));

const tabs = [$("request-tab"), $("websocket-tab")];
function selectTab(tab) {
  for (const item of tabs) {
    const selected = item === tab;
    item.setAttribute("aria-selected", String(selected));
    item.tabIndex = selected ? 0 : -1;
    $(item.getAttribute("aria-controls")).hidden = !selected;
  }
}
tabs.forEach((tab, index) => {
  tab.addEventListener("click", () => selectTab(tab));
  tab.addEventListener("keydown", (event) => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.isComposing) return;
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    const next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
    selectTab(tabs[next]); tabs[next].focus();
  });
});
const socketURL = new URL("ws", serviceBase);
socketURL.protocol = serviceBase.protocol === "https:" ? "wss:" : "ws:";
$("websocket-address").value = socketURL.href;
function socketState(message, connected, pending = false) {
  $("websocket-status").textContent = message;
  $("websocket-status").classList.toggle("connected", connected);
  $("connect-websocket").textContent = pending ? socket && socket.readyState === WebSocket.CLOSING ? "Disconnecting…" : "Connecting…" : connected ? "Disconnect" : "Connect";
  $("connect-websocket").disabled = pending;
  $("connect-websocket").classList.toggle("primary", !connected);
  $("connect-websocket").classList.toggle("secondary", connected);
  $("send-websocket").disabled = !connected;
  $("send-websocket").classList.toggle("primary", connected);
  $("send-websocket").classList.toggle("secondary", !connected);
  $("websocket-message").disabled = !connected;
}
function logSocket(label, text) {
  const log = $("websocket-log");
  const sent = label === "Sent";
  const received = label === "Received";
  const row = element("li", "socket-entry socket-" + (sent ? "sent" : received ? "received" : "event"));
  const meta = element("div", "log-meta");
  const heading = element("div", "log-heading");
  const icon = element("span", "log-icon", sent ? "↑" : received ? "↓" : "•");
  icon.setAttribute("aria-hidden", "true");
  heading.append(icon, element("span", "log-label", label));
  if (sent || received) heading.append(element("span", "log-direction", sent ? "You → Server" : "Server → You"));
  const now = new Date();
  const time = element("time", "", now.toLocaleTimeString([], { hour12: false }));
  time.dateTime = now.toISOString();
  meta.append(heading, time);
  const content = element("pre");
  renderCode(content, text.length > 2000 ? text.slice(0, 2000) + "\n[Preview shortened]" : text);
  row.append(meta, content);
  log.append(row);
  log.tabIndex = 0;
  if (sent || received) {
    const count = sent ? ++sentMessages : ++receivedMessages;
    $("websocket-announcement").textContent = label + " message " + count + ", " + new TextEncoder().encode(text).length.toLocaleString() + " bytes.";
  }
  while (log.children.length > 20) log.firstElementChild.remove();
  $("websocket-empty").hidden = true;
  $("clear-websocket").disabled = false;
  log.scrollTop = log.scrollHeight;
}
$("connect-websocket").addEventListener("click", () => {
  feedback("websocket-error", "");
  if (socket && socket.readyState === WebSocket.OPEN) {
    socketClosing = true;
    socket.close(1000, "Disconnected by user");
    socketState("Disconnecting…", false, true);
    return;
  }
  socketClosing = false;
  const current = new WebSocket(socketURL);
  socket = current;
  socketState("Opening a connection…", false, true);
  socketTimer = window.setTimeout(() => {
    if (socket !== current || current.readyState === WebSocket.OPEN) return;
    socket = null; current.close(); socketState("Connection timed out. Try again.", false);
    feedback("websocket-error", "Could not connect within 10 seconds. Check that WebSocket upgrades reach this service.");
  }, 10000);
  current.addEventListener("open", () => {
    if (socket !== current) { current.close(); return; }
    window.clearTimeout(socketTimer); socketState("Connected", true); logSocket("Connected", "Ready to echo text messages."); $("websocket-message").focus();
  });
  current.addEventListener("message", (event) => {
    if (socket === current) logSocket("Received", typeof event.data === "string" ? event.data : "Binary message received.");
  });
  current.addEventListener("error", () => {
    if (socket === current) feedback("websocket-error", "The connection failed. Check that WebSocket upgrades reach this service, then reconnect.");
  });
  current.addEventListener("close", (event) => {
    if (socket !== current) return;
    window.clearTimeout(socketTimer); socket = null;
    const message = socketClosing ? "Disconnected" : "Connection closed (" + event.code + "). Reconnect to continue.";
    socketState(message, false); logSocket("Disconnected", message);
  });
});
$("websocket-form").addEventListener("submit", (event) => {
  event.preventDefault(); feedback("websocket-error", "");
  if (!socket || socket.readyState !== WebSocket.OPEN) { feedback("websocket-error", "Connect before sending a message."); return; }
  const text = $("websocket-message").value;
  const limit = Math.min(config ? config.max_request_size : 65536, config?.max_response_size || previewLimit, previewLimit);
  if (new TextEncoder().encode(text).length > limit) { feedback("websocket-error", "The message exceeds the " + limit.toLocaleString() + " byte limit."); identifyError($("websocket-message"), "websocket-error"); $("websocket-message").focus(); return; }
  if (socket.bufferedAmount > previewLimit) { feedback("websocket-error", "The connection is still sending. Wait a moment and try again."); return; }
  try { socket.send(text); logSocket("Sent", text); } catch { feedback("websocket-error", "The message could not be sent. Reconnect and try again."); }
});
$("websocket-message").addEventListener("keydown", (event) => {
  if (event.key !== "Enter" || event.shiftKey || event.ctrlKey || event.altKey || event.metaKey || event.isComposing || event.keyCode === 229) return;
  event.preventDefault();
  if (!event.repeat) $("websocket-form").requestSubmit();
});
$("clear-websocket").addEventListener("click", () => {
  $("websocket-log").replaceChildren(); $("websocket-log").tabIndex = -1;
  $("websocket-empty").hidden = false; $("clear-websocket").disabled = true;
  sentMessages = receivedMessages = 0;
  $("websocket-announcement").textContent = "Connection activity cleared.";
  $("socket-log-title").focus();
});
window.addEventListener("pagehide", () => { if (socket) socket.close(1000, "Page closed"); });

function applyExample(settings) {
  const choices = new URL(window.location.href).searchParams.getAll("example");
  if (!choices.length) return;
  $("config-status").hidden = false;
  const preset = choices.length === 1 ? getPreset(choices[0], settings) : undefined;
  if (!preset) {
    $("config-status").textContent = "This example is unknown or unavailable on this instance. Choose another from Examples.";
    return;
  }
  if (preset.kind === "websocket") {
    selectTab($("websocket-tab"));
    $("websocket-message").value = preset.message;
    $("config-status").textContent = preset.title + " loaded. Connect, then send the sample message.";
    return;
  }
  $("method").value = preset.method;
  methodControl?.refresh();
  $("request-path").value = preset.path;
  headerEditor.set(preset.headers);
  $("request-body").value = preset.body || "";
  $("request-headers").closest("details").open = !!preset.body || Object.keys(preset.headers).length > 0;
  for (const key of ["status", "delay", "size"]) $("control-" + key).value = preset.controls?.[key] || "";
  $("response-controls").open = !!preset.controls;
  updateMethod(); updateCommand();
  $("config-status").textContent = preset.title + " loaded. Review the sample, then choose Send request.";
}
initTheme();
initTooltips();
const methodControl = enhanceSelect($("method"));
const headerEditor = createHeaderEditor($("request-headers"), $("add-header"), requestEdited);
updateMethod(); updateCommand();
initServiceStatus(uiBase, serviceBase).then((settings) => {
  config = settings;
  $("config-status").textContent = "";
  $("config-status").hidden = true;
  $("response-controls").hidden = !settings.response_controls;
  $("controls-hint").hidden = settings.response_controls;
  if (!settings.response_controls) $("controls-hint").textContent = "Response controls are not enabled on this instance.";
  $("header-policy").hidden = settings.print_headers && !settings.unsafe_headers;
  $("header-policy").textContent = !settings.print_headers ? "Request headers are not included in responses." : settings.unsafe_headers ? "Credential redaction is disabled on this instance." : "";
  const messageLimit = Math.min(settings.max_request_size, settings.max_response_size || previewLimit, previewLimit);
  $("websocket-limit").textContent = "Message limit: " + messageLimit.toLocaleString() + " bytes. Text messages only in this workbench.";
  $("websocket-message").maxLength = messageLimit;
  $("request-body").maxLength = Math.min(settings.max_request_size, 16777216);
  const responseLimit = settings.max_response_size === 0 ? 1048576 : Math.min(settings.max_response_size > 0 ? settings.max_response_size : 65536, 1048576);
  $("control-size").max = String(responseLimit);
  $("response-limits").textContent = "Up to 10 seconds and " + responseLimit.toLocaleString() + " bytes. Size generates a binary response. Entered values override matching query parameters.";
  updateCommand(); applyExample(settings);
}).catch(() => {
  $("config-status").textContent = "";
  applyExample(null);
  const exampleMessage = $("config-status").textContent;
  $("config-status").hidden = false;
  $("config-status").textContent = "Listener settings are unavailable. You can still send requests and connect to WebSocket." + (exampleMessage ? " " + exampleMessage : "");
  $("controls-hint").textContent = "Response control settings could not be loaded.";
});
