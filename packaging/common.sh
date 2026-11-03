#!/usr/bin/env bash
# Shared implementation; invoked only by this product's lifecycle scripts.
set -euo pipefail
umask 077
fail() { printf 'vyos-sflow: %s\n' "$*" >&2; exit 1; }
# --root is staging ONLY. A rooted install must never affect the live host.
setup_root() {
    if [[ -n $root ]]; then
        [[ $root == /* && $root != / && $root != //* ]] || fail '--root must be an absolute, non-/ path'
        [[ -d $root && ! -L $root ]] || fail '--root must be an existing non-symlink directory'
        root=$(cd -- "$root" && pwd -P)
        [[ $root != / ]] || fail 'Refusing staging at /'
    else
        [[ $(uname -s) == Linux && $EUID == 0 ]] || fail 'Live installation requires Linux and root; use --root for staging'
        command -v systemctl >/dev/null || fail 'systemd is required'
    fi
    base="$root/config/vyos-sflow"
    unit="$root/etc/systemd/system/vyos-sflow.service"
    ready_unit="$root/etc/systemd/system/vyos-sflow-ready.service"
    safe_path "$ready_unit"
    safe_path "$base/bin/vyos-sflow"
    safe_path "$base/bin/vyos-sflow.previous"
    safe_path "$base/data"
    safe_path "$base/service.env"
    safe_path "$base/packaging"
    safe_path "$base/systemd-override.conf"
    safe_path "$unit.d/90-vyos-sflow-local.conf"
    safe_path "$unit"
    local file
    for file in "$base/bin/vyos-sflow" "$base/bin/vyos-sflow.previous" "$base/service.env" "$unit" "$ready_unit"; do
        [[ ! -e $file || -f $file ]] || fail "Expected regular file: $file"
    done
}
# Reject redirected product targets. /config itself may be a VyOS-managed
# symlink on a live router; staging rejects every symlink below its root.
safe_path() {
    local path=$1 component current remainder
    if [[ -n $root ]]; then
        [[ $path == "$root/"* ]] || fail 'Target escapes staging root'
        current=$root; remainder=${path#"$root/"}
    elif [[ $path == /config/vyos-sflow* ]]; then
        current=/config; remainder=${path#/config/}
    else
        current=; remainder=${path#/}
    fi
    while [[ -n $remainder ]]; do
        component=${remainder%%/*}
        [[ $component != . && $component != .. && -n $component ]] || fail 'Invalid target component'
        current="$current/$component"
        [[ ! -L $current ]] || fail "Refusing symlink target: $current"
        if [[ $remainder == */* ]]; then remainder=${remainder#*/}; else remainder=; fi
    done
}
atomic_install() {
    local source=$1 target=$2 mode=$3 temporary
    safe_path "$target"
    [[ ! -e $target || -f $target ]] || fail "Expected regular file: $target"
    temporary=$(mktemp "${target%/*}/.vyos-sflow.XXXXXXXX")
    if ! install -m "$mode" -- "$source" "$temporary"; then
        rm -f -- "$temporary"; fail 'Could not stage file'
    fi
    if ! mv -f -- "$temporary" "$target"; then
        rm -f -- "$temporary"; fail 'Could not replace file'
    fi
}
install_unit() {
    atomic_install "$here/vyos-sflow-ready.service" "$ready_unit" 0644
    atomic_install "$here/vyos-sflow.service" "$unit" 0644
    if [[ -f $base/systemd-override.conf ]]; then
        (umask 022; mkdir -p -- "$unit.d")
        atomic_install "$base/systemd-override.conf" "$unit.d/90-vyos-sflow-local.conf" 0644
    fi
}
ensure_account() {
    [[ -z $root ]] || return 0
    if ! getent group vyos-sflow >/dev/null; then groupadd --system vyos-sflow; fi
    if ! getent passwd vyos-sflow >/dev/null; then
        useradd --system --gid vyos-sflow --home-dir /config/vyos-sflow/data --no-create-home --shell /usr/sbin/nologin vyos-sflow
    fi
    [[ $(id -u vyos-sflow) != 0 && $(id -g vyos-sflow) == "$(getent group vyos-sflow | cut -d: -f3)" ]] || fail 'Existing vyos-sflow account has unsafe uid/group'
    # Never make the program, environment or service definitions writable by
    # the daemon. Only the data directory belongs to the service account.
    chown root:root "$base" "$base/bin" "$base/packaging" "$base/service.env"
    chmod 0755 "$base" "$base/bin" "$base/packaging"
    chmod 0600 "$base/service.env"
    chown vyos-sflow:vyos-sflow "$base/data"
    chmod 0700 "$base/data"
}
