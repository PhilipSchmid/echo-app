import { $, baseURLs, copy, initTheme, initServiceStatus, legacyWorkbenchURL } from "./common.js";
import { renderCode } from "./highlight.js";
import { initCertificateDetails } from "./certificate.js";

const { uiBase, serviceBase } = baseURLs(import.meta.url);
function redirectLegacy() {
  const target = legacyWorkbenchURL(window.location.href, uiBase);
  if (target) window.location.replace(target.href);
  return target;
}
window.addEventListener("hashchange", redirectLegacy);
if (!redirectLegacy()) {
  initTheme();
  initCertificateDetails(uiBase);
  renderCode($("visit-body"), $("visit-body").textContent, "json");
  $("copy-visit").disabled = false;
  $("copy-visit").addEventListener("click", () => copy($("visit-body").textContent, $("copy-visit")));
  initServiceStatus(uiBase, serviceBase).catch(() => {});
}
