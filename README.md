<p align="center">
  <img src="docs/images/vyos-sflow-logo.png" alt="vyos-sflow logo" width="360">
</p>

# vyos-sflow

Local sFlow traffic analysis for VyOS and Linux. View bandwidth, top talkers and recent flow records in your browser or over SSH.

[Quick start](#get-started-in-3-steps) · [Terminal view](#temporary-terminal-view-tui) · [Screenshots](#screenshots) · [User manual](docs/MANUAL.md) · [Service installation](docs/VYOS.md)

| Feature | Details |
| --- | --- |
| Web | Traffic trends, client/server rankings and recent flow records |
| TUI | One command for a temporary view over SSH |
| History | The latest 48 hours, stored with DuckDB™ |
| UI text | 15 interface languages |
| Build | Linux amd64, including VyOS |

## Get started in 3 steps

### 1. Enable traffic sampling

Connect to the router over SSH. Replace `eth0` with the interface you want to monitor and `ROUTER_IP` with that interface's IPv4 address. Find it with `ip -4 addr show dev eth0`. Skip this step if sFlow already sends to `127.0.0.1:6343`.

```sh
configure
set system sflow interface eth0
set system sflow agent-address ROUTER_IP
set system sflow sampling-rate 1000
set system sflow polling 30
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

### 2. Download and run

In a writable directory on the router:

```sh
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.2.2/vyos-sflow-1.2.2-linux-amd64.tar.gz &&
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.2.2/SHA256SUMS &&
sha256sum --ignore-missing -c SHA256SUMS &&
tar -xzf vyos-sflow-1.2.2-linux-amd64.tar.gz &&
cd vyos-sflow-1.2.2-linux-amd64 &&
./vyos-sflow serve
```

Press **Enter** at the Web IP prompt to use all interfaces, or enter a private IP of this machine.

### 3. Open the dashboard

Open **`http://ROUTER_IP:8066`** in your browser. Sign in with **`admin` / `tryvyossflow66`**, change the password, then open **Overview** or **Top**. Charts fill as sampled traffic arrives; **Sources** helps diagnose an empty chart.

**Stop and restart:** keep this terminal or SSH session open while running. Press `Ctrl+C` to stop; run `./vyos-sflow serve` from the same directory to continue. Accounts and history are stored in `./vyso_sflow_data` — keep this folder.

> [!WARNING]
> Web TCP `8066` and sFlow UDP `6343` listen on all interfaces by default. Bind the Web UI to a private interface and restrict access with your firewall. HTTP is unencrypted; [TLS is optional](docs/VYOS.md).

## Temporary terminal view (TUI)

For a quick look over SSH, run one command:

```sh
./vyos-sflow tui
```

- **No local service running:** starts a temporary sFlow collector and opens the terminal dashboard. No separate `serve` command, browser or service installation is needed.
- **Local service already running:** connects to it using its local credentials instead of starting a second collector.
- **Quit with `q` or `Ctrl+C`:** closes the TUI and any temporary collector it started. An existing service keeps running.

Standalone TUI history lasts only for that session. Its temporary data is removed on exit; existing accounts and `./vyso_sflow_data` are not cleared. Keep using `serve` or the optional system service when you want continuous collection and persistent history.

Press `?` for keyboard help. For an explicitly selected service, use `-url http://ROUTER_IP:8066 -user admin`; the password is prompted without echoing. Explicit connection options never start a replacement collector if the connection fails.

## Reading the numbers

| View | What the numbers mean |
| --- | --- |
| Overview | Top indicators cover all traffic in the selected range. Inbound/outbound chart directions are relative to the displayed exporter and interface, not automatically a WAN link. |
| Top 66 | The top 30 clients and top 30 servers are two perspectives on the same traffic. Roles are inferred; do not add the two lists together. The all-traffic total also includes entries outside the ranking. |
| Flow records | Up to 500 recent records over at most 24 hours, even with a global 48-hour selection. The page shows the effective window. The estimated multiplier includes sampling/loss compensation; it is not the router's configured sampling ratio. |

## Screenshots

These earlier captures show the application running, rather than UI mockups. Current labels and layout may differ.

### Web dashboard

Native VyOS capture: the router's own sFlow samples real downloads at 1 in 1000. The Overview capture displays v1.1.2, running on a VyOS 2026.10.30 rolling live ISO in a VM.

![Overview on VyOS, earlier v1.1.2 capture](docs/images/screenshots/vyos/web-overview.png)

<details>
<summary>View client/server rankings and recent flow records</summary>

**Client and server rankings**

![Top 66 main talkers on VyOS, earlier capture](docs/images/screenshots/vyos/web-top.png)

**Recent flow records**

![Flow records on VyOS, earlier capture](docs/images/screenshots/vyos/web-records.png)

</details>

### Terminal UI

A real Linux terminal running v1.1.2 with synthetic sFlow sent to the collector:

![Terminal UI showing Top hosts](docs/images/screenshots/v1.1.2/tui-top.png)

## Optional setup

Want automatic startup? [Install as a service](docs/VYOS.md). Otherwise, keep running the executable directly.

[Configuration](docs/VYOS.md) · [User manual](docs/MANUAL.md) · [Compatibility and validation](docs/VALIDATION.md)

## License and DuckDB trademark

**Apache License 2.0** — see [LICENSE](LICENSE). Third-party licenses and notices are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

**DuckDB is a trademark of the DuckDB Foundation.** This project uses DuckDB as its embedded database. It is not an official DuckDB product and is not affiliated with, sponsored by or endorsed by the DuckDB Foundation. DuckDB is distributed under the MIT license; software licenses do not grant rights to DuckDB trademarks or logos.
