# Privacy notice

Updated: 2026-10-25

[README](../README.md) · [Usage](usage.md) · [Configuration](configuration.md)

This notice applies to the public demo at `echo.container.education`, operated
by **Philip Schmid**. Contact [privacy@container.education](mailto:privacy@container.education)
for privacy questions or to exercise your rights. Other installations have their
own operators and privacy arrangements.

## What we process and why

The service processes your IP address, URL, headers, protocol and connection
details, and data you deliberately send, to provide the diagnostics you request
and protect the service against misuse. Opening `/ui/` also displays the request
used to load that page. Use synthetic data, never real credentials or confidential
content.

Where the GDPR applies, this processing relies on our legitimate interests in
providing a network-testing service and keeping it secure (Article 6(1)(f)).
We do not sell personal data, use it for advertising, create visitor profiles,
or make automated decisions with legal or similarly significant effects.

## Storage and retention

- **Request contents:** HTTP bodies are consumed and discarded after handling.
  WebSocket messages are echoed over the live connection. The app keeps no
  persistent request history; workbench activity stays in page memory.
- **Browser preferences:** only your chosen theme and curl/wget preference enter
  this tab's `sessionStorage`. They contain no request data or visitor identifier.
  They last for the tab's session; browser session restoration can preserve them.
- **Operational logs:** the public demo disables routine request diagnostics.
  Hosting infrastructure retains operational logs for seven days. Error
  diagnostics can include hostnames, paths and technical connection information.
  We do not export a separate copy of these logs.
- **Service measurements:** aggregate operational metrics may remain for about
  15 days. Application metric labels contain no visitor IPs, header values or
  arbitrary request URLs.
- **Protection state:** temporary connection and rate-limit state lives in
  memory. Idle peer-prefix entries are removed during subsequent checks after
  ten minutes; restart clears them. Durable quotas contain only a UTC day and
  aggregate byte counts, without visitor identifiers.
- **Privacy correspondence:** if you email us, we use your address and message
  to handle the request and retain them only as needed to resolve it and meet
  applicable legal obligations.

The app sets no cookies and includes no analytics, ads, trackers, external fonts
or third-party asset loads. Session preferences are saved only after you choose
them. There is no consent banner for these requested preferences. GitHub receives
a request only if you follow a GitHub link.

## Recipients and international processing

Authorized operators and hosting, network and operational service providers
process data as needed to deliver and secure the service. The application runs
in Germany; infrastructure logs, metrics and support operations can involve the
United States. Transfers to certified US recipients rely on recognized data
privacy adequacy frameworks. You can request details of recipients and transfer
safeguards through the privacy contact.

## Your rights

Subject to applicable law, you can request access, correction, deletion,
restriction or portability of your personal data, and object to processing based
on legitimate interests. We may need proportionate identity verification. We do
not collect extra identifiers just to identify diagnostic requests that are no
longer retained.

You may complain to your local data-protection authority, including the
[Swiss FDPIC](https://www.edoeb.admin.ch/en/)
or a [competent EU supervisory authority](https://www.edpb.europa.eu/about-edpb/about-edpb/members_en).

## Technical privacy limits

Common credential headers are redacted in responses by default and always in
application diagnostic logs. `--unsafe-headers` can disable response redaction
on self-hosted instances; it does not disable log redaction or enable header
output. Header-name filtering is not exhaustive. Personal or confidential values
can still appear in paths, queries, custom headers or other diagnostic fields.

Use HTTPS, WSS or TLS-enabled gRPC with a trusted certificate to encrypt traffic.
Plain HTTP, h2c, raw TCP and plaintext gRPC are unencrypted. The service receives
the data you send even when TLS protects it in transit. Privacy and
confidentiality cannot be guaranteed.
