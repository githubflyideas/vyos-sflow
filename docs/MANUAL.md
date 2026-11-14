# vyos-sflow operator manual

## First use

Follow the three steps in the repository README: configure your exporter, extract and run the executable, and open the dashboard. Sign in with the initial account and change its password before using analytics.

## Read your traffic

- **Overview:** the top indicators describe all traffic in the selected range. The chart can show all traffic, inbound or outbound. Direction is relative to the exporter/interface shown beside the chart, selected as the busiest interface in that range; it is not automatically the WAN interface and may change with the range. Below it, the device card and the interface table: each interface's speed, utilisation and traffic in and out as the device itself counts them, busiest first, with whether the samples match.
- **Top 66:** the main talkers show the top 30 clients and top 30 servers side by side, or a table by host, application or protocol. Client/server roles are inferred, not necessarily the packet source and destination. Both lists describe the same traffic from different perspectives: do not add their totals. The all-traffic total includes entries outside the displayed ranking.
- **Flow records:** the newest records first, 100 to a page (use Previous/Next), over at most 24 hours, optionally for one host. The page shows its effective time window even when the global selection is 48 hours. Traffic and packet counts are estimates; the multiplier is the measured effective sampling rate and is not the exporter's configured sampling ratio.
- **Diagnostics:** received samples, configured stream identity, reception diagnostics and interface reconciliation: each interface's own counters against what the samples add up to. A direction far below 100% is usually not sampled on the device; enable sFlow counter polling for this table.

Select a time range from 15 minutes to 48 hours. The default is one hour. Traffic values are estimates derived from samples, not exact interface counters. Port/application labels describe service ports rather than deep packet inspection. Choose your preferred interface language from the language selector, and one of five themes, from light to dark, with the dots at the top right.

## Receiving samples

The default UDP listener is `0.0.0.0:6343`. Configure the exporter to send sFlow v5 to this machine; on the same VyOS router use `127.0.0.1:6343`. No sender-IP or sub-agent setup is required. Header identities remain available for diagnostics and do not authenticate senders. Restrict UDP access to trusted devices.

## Temporary terminal view

Run `./vyos-sflow tui` over SSH for a one-command session. It looks for a running service first: the address a service writes into its data directory at start (`./vyso_sflow_data`, or `VYOS_SFLOW_DATA`) when that address answers, else `127.0.0.1:8066`. Options `-data`, `-url` or `-user` always connect to an existing service. If no local service is running, it starts a temporary collector, stores session data in a private temporary directory and connects the TUI through a loopback-only API. It still receives sFlow on UDP `0.0.0.0:6343`; configure sampling as described in the README and restrict UDP access to trusted devices.

Press `q` or `Ctrl+C` to exit. The session's collector stops and its temporary data is removed; this does not clear existing persistent history or accounts. If TUI connected to an already running service, that service continues running after you quit. For an explicit service address and account, use `./vyos-sflow tui -url http://ROUTER_IP:8066 -user admin`. Existing account restrictions still apply.

Use `serve` or an installed service for continuous collection and rolling 48-hour history between viewing sessions.

## Clear history

Administrators can clear all historical flow records and summaries through the cleanup control. Confirm both prompts. This also clears records from the current 48-hour window. Accounts, configuration and existing metadata remain intact. Back up first: this operation cannot be undone by the application.

## Data and accounts

For the README foreground command, history and accounts are in `./vyso_sflow_data` relative to the directory where you launch the process. Keep that directory when restarting or replacing the executable. An installed service uses `/config/vyos-sflow/data` by default. History expires automatically after 48 hours. Stop the service before backing up the entire directory, including account files and Parquet data. Keep backups private.

## Audit trail

Signing in and out, failed sign-ins and lock-outs (five failures from one address lock it out for a minute), password changes and resets, accounts created and deleted, sources blocked and unblocked, and deleted data each write one line to the service log, starting with `audit:` and naming the account it concerns, who did it (`by`, `local` for the terminal view on this machine) and the address it came from. Passwords never appear in it. For an installed service:

```sh
sudo journalctl -u vyos-sflow.service --no-pager | grep audit:
```

The journal keeps these lines as long as the system's journal settings keep its logs; keep them longer by forwarding the journal to your log server.

The installer refuses nonempty existing data directories. Follow the installation guide for upgrades; never delete a data directory merely to get past a warning. Existing accounts are not reset. Restart after changing a password with the command-line tool to revoke existing sessions.

## Troubleshooting

Healthy HTTP alone does not prove samples are arriving. Check Sources, exporter destination and firewall rules when charts remain empty. For direct execution, inspect output in the running terminal. For an installed service, check status and logs:

```sh
sudo systemctl status vyos-sflow.service
sudo journalctl -u vyos-sflow.service -n 50 --no-pager
```

For direct execution the Web UI listens on `127.0.0.1:8066` unless an address is chosen at the prompt or with `-addr`; the sFlow UDP listener uses all interfaces. The installed service uses the addresses in its `service.env` (all interfaces in the shipped template). Restrict management access to a private network; HTTP is unencrypted and TLS is optional. Over TLS the dashboard asks browsers to keep using HTTPS for a year (HSTS); a browser ignores that while it has not accepted the certificate. See the installation guide for configuration and the validation document for compatibility limits.

## How the numbers are counted

- **Sampling.** Each sampled packet counts as the sampling rate's worth of packets and bytes. The rate used is measured from the agent's sample pool every 30 seconds once 30 samples have arrived; until then, and right after the agent's configured rate changes, the configured rate is used. A measured rate outside half to four times the configured one is not trusted and the configured rate is used instead.
- **Lost exports.** Gaps in an agent's sequence numbers are reported as loss on the diagnostics page. The measured rate makes up for lost samples from the next measurement on; samples counted before it are not corrected afterwards, so the first half minute of a lossy agent reads low.
- **Duplicates.** A datagram repeated straight after itself (same sequence number and pool) is dropped. A copy that arrives later, after other datagrams, is counted again: sFlow carries nothing that would tell a replay from new data.
- **Sampling rate 0.** Treated as 1 in 1 and shown as 1:0; such an agent is misconfigured.
- **Impossible values.** A sample with a frame larger than 65,599 bytes, a sampling rate above 1 in 16,777,216, or a record above 64 TiB is refused; the count is in the log and in the status data as `implausible`.
- **Recent minutes.** Data for a minute keeps arriving for about a minute after it; until then the minute is shown as received so far. *Current bandwidth* is the average over the last five complete minutes, so it reads 0 for the first minutes after start.

## Name lookups

vyos-sflow does not look up host names by itself: the dashboard and the terminal view show addresses. The API offers `POST /api/resolve` to a signed-in client, which asks for the reverse DNS (PTR) names of up to 500 addresses. Those queries go to the resolver this machine is configured to use, the same one its other programs use, at most two a second, and an answer is kept for two minutes. Private addresses are asked about as well, so a resolver outside your network would learn which internal addresses were looked up; point the system at an internal resolver if that matters.

## Deleting one device's data

**Data cleanup** lists the devices with stored traffic. Deleting one removes all of its flow records, for a device whose traffic should not count (forged or broken exports, or a test device). Block it first if it is still sending. A flow that another device also reported keeps counting through that device's copy.

