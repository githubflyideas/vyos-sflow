#!/usr/bin/env bash
# Beginner path: offline defaults only. Advanced layouts use install.sh directly.
set -euo pipefail
umask 077
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
fail() { printf 'vyos-sflow: %s\n' "$*" >&2; exit 1; }
(($# == 0)) || fail 'This wizard takes no arguments; use install.sh for advanced installation'
printf '%s\n' \
    'vyos-sflow 1.2.0 offline installation' \
    'Install in /config/vyos-sflow and enable/start its systemd service.' \
    'Existing configuration, data and accounts are retained; passwords are never reset.' \
    'New installs: Web 0.0.0.0:8066; sFlow UDP 0.0.0.0:6343 (all interfaces).' \
    'Native sFlow, firewall rules and VyOS boot hooks will NOT be changed.' \
    'No downloads. Review the publisher and checksum before continuing.'
printf 'Continue? [y/N] '
read -r answer || fail 'Confirmation required; nothing installed'
answer=${answer%$'\r'}
case $answer in y|Y|yes|YES) ;; *) printf 'Cancelled; nothing installed.\n'; exit 0 ;; esac
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || fail 'Installation requires Linux x86_64; --check is safe on other platforms'
[[ $(id -u) == 0 ]] || fail 'Run with sudo bash FILE.run (root required)'
[[ -d /run/systemd/system ]] || fail 'A running systemd host is required'
for cmd in systemctl curl cmp cat grep getent groupadd useradd id cut chown chmod install mv mktemp mkdir rm dirname sleep; do
    command -v "$cmd" >/dev/null || fail "Missing dependency: $cmd. Install it using your OS package manager, then retry."
done
base=/config/vyos-sflow
# Deliberately do not interpret EnvironmentFile syntax or execute its contents.
# Exact default comparison is conservative: even harmless edits use advanced mode.
# Frozen 0.1.0 template: compare trusted literal bytes, never parse user input.
legacy_defaults() {
    cat <<'LEGACY_ENV'
# systemd EnvironmentFile syntax, NOT a shell script. Never source this file.
VYOS_SFLOW_ADDR=127.0.0.1:8066
VYOS_SFLOW_LISTEN=127.0.0.1:6343
VYOS_SFLOW_DATA=/config/vyos-sflow/data
VYOS_SFLOW_RETENTION_DAYS=7
VYOS_SFLOW_MEMORY_FRACTION=0.05
VYOS_SFLOW_DB_THREADS=1
# First login requires changing the default password; existing accounts are retained.
# Do not persist an initial plaintext VYOS_SFLOW_PASSWORD here.
LEGACY_ENV
}
# Conservatively route every nonempty data directory to advanced migration.
if [[ -e $base/data || -L $base/data ]]; then
    [[ -d $base/data && ! -L $base/data ]] || fail 'Unsafe data path; use advanced migration; nothing changed'
    shopt -s nullglob dotglob
    data_entries=("$base/data"/*)
    ((${#data_entries[@]} == 0)) || fail 'Existing data requires advanced migration before upgrade; nothing changed. Back up history and accounts. Use a new data directory or separately authorize old-flow cleanup; retention consent is NOT permission to clear all history.'
fi
legacy=false
if [[ -e $base/service.env || -L $base/service.env ]]; then
    [[ -f $base/service.env && ! -L $base/service.env ]] || fail 'Unsafe service.env; use advanced installation'
    if cmp -s "$base/service.env" "$here/service.env.example"; then
        :
    elif cmp -s "$base/service.env" <(legacy_defaults); then
        legacy=true
    else
        fail 'Custom service.env detected; nothing changed. Use the advanced tar.gz + packaging/install.sh path and verify your own listen address/data layout.'
    fi
fi
[[ ! -e $base/systemd-override.conf && ! -L $base/systemd-override.conf && ! -e /etc/systemd/system/vyos-sflow.service.d && ! -L /etc/systemd/system/vyos-sflow.service.d ]] || fail 'Custom service override detected; use the advanced install path; nothing changed'
existing=false
[[ ! -e $base/data && ! -e $base/service.env && ! -e $base/bin/vyos-sflow ]] || existing=true
printf '%s\n' 'sFlow is accepted from all senders; restrict UDP access with a firewall.' 'The latest rolling 48 hours of flow history are retained.'
candidate=$(mktemp)
trap 'rm -f -- "$candidate"' EXIT
# Only trusted templates are rendered. Existing EnvironmentFiles are never parsed.
if "$legacy"; then
    legacy_defaults | grep -v '^VYOS_SFLOW_RETENTION_DAYS=' > "$candidate"
else
    cat "$here/service.env.example" > "$candidate"
fi
bash "$here/install.sh" --binary "$here/../vyos-sflow" --environment "$candidate"
systemctl start vyos-sflow.service || fail 'Service start failed. Inspect: sudo journalctl -u vyos-sflow.service -n 80 --no-pager'
healthy=false
for ((attempt=0; attempt<30; attempt++)); do
    if systemctl is-active --quiet vyos-sflow.service && \
        body=$(curl --disable --noproxy '*' --proto '=http' --fail --silent --show-error --connect-timeout 1 --max-time 2 http://127.0.0.1:8066/healthz 2>/dev/null) && \
        printf '%s\n' "$body" | grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"[[:space:]]*[,}]'; then
        healthy=true; break
    fi
    sleep 1
done
"$healthy" || fail 'Health check failed; installation is NOT verified. Data retained. Inspect: sudo systemctl status vyos-sflow.service; sudo journalctl -u vyos-sflow.service -n 80 --no-pager'
printf '\nService active; loopback /healthz verified.\n'
if "$existing"; then
    printf 'Existing installation/data detected: accounts preserved, password not reset. Use your existing credentials.\n'
else
    printf 'First installation default account: admin\nDefault password: tryvyossflow66\nYou MUST change this password on first login.\n'
fi
if "$legacy"; then
    printf 'Upgrade retained loopback listeners; use the SSH tunnel below for remote access.\n'
else
    printf 'Open http://ROUTER_IP:8066 in your browser; replace ROUTER_IP with the reachable management IP.\n'
fi
printf '%s\n' \
    'Security: bind to a private interface and restrict access with a firewall; do not expose to the Internet.' \
    'TLS is optional; configure a certificate and key when needed.' \
    'For a loopback-only installation or optional encrypted access, open an SSH tunnel on YOUR computer:' \
    '  ssh -N -L 8066:127.0.0.1:8066 SSH_USER@ROUTER_IP' \
    'Then open http://127.0.0.1:8066 in your local browser.' \
    'This confirms service health, not incoming sFlow traffic. Configure sampling separately.'
