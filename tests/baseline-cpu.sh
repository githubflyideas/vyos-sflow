#!/usr/bin/env bash
# The binary must run on any x86-64 CPU, not only on ones with AVX2: many
# routers (PC Engines APU, Atom, older Celeron) have none. Runs the binary on
# QEMU's baseline CPU, which has no SSE4 or AVX: it must print its version,
# open a new DuckDB database, answer /healthz and store an sFlow sample.
# Usage: tests/baseline-cpu.sh BINARY   (needs qemu-x86_64 and node)
set -euo pipefail
bin="$(realpath "$1")"
cpu=qemu64
qemu-x86_64 -cpu "$cpu" "$bin" version
data="$(mktemp -d)"
trap 'kill "$pid" 2>/dev/null || true; rm -rf "$data"' EXIT
VYOS_SFLOW_PASSWORD=baseline-cpu-check-1 qemu-x86_64 -cpu "$cpu" "$bin" serve -data "$data" \
  -addr 127.0.0.1:18066 -listen 127.0.0.1:16343 -db-threads 1 >"$data.log" 2>&1 &
pid=$!
for _ in $(seq 1 120); do
  if curl -fsS --max-time 2 http://127.0.0.1:18066/healthz >/dev/null 2>&1; then
    node -e '
      const h = require(process.argv[1]);
      (async () => {
        await h.send([h.sflow(1)], 16343);
        await h.waitFor(async () => (await h.request("http://127.0.0.1:18066/api/overview?range=1h",
          {auth: h.basic("baseline-cpu-check-1")})).totals.wire === 78 * 100, "no sample stored", {});
      })().catch(e => { console.error(e); process.exit(1); });' "$(realpath "$(dirname "$0")/runtime-helpers.cjs")"
    echo "PASS runs on a baseline x86-64 CPU ($cpu): version, new database, health, sFlow stored"
    exit 0
  fi
  kill -0 "$pid" 2>/dev/null || { cat "$data.log"; echo "FAIL the service died on a baseline x86-64 CPU"; exit 1; }
  sleep 1
done
cat "$data.log"
echo "FAIL the service did not become healthy on a baseline x86-64 CPU"
exit 1
