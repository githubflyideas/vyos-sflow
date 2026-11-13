# Optional installation and configuration

Start with the README's direct-execution steps. `./vyos-sflow serve` creates `./vyso_sflow_data` in the current directory. Run from the same directory to keep accounts and history. The interactive Web address prompt accepts a private local address, `0.0.0.0` (or `::`) for all interfaces, or Enter for loopback only (`127.0.0.1`); it does not change the sFlow listener. Without a terminal there is no prompt and the Web UI listens on `127.0.0.1:8066`.

**Compatibility:** up to 1.2.5, Enter and noninteractive starts bound the Web UI to all interfaces (`0.0.0.0:8066`). Scripts or wrappers that relied on that must now pass `-addr 0.0.0.0:8066` (or a private IP) or set `VYOS_SFLOW_ADDR`. Installed services are unaffected: their `service.env` sets `VYOS_SFLOW_ADDR` explicitly. The sFlow default `0.0.0.0:6343` is unchanged.

## Install for automatic startup

Use the optional offline installer on Linux amd64 with systemd, Bash, curl, administrator access and standard file/account utilities:

```sh
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.2.13/vyos-sflow-1.2.13-linux-amd64.run &&
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.2.13/SHA256SUMS &&
sha256sum --ignore-missing -c SHA256SUMS &&
sudo bash vyos-sflow-1.2.13-linux-amd64.run
```

Confirm with `y`. The installer enables the service and checks readiness. It does not configure native sampling or firewall rules. On a separate Linux host, configure the router to send sFlow to that host's address, UDP `6343`.

The simple wizard refuses nonempty existing data directories or custom configuration. It never resets accounts or clears history. Review an existing installation and back it up before upgrading.

## Paths and settings

| Mode | Data directory |
| --- | --- |
| Direct execution | `./vyso_sflow_data` relative to the launch directory |
| Installed service | `/config/vyos-sflow/data` |

For direct use, override paths/listeners with `-data`, `-addr` and `-listen`. Explicit options suppress the Web address prompt; noninteractive startup never waits for input.

For the installed service, edit `/config/vyos-sflow/service.env`:

```ini
VYOS_SFLOW_ADDR=0.0.0.0:8066
VYOS_SFLOW_LISTEN=0.0.0.0:6343
VYOS_SFLOW_DATA=/config/vyos-sflow/data
VYOS_SFLOW_DB_THREADS=1
```

This is a systemd EnvironmentFile, not a shell script. Restart after editing:

```sh
sudo systemctl restart vyos-sflow.service
```

## Private access and TLS

The shipped service template binds both listeners to all interfaces; direct execution keeps the Web UI on loopback unless you choose an address. Bind to private addresses or restrict access with a firewall. Any reachable sender can submit sFlow; header identity is not authentication.

For HTTPS, set `VYOS_SFLOW_TLS_CERT` and `VYOS_SFLOW_TLS_KEY` together to valid files readable by the service account. Alternatively, tunnel the Web port from your computer:

```sh
ssh -N -L 8066:127.0.0.1:8066 SSH_USER@ROUTER_IP
```

Then open `http://127.0.0.1:8066`. Adapt addresses for custom listener settings.

Behind a reverse proxy, pass the browser's host name on: with nginx, `proxy_set_header Host $host;` (or `proxy_set_header X-Forwarded-Host $host;`). Changes such as passwords and users are refused when the request's origin does not match the host the service sees.

## Existing installations and backups

Stop the process/service before backing up the complete data directory and service configuration. Keep account files and Parquet history together and keep backups private.

For an existing installation, extract the new complete tar package and run from its directory:

```sh
sudo systemctl stop vyos-sflow.service
# Back up the actual data directory and service.env before proceeding.
sudo bash packaging/install.sh --binary ./vyos-sflow
sudo systemctl start vyos-sflow.service
```

Existing accounts and configuration remain intact. Legacy sender-selection environment variables are ignored; restrict network access using listener addresses and firewall policy. Historical stream metadata is retained, not rewritten to label old records as new data.

A database without the 48-hour retention policy needs explicit `VYOS_SFLOW_ACCEPT_48H_RETENTION=true` before older flow history may expire. Review and back up before enabling it; this is not permission to clear all history. Never delete data merely to bypass an upgrade warning.

Binary replacement does not roll back database changes. Recovery after a VyOS image upgrade is not supported; plan OS changes separately.

Since 1.2.10 the password file records who the administrator is. Going back to 1.2.9 or older uses the old rule again (a user named `admin`, else the first user), which can make another user the administrator until you upgrade again.

## Resources and troubleshooting

The installed service runs as the unprivileged `vyos-sflow` account. Initial systemd limits are `MemoryHigh=768M` and `MemoryMax=1G`, not measured memory requirements or throughput guarantees. Leave resources for routing and monitor disk use.

```sh
sudo systemctl status vyos-sflow.service
sudo journalctl -u vyos-sflow.service -n 50 --no-pager
curl --fail http://127.0.0.1:8066/healthz
```

Adapt the health URL for custom listeners/HTTPS. Healthy HTTP does not prove samples are arriving: check Sources, exporter destination and firewall rules. For direct execution, read the terminal output.

Uninstall with `sudo bash /config/vyos-sflow/packaging/uninstall.sh`. Data, accounts and configuration are retained; native sampling and firewall settings are unchanged.

See [service management](SERVICE-LIFECYCLE.md) and [compatibility](VALIDATION.md).
