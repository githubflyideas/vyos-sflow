"use strict";
// Disposable live-ISO acceptance; no host install, SSH or synthetic sFlow.
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const crypto = require("node:crypto");
const http = require("node:http");
const net = require("node:net");
const { spawn } = require("node:child_process");
const { pipeline } = require("node:stream/promises");
const { Readable } = require("node:stream");
class BootTranscript {
  constructor(file, privateValues = []) {
    this.file = file;
    this.privateValues = privateValues
      .filter(Boolean)
      .sort((a, b) => b.length - a.length);
    this.pending = "";
    this.text = "";
    this.closed = false;
    this.discard = false;
    fs.writeFileSync(file, "");
  }
  record(line) {
    line = line
      .replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "")
      .replace(/[\x00-\x08\x0b-\x1f\x7f]/g, "");
    for (const value of this.privateValues)
      line = line.split(value).join("[REDACTED]");
    if (/authorization|password\s*[:=]|token\s*[:=]|secret\s*[:=]/i.test(line))
      line = "[REDACTED sensitive console line]";
    this.text = (this.text + line + "\n").slice(-262144);
    fs.writeFileSync(this.file, this.text);
  }
  write(data) {
    if (this.closed) return;
    for (const char of data) {
      if (char === "\n") {
        this.record(
          this.discard ? "[oversized console line omitted]" : this.pending,
        );
        this.pending = "";
        this.discard = false;
      } else if (!this.discard) {
        this.pending += char;
        if (this.pending.length > 16384) {
          this.pending = "";
          this.discard = true;
        }
      }
    }
  }
  finish() {
    if (!this.closed && (this.pending || this.discard))
      this.record(
        this.discard ? "[oversized console line omitted]" : this.pending,
      );
    this.pending = "";
    this.closed = true;
  }
}
const VERSION = "2026.10.30-0020-rolling";
const ISO_URL = `https://github.com/vyos/vyos-nightly-build/releases/download/${VERSION}/vyos-${VERSION}-generic-amd64.iso`;
// Official release asset digest, pinned alongside the pre-migration ISO version.
const ISO_SHA256 =
  "786e33a26a6645d2791aa7fb40980953e4578f45f92c7b1a20f253750989d1f3";
const LIVE_BOOT_ARGS =
  "boot=live components hostname=vyos username=live nopersistence noautologin nonetworking union=overlay console=tty0 console=ttyS0,115200n8 net.ifnames=0 biosdevname=0";
const NATIVE_CONFIG = [
  "set system sflow interface eth0",
  "set system sflow sampling-rate 1",
  "set system sflow polling 1",
  "set system sflow server 127.0.0.1 port 6343",
  "set system sflow agent-interface eth0",
];
const quote = (s) => "'" + String(s).replace(/'/g, "'\\''") + "'";
function bootMembers(listing) {
  const candidates = [[], []];
  for (const block of listing.split(/\r?\n\s*\r?\n/)) {
    const fields = Object.fromEntries(
      block
        .split(/\r?\n/)
        .filter((x) => x.includes(" = "))
        .map((x) => [
          x.slice(0, x.indexOf(" = ")),
          x.slice(x.indexOf(" = ") + 3),
        ]),
    );
    const p = fields.Path || "";
    if (!p.startsWith("live/")) continue;
    if (p.split("/").includes("..") || p.includes("\\"))
      throw new Error("unsafe live image member");
    if (
      p.split("/").length !== 2 ||
      !/^\d+$/.test(fields.Size) ||
      Number(fields.Size) <= 0 ||
      fields["Symbolic Link"] ||
      fields.Folder === "+"
    )
      continue;
    const name = p.split("/")[1];
    if (/^vmlinuz(?:-|$)/.test(name)) candidates[0].push(p);
    if (/^(?:initrd\.img(?:-|$)|initrd-)/.test(name)) candidates[1].push(p);
  }
  const versioned = candidates.map((a) =>
    a.filter((p) => !["live/vmlinuz", "live/initrd.img"].includes(p)),
  );
  if (versioned.some((a) => a.length)) {
    if (versioned.some((a) => a.length !== 1))
      throw new Error("ambiguous versioned images");
    const [k, i] = versioned.map((a) => a[0]);
    if (
      k.replace("live/vmlinuz-", "") !==
      i.replace(/^live\/initrd(?:\.img)?-/, "")
    )
      throw new Error("boot version mismatch");
    return [k, i];
  }
  if (candidates.some((a) => a.length !== 1))
    throw new Error("missing or ambiguous boot images");
  return candidates.map((a) => a[0]);
}
function releaseAsset(release, name) {
  const a = (release.assets || []).filter((a) => a.name === name);
  if (a.length !== 1 || !/^sha256:[a-f0-9]{64}$/i.test(a[0].digest || ""))
    throw new Error("official exact asset digest required");
  return [a[0].browser_download_url, a[0].digest.slice(7).toLowerCase()];
}
function decodeNativeHeader(b) {
  if (b.length < 8) throw new Error("truncated header");
  const size = { 1: 4, 2: 16 }[b.readUInt32BE(4)];
  if (b.readUInt32BE(0) !== 5 || !size || b.length < 24 + size)
    throw new Error("invalid sFlow header");
  const addr = b.subarray(8, 8 + size);
  const agent =
    size === 4
      ? [...addr].join(".")
      : new URL(
          "http://[" +
            Array.from({ length: 8 }, (_, i) =>
              addr.readUInt16BE(i * 2).toString(16),
            ).join(":") +
            "]/",
        ).hostname.slice(1, -1);
  return {
    version: 5,
    agent,
    sub_agent: b.readUInt32BE(8 + size),
    sequence: b.readUInt32BE(12 + size),
    uptime_ms: b.readUInt32BE(16 + size),
    samples: b.readUInt32BE(20 + size),
    header_hex: b.subarray(0, 24 + size).toString("hex"),
  };
}
function decodeNativeDatagram(b) {
  const h = decodeNativeHeader(b);
  let offset = h.header_hex.length / 2;
  h.sample_headers_hex = [];
  if (h.samples > 64) throw new Error("too many samples");
  for (let n = 0; n < h.samples; n++) {
    if (offset + 8 > b.length) throw new Error("truncated sample header");
    h.sample_headers_hex.push(b.subarray(offset, offset + 8).toString("hex"));
    offset += 8 + b.readUInt32BE(offset + 4);
    if (offset > b.length) throw new Error("truncated sample");
  }
  return h;
}
const flowHeader = (h) =>
  (h.sample_headers_hex || []).some((t) =>
    [1, 3].includes(parseInt(t.slice(0, 8), 16)),
  );
function observedIdentity(headers, agent) {
  const flows = headers.filter(flowHeader);
  if (flows.length < 3 || flows.length > 8)
    throw new Error("need repeated native flow datagrams");
  const sub_agent = flows[0].sub_agent;
  if (flows.some((h) => h.agent !== agent || h.sub_agent !== sub_agent))
    throw new Error("inconsistent native flow identity");
  return { agent, sub_agent };
}
function safeDiagnostic(value, depth = 0) {
  if (!value || typeof value !== "object" || Array.isArray(value) || depth > 4)
    return {};
  const result = {};
  const numbers = new Set([
    "sub_agent",
    "rejected_streams",
    "packets",
    "records",
    "decode_errors",
    "undecoded",
    "dropped",
    "panics",
    "counter_samples",
    "write_errors",
    "hot_rows",
    "disk_bytes",
    "sources_http_status",
    "status_http_status",
    "http_status",
    "hsflowd_processes",
    "exit_code",
  ]);
  const enums = {
    code: [
      "health_wait",
      "baseline",
      "native_wait",
      "native_sources",
      "native_storage",
      "snapshot",
      "history",
      "native_probe_failed",
      "guest_check",
    ],
    error_type: [
      "Error",
      "RuntimeError",
      "AssertionError",
      "HTTPError",
      "URLError",
      "TimeoutError",
      "ValueError",
      "KeyError",
      "OSError",
    ],
    proto: ["sFlow v5", "sflow"],
  };
  for (const [key, item] of Object.entries(value)) {
    if (numbers.has(key) && Number.isSafeInteger(item) && item >= 0)
      result[key] = item;
    else if (
      ["enforced", "configured"].includes(key) &&
      typeof item === "boolean"
    )
      result[key] = item;
    else if (enums[key]?.includes(item)) result[key] = item;
    else if (
      ["agent", "exporter"].includes(key) &&
      typeof item === "string" &&
      net.isIP(item)
    )
      result[key] = item;
    else if (
      ["sources", "status", "stream"].includes(key) &&
      item &&
      !Array.isArray(item) &&
      typeof item === "object"
    )
      result[key] = safeDiagnostic(item, depth + 1);
    else if (["sources", "listeners"].includes(key) && Array.isArray(item))
      result[key] = item.slice(0, 8).map((r) => safeDiagnostic(r, depth + 1));
    else if (key === "headers" && Array.isArray(item)) {
      result[key] = [];
      for (const row of item.slice(0, 8)) {
        try {
          const raw = row.header_hex;
          if (
            typeof raw !== "string" ||
            !/^(?:[a-f0-9]{56}|[a-f0-9]{80})$/i.test(raw)
          )
            continue;
          const h = decodeNativeHeader(Buffer.from(raw, "hex")),
            tags = row.sample_headers_hex;
          if (
            !Array.isArray(tags) ||
            tags.length !== h.samples ||
            tags.length > 64 ||
            !tags.every(
              (t) => typeof t === "string" && /^[a-f0-9]{16}$/i.test(t),
            )
          )
            continue;
          h.sample_headers_hex = tags;
          result[key].push(h);
        } catch {}
      }
    }
  }
  return result;
}
class StageEvidence {
  constructor(directory, result) {
    this.directory = directory;
    this.result = result;
    this.started = performance.now();
    result.stages = [];
    this.save();
  }
  elapsed() {
    return Math.round(performance.now() - this.started) / 1000;
  }
  save() {
    const file = path.join(this.directory, "result.json");
    fs.writeFileSync(
      file + ".tmp",
      JSON.stringify(this.result, null, 2) + "\n",
    );
    fs.renameSync(file + ".tmp", file);
  }
  stage(name) {
    const previous = this.result.stages.at(-1);
    if (previous?.status === "RUNNING")
      Object.assign(previous, {
        status: "PASS",
        ended_seconds: this.elapsed(),
      });
    this.result.stage = name;
    this.result.stages.push({
      name,
      status: "RUNNING",
      started_seconds: this.elapsed(),
    });
    this.save();
  }
  fail(error) {
    this.result.status = "FAIL";
    this.result.error_type = [
      "Error",
      "TimeoutError",
      "AbortError",
      "TypeError",
      "SyntaxError",
    ].includes(error?.name)
      ? error.name
      : "Error";
    const s = this.result.stages.at(-1);
    if (s) Object.assign(s, { status: "FAIL", ended_seconds: this.elapsed() });
    this.save();
  }
  complete() {
    const s = this.result.stages.at(-1);
    if (s) Object.assign(s, { status: "PASS", ended_seconds: this.elapsed() });
    Object.assign(this.result, { status: "PASS", stage: "complete" });
    this.save();
  }
}
function saveGuestFailure(e, output, rc) {
  if (!(e instanceof StageEvidence)) return;
  let d = {};
  for (const line of output.split("\n"))
    if (line.startsWith("VM_DIAGNOSTIC ") && line.length <= 16384) {
      try {
        d = safeDiagnostic(JSON.parse(line.slice(14)));
      } catch {}
    }
  if (rc !== undefined) d.exit_code = rc;
  e.result.guest_failure = d;
  e.save();
}
async function loginConsole(child, evidence, bootTimeout) {
  const trace = new BootTranscript(
    path.join(evidence.directory, "boot-console.txt"),
    [process.env.GH_TOKEN, process.env.GITHUB_TOKEN],
  );
  child.trace = trace;
  try {
    evidence.stage("boot-wait-login");
    await child.expect("[^\\r\\n]*login:\\s*$", bootTimeout);
    evidence.stage("login-wait-password");
    await child.sendline("vyos");
    await child.expect("[Pp]assword:\\s*$", 60);
    evidence.stage("login-wait-user-shell");
    await child.sendline("vyos");
    await child.expect("[^\\r\\n]*\\$\\s*$", 60);
    evidence.stage("sudo-wait-root-shell");
    await child.sendline("sudo -n /bin/bash --noprofile --norc");
    await child.expect("[^\\r\\n]*#\\s*$", 60);
    evidence.stage("console-disable-echo");
    await child.sendline(
      "stty -echo; unset HISTFILE; export PS1='VM''# '; export TERM=dumb",
    );
    await child.expect("VM# ", 60);
    evidence.stage("console-ready");
  } finally {
    child.trace = null;
    trace.finish();
  }
  return new Console(child, evidence);
}
class Console {
  constructor(child, evidence) {
    this.child = child;
    this.evidence = evidence;
  }
  async run(command, timeout = 120) {
    const marker = "__VM_RC_" + crypto.randomBytes(12).toString("hex") + "__";
    await this.child.sendline(command + `; printf '\\n${marker}%s\\n' "$?"`);
    let frame;
    try {
      frame = await this.child.expect(marker + "(\\d+)\\r*\\n", timeout);
    } catch (error) {
      saveGuestFailure(this.evidence, error.before || "");
      if (this.evidence.result.stage === "configure-dhcp") {
        const trace = new BootTranscript(
          path.join(this.evidence.directory, "network-console.txt"),
        );
        trace.write(error.before || "");
        trace.finish();
        this.evidence.result.serial_crcrlf_count = (
          error.before?.match(/\r\r\n/g) || []
        ).length;
        this.evidence.save();
      }
      throw error;
    }
    const output = frame.before.replace(/\r/g, ""),
      match = frame.match.match(new RegExp(marker + "(\\d+)"));
    if (!match) throw new Error("invalid console frame");
    const rc = Number(match[1]);
    await this.child.expect("VM# ", timeout);
    if (rc) {
      saveGuestFailure(this.evidence, output, rc);
      throw new Error("guest command failed");
    }
    return output.trim();
  }
  async upload(file, content) {
    const b64 = Buffer.from(content).toString("base64");
    await this.run("umask 077; : > /tmp/vyos-vm-upload.b64");
    for (let start = 0; start < b64.length; start += 768)
      await this.run(
        "printf '%s' " +
          quote(b64.slice(start, start + 768)) +
          " >> /tmp/vyos-vm-upload.b64",
      );
    await this.run(
      "base64 -d /tmp/vyos-vm-upload.b64 > " +
        quote(file) +
        " && rm /tmp/vyos-vm-upload.b64",
    );
  }
  async configure(statements) {
    const text =
      "#!/bin/vbash\nsource /opt/vyatta/etc/functions/script-template\nconfigure\n" +
      [...statements, "commit", "save", "exit"]
        .map((x) => x + " || exit $?")
        .join("\n") +
      "\n";
    await this.upload("/tmp/vyos-vm-config", text);
    await this.run(
      "chmod 0644 /tmp/vyos-vm-config && sudo -u vyos /bin/vbash /tmp/vyos-vm-config",
      180,
    );
  }
}
async function installProduct(console, artifact, digest, url, tarCommand) {
  const single = artifact.endsWith(".run");
  const dest = "/tmp/vyos-vm-package" + (single ? ".run" : ".tar.gz");
  await console.run(
    "mkdir -p /tmp/vyos-vm-package && curl -fsS --max-time 180 " +
      quote(url) +
      " -o " +
      dest,
    200,
  );
  await console.run(
    'printf "%s  %s\\n" ' +
      quote(digest) +
      " " +
      dest +
      " | sha256sum -c - >/dev/null",
  );
  if (single)
    await console.run("printf '%s\\n' y | bash /tmp/vyos-vm-package.run", 180);
  else
    await console.run(
      "tar -xzf /tmp/vyos-vm-package.tar.gz -C /tmp/vyos-vm-package && cd /tmp/vyos-vm-package && " +
        tarCommand,
      180,
    );
}
async function probeNativeIdentity(console, evidence, agent, url) {
  evidence.stage("native-header-preinstall-probe");
  await console.configure(NATIVE_CONFIG);
  try {
    await console.run(
      'systemctl is-active --quiet hsflowd.service && test "$(pgrep -xc hsflowd)" = 1',
    );
    const captured = JSON.parse(
      await console.run("/tmp/vyos-vm-helper probe " + quote(url), 60),
    );
    const headers = safeDiagnostic(captured).headers || [];
    evidence.result.checks.native_header_probe = { headers };
    evidence.save();
    return observedIdentity(headers, agent);
  } finally {
    await console.run("systemctl stop hsflowd.service");
  }
}
async function artifactServer(bundle, helper, token) {
  const objects = new Map([
    ["/" + token + "/bundle.tar.gz", bundle],
    ["/" + token + "/helper", helper],
  ]);
  const server = http.createServer((request, response) => {
    if (request.method !== "GET") {
      response.writeHead(404).end();
      return;
    }
    const file = objects.get(request.url);
    if (file) {
      response.writeHead(200, { "Content-Length": fs.statSync(file).size });
      const stream = fs.createReadStream(file);
      stream.on("error", () => response.destroy());
      response.on("close", () => stream.destroy());
      stream.pipe(response);
    } else if (request.url === "/" + token + "/traffic") {
      const payload = Buffer.from("vyos-native-sflow-traffic\n".repeat(4096));
      response
        .writeHead(200, { "Content-Length": payload.length })
        .end(payload);
    } else response.writeHead(404).end();
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  return server;
}
class ExpectBridge {
  constructor(command, env, signal) {
    this.trace = null;
    this.pending = null;
    this.buffer = "";
    this.closed = false;
    this.process = spawn(
      "expect",
      [path.join(__dirname, "vyos_vm_console.exp"), ...command],
      { env, stdio: ["pipe", "pipe", "ignore"] },
    );
    this.exited = new Promise((resolve) =>
      this.process.once("close", () => {
        this.closed = true;
        this.rejectPending(new Error("console closed"));
        resolve();
      }),
    );
    this.process.on("error", () =>
      this.rejectPending(new Error("console unavailable")),
    );
    this.process.stdin.on("error", () =>
      this.rejectPending(new Error("console write failed")),
    );
    this.process.stdout.setEncoding("utf8");
    this.process.stdout.on("data", (data) => {
      this.buffer += data;
      if (this.buffer.length > 4000000) {
        this.rejectPending(new Error("console protocol overflow"));
        void this.close();
        return;
      }
      let index;
      while ((index = this.buffer.indexOf("\n")) >= 0) {
        const line = this.buffer.slice(0, index).replace(/\r$/, "");
        this.buffer = this.buffer.slice(index + 1);
        const fields = line.split("\t");
        if (fields[0] === "PID") {
          this.childPid = Number(fields[1]);
          continue;
        }
        const pending = this.pending;
        this.pending = null;
        if (!pending) continue;
        clearTimeout(pending.timer);
        if (fields[0] !== "R" || fields.length !== 4) {
          pending.reject(new Error("invalid console protocol"));
          continue;
        }
        const before = Buffer.from(fields[2], "base64").toString("utf8"),
          match = Buffer.from(fields[3], "base64").toString("utf8");
        this.trace?.write(before + match);
        if (fields[1] === "OK") pending.resolve({ before, match });
        else {
          const error = new Error("console operation failed");
          error.name = fields[1] === "TIMEOUT" ? "TimeoutError" : "Error";
          error.before = before;
          pending.reject(error);
        }
      }
    });
    if (signal) {
      this.abort = () => {
        this.rejectPending(new Error("whole-run deadline"));
        void this.close();
      };
      signal.addEventListener("abort", this.abort, { once: true });
      this.exited.then(() => signal.removeEventListener("abort", this.abort));
    }
  }
  rejectPending(error) {
    if (this.pending) {
      clearTimeout(this.pending.timer);
      this.pending.reject(error);
      this.pending = null;
    }
  }
  request(op, text = "", timeout = 120) {
    if (this.closed || this.closing)
      return Promise.reject(new Error("console closed"));
    if (this.pending)
      return Promise.reject(new Error("concurrent console operation"));
    return new Promise((resolve, reject) => {
      const timer = setTimeout(
        () => {
          this.rejectPending(new Error("console bridge stalled"));
          void this.close();
        },
        (timeout + 5) * 1000,
      );
      this.pending = { resolve, reject, timer };
      this.process.stdin.write(
        op +
          "\t" +
          Math.ceil(timeout) +
          "\t" +
          Buffer.from(text).toString("base64") +
          "\n",
      );
    });
  }
  sendline(text) {
    return this.request("send", text, 10);
  }
  expect(pattern, timeout = 120) {
    return this.request("expect", pattern, timeout);
  }
  async close() {
    if (this.closing) return this.closing;
    if (this.closed) return;
    this.closing = (async () => {
      this.rejectPending(new Error("console closing"));
      // Wake the bridge's blocking stdin read with its explicit close frame.
      // A signal alone can remain pending while Tcl waits in gets.
      this.process.stdin.end("close\t1\t\n");
      const force = setTimeout(() => {
        if (this.childPid > 0) {
          try {
            process.kill(this.childPid, "SIGKILL");
          } catch {}
        }
        this.process.kill("SIGKILL");
      }, 3000);
      await this.exited;
      clearTimeout(force);
    })();
    return this.closing;
  }
}
async function run(
  command,
  args,
  { signal, env = process.env, timeout = 120000 } = {},
) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      env,
      signal,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let output = "",
      overflow = false;
    const timer = setTimeout(() => child.kill("SIGKILL"), timeout);
    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (data) => {
      output += data;
      if (output.length > 8 * 1024 * 1024) {
        overflow = true;
        child.kill("SIGKILL");
      }
    });
    child.stderr.resume();
    child.once("error", () => {
      clearTimeout(timer);
      reject(new Error("host command unavailable"));
    });
    child.once("close", (code) => {
      clearTimeout(timer);
      if (code !== 0 || overflow) reject(new Error("host command failed"));
      else resolve(output);
    });
  });
}
async function sha256(file) {
  const hash = crypto.createHash("sha256");
  await pipeline(fs.createReadStream(file), hash);
  return hash.digest("hex");
}
async function fetchISO(url, expected, dest, signal) {
  const match = url.match(
    /^https:\/\/github.com\/vyos\/vyos-nightly-build\/releases\/download\/([^/]+)\/([^/]+\.iso)$/,
  );
  if (!match) throw new Error("exact official ISO URL required");
  if (!expected) {
    const headers = {
      Accept: "application/vnd.github+json",
      "User-Agent": "vyos-sflow-vm-test",
    };
    if (process.env.GH_TOKEN)
      headers.Authorization = "Bearer " + process.env.GH_TOKEN;
    const response = await fetch(
      "https://api.github.com/repos/vyos/vyos-nightly-build/releases/tags/" +
        match[1],
      { headers, signal },
    );
    if (!response.ok) throw new Error("official release unavailable");
    const [official, digest] = releaseAsset(await response.json(), match[2]);
    if (official !== url) throw new Error("asset URL differs");
    expected = digest;
  }
  if (!/^[a-f0-9]{64}$/i.test(expected))
    throw new Error("invalid ISO checksum");
  const response = await fetch(url, { signal });
  if (!response.ok || !response.body) throw new Error("ISO download failed");
  await pipeline(Readable.fromWeb(response.body), fs.createWriteStream(dest), {
    signal,
  });
  const actual = await sha256(dest);
  if (actual !== expected.toLowerCase()) {
    fs.unlinkSync(dest);
    throw new Error("ISO digest mismatch");
  }
  return actual;
}
function parseArgs(argv) {
  const aliases = { "--bundle": "--artifact", "--tarball": "--artifact" },
    allowed = new Set([
      "--artifact",
      "--iso-url",
      "--iso-sha256",
      "--work-dir",
      "--evidence",
      "--timeout",
      "--boot-timeout",
      "--install-command",
    ]);
  const a = {
    artifact: "",
    "iso-url": ISO_URL,
    "work-dir": process.env.RUNNER_TEMP || os.tmpdir(),
    evidence: "evidence/vyos-vm",
    timeout: 1800,
    "boot-timeout": 600,
    "install-command": "bash packaging/install.sh --binary ./vyos-sflow",
  };
  for (let i = 0; i < argv.length; i++) {
    const key = aliases[argv[i]] || argv[i];
    if (!allowed.has(key) || i + 1 === argv.length)
      throw new Error("invalid CLI option");
    a[key.slice(2)] = argv[++i];
  }
  a.timeout = Number(a.timeout);
  a["boot-timeout"] = Number(a["boot-timeout"]);
  if (
    !a.artifact ||
    !Number.isInteger(a.timeout) ||
    a.timeout < 60 ||
    !Number.isInteger(a["boot-timeout"]) ||
    a["boot-timeout"] < 1
  )
    throw new Error("artifact and valid deadlines required");
  if (!a["iso-sha256"] && a["iso-url"] === ISO_URL)
    a["iso-sha256"] = ISO_SHA256;
  return a;
}
function requireStream(value) {
  const stream = value?.sources?.stream;
  if (stream?.enforced !== false)
    throw new Error("sender restriction unexpectedly enforced");
}
async function main(argv) {
  if (argv.includes("--help")) {
    console.log(
      "node tests/vyos_vm.cjs --artifact FILE --work-dir DIR --evidence DIR --timeout 1800 [--boot-timeout 600] [--iso-url URL --iso-sha256 HASH] [--install-command COMMAND]",
    );
    return 0;
  }
  const a = parseArgs(argv);
  if (process.platform !== "linux" || process.env.GITHUB_ACTIONS !== "true")
    throw new Error("VM requires GitHub Actions Linux runner");
  if (!fs.statSync(a.artifact).isFile()) throw new Error("artifact missing");
  fs.mkdirSync(a.evidence, { recursive: true });
  fs.mkdirSync(a["work-dir"], { recursive: true });
  const result = {
    status: "FAIL",
    stage: "prepare",
    iso_url: a["iso-url"],
    checks: {},
  };
  const evidence = new StageEvidence(a.evidence, result);
  const controller = new AbortController();
  const signal = controller.signal;
  const deadline = setTimeout(() => controller.abort(), a.timeout * 1000),
    terminate = () => controller.abort();
  process.once("SIGTERM", terminate);
  process.once("SIGINT", terminate);
  let child, server, tmp;
  try {
    result.artifact_sha256 = await sha256(a.artifact);
    tmp = fs.mkdtempSync(path.join(path.resolve(a["work-dir"]), "vyos-live-"));
    evidence.stage("build-test-only-guest-helper");
    const helper = path.join(tmp, "vyos-vm-helper");
    await run(
      process.env.GO || "go",
      [
        "build",
        "-trimpath",
        "-ldflags=-s -w",
        "-o",
        helper,
        path.join(__dirname, "vmguest", "main.go"),
      ],
      {
        signal,
        env: {
          ...process.env,
          GOOS: "linux",
          GOARCH: "amd64",
          CGO_ENABLED: "0",
        },
        timeout: 180000,
      },
    );
    const helperDigest = await sha256(helper);
    result.helper_sha256 = helperDigest;
    evidence.stage("download-and-verify-iso");
    const iso = path.join(tmp, "vyos.iso");
    result.iso_sha256 = await fetchISO(
      a["iso-url"],
      a["iso-sha256"],
      iso,
      signal,
    );
    evidence.stage("inspect-and-extract-boot-images");
    const listing = await run("7z", ["l", "-slt", iso], {
      signal,
      timeout: 90000,
    });
    fs.writeFileSync(path.join(a.evidence, "iso-listing.txt"), listing);
    const [kernel, initrd] = bootMembers(listing);
    result.boot_members = [kernel, initrd];
    await run("7z", ["x", "-y", "-o" + tmp, iso, kernel, initrd], { signal });
    for (const member of [kernel, initrd]) {
      const stat = fs.lstatSync(path.join(tmp, member));
      if (!stat.isFile() || stat.isSymbolicLink())
        throw new Error("unsafe extracted boot member");
    }
    const token = crypto.randomBytes(20).toString("hex");
    server = await artifactServer(path.resolve(a.artifact), helper, token);
    const host = "http://10.0.2.2:" + server.address().port + "/" + token;
    let acceleration = "tcg";
    try {
      fs.accessSync("/dev/kvm", fs.constants.R_OK | fs.constants.W_OK);
      acceleration = "kvm";
    } catch {}
    result.acceleration = acceleration;
    result.boot_args = LIVE_BOOT_ARGS;
    result.boot_timeout_seconds = a["boot-timeout"];
    const command = [
      "qemu-system-x86_64",
      "-accel",
      acceleration,
      // QEMU's baseline CPU (no SSE4, no AVX), as on older routers: a
      // build that needs more must fail here, with KVM too.
      "-cpu",
      "qemu64",
      "-m",
      "3072",
      "-smp",
      "2",
      "-kernel",
      path.join(tmp, kernel),
      "-initrd",
      path.join(tmp, initrd),
      "-append",
      LIVE_BOOT_ARGS,
      "-cdrom",
      iso,
      "-boot",
      "d",
      "-display",
      "none",
      "-serial",
      "stdio",
      "-monitor",
      "none",
      "-no-user-config",
      "-no-reboot",
      "-nic",
      "user,model=virtio-net-pci,mac=52:54:00:12:34:56",
    ];
    evidence.stage("qemu-launch");
    child = new ExpectBridge(
      command,
      { PATH: process.env.PATH, HOME: tmp, LANG: "C.UTF-8", TERM: "dumb" },
      signal,
    );
    const console = await loginConsole(child, evidence, a["boot-timeout"]);
    evidence.stage("configure-dhcp");
    await console.configure(["set interfaces ethernet eth0 address dhcp"]);
    await console.run(
      "timeout 90 bash -c 'until ip -4 -o addr show dev eth0 | grep -q \"inet 10.0.2.\"; do sleep 2; done'",
      100,
    );
    evidence.stage("install-test-only-guest-helper");
    await console.run(
      "curl -fsS --max-time 120 " +
        quote(host + "/helper") +
        " -o /tmp/vyos-vm-helper",
      130,
    );
    await console.run(
      "printf '%s  %s\\n' " +
        quote(helperDigest) +
        " /tmp/vyos-vm-helper | sha256sum -c - >/dev/null && chmod 0700 /tmp/vyos-vm-helper",
    );
    evidence.stage("query-native-agent");
    const interfaces = JSON.parse(
      await console.run("ip -j -4 address show dev eth0"),
    );
    const agents = interfaces.flatMap((i) =>
      (i.addr_info || [])
        .filter((a) => a.scope === "global")
        .map((a) => a.local),
    );
    if (agents.length !== 1 || net.isIP(agents[0]) !== 4)
      throw new Error("unique eth0 IPv4 required");
    const identity = await probeNativeIdentity(
      console,
      evidence,
      agents[0],
      host + "/traffic",
    );
    result.checks.observed_native_identity = identity;
    evidence.stage("install-product");
    await installProduct(
      console,
      a.artifact,
      result.artifact_sha256,
      host + "/bundle.tar.gz",
      a["install-command"],
    );
    result.checks.installer_format = a.artifact.endsWith(".run")
      ? "single-file-run"
      : "tar-advanced";
    result.checks.version = await console.run(
      "/config/vyos-sflow/bin/vyos-sflow --version",
    );
    await console.run(
      "systemctl start vyos-sflow.service && systemctl is-active --quiet vyos-sflow.service",
    );
    result.checks.baseline = JSON.parse(
      await console.run("/tmp/vyos-vm-helper baseline", 150),
    );
    evidence.stage("configure-native-hsflowd");
    await console.run("systemctl start hsflowd.service");
    await console.run(
      'systemctl is-active --quiet hsflowd.service && test "$(pgrep -xc hsflowd)" = 1',
    );
    result.checks.native_config = await console.run(
      "cat /run/sflow/hsflowd.conf",
    );
    evidence.stage("native-network-traffic-and-storage");
    await console.run(
      "for n in $(seq 1 20); do curl -fsS --max-time 10 " +
        quote(host + "/traffic") +
        " -o /dev/null || exit; sleep 1; done",
      240,
    );
    result.checks.native_sflow = JSON.parse(
      await console.run("/tmp/vyos-vm-helper native", 150),
    );
    requireStream(result.checks.native_sflow);
    evidence.stage("restart-persistence");
    await console.run(
      "systemctl stop hsflowd.service && sleep 3 && systemctl restart vyos-sflow.service",
      150,
    );
    result.checks.snapshot = JSON.parse(
      await console.run("/tmp/vyos-vm-helper snapshot", 150),
    );
    await console.run(
      "systemctl restart vyos-sflow.service && systemctl is-active --quiet vyos-sflow.service",
      150,
    );
    result.checks.restart = JSON.parse(
      await console.run("/tmp/vyos-vm-helper history", 150),
    );
    // No exporter is restarted before exact persisted-history comparison.
    // The product must remain unrestricted after reopening.
    result.checks.reopened_stream = JSON.parse(
      await console.run("/tmp/vyos-vm-helper stream", 150),
    );
    requireStream(result.checks.reopened_stream);
    evidence.complete();
  } catch (error) {
    evidence.fail(error);
    console.error(
      "FAIL VyOS VM stage=" + result.stage + " error=" + result.error_type,
    );
  } finally {
    clearTimeout(deadline);
    process.removeListener("SIGTERM", terminate);
    process.removeListener("SIGINT", terminate);
    if (child) await child.close();
    if (server) {
      server.closeAllConnections();
      await new Promise((resolve) => server.close(resolve));
    }
    if (tmp) fs.rmSync(tmp, { recursive: true, force: true });
    evidence.save();
  }
  console.log(
    result.status +
      " VyOS live-ISO acceptance; evidence=" +
      path.join(a.evidence, "result.json"),
  );
  return result.status === "PASS" ? 0 : 1;
}
module.exports = {
  artifactServer,
  ExpectBridge,
  parseArgs,
  requireStream,
  main,
  BootTranscript,
  StageEvidence,
  bootMembers,
  releaseAsset,
  decodeNativeHeader,
  decodeNativeDatagram,
  observedIdentity,
  safeDiagnostic,
  saveGuestFailure,
  loginConsole,
  Console,
  installProduct,
  probeNativeIdentity,
  LIVE_BOOT_ARGS,
  ISO_SHA256,
  NATIVE_CONFIG,
};
if (require.main === module)
  main(process.argv.slice(2))
    .then((code) => {
      process.exitCode = code;
    })
    .catch(() => {
      console.error(
        "FAIL VM preparation; use --help and verify Linux CI prerequisites",
      );
      process.exitCode = 1;
    });
