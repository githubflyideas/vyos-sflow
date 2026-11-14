#!/usr/bin/env node
'use strict';
// Upgrade over a real data directory: the previous release collects sFlow
// and stops; this build opens the same directory, keeps every record,
// collects more and keeps it all across a restart. Run with the previous
// release's binary first and this build's second.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');
async function main() {
  const [oldBinary, newBinary] = process.argv.slice(2).map(p => path.resolve(p));
  assert.ok(oldBinary && newBinary && fs.existsSync(oldBinary) && fs.existsSync(newBinary), 'Usage: node upgrade-data.cjs OLD_BINARY NEW_BINARY');
  const password = h.password(), env = {...h.cleanEnv(), VYOS_SFLOW_PASSWORD: password};
  const webport = await h.port(), udpport = await h.port('udp'), base = `http://127.0.0.1:${webport}`;
  const total = async () => (await h.request(base + '/api/overview?range=1h', {auth: h.basic(password)})).totals.wire;
  fs.mkdirSync('evidence', {recursive: true});
  await h.withTemp('vyos-sflow-upgrade-', async tmp => {
    const args = ['serve', '-data', tmp, '-addr', `127.0.0.1:${webport}`, '-listen', `127.0.0.1:${udpport}`, '-db-threads', '1'];
    const run = async (binary, name, during) => {
      const service = new h.Service(binary, args, env, path.resolve(`evidence/upgrade-${name}.log`));
      const options = {alive: () => service.alive()};
      try {
        service.start();
        await h.waitHealth(base, options);
        await during(options);
        await service.stop();
      } finally { await service.cleanup(); }
    };
    await run(oldBinary, 'old', async options => {
      await h.send(Array.from({length: 10}, (_, i) => h.sflow(i + 1)), udpport);
      await h.waitFor(async () => await total() === 10 * 78 * 100, 'the previous release did not store the samples', options);
    });
    await run(newBinary, 'new', async options => {
      assert.equal(await total(), 10 * 78 * 100, 'the upgrade lost or changed stored history');
      await h.send([h.sflow(11, '192.0.2.77', 0, 40003)], udpport);
      await h.waitFor(async () => await total() === 11 * 78 * 100, 'the upgraded build does not store new samples', options);
    });
    await run(newBinary, 'new-restart', async () => {
      assert.equal(await total(), 11 * 78 * 100, 'history written after the upgrade was lost on restart');
    });
  });
  console.log('PASS previous release -> this build on one data directory: history kept, new samples stored, restart kept');
}
h.cli(main);
