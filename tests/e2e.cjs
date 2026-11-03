#!/usr/bin/env node
'use strict';
// Real Linux UDP -> DuckDB -> authenticated HTTP/browser -> restart.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');
async function main() {
  const binary = h.binaryArg(process.argv.slice(2), true);
  const password = h.password(), env = {...h.cleanEnv(), VYOS_SFLOW_PASSWORD: password};
  const webport = await h.port(), udpport = await h.port('udp'), base = `http://127.0.0.1:${webport}`;
  const get = route => h.request(base + route, {auth: h.basic(password)});
  fs.mkdirSync('evidence', {recursive: true});
  await h.withTemp('vyos-sflow-e2e-', async tmp => {
    const service = new h.Service(binary, ['serve', '-data', tmp, '-addr', `127.0.0.1:${webport}`, '-listen', `127.0.0.1:${udpport}`, '-db-threads', '1'], env, path.resolve('evidence/e2e-server.log'));
    const options = {alive: () => service.alive()};
    const start = async () => { service.start(); await h.waitHealth(base, options); };
    const total = async () => (await get('/api/overview?range=1h')).totals.wire;
    try {
      await start();
      await h.request(base + '/api/overview?range=1h', {expected: 401});
      await h.send(Array.from({length: 10}, (_, i) => h.sflow(i + 1)), udpport);
      await h.waitFor(async () => await total() === 10 * 78 * 100, 'sFlow persisted byte total mismatch', options);
      assert.equal((await get('/api/sources')).stream.enforced, false);
      // Reuse sequence numbers across identities, with distinct flows so cross-observer dedup does not apply.
      await h.send([h.sflow(1, '192.0.2.99', 0, 40001), h.sflow(1, '127.0.0.1', 1, 40002)], udpport);
      const expected = 12 * 78 * 100;
      await h.waitFor(async () => await total() === expected, 'Distinct valid sFlow identities were rejected or deduplicated together', options);
      const rejected = [];
      for (let i = 0; i < 5; i++) rejected.push(h.netflow(), h.ipfix(), Buffer.from([0, 0, 0, 5]));
      await h.send(rejected, udpport); await h.sleep(2000);
      assert.equal(await total(), expected, 'Malformed or non-sFlow datagrams changed totals');
      if (process.argv.includes('--browser')) h.run(process.execPath, [path.join(__dirname, 'browser-e2e.cjs')], {env: {...env, E2E_URL: base, E2E_PASSWORD: password}, timeout: 120000});
      await service.stop();
      assert.ok(fs.readdirSync(tmp).some(name => name.endsWith('.duckdb')), 'DuckDB file absent');
      await start(); assert.equal(await total(), expected, 'Restart lost or duplicated history');
      assert.equal((await get('/api/overview?range=48h')).totals.wire, expected);
      await h.request(base + '/api/overview?range=72h', {auth:h.basic(password),expected:400});
      const stream = (await get('/api/sources')).stream;
      await h.request(base + '/api/cleanup', {auth:h.basic(password),payload:{all:true,confirm:true}});
      assert.equal(await total(),0,'Clear-all left recent history');
      assert.deepEqual((await get('/api/sources')).stream,stream,'Clear-all changed receiver policy');
      await service.stop(); await start();
      assert.equal(await total(),0,'Cleared history reappeared after restart');
      // The same account remains authorized and a new identity is still accepted after cleanup/restart.
      await h.send([h.sflow(1,'192.0.2.100',42)],udpport);
      await h.waitFor(async () => await total() === 78 * 100,'Receiver did not resume after cleanup/restart',options);
      await service.stop();
      console.log('PASS real multi-identity sFlow -> DuckDB -> API; malformed/NetFlow/IPFIX/auth rejection; restart persistence' + (process.argv.includes('--browser') ? '; real browser' : ''));
    } finally { await service.cleanup(); }
  });
}
if (require.main === module) h.cli(main);
module.exports = {main, sflow: h.sflow};
