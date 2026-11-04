'use strict';
// Exercise the README commands with the actual packaged artifact staged locally.
// Download URLs are checked separately; this does not pretend to fetch an unpublished release.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');
async function main() {
  assert.equal(process.platform, 'linux');
  assert.equal(process.argv.length, 4, 'Usage: node quick-start.cjs ARCHIVE SHA256SUMS');
  const archive = path.resolve(process.argv[2]), sums = path.resolve(process.argv[3]);
  const readme = fs.readFileSync(path.join(__dirname, '../README.md'), 'utf8');
  const block = readme.split('### 2. Download and run')[1].match(/```sh\r?\n([\s\S]*?)```/)[1].trim();
  const lines = block.split(/\r?\n/);
  assert.ok(lines[0].endsWith('/' + path.basename(archive) + ' &&'), 'README download does not name the tested archive');
  assert.match(lines[1], /\/SHA256SUMS &&$/);
  assert.equal(lines.at(-1), './vyos-sflow serve');
  const root = lines.find(line => line.startsWith('cd ')).match(/^cd ([\w.-]+) &&$/)[1];
  const version = fs.readFileSync(path.join(__dirname, '../CHANGELOG.md'), 'utf8').match(/^## (\d+\.\d+\.\d+)\s*$/m)[1];
  await h.withTemp('vyos-sflow-quick-start-', async tmp => {
    fs.copyFileSync(archive, path.join(tmp, path.basename(archive)));
    fs.copyFileSync(sums, path.join(tmp, 'SHA256SUMS'));
    // Execute checksum, extraction and cd exactly as documented, without downloads.
    h.run('bash', ['-c', lines.slice(2, -1).join('\n').replace(/&&\s*$/, '')], {cwd: tmp});
    const cwd = path.join(tmp, root);
    assert.ok(h.run(path.join(cwd, 'vyos-sflow'), ['version']).startsWith(`vyos-sflow ${version} (`));
    const password = h.password();
    const service = new h.Service('./vyos-sflow', ['serve'], {...h.cleanEnv(), VYOS_SFLOW_PASSWORD: password}, path.join(tmp, 'private.log'), {cwd});
    try {
      service.start();
      await h.waitHealth('http://127.0.0.1:8066', {alive: () => service.alive()});
      assert.ok(fs.statSync(path.join(cwd, 'vyso_sflow_data')).isDirectory(), 'default relative data directory absent');
      await h.send([h.sflow(1)], 6343);
      await h.waitFor(async () => (await h.request('http://127.0.0.1:8066/api/overview?range=1h', {auth: h.basic(password)})).totals.wire === 7800, 'README foreground command did not persist actual UDP traffic');
      await service.stop();
      console.log('PASS packaged README checksum/extract/cd/serve, default data directory and real UDP');
    } finally { await service.cleanup(); }
  });
}
h.cli(main);
