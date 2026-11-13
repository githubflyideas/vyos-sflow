# Native VyOS acceptance

The Node.js harness uses Expect for the private serial console and a statically compiled Go guest helper. These are test tools, not analyzer runtime dependencies.

On a disposable Linux runner, install QEMU, p7zip, Expect, Go and Node.js 22. Then run:

```sh
go test ./tests/vmguest
node --test tests/vyos_vm_test.cjs
node tests/vyos_vm.cjs \
  --artifact dist/vyos-sflow-1.2.13-linux-amd64.run \
  --work-dir "$RUNNER_TEMP" --evidence evidence/vyos-vm \
  --timeout 1800
```

## Checks

- Verify the pinned official VyOS ISO and boot an isolated guest.
- Configure native hsflowd through the VyOS interface.
- Observe real flow headers with a temporary probe, then close its socket before installation. Header identity is diagnostic, not a receiver restriction.
- Verify and execute the original release installer.
- Exercise first-login password change, native samples, persisted history and service restart.

The harness records artifact/ISO hashes, stage results and credential-safe diagnostics in `result.json`. It does not record private passwords, authorization headers or sampled packet payloads. A failed or incomplete run is not a pass.

The guest has no host disk or filesystem mounts. Timeouts and cleanup bound execution. Use disposable runners rather than credential-rich shared hosts.

Live-ISO/service-restart acceptance does not establish disk-installed OS reboot, image upgrades or hardware performance. See [compatibility](VALIDATION.md).
