#!/usr/bin/env bash
# vyos-sflow v1.2.12: offline install/upgrade; never downloads or executes input.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/common.sh"
root= binary= environment=
while (($#)); do
    case $1 in
        --root) root=${2:?--root requires a path}; shift 2 ;;
        --environment) environment=${2:?--environment requires a file}; shift 2 ;;
        --binary) binary=${2:?--binary requires a file}; shift 2 ;;
        --help) printf 'Usage: bash install.sh --binary FILE [--root EXISTING_ABSOLUTE_DIRECTORY]\n'; exit 0 ;;
        *) fail "Unknown argument: $1" ;;
    esac
done
[[ -z $environment || ( -f $environment && ! -L $environment ) ]] || fail '--environment must be a regular file'
[[ -f $binary && -s $binary ]] || fail '--binary must be a nonempty regular file'
setup_root
files=(common.sh install.sh uninstall.sh vyos-sflow.service vyos-sflow-ready.service service.env.example)
for file in "${files[@]}"; do
    [[ -f $here/$file ]] || fail "Missing package file: $file"
    safe_path "$base/packaging/$file"
done
(umask 022; mkdir -p -- "$base/bin" "$base/data" "$base/packaging" "${unit%/*}")
chmod 0700 "$base/data"
if [[ -n $environment ]]; then
    atomic_install "$environment" "$base/service.env" 0600
elif [[ ! -e $base/service.env ]]; then
    atomic_install "$here/service.env.example" "$base/service.env" 0600
fi
ensure_account
was_active=false
if [[ -z $root ]] && systemctl is-active --quiet vyos-sflow.service; then
    systemctl stop vyos-sflow.service
    was_active=true
fi
if [[ -f $base/bin/vyos-sflow ]]; then
    atomic_install "$base/bin/vyos-sflow" "$base/bin/vyos-sflow.previous" 0755
fi
atomic_install "$binary" "$base/bin/vyos-sflow" 0755
for file in "${files[@]}"; do
    mode=0644; [[ $file != *.sh ]] || mode=0755
    atomic_install "$here/$file" "$base/packaging/$file" "$mode"
done
# Remove only the obsolete product restore entry point, never user data.
safe_path "$base/packaging/restore.sh"
rm -f -- "$base/packaging/restore.sh"
install_unit
if [[ -z $root ]]; then
    systemctl daemon-reload
    systemctl enable vyos-sflow.service
    if "$was_active"; then
        systemctl start vyos-sflow.service
        systemctl is-active --quiet vyos-sflow.service || fail 'Updated service is not active; inspect journal'
    fi
fi
printf 'Installed vyos-sflow v1.2.12. Existing environment and data retained.\n'
printf 'Start vyos-sflow.service when ready. First login requires a password change; existing credentials are never reset.\n'
