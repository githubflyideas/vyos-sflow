#!/usr/bin/env node
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');
async function main() {
  const binary = h.binaryArg();
  assert.ok(process.platform === 'linux' && fs.existsSync('/proc/net/tcp'), 'Linux runtime required');
  const password = h.password(), env = {...h.cleanEnv(), VYOS_SFLOW_PASSWORD: password};
  fs.mkdirSync('evidence', {recursive: true});
  const log = path.resolve('evidence/network-defaults.log'); fs.writeFileSync(log, '', {mode: 0o600});
  await h.withTemp('vyos-network-', async tmp => {
    const args = ['serve'];
    const service = new h.Service(binary, args, env, log, {cwd:tmp}), options = {alive: () => service.alive()};
    try {
      // Do not supply bind addresses, TLS or an HTTP opt-in: exercise actual defaults.
      service.start(); await h.waitHealth('http://127.0.0.2:8066', options);
      assert.ok(fs.statSync(path.join(tmp,'vyso_sflow_data')).isDirectory(), 'Default data directory is not relative to launch cwd');
      assert.ok(h.wildcard('tcp', 8066), 'HTTP is not wildcard bound');
      assert.ok(h.wildcard('udp', 6343), 'sFlow is not wildcard bound');
      await h.send([h.sflow(1)], 6343, '127.0.0.2');
      await h.waitFor(async () => ((await h.request('http://127.0.0.2:8066/api/sources', {auth: h.basic(password)})).sources || []).some(s => s.records > 0), 'sFlow not received on alternate address', options);
      await service.stop();
      const output = fs.readFileSync(log, 'utf8');
      assert.ok(output.includes('private/internal interface') && output.includes('unencrypted'), 'Missing HTTP safety notice');
      const cert = path.join(tmp, 'cert.pem'), key = path.join(tmp, 'key.pem');
      h.run('openssl', ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', key, '-out', cert, '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1']);
      const ca = fs.readFileSync(cert);
      service.args = [...args, '-tls-cert', cert, '-tls-key', key]; service.start();
      await h.waitHealth('https://127.0.0.1:8066', {...options, ca});
      await h.request('https://127.0.0.1:8066/api/overview', {ca, auth: h.basic(password)});
      await h.waitFor(async () => (await h.request('https://127.0.0.1:8066/api/overview?range=1h', {ca, auth: h.basic(password)})).totals.wire === 78 * 100, 'Default-directory restart lost history', {...options,ca});
      // The generated CA must be explicitly trusted; no insecure TLS fallback.
      await assert.rejects(h.request('https://127.0.0.1:8066/healthz'), /transport failed/);
      await service.stop();
    } finally { await service.cleanup(); }
  });
  console.log('PASS wildcard HTTP/UDP, alternate-address sFlow, safety notice and verified optional TLS');
}
if (require.main === module) h.cli(main);
module.exports = {main, wildcard: h.wildcard};
