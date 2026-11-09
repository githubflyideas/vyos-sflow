# Changelog

## 1.2.6

- Security: `./vyos-sflow serve` keeps the Web UI on `127.0.0.1:8066` unless an address is chosen. At the interactive prompt, Enter now means loopback only; type a private local IP for LAN access or `0.0.0.0`/`::` for all interfaces. Noninteractive starts without `-addr` or `VYOS_SFLOW_ADDR` also use `127.0.0.1`. **Compatibility:** wrappers that relied on the old all-interfaces default must pass `-addr 0.0.0.0:8066` (or a private IP) or set `VYOS_SFLOW_ADDR`. Installed services keep the address in their `service.env`. The sFlow listener default `0.0.0.0:6343` is unchanged, so remote exporters keep working.
- The startup log explains how to reach a loopback-only Web UI (choose `-addr` or use an SSH tunnel).
- Security: every Web response carries a Content-Security-Policy (same-origin scripts, no framing), `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` and `Cross-Origin-Opener-Policy: same-origin`.
- Release archives store files with owner and group 0 instead of the build user's IDs.
- The Go module path and the service installation download links use the current repository name `githubflyideas/vyos-sflow`; the help page links to this repository instead of traffic66. The TLS certificate download is named `vyos-sflow.crt`.
- License: vyos-sflow is licensed under Apache-2.0 only. The inherited PolyForm Noncommercial `LICENSE.md` and `ADDITIONAL-USE-GRANT.md` are removed; the Apache-2.0 `LICENSE` text is unchanged. New `NOTICE` (copyright and Traffic66 attribution) and `TRADEMARKS.md` (names and logo reserved, Apache-2.0 section 6) are shipped in release archives. `upstream-manifest.json` remains a historical provenance record.
- Documentation: the development build command embeds the commit so `./vyos-sflow version` matches the source. The legacy names `./vyso_sflow_data` and `traffic66.duckdb` are kept deliberately so existing data is still found.

## 1.2.5

- The top bar fits narrower windows: account and sign-out are never cut off.

## 1.2.4

- New look: navigation at the top, device and interface traffic on the overview, five themes from light to dark.
- Flow records are shown 100 to a page.

## 1.2.3

- Traffic is counted as in 1.1.2 again.
- Sources: interface reconciliation, each interface's own counters against the sampled traffic.

## 1.2.2

- sFlow: traffic is counted in full from devices that send two samplers' samples under one interface.

## 1.2.1

- Ignore repeated samples and keep sampling diagnostics stable when samples arrive out of order.
- Isolate loss compensation across exporter restarts and manual history cleanup.
- Preserve accepted in-memory history when downstream cleanup fails.
- Explain chart direction, ranking totals, estimated record multipliers and effective record windows in all 15 languages.
- Keep download, verification and extraction instructions consistent with the release.

## 1.2.0

- Overview: total, inbound and outbound bandwidth.
- Top 66: main talkers are back.
- Flow records page; the trends page is removed.
- More accurate traffic figures when sFlow samples are lost.

## 1.1.2

- Use the vyos-sflow project logo on the login screen and dashboard, with proportions preserved.

## 1.1.1

- Open a temporary terminal dashboard with `./vyos-sflow tui`; no separate service command is required.
- Stop owned collection and remove session-only data when quitting, without changing existing service history or accounts.
- Reuse an existing local service when available and preserve explicit connection options.

## 1.1.0

- Start directly with `./vyos-sflow serve` and an optional Web listen-address prompt.
- Keep accounts and rolling 48-hour history in `./vyso_sflow_data` by default.
- Receive sFlow v5 from reachable exporters without sender-identity setup.
- Browse traffic trends and Top IP, port and protocol rankings in 15 languages.
- Use the optional offline Shell installer for automatic systemd startup.
- Build and validate releases with Shell, Node.js, Go and Expect tooling.
