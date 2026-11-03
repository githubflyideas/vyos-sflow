#!/usr/bin/env node
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const h = require('./runtime-helpers.cjs');
const DEFAULT = 'tryvyossflow66';
async function main() {
  const binary = h.binaryArg(), changed = h.password(), env = h.cleanEnv(), webport = await h.port();
  const base = `http://127.0.0.1:${webport}`, jar = {};
  await h.withTemp('vyos-first-login-', async tmp => {
    const data = path.join(tmp, 'data');
    const service = new h.Service(binary, ['serve', '-data', data, '-addr', `127.0.0.1:${webport}`, '-listen', '127.0.0.1:0'], env, path.join(tmp, 'service.log'));
    const req = (route, expected, options = {}) => h.request(base + route, {expected, ...options});
    const bearer = () => 'Bearer ' + fs.readFileSync(path.join(data, '.tui-token'), 'utf8').trim();
    const start = async () => { service.start(); await h.waitHealth(base, {timeout: 60000, alive: () => service.alive()}); };
    try {
      await start();
      assert.ok(!fs.existsSync(path.join(data, 'initial-password')), 'Plaintext credential file created');
      assert.equal((await req('/api/login', 200, {jar, payload: {user: 'admin', password: DEFAULT}})).must_change, true);
      assert.equal((await req('/api/me', 200, {jar})).must_change, true);
      assert.equal((await req('/api/overview', 403, {jar})).error, 'change_password');
      for (const auth of [h.basic(DEFAULT), bearer()]) assert.equal((await req('/api/overview', 403, {auth})).error, 'change_password');
      await req('/api/password', 400, {jar, payload: {old: DEFAULT, new: DEFAULT}});
      await req('/api/password', 200, {jar, payload: {old: DEFAULT, new: changed}});
      await req('/api/overview', 200, {jar});
      await req('/api/overview', 401, {auth: h.basic(DEFAULT)});
      await req('/api/overview', 200, {auth: h.basic(changed)});
      await req('/api/password', 400, {auth: h.basic(changed), payload: {old: changed, new: DEFAULT}});
      await service.stop();
      env.VYOS_SFLOW_PASSWORD = h.password();
      await start();
      await req('/api/overview', 401, {auth: h.basic(DEFAULT)});
      await req('/api/overview', 401, {auth: h.basic(env.VYOS_SFLOW_PASSWORD)});
      await req('/api/overview', 200, {auth: h.basic(changed)});
      await req('/api/overview', 200, {auth: bearer()});
      await service.stop();
    } finally { await service.cleanup(); }
  });
  console.log('PASS factory login, cookie/Basic/Bearer gate, password change/rejection and restart persistence');
}
if (require.main === module) h.cli(main);
module.exports = {main};
