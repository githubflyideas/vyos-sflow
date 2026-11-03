#!/usr/bin/env bash
# Filesystem acceptance tests. No root, daemon, container or real binary needed.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "$repo/packaging/.install-test.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT
root="$work/root with spaces"
mkdir -p "$root" "$work/forbidden-bin"
export INSTALL_TEST_FORBIDDEN_CALL="$work/forbidden-call"
for command in systemctl useradd groupadd chown getent; do
    printf '#!/usr/bin/env bash\nprintf forbidden > "$INSTALL_TEST_FORBIDDEN_CALL"\nexit 99\n' > "$work/forbidden-bin/$command"
    chmod +x "$work/forbidden-bin/$command"
done
export PATH="$work/forbidden-bin:$PATH"
printf 'fixture-binary-one\n' > "$work/binary"
bash "$repo/packaging/install.sh" --root "$root" --binary "$work/binary"
cmp "$work/binary" "$root/config/vyos-sflow/bin/vyos-sflow"
test -f "$root/config/vyos-sflow/service.env"
test -f "$root/etc/systemd/system/vyos-sflow.service"
cmp "$repo/packaging/vyos-sflow-ready.service" "$root/etc/systemd/system/vyos-sflow-ready.service"
cmp "$repo/packaging/vyos-sflow-ready.service" "$root/config/vyos-sflow/packaging/vyos-sflow-ready.service"
test -d "$root/config/vyos-sflow/data"
test ! -e "$root/config/vyos-sflow/packaging/restore.sh"
printf 'PASS: staged installation layout\n'
# Existing config is opaque: not evaluated, replaced, or printed.
printf 'VYOS_SFLOW_DATA=/config/vyos-sflow/data\n# sentinel secret\nEVIL=$(touch %s)\n' "$work/executed" > "$root/config/vyos-sflow/service.env"
cp "$root/config/vyos-sflow/service.env" "$work/env.expected"
printf 'password-state\n' > "$root/config/vyos-sflow/data/auth.json"
cp "$work/binary" "$work/previous.expected"
printf 'fixture-binary-two\n' > "$work/binary"
bash "$repo/packaging/install.sh" --root "$root" --binary "$work/binary"
cmp "$work/env.expected" "$root/config/vyos-sflow/service.env"
cmp "$work/previous.expected" "$root/config/vyos-sflow/bin/vyos-sflow.previous"
cmp "$work/binary" "$root/config/vyos-sflow/bin/vyos-sflow"
test "$(<"$root/config/vyos-sflow/data/auth.json")" = password-state
test ! -e "$work/executed"
printf 'PASS: upgrade preserves environment, password state and previous binary\n'
# Preserve operator boot hooks; image-upgrade restore is not supported.
mkdir -p "$root/config/scripts"
printf '#!/bin/bash\nprintf existing-hook\\n\n' > "$root/config/scripts/vyos-postconfig-bootup.script"
cp "$root/config/scripts/vyos-postconfig-bootup.script" "$work/hook.expected"
bash "$repo/packaging/uninstall.sh" --root "$root"
test ! -e "$root/etc/systemd/system/vyos-sflow.service"
test ! -e "$root/etc/systemd/system/vyos-sflow-ready.service"
test ! -e "$root/config/vyos-sflow/packaging/vyos-sflow-ready.service"
test ! -e "$root/config/vyos-sflow/bin/vyos-sflow"
test -f "$root/config/vyos-sflow/data/auth.json"
cmp "$work/env.expected" "$root/config/vyos-sflow/service.env"
cmp "$work/hook.expected" "$root/config/scripts/vyos-postconfig-bootup.script"
bash "$repo/packaging/uninstall.sh" --root "$root"
printf 'PASS: uninstall is idempotent and retains data/configuration\n'
# A directory at the binary path must fail before installing anything.
mkdir -p "$root/config/vyos-sflow/bin/vyos-sflow"
if bash "$repo/packaging/install.sh" --root "$root" --binary "$work/binary" > "$work/rejected.log" 2>&1; then
    printf 'FAIL: directory accepted as installed binary\n' >&2; exit 1
fi
rmdir "$root/config/vyos-sflow/bin/vyos-sflow"
printf 'PASS: refuses directory masquerading as binary\n'
for entry in install uninstall; do
    args=(); [[ $entry != install ]] || args=(--binary "$work/binary")
    if bash "$repo/packaging/$entry.sh" --root relative "${args[@]}" > "$work/rejected.log" 2>&1; then
        printf 'FAIL: relative staging root accepted\n' >&2; exit 1
    fi
    if bash "$repo/packaging/$entry.sh" --root / "${args[@]}" > "$work/rejected.log" 2>&1; then
        printf 'FAIL: live root accepted as staging\n' >&2; exit 1
    fi
done
printf 'PASS: all lifecycle entry points reject relative and live staging roots\n'
bash "$repo/packaging/install.sh" --root "$root" --binary "$work/binary"
cp "$root/config/vyos-sflow/bin/vyos-sflow" "$work/installed.expected"
if bash "$repo/packaging/install.sh" --root "$root" --binary "$work/missing" > "$work/rejected.log" 2>&1; then
    printf 'FAIL: missing binary accepted\n' >&2; exit 1
fi
cmp "$work/installed.expected" "$root/config/vyos-sflow/bin/vyos-sflow"
printf 'PASS: invalid upgrade leaves installed binary unchanged\n'
if [[ $(uname -s) == Linux ]]; then
    test "$(stat -c %a "$root/config")" = 755
    test "$(stat -c %a "$root/config/vyos-sflow/service.env")" = 600
    test "$(stat -c %a "$root/config/vyos-sflow/data")" = 700
    test "$(stat -c %a "$root/config/vyos-sflow/bin/vyos-sflow")" = 755
    printf 'PASS: Linux mode bits\n'
    mkdir -p "$work/escape" "$work/linked-root"
    ln -s "$work/escape" "$work/linked-root/config"
    if bash "$repo/packaging/install.sh" --root "$work/linked-root" --binary "$work/binary" > "$work/rejected.log" 2>&1; then
        printf 'FAIL: symlink escape accepted\n' >&2; exit 1
    fi
    test ! -e "$work/escape/vyos-sflow"
    ln -s "$work/installed.expected" "$root/config/vyos-sflow/bin/linked-target"
    rm "$root/config/vyos-sflow/bin/vyos-sflow"
    mv "$root/config/vyos-sflow/bin/linked-target" "$root/config/vyos-sflow/bin/vyos-sflow"
    if bash "$repo/packaging/install.sh" --root "$root" --binary "$work/binary" > "$work/rejected.log" 2>&1; then
        printf 'FAIL: symlink binary accepted\n' >&2; exit 1
    fi
    cmp "$work/installed.expected" "$work/binary"
    printf 'PASS: staging symlink escapes and binary links rejected\n'
else
    printf 'SKIP: Unix ownership/mode bits and symlink semantics require Linux CI\n'
fi
test ! -e "$INSTALL_TEST_FORBIDDEN_CALL"
printf 'PASS: staging never calls systemctl, account tools or chown\n'
for script in "$repo"/packaging/*.sh; do bash -n "$script"; done
printf 'PASS: bash syntax (all lifecycle scripts and test)\n'
test "$(sed -n 's/^MemoryMax=//p' "$repo/packaging/vyos-sflow.service")" = 1G
printf 'PASS: service hard memory budget is 1G\n'
grep -Fxq 'ExecStart=/config/vyos-sflow/bin/vyos-sflow serve' "$repo/packaging/vyos-sflow.service"
grep -Fxq 'User=vyos-sflow' "$repo/packaging/vyos-sflow.service"
grep -Fxq 'EnvironmentFile=/config/vyos-sflow/service.env' "$repo/packaging/vyos-sflow.service"
grep -Fxq 'VYOS_SFLOW_ADDR=0.0.0.0:8066' "$repo/packaging/service.env.example"
if grep -Eq '^VYOS_SFLOW_(PASSWORD|ALLOW_REMOTE_HTTP)=' "$repo/packaging/service.env.example"; then
    printf 'FAIL: insecure default environment\n' >&2; exit 1
fi
printf 'PASS: non-root fixed ExecStart and all-interface defaults; no persisted password or remote HTTP exception\n'
