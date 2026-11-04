# vyos-sflow operator manual

## First use

Follow the three steps in the repository README: configure your exporter, extract and run the executable, and open the dashboard. Sign in with the initial account and change its password before using analytics.

## Read your traffic

- **Overview:** the top indicators describe all traffic in the selected range. The chart can show all traffic, inbound or outbound. Direction is relative to the exporter/interface shown beside the chart, selected as the busiest interface in that range; it is not automatically the WAN interface and may change with the range.
- **Top 66:** the main talkers show the top 30 clients and top 30 servers side by side, or a table by host, application or protocol. Client/server roles are inferred, not necessarily the packet source and destination. Both lists describe the same traffic from different perspectives: do not add their totals. The all-traffic total includes entries outside the displayed ranking.
- **Flow records:** the newest records, at most 500 over at most 24 hours, optionally for one host. The page shows its effective time window even when the global selection is 48 hours. Traffic and packet counts are estimates; the multiplier includes sampling/loss compensation and is not the exporter's configured sampling ratio.
- **Sources:** received samples, configured stream identity and reception diagnostics.

Select a time range from 15 minutes to 48 hours. The default is one hour. Traffic values are estimates derived from samples, not exact interface counters. Port/application labels describe service ports rather than deep packet inspection. Choose your preferred interface language from the language selector.

## Receiving samples

The default UDP listener is `0.0.0.0:6343`. Configure the exporter to send sFlow v5 to this machine; on the same VyOS router use `127.0.0.1:6343`. No sender-IP or sub-agent setup is required. Header identities remain available for diagnostics and do not authenticate senders. Restrict UDP access to trusted devices.

## Temporary terminal view

Run `./vyos-sflow tui` over SSH for a one-command session. If no local service is running, it starts a temporary collector, stores session data in a private temporary directory and connects the TUI through a loopback-only API. It still receives sFlow on UDP `0.0.0.0:6343`; configure sampling as described in the README and restrict UDP access to trusted devices.

Press `q` or `Ctrl+C` to exit. The session's collector stops and its temporary data is removed; this does not clear existing persistent history or accounts. If TUI connected to an already running service, that service continues running after you quit. For an explicit service address and account, use `./vyos-sflow tui -url http://ROUTER_IP:8066 -user admin`. Existing account restrictions still apply.

Use `serve` or an installed service for continuous collection and rolling 48-hour history between viewing sessions.

## Clear history

Administrators can clear all historical flow records and summaries through the cleanup control. Confirm both prompts. This also clears records from the current 48-hour window. Accounts, configuration and existing metadata remain intact. Back up first: this operation cannot be undone by the application.

## Data and accounts

For the README foreground command, history and accounts are in `./vyso_sflow_data` relative to the directory where you launch the process. Keep that directory when restarting or replacing the executable. An installed service uses `/config/vyos-sflow/data` by default. History expires automatically after 48 hours. Stop the service before backing up the entire directory, including account files and Parquet data. Keep backups private.

The installer refuses nonempty existing data directories. Follow the installation guide for upgrades; never delete a data directory merely to get past a warning. Existing accounts are not reset. Restart after changing a password with the command-line tool to revoke existing sessions.

## Troubleshooting

Healthy HTTP alone does not prove samples are arriving. Check Sources, exporter destination and firewall rules when charts remain empty. For direct execution, inspect output in the running terminal. For an installed service, check status and logs:

```sh
sudo systemctl status vyos-sflow.service
sudo journalctl -u vyos-sflow.service -n 50 --no-pager
```

Web and UDP listeners use all interfaces by default. Restrict management access to a private network; HTTP is unencrypted and TLS is optional. See the installation guide for configuration and the validation document for compatibility limits.
