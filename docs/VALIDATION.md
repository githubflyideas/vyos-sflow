# Compatibility and validation

## Supported environment

- Linux amd64 (x86_64).
- Direct executable use requires a writable data directory; systemd is optional.
- Managed installation requires Bash, systemd, administrator access and standard Linux file/account utilities.
- sFlow v5 input, with embedded DuckDB/Parquet storage and rolling 48-hour history.

## Release checks

The release pipeline checks Go code and race-sensitive packages, frontend regressions, archive validation, installer protections, real UDP ingestion, DuckDB history, authentication and Chromium first-login behavior. It exercises the packaged installer on a disposable systemd host and inside a native VyOS live-ISO guest. Release artifacts include SHA256SUMS.

The native guest test checks installation, genuine hsflowd samples and history preservation across service restart. It does not establish disk-installed OS reboot or image-upgrade recovery.

## Operational limits

- Recovery after VyOS image upgrades is not supported. Back up before OS changes.
- Hardware forwarding throughput, VPP and ARM64 have not been validated.
- UDP can lose packets. Traffic totals are sample-derived estimates, not exact interface counters.
- Memory limits are safeguards, not measured throughput guarantees. Leave resources for routing and monitor disk use.
- Maintain independent backups; full recovery from every disk-full/storage-failure scenario is not guaranteed.

## Development checks

Linux development uses Go 1.27.4, CGO, a C/C++ compiler and Node.js 22. VM acceptance additionally needs QEMU, p7zip and Expect.

```sh
go test ./...
go vet ./...
go build -trimpath -ldflags "-X main.commit=$(git rev-parse HEAD)" -o vyos-sflow ./cmd/vyos-sflow
./vyos-sflow version
node tests/e2e.cjs ./vyos-sflow
bash packaging/test-install.sh
node --test tests/self_installer_test.cjs
```

The version number comes from `cmd/vyos-sflow/startup.go`; the commit is only known when passed with `-ldflags "-X main.commit=..."`, otherwise `version` prints `(unknown)`. `./vyos-sflow version` should print the checked-out commit, for example `vyos-sflow 1.2.15 (<commit>)`. Release binaries are additionally linked statically; the exact command is the "Build static Linux release candidate" step in `.github/workflows/ci.yml`. Release archives store files with owner and group `0`/`root`.

See [VM acceptance](VYOS-VM-TEST.md). Run installation tests only on disposable hosts.
