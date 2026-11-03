#!/usr/bin/env bash
# Deliberately no --purge: data, auth state, environment and account survive.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/common.sh"
root=
while (($#)); do
    case $1 in
        --root) root=${2:?--root requires a path}; shift 2 ;;
        --help) printf 'Usage: bash uninstall.sh [--root EXISTING_ABSOLUTE_DIRECTORY]\nData, environment, service account and operator drop-ins are retained.\n'; exit 0 ;;
        *) fail "Unknown argument: $1" ;;
    esac
done
setup_root
if [[ -z $root && -f $unit ]]; then
    systemctl stop vyos-sflow.service
    systemctl disable vyos-sflow.service
fi
if [[ -z $root && -f $ready_unit ]]; then
    systemctl stop vyos-sflow-ready.service
fi
rm -f -- "$unit" "$ready_unit" "$base/bin/vyos-sflow" "$base/bin/vyos-sflow.previous"
# Remove only known product files, never recursively remove operator content.
for file in common.sh install.sh restore.sh uninstall.sh vyos-sflow.service vyos-sflow-ready.service service.env.example; do
    safe_path "$base/packaging/$file"
    rm -f -- "$base/packaging/$file"
done
if [[ -z $root ]]; then systemctl daemon-reload; fi
printf 'Uninstalled vyos-sflow. Data, environment, account and operator drop-ins retained.\n'
