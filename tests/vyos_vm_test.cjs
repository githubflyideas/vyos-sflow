"use strict";
// Offline parser/console fixtures only: these are NOT native VyOS evidence.
const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
let vm;
try {
  vm = require("./vyos_vm.cjs");
} catch (e) {
  if (e.code !== "MODULE_NOT_FOUND") throw e;
}
function temp(t) {
  const d = fs.mkdtempSync(path.join(os.tmpdir(), "vm-test-"));
  t.after(() => fs.rmSync(d, { recursive: true, force: true }));
  return d;
}
function packet(sub = 123456, tag = 1, ipv6 = false) {
  const addr = ipv6
    ? Buffer.from("20010db8000000000000000000000001", "hex")
    : Buffer.from([10, 0, 2, 15]);
  const b = Buffer.alloc(24 + addr.length + 12);
  b.writeUInt32BE(5);
  b.writeUInt32BE(ipv6 ? 2 : 1, 4);
  addr.copy(b, 8);
  [sub, 9, 2000, 1, tag, 4].forEach((x, i) =>
    b.writeUInt32BE(x, 8 + addr.length + i * 4),
  );
  return b;
}
test("ISO selection rejects unsafe, ambiguous and mismatched members", () => {
  const listing = (names) =>
    names.map((n) => "Path = live/" + n + "\nSize = 100").join("\n\n");
  assert.deepEqual(
    vm.bootMembers(
      listing(["vmlinuz", "vmlinuz-a", "initrd.img", "initrd.img-a"]),
    ),
    ["live/vmlinuz-a", "live/initrd.img-a"],
  );
  assert.deepEqual(vm.bootMembers(listing(["vmlinuz", "initrd.img"])), [
    "live/vmlinuz",
    "live/initrd.img",
  ]);
  for (const names of [
    ["../vmlinuz"],
    ["vmlinuz-a", "vmlinuz-b", "initrd.img"],
    ["vmlinuz-a", "initrd.img-b"],
  ])
    assert.throws(() => vm.bootMembers(listing(names)));
  assert.throws(() =>
    vm.bootMembers(
      "Path = live/vmlinuz\nSize = 10\nSymbolic Link = target\n\nPath = live/initrd.img\nSize = 10",
    ),
  );
  for (const arg of [
    "boot=live",
    "components",
    "hostname=vyos",
    "username=live",
    "nopersistence",
    "noautologin",
    "nonetworking",
    "union=overlay",
    "net.ifnames=0",
    "biosdevname=0",
    "console=ttyS0,115200n8",
  ])
    assert.ok(vm.LIVE_BOOT_ARGS.split(" ").includes(arg));
  assert.equal(
    vm.ISO_SHA256,
    "786e33a26a6645d2791aa7fb40980953e4578f45f92c7b1a20f253750989d1f3",
  );
});
test("exact official release digest required", () => {
  const r = {
    assets: [
      {
        name: "test.iso",
        digest: "sha256:" + "a".repeat(64),
        browser_download_url: "url",
      },
    ],
  };
  assert.deepEqual(vm.releaseAsset(r, "test.iso"), ["url", "a".repeat(64)]);
  assert.throws(() => vm.releaseAsset(r, "other.iso"));
  r.assets[0].digest = null;
  assert.throws(() => vm.releaseAsset(r, "test.iso"));
});
test("fixed header parsing never keeps payload and rejects truncation", () => {
  for (const v6 of [false, true]) {
    const b = packet(123456, 1, v6),
      h = vm.decodeNativeDatagram(b);
    assert.equal(h.agent, v6 ? "2001:db8::1" : "10.0.2.15");
    assert.equal(h.sub_agent, 123456);
    assert.equal(h.header_hex.length, v6 ? 80 : 56);
    for (let n = 0; n < b.length; n++)
      assert.throws(() => vm.decodeNativeDatagram(b.subarray(0, n)));
  }
  const flow = vm.decodeNativeDatagram(packet()),
    counter = vm.decodeNativeDatagram(packet(123457, 2));
  assert.deepEqual(
    vm.observedIdentity([counter, flow, flow, flow], "10.0.2.15"),
    { agent: "10.0.2.15", sub_agent: 123456 },
  );
  for (const rows of [
    [counter, counter, counter],
    [flow],
    [flow, flow, { ...flow, sub_agent: 0 }],
    [flow, flow, { ...flow, agent: "10.0.2.16" }],
  ])
    assert.throws(() => vm.observedIdentity(rows, "10.0.2.15"));
  assert.deepEqual(vm.safeDiagnostic({ headers: [flow, counter] }).headers, [
    flow,
    counter,
  ]);
  assert.deepEqual(
    vm.safeDiagnostic({
      headers: [{ ...flow, sample_headers_hex: ["secret"] }],
    }).headers,
    [],
  );
});
test("typed diagnostics and evidence never persist exception or command buffers", (t) => {
  const d = temp(t),
    e = new vm.StageEvidence(d, { status: "FAIL", checks: {} });
  e.stage("first");
  e.stage("second");
  vm.saveGuestFailure(
    e,
    "Authorization: PRIVATE\nVM_DIAGNOSTIC " +
      JSON.stringify({
        code: "native_wait",
        secret: "PRIVATE",
        sources: {
          stream: {
            agent: "10.0.2.15",
            sub_agent: 123456,
            enforced: true,
            configured: true,
            rejected_streams: 42,
          },
          sources: [{ proto: "sFlow v5", records: 3, password: "PRIVATE" }],
        },
        status: { write_errors: 2, last_error: "PRIVATE" },
        http_status: 401,
      }),
    1,
  );
  e.fail(new Error("PRIVATE"));
  const saved = JSON.parse(fs.readFileSync(path.join(d, "result.json")));
  assert.deepEqual(
    saved.stages.map((x) => x.status),
    ["PASS", "FAIL"],
  );
  assert.equal(saved.guest_failure.sources.stream.rejected_streams, 42);
  assert.deepEqual(saved.guest_failure.status, { write_errors: 2 });
  assert.ok(!JSON.stringify(saved).includes("PRIVATE"));
  e.complete();
  assert.equal(
    JSON.parse(fs.readFileSync(path.join(d, "result.json"))).status,
    "PASS",
  );
});
test("login prompts, split PS1, failure stages and log boundary", async (t) => {
  const frames = [
    "Linux\r\nvyos login: ",
    "vyos\r\nPassword: ",
    "\r\nvyos@vyos:~$ ",
    "bash-5.2# ",
    "\x1b[?2004hVM# ",
  ];
  const names = [
    "boot-wait-login",
    "login-wait-password",
    "login-wait-user-shell",
    "sudo-wait-root-shell",
    "console-disable-echo",
  ];
  for (let fail = 0; fail <= frames.length; fail++) {
    const d = temp(t),
      e = new vm.StageEvidence(d, { status: "FAIL", checks: {} });
    let cursor = 0;
    const sent = [];
    const child = {
      sendline: async (x) => sent.push(x),
      expect: async function (pattern, timeout) {
        if (cursor === 0) assert.equal(timeout, 7);
        if (cursor === fail) {
          this.trace?.write("last partial text");
          throw new Error("PRIVATE");
        }
        assert.match(frames[cursor], new RegExp(pattern));
        this.trace?.write(frames[cursor++]);
        return { before: "", match: "" };
      },
    };
    if (fail < frames.length) {
      await assert.rejects(vm.loginConsole(child, e, 7));
      assert.equal(e.result.stage, names[fail]);
      if (!fail) assert.equal(sent.length, 0);
    } else {
      assert.ok((await vm.loginConsole(child, e, 7)) instanceof vm.Console);
      assert.equal(e.result.stage, "console-ready");
      assert.ok(!sent.at(-1).includes("VM# "));
    }
    assert.equal(child.trace, null);
    assert.ok(
      !fs
        .readFileSync(path.join(d, "boot-console.txt"), "utf8")
        .includes("PRIVATE"),
    );
  }
});
test("console consumes prompt, handles CRCRLF and hides failed command output", async (t) => {
  const e = new vm.StageEvidence(temp(t), { checks: {} });
  e.stage("native");
  let sent,
    patterns = [];
  const child = {
    sendline: async (x) => (sent = x),
    expect: async (p) => {
      patterns.push(p);
      return patterns.length === 1
        ? {
            before: '\r\n{"wire":123}\r\n',
            match: sent.match(/__VM_RC_[a-f0-9]+__/)[0] + "0\r\r\n",
          }
        : { before: "", match: "VM# " };
    },
  };
  assert.equal(await new vm.Console(child, e).run("true"), '{"wire":123}');
  assert.match(patterns[1], /VM#/);
  assert.match(
    sent.match(/__VM_RC_[a-f0-9]+__/)[0] + "0\r\r\n",
    new RegExp(patterns[0]),
  );
  child.expect = async () => ({
    before: 'PRIVATE\nVM_DIAGNOSTIC {"code":"guest_check","secret":"PRIVATE"}',
    match: sent.match(/__VM_RC_[a-f0-9]+__/)[0] + "1\r\n",
  });
  await assert.rejects(new vm.Console(child, e).run("PRIVATE command"));
  assert.ok(!JSON.stringify(e.result).includes("PRIVATE"));
});
test("installer checks digest and confirmation-only wizard; tar retains advanced path", async () => {
  let cmds = [];
  const c = { run: async (x) => cmds.push(x) };
  await vm.installProduct(
    c,
    "product.run",
    "a".repeat(64),
    "http://10.0.2.2/bundle",
    "unused",
  );
  assert.ok(cmds.some((x) => x.includes("sha256sum -c")));
  assert.equal(cmds.at(-1), "printf '%s\\n' y | bash /tmp/vyos-vm-package.run");
  assert.ok(!cmds.some((x) => x.includes("tar -x")));
  cmds = [];
  await vm.installProduct(
    c,
    "product.tar.gz",
    "a".repeat(64),
    "url",
    "cd vyos-sflow-1.2.13-linux-amd64 && bash packaging/install.sh",
  );
  assert.ok(
    cmds
      .at(-1)
      .endsWith("cd vyos-sflow-1.2.13-linux-amd64 && bash packaging/install.sh"),
  );
});
test("CLI compatibility and unrestricted stream contract", () => {
  const args = vm.parseArgs([
    "--artifact",
    "product.run",
    "--work-dir",
    "work",
    "--evidence",
    "proof",
    "--timeout",
    "1800",
  ]);
  assert.equal(args.timeout, 1800);
  assert.equal(args["iso-sha256"], vm.ISO_SHA256);
  assert.throws(() => vm.parseArgs(["--artifact", "a", "--timeout", "0"]));
  vm.requireStream({ sources: { stream: { enforced: false } } });
  assert.throws(() =>
    vm.requireStream({ sources: { stream: { enforced: true } } }),
  );
  assert.throws(() => vm.requireStream({}));
});
test("pre-install probe stops exporter even on malformed output", async (t) => {
  for (const malformed of [false, true]) {
    const e = new vm.StageEvidence(temp(t), { checks: {} }),
      commands = [];
    let configured;
    const h = vm.decodeNativeDatagram(packet());
    const c = {
      configure: async (x) => (configured = x),
      run: async (x) => {
        commands.push(x);
        return x.startsWith("/tmp/vyos-vm-helper probe")
          ? malformed
            ? "{bad"
            : JSON.stringify({ headers: [h, h, h] })
          : "";
      },
    };
    if (malformed)
      await assert.rejects(
        vm.probeNativeIdentity(c, e, "10.0.2.15", "http://10.0.2.2/traffic"),
      );
    else
      assert.equal(
        (
          await vm.probeNativeIdentity(
            c,
            e,
            "10.0.2.15",
            "http://10.0.2.2/traffic",
          )
        ).sub_agent,
        123456,
      );
    assert.deepEqual(configured, vm.NATIVE_CONFIG);
    assert.equal(commands.at(-1), "systemctl stop hsflowd.service");
  }
});
test("artifact server exposes exactly allowlisted objects and no directory or query", async (t) => {
  const d = temp(t);
  fs.writeFileSync(path.join(d, "bundle"), "BUNDLE");
  fs.writeFileSync(path.join(d, "helper"), "HELPER");
  const s = await vm.artifactServer(
    path.join(d, "bundle"),
    path.join(d, "helper"),
    "opaque",
  );
  t.after(() => new Promise((resolve) => s.close(resolve)));
  const root = "http://127.0.0.1:" + s.address().port;
  for (const [url, value] of [
    ["/opaque/bundle.tar.gz", "BUNDLE"],
    ["/opaque/helper", "HELPER"],
  ])
    assert.equal(await (await fetch(root + url)).text(), value);
  assert.ok(
    (await (await fetch(root + "/opaque/traffic")).text()).startsWith(
      "vyos-native-sflow-traffic",
    ),
  );
  for (const url of [
    "/",
    "/opaque/",
    "/opaque/helper?secret",
    "/other",
    "/opaque/../helper",
  ])
    assert.equal((await fetch(root + url)).status, 404);
});
test("real Expect PTY framing, private upload, command failure and cleanup", async (t) => {
  const { spawnSync } = require("node:child_process");
  const available =
    spawnSync("expect", ["-v"], { stdio: "ignore" }).status === 0;
  if (!available) {
    if (process.env.GITHUB_ACTIONS === "true" && process.platform === "linux")
      assert.fail("CI requires apt-get install expect");
    t.skip(
      "Expect unavailable on this host; Ubuntu CI must execute real PTY regression",
    );
    return;
  }
  const child = new vm.ExpectBridge(["bash", "--noprofile", "--norc"], {
    PATH: process.env.PATH,
    HOME: temp(t),
    TERM: "dumb",
    PS1: "INITIAL# ",
  });
  t.after(() => child.close());
  await child.expect("INITIAL# ", 5);
  await child.sendline("stty -echo; export PS1='VM''# '");
  await child.expect("VM# ", 5);
  const e = new vm.StageEvidence(temp(t), { checks: {} });
  e.stage("offline-pty-fixture");
  const c = new vm.Console(child, e);
  assert.equal(await c.run("printf '%s\\n' '{\"wire\":123}'"), '{"wire":123}');
  await assert.rejects(
    c.run(
      'printf \'%s\\n\' \'PRIVATE\' \'VM_DIAGNOSTIC {"code":"guest_check","secret":"PRIVATE"}\'; false',
    ),
  );
  assert.ok(!JSON.stringify(e.result).includes("PRIVATE"));
  await child.close();
  assert.equal(child.process.exitCode, 0);
});
test("bounded receive-only transcript buffers split secrets before writing", (t) => {
  assert.ok(vm, "migrated VM harness is required");
  const file = path.join(temp(t), "boot.txt");
  const log = new vm.BootTranscript(file, ["private-value"]);
  log.write("\x1b[32mBooting\x1b[0m\nprivate-");
  assert.equal(fs.readFileSync(file, "utf8"), "Booting\n");
  log.write("value\nAuthorization: Basic PRIVATE\nPassword: arbitrary\n");
  log.write(
    "x".repeat(20000) +
      "\n" +
      ("normal boot output".repeat(100) + "\n").repeat(160),
  );
  log.write("login: ");
  log.finish();
  log.write("not recorded\n");
  const text = fs.readFileSync(file, "utf8");
  assert.ok(text.length <= 262144);
  assert.match(text, /login: /);
  for (const bad of [
    "private-value",
    "PRIVATE",
    "arbitrary",
    "\x1b",
    "x".repeat(100),
    "not recorded",
  ])
    assert.ok(!text.includes(bad));
});
