'use strict';
const assert = require('node:assert/strict');
const http = require('node:http');
const https = require('node:https');
const {performance} = require('node:perf_hooks');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
function request(url, {expected = 200, payload, auth, jar, ca, timeout = 5000} = {}) {
  return new Promise((resolve, reject) => {
    const headers = {};
    if (auth) headers.Authorization = auth;
    if (jar && jar.cookie) headers.Cookie = jar.cookie;
    const body = payload === undefined ? undefined : JSON.stringify(payload);
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const req = (url.startsWith('https:') ? https : http).request(url,
      {method: body === undefined ? 'GET' : 'POST', headers, ca, rejectUnauthorized: true}, res => {
        let raw = '';
        res.on('data', chunk => { raw += chunk; });
        res.on('error', () => reject(new Error('HTTP response interrupted')));
        res.on('end', () => {
          try {
            // Never include response bodies, credentials, cookies or headers in failures.
            assert.equal(res.statusCode, expected, `${new URL(url).pathname}: HTTP status mismatch`);
            if (jar && res.headers['set-cookie']) jar.cookie = res.headers['set-cookie'].map(c => c.split(';')[0]).join('; ');
            resolve(raw ? JSON.parse(raw) : {});
          } catch (e) { reject(e instanceof SyntaxError ? new Error('Invalid JSON response') : e); }
        });
      });
    const timer = setTimeout(() => req.destroy(new Error('HTTP request deadline exceeded')), timeout);
    req.on('close', () => clearTimeout(timer));
    req.on('error', () => reject(new Error('HTTP transport failed')));
    req.end(body);
  });
}
async function waitFor(check, message, {timeout = 45000, interval = 200, alive = () => true} = {}) {
  const end = performance.now() + timeout;
  while (performance.now() < end) {
    assert.ok(alive(), `Service exited: ${message}`);
    try { if (await check(Math.max(1, end - performance.now()))) return; } catch (_) { /* transient startup/network errors */ }
    await sleep(Math.min(interval, Math.max(0, end - performance.now())));
  }
  throw new Error(message);
}
async function waitHealth(base, options = {}) {
  await waitFor(async remaining => (await request(base + '/healthz', {timeout: Math.min(2000, remaining), ca: options.ca})).status === 'ok',
    'Installed service HTTP/database health did not become ready', options);
}
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const net = require('node:net');
const dgram = require('node:dgram');
const {spawn, spawnSync} = require('node:child_process');
const crypto = require('node:crypto');
const basic = password => 'Basic ' + Buffer.from('admin:' + password).toString('base64');
const cleanEnv = () => Object.fromEntries(Object.entries(process.env).filter(([k]) => !k.startsWith('VYOS_SFLOW_')));
const password = () => crypto.randomBytes(24).toString('base64url');
const hash = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
function u32(...values) { const b = Buffer.alloc(values.length * 4); values.forEach((v, i) => b.writeUInt32BE(v, i * 4)); return b; }
function ipv4(ip) { assert.equal(net.isIP(ip), 4, 'IPv4 required'); return Buffer.from(ip.split('.').map(Number)); }
function sflow(sequence, agent = '127.0.0.1', subAgent = 0, sourcePort = 40000) {
  // Independent wire fixture, not the product encoder. Explicit identity, not product defaults.
  const header = Buffer.from('02000000000102000000000208004500003c0000000040110000c000020ac63364149c4001bb00280000', 'hex');
  header.writeUInt16BE(sourcePort, 34);
  const raw = Buffer.concat([u32(1, 78, 4, header.length), header, Buffer.alloc((4 - header.length % 4) % 4)]);
  const sample = Buffer.concat([u32(sequence, 1, 100, sequence * 100, 0, 1, 2, 1, 1, raw.length), raw]);
  return Buffer.concat([u32(5, 1), ipv4(agent), u32(subAgent, sequence, sequence * 100, 1, 1, sample.length), sample]);
}
function netflow() {
  const b = Buffer.alloc(72); b.writeUInt16BE(5); b.writeUInt16BE(1, 2);
  u32(1000, Math.floor(Date.now() / 1000), 0, 1).copy(b, 4);
  ipv4('192.0.2.10').copy(b, 24); ipv4('198.51.100.20').copy(b, 28);
  u32(1, 777, 0, 1000).copy(b, 40); b.writeUInt16BE(40000, 56); b.writeUInt16BE(443, 58); b[62] = 17; return b;
}
function ipfix() { const b = Buffer.alloc(16); b.writeUInt16BE(10); b.writeUInt16BE(16, 2); u32(Math.floor(Date.now() / 1000), 1, 9).copy(b, 4); return b; }
function parseWildcard(text, protocol, port) {
  return text.split('\n').slice(1).some(row => {
    const fields = row.trim().split(/\s+/), local = (fields[1] || '').split(':');
    return /^0+$/.test(local[0]) && parseInt(local[1], 16) === port && (protocol !== 'tcp' || fields[3] === '0A');
  });
}
function wildcard(protocol, port) { return ['', '6'].some(suffix => { const f = '/proc/net/' + protocol + suffix; return fs.existsSync(f) && parseWildcard(fs.readFileSync(f, 'utf8'), protocol, port); }); }
function assertInstallSafe({platform = process.platform, uid = process.geteuid?.(), githubActions = process.env.GITHUB_ACTIONS, existing = fs.existsSync('/config/vyos-sflow')} = {}) {
  assert.ok(platform === 'linux' && uid === 0 && githubActions === 'true', 'Only run as root on a disposable GitHub Actions Linux runner');
  assert.ok(!existing, 'Refusing to touch a pre-existing installation');
}
async function port(kind = 'tcp') {
  const s = kind === 'udp' ? dgram.createSocket('udp4') : net.createServer();
  await new Promise((resolve, reject) => { s.once('error', reject); kind === 'udp' ? s.bind(0, '127.0.0.1', resolve) : s.listen(0, '127.0.0.1', resolve); });
  const value = s.address().port; await new Promise(r => s.close(r)); return value;
}
async function send(packets, port, host = '127.0.0.1') {
  const s = dgram.createSocket('udp4');
  try { for (const b of packets) { await new Promise((resolve, reject) => s.send(b, port, host, e => e ? reject(e) : resolve())); await sleep(10); } }
  finally { s.close(); }
}
function run(command, args, options = {}) {
  const result = spawnSync(command, args, {encoding: 'utf8', timeout: 180000, ...options});
  // Deliberately do not dump subprocess output: it can contain credentials.
  assert.ok(!result.error && result.status === 0, `${path.basename(command)} subprocess failed`);
  return result.stdout;
}
class Service {
  constructor(binary, args, env, log, {cwd} = {}) { Object.assign(this, {binary, args, env, log, cwd}); }
  alive() { return !!this.child && this.child.exitCode === null && this.child.signalCode === null && !this.failed; }
  start() {
    assert.ok(!this.child, 'Service already started'); this.failed = false;
    const fd = fs.openSync(this.log, 'a', 0o600);
    try { this.child = spawn(this.binary, this.args, {cwd: this.cwd, env: this.env, stdio: ['ignore', fd, fd]}); }
    finally { fs.closeSync(fd); }
    this.exited = new Promise(resolve => { this.child.once('error', () => { this.failed = true; resolve(-1); }); this.child.once('exit', code => resolve(code)); });
  }
  async stop() {
    if (!this.child) return;
    this.child.kill('SIGTERM');
    let timer;
    const code = await Promise.race([this.exited, new Promise(resolve => { timer = setTimeout(() => resolve('timeout'), 30000); })]);
    clearTimeout(timer);
    if (code === 'timeout') { this.child.kill('SIGKILL'); await this.exited; }
    this.child = null;
    assert.equal(code, 0, 'Service did not shut down cleanly');
  }
  async cleanup() { if (this.child) { this.child.kill('SIGKILL'); await this.exited; this.child = null; } }
}
function binaryArg(args = process.argv.slice(2), browser = false) {
  const positional = args.filter(arg => !arg.startsWith('-'));
  const flags = args.filter(arg => arg.startsWith('-'));
  assert.ok(positional.length === 1 && flags.length <= (browser ? 1 : 0) && flags.every(arg => arg === '--browser'), 'Usage: node test.cjs BINARY' + (browser ? ' [--browser]' : ''));
  const binary = path.resolve(positional[0]); assert.ok(fs.statSync(binary).isFile(), 'Product binary missing'); return binary;
}
async function withTemp(prefix, fn) { const tmp = fs.mkdtempSync(path.join(os.tmpdir(), prefix)); try { return await fn(tmp); } finally { fs.rmSync(tmp, {recursive: true, force: true}); } }
function cli(main) { main().catch(error => { console.error(error.message); process.exitCode = 1; }); }
module.exports = {request, waitFor, waitHealth, sleep, basic, cleanEnv, password, hash, sflow, netflow, ipfix, parseWildcard, wildcard, assertInstallSafe, port, send, run, Service, binaryArg, withTemp, cli};
