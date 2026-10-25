import { $, element } from "./common.js";

function renderCertificate(data) {
  if (!data || !["disabled", "demo", "file", "acme", "acme-staging"].includes(data.mode)) throw new Error("Invalid certificate details");
  const certificate = data.certificate;
  const textFields = ["subject", "issuer", "not_before", "not_after", "sha256_fingerprint", "serial_number", "public_key_algorithm", "signature_algorithm"];
  if (certificate !== null && (!certificate || textFields.some((field) => typeof certificate[field] !== "string") || ["dns_names", "ip_addresses"].some((field) => certificate[field] !== null && (!Array.isArray(certificate[field]) || certificate[field].some((name) => typeof name !== "string"))) || typeof certificate.self_signed !== "boolean")) throw new Error("Invalid certificate details");
  const selfSigned = data.mode === "demo" || certificate?.self_signed === true;
  $("certificate-note").textContent = data.mode === "disabled" ? "No app TLS listener is enabled."
    : data.mode === "acme-staging" ? "The app uses Let's Encrypt staging certificates. Normal clients do not trust this testing CA; configure its CA only for testing."
    : selfSigned ? "The app uses a self-signed certificate. Add its certificate to your client's trust configuration when connecting directly."
    : data.mode === "acme" ? "The app uses Let's Encrypt certificates, managed and renewed automatically. Clients normally trust a valid certificate chain for the matching hostname."
    : "The app uses an operator-supplied certificate. Client trust depends on its issuer, certificate chain and the hostname you connect to.";
  $("certificate-fields").hidden = !certificate;
  $("certificate-technical-details").hidden = !certificate;
  if (!certificate) {
    $("certificate-status").textContent = data.mode === "disabled" ? "No certificate is in use." : "No certificate is available yet. Reopen after a TLS connection loads it.";
    return;
  }
  const date = (value) => {
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? "Unavailable" : parsed.toISOString().replace("T", " ").replace(/\.\d{3}Z$/, " UTC");
  };
  const fields = (id, rows) => {
    $(id).replaceChildren(...rows.map(([label, value]) => {
      const row = element("div", "");
      row.append(element("dt", "", label), element("dd", "", value || "Not specified"));
      return row;
    }));
  };
  fields("certificate-fields", [
    ["Subject", certificate.subject], ["Issuer", certificate.issuer],
    ["DNS names", (certificate.dns_names || []).join(", ")], ["IP addresses", (certificate.ip_addresses || []).join(", ")],
    ["Valid from", date(certificate.not_before)], ["Valid until", date(certificate.not_after)],
    ["SHA-256 fingerprint", certificate.sha256_fingerprint]
  ]);
  fields("certificate-technical-fields", [
    ["Serial number", certificate.serial_number], ["Public key", certificate.public_key_algorithm], ["Signature", certificate.signature_algorithm]
  ]);
  $("certificate-status").textContent = "Certificate loaded.";
}

export function initCertificateDetails(uiBase) {
  const help = $("certificate-help");
  if (!help) return;
  let version = 0;
  let controller;
  help.addEventListener("toggle", async () => {
    const request = ++version;
    controller?.abort();
    if (!help.open || help.hidden) return;
    const activeController = new AbortController();
    controller = activeController;
    const timer = window.setTimeout(() => activeController.abort(), 5000);
    $("certificate-fields").hidden = true;
    $("certificate-technical-details").hidden = true;
    $("certificate-status").textContent = "Loading app TLS listener certificate…";
    try {
      const response = await fetch(new URL("certificate.json", uiBase), { credentials: "same-origin", cache: "no-store", redirect: "error", signal: activeController.signal });
      if (!response.ok) throw new Error("Certificate details unavailable");
      const data = await response.json();
      if (request === version && help.open) renderCertificate(data);
    } catch {
      if (request === version && help.open) $("certificate-status").textContent = "Certificate details are unavailable. Reopen this section to try again.";
    } finally { window.clearTimeout(timer); }
  });
  const openLinkedDetails = () => {
    if (window.location.hash === "#certificate-help") help.open = true;
  };
  window.addEventListener("hashchange", openLinkedDetails);
  openLinkedDetails();
}
