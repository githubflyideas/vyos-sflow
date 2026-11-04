# Changelog

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
