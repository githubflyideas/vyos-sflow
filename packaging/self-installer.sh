#!/usr/bin/env bash
# Generated offline wrapper. Payload is a trusted CI archive, not a download.
set -euo pipefail
umask 077
fail() { printf 'vyos-sflow: %s\n' "$*" >&2; exit 1; }
mode=install
case ${1-} in
    --help) printf 'Usage: sudo bash FILE.run\n       bash FILE.run --check\nOffline Linux x86_64 or arm64 (aarch64) installation. --check verifies and extracts privately, without installing.\n'; exit 0 ;;
    --check) mode=check; shift ;;
    '') ;;
    *) fail 'Unknown option; use --help' ;;
esac
(($# == 0)) || fail 'Unexpected arguments'
for cmd in mktemp tail sha256sum tar rm mkdir; do
    command -v "$cmd" >/dev/null || fail "Required command missing: $cmd"
done
tmp=$(mktemp -d "${TMPDIR:-/tmp}/vyos-sflow.XXXXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
tail -n +@PAYLOAD_LINE@ -- "${BASH_SOURCE[0]}" > "$tmp/payload.tar.gz"
printf '%s  %s\n' '@SHA256@' "$tmp/payload.tar.gz" | sha256sum -c - >/dev/null || fail 'Payload checksum mismatch; do not install'
mkdir "$tmp/content"
tar -xzf "$tmp/payload.tar.gz" -C "$tmp/content" --no-same-owner --no-same-permissions
package="$tmp/content/@ROOT@"
[[ -s $package/vyos-sflow && -f $package/packaging/install.sh ]] || fail 'Incomplete release archive'
if [[ $mode == check ]]; then
    printf 'Payload SHA256 and extraction verified; no installation performed.\n'
    exit 0
fi
bash "$package/packaging/setup.sh"
exit $?
