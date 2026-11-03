#!/usr/bin/env bash
# Compatibility wrapper; canonical CI/release entry point is packaging/test-install.sh.
set -euo pipefail
repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
exec bash "$repo/packaging/test-install.sh" "$@"
