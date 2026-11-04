'use strict';
// Real UDP and HTTP against the supplied executable; no database injection.
const assert = require('node:assert/strict');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');

function packet(seq, client = '192.0.2.10', uptime) {
  const b = h.sflow(seq);
  if (uptime !== undefined) b.writeUInt32BE(uptime, 20);
  const offset = b.indexOf(Buffer.from('c000020a', 'hex'));
  assert.ok(offset > 0, 'fixture source address absent');
  Buffer.from(client.split('.').map(Number)).copy(b, offset);
  return b;
}

async function scenario(binary, name, exercise) {
  await h.withTemp('vyos-sflow-sampling-', async tmp => {
    const password = h.password();
    const web = await h.port(), udp = await h.port('udp');
    const base = `http://127.0.0.1:${web}`;
    const service = new h.Service(binary, ['serve', '-data', path.join(tmp, 'data'), '-addr', `127.0.0.1:${web}`, '-listen', `127.0.0.1:${udp}`, '-db-threads', '1'], {...h.cleanEnv(), VYOS_SFLOW_PASSWORD: password}, path.join(tmp, 'private-server.log'));
    const get = route => h.request(base + route, {auth: h.basic(password)});
    const records = async () => (await get('/api/records?range=1h')).rows || [];
    const bytes = (rows, client) => rows.filter(r => r.client === client).reduce((n, r) => n + r.wire, 0);
    const options = {alive: () => service.alive()};
    try {
      service.start(); await h.waitHealth(base, options);
      await exercise({
        send: packets => h.send(packets, udp), records, bytes,
        wait: (fn, why) => h.waitFor(fn, why, options),
        clear: () => h.request(base + '/api/cleanup', {auth: h.basic(password), payload: {all: true, confirm: true}}),
        total: async () => (await get('/api/overview?range=1h')).totals.wire,
      });
      console.log(`PASS sampling lifecycle: ${name}`);
    } finally { await service.cleanup(); }
  });
}

async function main() {
  const binary = h.binaryArg(process.argv.slice(2));
  const failures = [];
  const cases = [];
  for (const count of [1, 11]) {
    cases.push([`latest duplicate after ${count} samples`, async x => {
      await x.send(Array.from({length: count}, (_, i) => packet(i + 1)));
      await x.wait(async () => await x.total() === count * 7800, 'initial samples persisted');
      await x.send([packet(count), packet(count + 1, '192.0.2.99')]);
      // A later distinct row is a persistence barrier for the earlier duplicate.
      await x.wait(async () => x.bytes(await x.records(), '192.0.2.99') > 0, 'marker sample persisted');
      assert.equal(x.bytes(await x.records(), '192.0.2.10'), count * 7800, 'duplicate changed old-client totals');
    }]);
  }
  cases.push(['clear then sequence gap without service restart', async x => {
    await x.send(Array.from({length: 11}, (_, i) => packet(i + 1)));
    await x.wait(async () => await x.total() === 11 * 7800, 'pre-clear samples persisted');
    await x.clear(); assert.equal(await x.total(), 0, 'clear did not empty history');
    await x.send([packet(13, '192.0.2.99')]);
    await x.wait(async () => x.bytes(await x.records(), '192.0.2.99') > 0, 'post-clear sample persisted');
    assert.equal(x.bytes(await x.records(), '192.0.2.10'), 0, 'cleared old-client history reappeared');
  }]);
  for (const uptimeDays of [0, 30]) {
    cases.push([`exporter restart after ${uptimeDays || 'short'} days cannot modify old-client history`, async x => {
      await x.send(Array.from({length: 11}, (_, i) => packet(50001 + i, '192.0.2.10', uptimeDays ? uptimeDays * 24 * 60 * 60 * 1000 + i * 1000 : undefined)));
      await x.wait(async () => await x.total() === 11 * 7800, 'pre-restart samples persisted');
      await x.send([packet(1, '192.0.2.99'), packet(3, '192.0.2.99')]);
      await x.wait(async () => x.bytes(await x.records(), '192.0.2.99') >= 2 * 7800, 'new exporter epoch persisted');
      assert.equal(x.bytes(await x.records(), '192.0.2.10'), 11 * 7800, 'new epoch compensation modified old-client history');
    }]);
  }
  for (const [name, fn] of cases) {
    try { await scenario(binary, name, fn); }
    catch (err) { failures.push(name); console.error(`FAIL sampling lifecycle: ${name}: ${err.message}`); }
  }
  assert.deepEqual(failures, [], 'sampling lifecycle regressions');
}
if (require.main === module) h.cli(main);
module.exports = {packet, main};
