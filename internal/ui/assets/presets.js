export const presets = Object.freeze([
  { id: "inspect", title: "Inspect a request", kind: "http", method: "GET", path: "/?from=example", description: "See the host, original URI, protocol, and actual socket peer reported by this instance." },
  { id: "json-post", title: "Send a JSON body", kind: "http", method: "POST", path: "/?from=json-example", headers: { "Content-Type": "application/json" }, body: '{"message":"Hello from Echo Observatory","sample":true}', description: "Send a small synthetic JSON body. The service reports request metadata, not the body itself." },
  { id: "forwarded", title: "Compare forwarded headers", kind: "http", method: "GET", path: "/?from=forwarded-example", headers: { "X-Forwarded-For": "203.0.113.42", "X-Forwarded-Proto": "https" }, requires: "print_headers", description: "Send a sample X-Forwarded-For header and compare it with the actual socket peer." },
  { id: "websocket", title: "Echo a WebSocket message", kind: "websocket", message: "Hello from Echo Observatory", description: "Open a WebSocket connection and send a sample text message. The server echoes it back unchanged." },
  { id: "status-418", title: "Return a different status", kind: "http", method: "GET", path: "/", controls: { status: "418" }, requires: "response_controls", description: "Ask for HTTP 418 and inspect the status alongside the normal request metadata." },
  { id: "delay-250ms", title: "Add a short delay", kind: "http", method: "GET", path: "/", controls: { delay: "250ms" }, requires: "response_controls", description: "Add 250 milliseconds before the response, then compare the measured request duration." },
  { id: "size-1024", title: "Receive a sized response", kind: "http", method: "GET", path: "/", controls: { size: "1024" }, requires: "response_controls", description: "Receive 1,024 generated binary bytes. The workbench reports the size instead of displaying binary data." }
].map((preset) => Object.freeze({ ...preset, headers: Object.freeze(preset.headers || {}), ...(preset.controls ? { controls: Object.freeze(preset.controls) } : {}) })));

export function getPreset(id, settings) {
  const preset = presets.find((item) => item.id === id);
  if (!preset) return undefined;
  if (preset.requires && (!settings || !Object.hasOwn(settings, preset.requires) || settings[preset.requires] !== true)) return undefined;
  return preset;
}
