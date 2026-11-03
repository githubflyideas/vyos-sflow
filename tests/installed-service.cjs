#!/usr/bin/env node
'use strict';
// Destructive acceptance ONLY on a disposable root GitHub Actions Linux runner.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');
const h = require('./runtime-helpers.cjs');
function parseArgs(args) {
  const values = {};
  for (let i = 0; i < args.length; i += 2) {
    assert.ok(['--installer', '--binary'].includes(args[i]) && args[i + 1] && !args[i + 1].startsWith('--') && !values[args[i]], 'Usage: node installed-service.cjs --installer FILE.run --binary BINARY');
    values[args[i]] = path.resolve(args[i + 1]);
  }
  assert.ok(values['--installer'] && values['--binary'], 'Both --installer and --binary are required');
  return {installer: values['--installer'], binary: values['--binary']};
}
async function main() {
  const {installer, binary} = parseArgs(process.argv.slice(2));
  // lstat also rejects a dangling symlink; no side effects occur before this gate.
  let existing = false;
  try { fs.lstatSync('/config/vyos-sflow'); existing = true; } catch (e) { if (e.code !== 'ENOENT') throw e; }
  h.assertInstallSafe({existing});
  assert.ok(fs.statSync(installer).isFile(), 'Installer missing');
  const expectedHash = h.hash(binary), password = h.password(), base = '/config/vyos-sflow', url = 'http://127.0.0.1:8066';
  const req = (expected, secret, payload) => h.request(url + (payload === undefined ? '/api/overview?range=1h' : '/api/password'), {expected, auth: h.basic(secret), payload, timeout: 15000});
  const systemctl = (...args) => h.run('systemctl', args);
  const verifyInstall = async () => {
    systemctl('is-active', '--quiet', 'vyos-sflow.service');
    assert.equal(h.hash(base + '/bin/vyos-sflow'), expectedHash, 'Installed binary differs from candidate');
    assert.equal(systemctl('show', 'vyos-sflow.service', '-p', 'User', '--value').trim(), 'vyos-sflow', 'Service must run unprivileged');
    await h.waitHealth(url, {timeout: 30000});
  };
  const assertHistory = async expected => assert.equal((await req(200, password)).totals.wire, expected, 'Restart/reinstall lost or duplicated history');
  try {
    h.run('bash', [installer], {input: 'y\n', env: h.cleanEnv()});
    await verifyInstall();
    assert.ok(!fs.existsSync(base + '/data/initial-password'), 'Plaintext credential file created');
    assert.equal((await req(403, 'tryvyossflow66')).error, 'change_password');
    await req(200, 'tryvyossflow66', {old: 'tryvyossflow66', new: password});
    await req(200, password);
    assert.equal((await h.request(url + '/api/sources', {auth: h.basic(password)})).stream.enforced, false);
    await h.send([...Array.from({length: 10}, (_, i) => h.sflow(i + 1)), h.sflow(1, '192.0.2.99', 0, 40001), h.sflow(1, '127.0.0.1', 1, 40002)], 6343);
    const expected = 12 * 78 * 100;
    await h.waitFor(async () => (await req(200, password)).totals.wire === expected, 'Installed service did not persist real sFlow');
    const before = fs.readFileSync(base + '/service.env');
    const rejected = spawnSync('bash', [installer], {input: 'y\n', encoding: 'utf8', timeout: 180000, env: h.cleanEnv()});
    assert.ok(!rejected.error && rejected.status !== 0 && /advanced/.test(rejected.stderr), 'Wizard failed to reject existing installation safely');
    assert.ok(fs.readFileSync(base + '/service.env').equals(before), 'Rejected wizard changed environment');
    assert.equal(h.hash(base + '/bin/vyos-sflow'), expectedHash, 'Rejected wizard replaced binary');
    await assertHistory(expected);
    systemctl('restart', 'vyos-sflow.service'); await verifyInstall(); await assertHistory(expected);
    h.run('bash', [base + '/packaging/install.sh', '--binary', binary], {env: h.cleanEnv()});
    assert.ok(fs.readFileSync(base + '/service.env').equals(before), 'Advanced reinstall changed environment');
    await verifyInstall();
    await req(401, 'tryvyossflow66'); await assertHistory(expected);
    console.log('PASS real .run/systemd: exact binary, unprivileged service, first login, real sFlow history and credentials survive restart/reinstall; existing wizard safely rejected');
  } finally { spawnSync('systemctl', ['stop', 'vyos-sflow.service'], {timeout: 30000, stdio: 'ignore'}); }
}
if (require.main === module) h.cli(main);
module.exports = {main, parseArgs, waitHealth: h.waitHealth};
