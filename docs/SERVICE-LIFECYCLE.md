# Service management

The installer enables `vyos-sflow.service`. The service runs as the unprivileged `vyos-sflow` account and stores data under `/config/vyos-sflow/data`.

## Common commands

```sh
sudo systemctl status vyos-sflow.service
sudo systemctl stop vyos-sflow.service
sudo systemctl start vyos-sflow.service
sudo systemctl restart vyos-sflow.service
```

After starting, check `http://127.0.0.1:8066/healthz` with the default HTTP configuration. Adapt the address and TLS options if you changed them. A systemd active state alone does not mean HTTP/database startup has finished.

## Startup readiness

`vyos-sflow-ready.service` waits for the persistent environment file, executable and data directory before the collector starts. It waits at most 300 checks, with a systemd timeout of 310 seconds. Missing files cause a visible failure rather than an unlimited restart loop.

Inspect both services when startup fails:

```sh
sudo journalctl -b -u vyos-sflow-ready.service -u vyos-sflow.service --no-pager
sudo systemctl status vyos-sflow-ready.service vyos-sflow.service
```

Check mounted storage, paths and permissions. After repairing the cause:

```sh
sudo systemctl reset-failed vyos-sflow-ready.service vyos-sflow.service
sudo systemctl start vyos-sflow.service
```

## Backup and removal

Stop the service before backing up configuration and the complete data directory. Uninstall with:

```sh
sudo bash /config/vyos-sflow/packaging/uninstall.sh
```

Uninstall retains data, accounts and configuration. Recovery after a VyOS image upgrade is not supported. See [compatibility and validation](VALIDATION.md) for tested lifecycle scenarios.
