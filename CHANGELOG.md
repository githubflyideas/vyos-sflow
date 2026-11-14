# Changelog

## 1.2.15

- Sign-ins, failed sign-ins and lock-outs, sign-outs, password changes and resets, accounts created and deleted, sources blocked and unblocked, and deleted data are written to the service log as `audit:` lines naming who did it and from where. Clearing all history used to leave no trace at all.
- A sign-in with a name that has no account takes as long to refuse as one with a wrong password, so the time a refusal takes no longer tells which names exist.
- Over TLS the dashboard sends `Strict-Transport-Security`.
- Releases carry a signature for `SHA256SUMS` (Sigstore, made by the release workflow) and a software bill of materials (CycloneDX); `SECURITY.md` says how to verify them and how to report a vulnerability.
- Every change is scanned with `govulncheck` before a release; CodeQL and Dependabot watch the code and its dependencies.
- The manual describes the audit trail and how host-name lookups are made.

## 1.2.14

- Samples from hosts that sample after receive offload (frames over 64 KB) are counted again.

## 1.2.13

- Data cleanup can delete one device's stored data.
- Accessibility improvements for screen readers and keyboard use.
- Documentation of how the numbers are counted and how `tui` finds a service.

## 1.2.12

- Stricter checks on incoming samples; refused records are logged and counted.
- Works behind a reverse proxy that passes the host name on (see docs/VYOS.md).
- Account changes from `passwd` and the web UI at the same time are all kept; `passwd` accepts a piped password.
- Smaller fixes to sign-in, user names and the terminal UI.

## 1.2.11

- Build with Go 1.27.4.

## 1.2.10

- Security and robustness fixes for incoming data and account management.
- `tui` without options finds a running service on another port.
- Minimum password length is 12 characters everywhere.
- Better contrast in all themes; text fixes.

## 1.2.9

- Terminal UI: `tui -data` attaches to the running service on whatever port it uses; the third tab has its name again.
- Messages name vyos-sflow instead of traffic66.

## 1.2.8

- Users: resetting a user's password signs that user out everywhere; DELETE /api/users also takes the name in a JSON body.
- `passwd` says whether it added a user or changed a password.
- /api/overview answers 400 for a `dir` other than `in` or `out`.
- Text fixes.

## 1.2.7

- Users: adding a name that already exists no longer changes that user's password; use Reset password instead.

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
