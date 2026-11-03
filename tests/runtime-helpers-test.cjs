'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const h = require('./runtime-helpers.cjs');
const fs = require('node:fs');
const path = require('node:path');
const https = require('node:https');
const dgram = require('node:dgram');
const {spawnSync} = require('node:child_process');

test('CLI preserves installer/binary paths and refuses destructive invocation on this non-Linux host', () => {
  const {parseArgs} = require('./installed-service.cjs');
  const args = parseArgs(['--binary', 'candidate with spaces', '--installer', 'release-1.1.0.run']);
  assert.equal(args.binary, path.resolve('candidate with spaces'));
  assert.equal(args.installer, path.resolve('release-1.1.0.run'));
  for (const bad of [[], ['--binary', 'a'], ['--installer', 'a', '--binary', 'b', '--binary', 'c'], ['--other', 'x']]) assert.throws(() => parseArgs(bad));
  if (process.platform !== 'linux') {
    const result = spawnSync(process.execPath, [path.join(__dirname, 'installed-service.cjs'), '--installer', 'nonexistent.run', '--binary', 'nonexistent'], {encoding: 'utf8'});
    assert.equal(result.status, 1); assert.match(result.stderr, /disposable GitHub Actions Linux/);
  }
});

test('service launches in the requested working directory for default-data acceptance', async () => {
 await h.withTemp('runtime-cwd-', async tmp => {
  const service = new h.Service(process.execPath, ['-e', 'console.log(process.cwd())'], h.cleanEnv(), path.join(tmp,'log'), {cwd:tmp});
  service.start(); await service.exited;
  assert.equal(fs.readFileSync(path.join(tmp,'log'),'utf8').trim(),tmp);
  await service.cleanup();
 });
});

test('binary CLI accepts the browser flag before or after the binary', () => {
  assert.equal(h.binaryArg(['--browser', __filename], true), __filename);
  assert.equal(h.binaryArg([__filename, '--browser'], true), __filename);
  assert.throws(() => h.binaryArg([__filename, '--browser', '--browser'], true));
  assert.throws(() => h.binaryArg([__filename, '--browser'], false));
});

test('real UDP transport delivers the independent wire fixture intact', async () => {
  const socket = dgram.createSocket('udp4');
  await new Promise(r => socket.bind(0, '127.0.0.1', r));
  let timer;
  const received = new Promise((resolve, reject) => { socket.once('message', resolve); timer = setTimeout(() => reject(new Error('UDP receive timeout')), 2000); });
  try { const packet = h.sflow(3); await h.send([packet], socket.address().port); assert.deepEqual(await received, packet); }
  finally { clearTimeout(timer); socket.close(); }
  assert.ok(await h.port() > 0); assert.ok(await h.port('udp') > 0);
});

test('real UDP transport preserves distinct native header identities from the same peer', async () => {
 const socket = dgram.createSocket('udp4');
 await new Promise(r=>socket.bind(0,'127.0.0.1',r));
 const packets = [h.sflow(1),h.sflow(1,'192.0.2.99'),h.sflow(1,'127.0.0.1',1)], received = [], peers = [];
 let timer;
 const complete = new Promise((resolve,reject)=>{
  socket.on('message',(b,peer)=>{received.push(b);peers.push(peer.address);if(received.length===packets.length)resolve();});
  timer=setTimeout(()=>reject(new Error('Multi-identity UDP receive timeout')),2000);
 });
 try {
  await h.send(packets,socket.address().port); await complete;
  assert.deepEqual(received,packets);
  assert.deepEqual(peers,['127.0.0.1','127.0.0.1','127.0.0.1']);
  assert.equal(new Set(received.map(b=>b.subarray(8,16).toString('hex'))).size,3);
 } finally {clearTimeout(timer);socket.close();}
});

test('HTTP deadline also bounds a peer that never responds', async () => {
  const server = http.createServer(() => {});
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  try { await assert.rejects(h.request(`http://127.0.0.1:${server.address().port}`, {timeout: 40}), /transport failed/); }
  finally { server.closeAllConnections(); await new Promise(r => server.close(r)); }
});

test('TLS helper verifies trusted CA and IP identity and rejects untrusted CA', async () => {
  await h.withTemp('runtime-tls-', async tmp => {
    const key = path.join(tmp, 'key.pem'), cert = path.join(tmp, 'cert.pem');
    h.run('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', key, '-out', cert, '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1']);
    const ca = fs.readFileSync(cert);
    const server = https.createServer({key: fs.readFileSync(key), cert: ca}, (req, res) => res.end('{"status":"ok"}'));
    await new Promise(r => server.listen(0, '127.0.0.1', r));
    const url = `https://127.0.0.1:${server.address().port}`;
    try { await h.waitHealth(url, {ca, timeout: 1000}); await assert.rejects(h.request(url), /transport failed/); }
    finally { server.closeAllConnections(); await new Promise(r => server.close(r)); }
  });
});

test('service helper detects early child exit and cleans up an owned process', async () => {
  await h.withTemp('runtime-child-', async tmp => {
    const service = new h.Service(process.execPath, ['-e', 'process.exit(7)'], h.cleanEnv(), path.join(tmp, 'log'));
    service.start(); await service.exited;
    assert.equal(service.alive(), false);
    await assert.rejects(h.waitFor(() => true, 'early exit', {alive: () => service.alive()}), /Service exited/);
    await service.cleanup();
    const running = new h.Service(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], h.cleanEnv(), path.join(tmp, 'log'));
    running.start(); assert.equal(running.alive(), true); await running.cleanup(); assert.equal(running.alive(), false);
  });
});
test('independent sFlow wire fixture has exact IPv4 UDP and sample layout', () => {
  assert.equal(typeof h.sflow, 'function');
  assert.ok(h.sflow(1, '192.0.2.99', 0, 40001).includes(Buffer.from('c000020ac63364149c4101bb00280000', 'hex')));
  assert.ok(h.sflow(1, '127.0.0.1', 1, 40002).includes(Buffer.from('c000020ac63364149c4201bb00280000', 'hex')));
  const b = h.sflow(7, '192.0.2.9', 11);
  assert.equal(b.toString('hex'), '0000000500000001c00002090000000b00000007000002bc000000010000000100000064000000070000000100000064000002bc00000000000000010000000200000001000000010000003c000000010000004e000000040000002a02000000000102000000000208004500003c0000000040110000c000020ac63364149c4001bb002800000000');
  assert.throws(() => h.sflow(1, 'not-ip'), /IPv4/);
  assert.equal(h.netflow().length, 72);
  assert.equal(h.ipfix().readUInt16BE(0), 10);
});
test('wildcard parser distinguishes listen state, loopback and IPv6', () => {
  assert.equal(typeof h.parseWildcard, 'function');
  const table = addr => `header\n 0: ${addr}:1F82 00000000:0000 0A rest\n`;
  assert.equal(h.parseWildcard(table('00000000'), 'tcp', 8066), true);
  assert.equal(h.parseWildcard(table('0100007F'), 'tcp', 8066), false);
  assert.equal(h.parseWildcard(table('00000000000000000000000000000000'), 'tcp', 8066), true);
  assert.equal(h.parseWildcard(table('00000000').replace('0A', '01'), 'tcp', 8066), false);
  assert.equal(h.parseWildcard(table('00000000').replace('0A', '07'), 'udp', 8066), true);
});
test('installer safety gate fails closed outside disposable root Linux GHA or with existing path', () => {
  assert.equal(typeof h.assertInstallSafe, 'function');
  const safe = {platform: 'linux', uid: 0, githubActions: 'true', existing: false};
  assert.doesNotThrow(() => h.assertInstallSafe(safe));
  for (const change of [{platform: 'win32'}, {platform: 'darwin'}, {uid: 1000}, {githubActions: ''}, {githubActions: 'false'}, {githubActions: null}, {existing: true}]) {
    assert.throws(() => h.assertInstallSafe({...safe, ...change}), /disposable|existing/);
  }
  const original = process.env.GITHUB_ACTIONS;
  try {
    delete process.env.GITHUB_ACTIONS;
    assert.throws(() => h.assertInstallSafe({...safe, githubActions: undefined}), /disposable/);
  } finally {
    if (original === undefined) delete process.env.GITHUB_ACTIONS;
    else process.env.GITHUB_ACTIONS = original;
  }
});
test('HTTP cookie login, JSON payload and Basic requests preserve status checks without secret logging', async () => {
  const server = http.createServer((req, res) => {
    let body = ''; req.on('data', b => body += b);
    req.on('end', () => {
      if (req.url === '/login') { assert.deepEqual(JSON.parse(body), {user: 'admin'}); res.setHeader('Set-Cookie', 'session=synthetic; HttpOnly'); }
      else if (req.url === '/cookie') assert.equal(req.headers.cookie, 'session=synthetic');
      else { res.statusCode = 401; }
      res.end('{"ok":true}');
    });
  });
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${server.address().port}`, jar = {};
  try {
    await h.request(base + '/login', {jar, payload: {user: 'admin'}});
    await h.request(base + '/cookie', {jar});
    await assert.rejects(h.request(base + '/denied', {auth: 'Basic private-test-token'}), e => !e.message.includes('private-test-token') && /status mismatch/.test(e.message));
  } finally { await new Promise(r => server.close(r)); }
});
