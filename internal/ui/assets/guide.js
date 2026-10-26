import { $, baseURLs, element, copy, setCopyLabel, initTheme, initTooltips, loadConfig, shellQuote } from "./common.js";
import { presets, getPreset } from "./presets.js";
import { renderCode } from "./highlight.js";
import { httpExamples, directCommands, createCommandPicker } from "./terminal.js";

const { uiBase, serviceBase } = baseURLs(import.meta.url);
const page = document.body.dataset.uiPage;
initTheme();
initTooltips();
const commandPicker = createCommandPicker(commandBlock);

function commandBlock(label, command, context) {
  const block = $("terminal-command-template").content.firstElementChild.cloneNode(true);
  block.querySelector("[data-command-label]").textContent = label;
  const code = block.querySelector("pre");
  code.setAttribute("aria-label", context + ": " + label + " command");
  renderCode(code, command, "bash");
  const button = block.querySelector("button");
  setCopyLabel(button, "Copy", "Copy " + context + " " + label + " command");
  button.addEventListener("click", () => copy(command, button));
  return block;
}

function renderTerminalExamples(settings) {
  for (const example of httpExamples(serviceBase, settings)) {
    if ($("terminal-" + example.id)) continue;
    const recipe = element("details", "terminal-recipe");
    recipe.id = "terminal-" + example.id;
    recipe.open = example.id === "get";
    recipe.append(element("summary", "", example.title), element("p", "field-hint", example.description));
    if (example.id === "status-418") recipe.append(element("p", "field-hint", "HTTP 418 is intentional. wget prints the response body and exits with status 8 for this HTTP error."));
    recipe.append(commandPicker(example.commands, example.title));
    $("http-recipes").append(recipe);
  }
}

function renderExamples(settings) {
  const cards = $("examples-grid");
  cards.replaceChildren();
  for (const preset of presets) {
    const available = getPreset(preset.id, settings);
    if (preset.requires === "response_controls" && !available) continue;
    const card = element("article", "example-card");
    const kind = preset.kind === "websocket" ? "WebSocket" : preset.method + " request";
    card.append(element("p", "example-meta", kind), element("h2", "", preset.title), element("p", "", preset.description));
    if (available) {
      const target = new URL("workbench/", uiBase);
      target.searchParams.set("example", preset.id);
      const link = element("a", "example-link", "Open in workbench");
      link.href = target.href;
      card.append(link);
    } else {
      card.append(element("p", "field-hint", settings ? "Header output unavailable on this instance." : "Listener settings unavailable; this example cannot be loaded."));
    }
    cards.append(card);
  }
}

function renderListeners(settings) {
  const definitions = [
    ["HTTP", true, settings.http_port, "TCP", settings.h2c ? "HTTP/1.1 and cleartext HTTP/2." : "HTTP/1.1."],
    ["HTTPS", settings.tls, settings.tls_port, "TCP", "TLS for HTTP/1.1 and HTTP/2."],
    ["TCP", settings.tcp, settings.tcp_port, "TCP", "Open a TCP connection with netcat to receive the server’s details as JSON."],
    ["gRPC", settings.grpc, settings.grpc_port, "TCP", settings.grpc_tls ? "Connect with grpcurl over TLS." : "Connect with grpcurl over plaintext HTTP/2."],
    ["HTTP/3", settings.quic, settings.quic_port, "UDP", "Serves the UI and JSON responses over QUIC."]
  ];
  const cards = $("listener-cards");
  cards.replaceChildren();
  for (const [name, configured, port, transport, description] of definitions) {
    const card = element(configured ? "details" : "article", "listener-card");
    const summary = element(configured ? "summary" : "div", "listener-summary");
    const identity = element("span", "listener-identity");
    identity.append(element("span", "listener-name", name), element("code", "listener-port", port + " / " + transport));
    summary.append(identity, element("span", "availability" + (configured ? "" : " disabled"), configured ? "Configured" : "Off"));
    card.append(summary);
    if (configured) {
      const body = element("div", "listener-body");
      body.append(element("p", "listener-description", description));
      const hint = name === "HTTP" && settings.h2c ? "h2c" : name === "HTTPS" ? "http2" : name === "HTTP/3" ? "http3" : "";
      if (hint) body.append($(hint + "-hint-template").content.firstElementChild.cloneNode(true));
      body.append(commandPicker(directCommands(name, settings, serviceBase), name));
      if (name === "HTTPS" || name === "HTTP/3" || (name === "gRPC" && settings.grpc_tls)) {
        const help = element("a", "certificate-link", "Certificate details →");
        help.href = new URL("./#certificate-help", uiBase).href;
        body.append(help);
      }
      card.append(body);
    }
    cards.append(card);
  }
}

if (page === "connect") {
  $("public-service-address").textContent = serviceBase.href;
  renderTerminalExamples(null);
  const websocketURL = new URL("ws", serviceBase);
  websocketURL.protocol = serviceBase.protocol === "https:" ? "wss:" : "ws:";
  $("public-websocket-address").textContent = websocketURL.href;
  $("websocket-command").append(commandBlock("websocat", "websocat -t " + shellQuote(websocketURL.href), "WebSocket echo"));
}

loadConfig(uiBase).then((settings) => {
  if (page === "examples") {
    renderExamples(settings);
    $("examples-status").textContent = "Open an example to prefill the workbench. Requests are sent only when you choose Send request or Connect.";
  } else {
    renderTerminalExamples(settings);
    renderListeners(settings);
    $("config-status").textContent = "Connection examples ready.";
    $("config-status").classList.add("sr-only");
  }
}).catch(() => {
  if (page === "examples") {
    renderExamples(null);
    $("examples-status").textContent = "Listener settings are unavailable. Basic examples are available; optional examples are hidden or marked unavailable.";
  } else {
    $("config-status").classList.remove("sr-only");
    $("config-status").textContent = "Listener settings are unavailable. You can still use the commands below.";
    $("listener-cards").replaceChildren(element("p", "field-hint", "Reload to try loading the backend listener reference again."));
  }
});
