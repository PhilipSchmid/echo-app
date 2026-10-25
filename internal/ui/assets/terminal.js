import { curlCommand, shellQuote, requestURL, element } from "./common.js";
import { getPreset } from "./presets.js";

export function createCommandPicker(renderBlock) {
  const groups = [];
  const storageKey = "echo-observatory-terminal-client";
  const client = (label) => label.split(" · ")[0];
  let preferred = "curl";
  try { if (sessionStorage.getItem(storageKey) === "wget") preferred = "wget"; } catch {}

  function update(group, selected) {
    group.selected = selected;
    group.entries.forEach(({ button, panel }, index) => {
      const active = index === selected;
      button.setAttribute("aria-selected", String(active));
      button.tabIndex = active ? 0 : -1;
      panel.hidden = !active;
    });
  }

  return (definitions, context) => {
    if (definitions.length === 1) return renderBlock(definitions[0].label, definitions[0].command, context);
    const container = element("div", "command-picker");
    const tabs = element("div", "command-tablist");
    tabs.setAttribute("role", "tablist");
    tabs.setAttribute("aria-label", context + " command examples");
    container.append(tabs);
    const group = { entries: [], selected: 0 };
    const groupID = "command-picker-" + groups.length;
    const select = (index) => {
      preferred = client(group.entries[index].label);
      for (const other of groups) {
        const match = other.entries.findIndex((entry) => client(entry.label) === preferred);
        let selected = other.selected;
        if (other === group) selected = index;
        else if (client(other.entries[selected].label) !== preferred && match >= 0) selected = match;
        update(other, selected);
      }
      try { sessionStorage.setItem(storageKey, preferred); } catch {}
    };
    definitions.forEach(({ label, command }, index) => {
      const button = element("button", "command-tab", label);
      const panel = element("div", "command-tabpanel");
      button.type = "button";
      button.id = groupID + "-tab-" + index;
      panel.id = groupID + "-panel-" + index;
      button.setAttribute("role", "tab");
      button.setAttribute("aria-controls", panel.id);
      panel.setAttribute("role", "tabpanel");
      panel.setAttribute("aria-labelledby", button.id);
      panel.append(renderBlock(label, command, context));
      button.addEventListener("click", () => select(index));
      button.addEventListener("keydown", (event) => {
        if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.isComposing) return;
        const next = { ArrowRight: (index + 1) % definitions.length, ArrowLeft: (index + definitions.length - 1) % definitions.length, Home: 0, End: definitions.length - 1 }[event.key];
        if (next === undefined) return;
        event.preventDefault();
        select(next);
        group.entries[next].button.focus();
      });
      group.entries.push({ label, button, panel });
      tabs.append(button);
      container.append(panel);
    });
    groups.push(group);
    update(group, Math.max(0, group.entries.findIndex((entry) => client(entry.label) === preferred)));
    return container;
  };
}

export function wgetCommand(request) {
  const groups = ["wget --no-verbose --output-document=-", "--timeout=15 --tries=1 --max-redirect=0", "--content-on-error"];
  if (request.method !== "GET") groups.push("--method " + shellQuote(request.method));
  if (request.method === "HEAD") groups.push("--server-response");
  for (const [name, value] of request.headers) groups.push("--header " + shellQuote(name + ": " + value));
  if (request.body !== undefined) groups.push("--body-data " + shellQuote(request.body));
  groups.push(shellQuote(request.url.href));
  return groups.join(" \\\n  ");
}

export function httpExamples(serviceBase, settings) {
  const definitions = [
    { id: "get", title: "Inspect a request", description: "Print the server-observed request as JSON. Any ordinary path works; / stays the simplest starting point.", method: "GET", path: "/", headers: {} },
    { ...getPreset("json-post"), id: "post" },
    { id: "head", title: "Fetch response headers", description: "Send HEAD to inspect response headers without downloading a body. wget prints the headers to stderr.", method: "HEAD", path: "/", headers: {} },
    ...(!serviceBase.hostname.includes(":") && !/^[\d.]+$/.test(serviceBase.hostname) ? [{ id: "ip-family", title: "Compare IPv4 and IPv6", description: "Force IPv4 or IPv6 for this hostname. Both need DNS answers and a reachable listener. The response shows which family this request used.", method: "GET", path: "/", headers: {} }] : []),
    ...["forwarded", "status-418", "delay-250ms", "size-1024"].map((id) => getPreset(id, settings)).filter(Boolean)
  ];
  return definitions.map((example) => {
    const url = requestURL(example.path, serviceBase);
    for (const [name, value] of Object.entries(example.controls || {})) url.searchParams.set(name, value);
    const request = { url, method: example.method, headers: new Headers(example.headers), body: example.body };
    const commands = [
      { label: "curl", command: curlCommand(request) },
      { label: "wget", command: wgetCommand(request) }
    ];
    if (example.id === "ip-family") return { ...example, commands: [
      { label: "curl · IPv4", command: curlCommand(request).replace("curl ", "curl -4 ") },
      { label: "curl · IPv6", command: curlCommand(request).replace("curl ", "curl -6 ") }
    ] };
    if (example.id === "size-1024") {
      commands[0].command = commands[0].command.replace("curl ", "curl --output /dev/null --write-out " + shellQuote("%{size_download} bytes\\n") + " ");
      commands[1].command = commands[1].command.replace("--output-document=-", "--output-document=/dev/null");
      return { ...example, description: "Receive 1,024 binary bytes without printing binary data. curl prints the byte count; wget reports the download on stderr. The data is discarded using /dev/null.", commands };
    }
    return { ...example, commands };
  });
}

export function directCommands(name, settings, serviceBase) {
  const portKey = { HTTP: "http_port", HTTPS: "tls_port", TCP: "tcp_port", gRPC: "grpc_port", "HTTP/3": "quic_port" }[name];
  const port = settings?.[portKey];
  if (!portKey || typeof port !== "string" || !/^[1-9]\d{0,4}$/.test(port) || Number(port) > 65535) return [];
  let base;
  try { base = new URL(serviceBase); } catch { return []; }
  if (base.protocol !== "http:" && base.protocol !== "https:") return [];
  const host = base.hostname;
  const socketHost = host.startsWith("[") ? host.slice(1, -1) : host;
  const request = (scheme) => ({ url: new URL(`${scheme}://${host}:${port}/`), method: "GET", headers: new Headers() });
  const http = (scheme) => {
    const value = request(scheme);
    return [{ label: "curl", command: curlCommand(value) }, { label: "wget", command: wgetCommand(value) }];
  };
  switch (name) {
    case "HTTP": {
      const commands = http("http");
      commands[0].command = commands[0].command.replace("curl ", "curl --http1.1 ");
      if (settings.h2c === true) commands.push({ label: "curl · h2c", command: curlCommand(request("http")).replace("curl ", "curl --http2-prior-knowledge ") });
      return commands;
    }
    case "HTTPS": return settings.tls === true ? [
      ...http("https"),
      { label: "curl · HTTP/2", command: curlCommand(request("https")).replace("curl ", "curl --http2 ") }
    ] : [];
    case "TCP": return settings.tcp === true ? [{ label: "netcat", command: "nc -w 5 -- " + shellQuote(socketHost) + " " + shellQuote(port) }] : [];
    case "gRPC": return settings.grpc === true ? [{ label: "grpcurl", command: "grpcurl -max-time 15 " + (settings.grpc_tls === true ? "" : "-plaintext ") + "-d '{}' -- " + shellQuote(host + ":" + port) + " echo.EchoService/Echo" }] : [];
    case "HTTP/3": return settings.quic === true ? [{ label: "curl · HTTP/3", command: curlCommand(request("https")).replace("curl ", "curl --http3-only ") }] : [];
    default: return [];
  }
}
