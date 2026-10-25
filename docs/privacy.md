# Privacy and logging

Updated: 2026-10-25

[README](../README.md) · [Usage](usage.md) · [Configuration](configuration.md)

This describes the application's behavior, not a complete privacy notice for a
particular deployment. The UI footer links to a summary at `/ui/privacy/`.
Use synthetic data: diagnostics expose request and connection information and
cannot guarantee privacy or confidentiality.

Privacy contact for the public demo at `echo.container.education`:
[privacy@container.education](mailto:privacy@container.education).
For self-hosted instances, contact your operator.

## Browser storage and external services

The application sets no cookies and includes no analytics, advertising,
fingerprinting, or third-party asset loads. UI assets are bundled; browser
requests and WebSocket connections use this service. External GitHub links only
contact GitHub when you follow them.

The UI writes these `sessionStorage` values only after you choose a preference:

| Key | Purpose |
| --- | --- |
| `echo-observatory-theme` | Remember the selected light or dark theme. |
| `echo-observatory-terminal-client` | Remember curl or GNU wget for command examples. |

They are scoped to this origin and browser tab, survive reloads, and last for
the tab's session. Browser session restoration can preserve them. They contain
no request contents or visitor identifier.
Without a saved choice, the theme follows the system and commands use curl.
The controls still work when browser storage is unavailable. The UI does not
save request history or use persistent `localStorage`.

Requested session preferences do not by themselves create a blanket requirement
for a consent banner. Storage rules also cover technologies other than cookies;
their purpose matters. Reassess consent and notice requirements if tracking,
third-party services, or other storage purposes are introduced. See the
[CNIL guidance on cookies and other trackers](https://www.cnil.fr/fr/cookies-et-autres-traceurs/regles/cookies/comment-mettre-mon-site-web-en-conformite).

## Requests, logs, and redaction

The server processes IP addresses, URLs, headers, and other connection metadata
to produce diagnostics. HTTP bodies are read within the configured size limit
and discarded. WebSocket messages are echoed over the live connection. The
application does not log request or message payloads or keep request history.

Rate protection keeps a bounded set of socket IP prefixes in memory. It removes
entries idle for ten minutes during peer checks; process exit clears them all.
Optional durable quota state records
only the UTC day and aggregate reserved bytes, without visitor identifiers.

At debug level, HTTP diagnostics include the socket peer IP, request path and
method. Request log lines omit the query string, but diagnostic responses include
the original URI. Enabling HTTP header output also adds selected headers and the
redacted header map to debug logs. TCP and gRPC connection or request diagnostics
are debug-only; gRPC debug logs include metadata. Successful requests are not
logged at the default info level. Operators, reverse proxies, and hosting
platforms may record additional data and determine log retention.

HTTP readiness probe logs identify only the target host and port. Probe logs
and readiness errors omit URL credentials, paths, queries and fragments, and
report fixed failure categories rather than raw HTTP client errors.

The default filter redacts header names matching Authorization,
Proxy-Authorization, Cookie, or Set-Cookie, and names containing token, secret,
key, password, passphrase, or credential, case-insensitively.
`--unsafe-headers` disables this filter in diagnostic output and debug metadata
logs. It does not enable HTTP header output by itself.

This name filter is not exhaustive. Paths, custom headers, user agents, and debug
metadata may contain personal or confidential data; headers such as Referer may
contain URLs. Never send real credentials or confidential content in paths,
queries, headers, bodies, or WebSocket messages.

Use HTTPS and WSS with a trusted certificate for public browser traffic, and TLS
for gRPC. Plain HTTP, h2c, raw TCP, and plaintext gRPC are unencrypted. TLS protects
traffic in transit; the service and any TLS-terminating proxy still receive it.

## Public deployment responsibilities

Before operating a public instance, provide an accessible notice identifying the
controller and a real privacy contact. Describe the data processed, purposes and
applicable legal basis, hosting and other recipients, countries and transfer
safeguards where applicable, actual retention periods or their criteria, and
users' rights and complaint route. Explain both application and infrastructure
logs. Choose and enforce retention rather than assuming that discarded payloads
mean no data is retained.

The operator must supply these facts; this repository does not establish an
operator, contact address, hosting arrangement, or retention period for every
instance. Consent to browser storage and transparency about personal-data
processing are separate questions. See the
[European Commission's information requirements](https://commission.europa.eu/law/law-topic/data-protection/information-individuals_en)
and the
[Swiss FDPIC's duty to provide information](https://www.edoeb.admin.ch/en/duty-to-provide-information).
