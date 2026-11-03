// Real first-login browser acceptance against a fresh product process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const dgram = require('node:dgram');
const crypto = require('node:crypto');
const {spawn} = require('node:child_process');
const {chromium} = require('playwright');
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
async function freePort(udp = false) {
  const socket = udp ? dgram.createSocket('udp4') : net.createServer();
  await new Promise((resolve, reject) => {
    socket.once('error', reject);
    if (udp) socket.bind(0, '127.0.0.1', resolve);
    else socket.listen(0, '127.0.0.1', resolve);
  });
  const port = socket.address().port;
  await new Promise(resolve => socket.close(resolve));
  return port;
}
(async () => {
  const binary = path.resolve(process.argv[2]);
  const data = fs.mkdtempSync(path.join(os.tmpdir(), 'vyos-first-browser-'));
  fs.mkdirSync('evidence', {recursive: true});
  const log = fs.openSync('evidence/first-login-server.log', 'w');
  const env = require('./runtime-helpers.cjs').cleanEnv();
  const port = await freePort(), udp = await freePort(true);
  const base = `http://127.0.0.1:${port}`;
  const server = spawn(binary, ['serve', '-data', data, '-addr', `127.0.0.1:${port}`, '-listen', `127.0.0.1:${udp}`], {env, stdio: ['ignore', log, log]});
  let browser;
  try {
    let ready = false;
    for (let i = 0; i < 300; i++) {
      assert.equal(server.exitCode, null, 'product exited before readiness');
      try { ready = (await fetch(base + '/healthz')).ok; } catch {}
      if (ready) break;
      await delay(100);
    }
    assert.ok(ready, 'fresh product did not become ready');
    browser = await chromium.launch({headless: true});
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', err => errors.push(err.message));
    await page.route('**/*', route => route.request().url().startsWith(base + '/') || route.request().url().startsWith('data:') ? route.continue() : route.abort());
    await page.goto(base);
    await page.locator('#loginForm input[name=user]').fill('admin');
    await page.locator('#loginForm input[name=password]').fill('tryvyossflow66');
    await page.locator('#loginForm button').click();
    await page.locator('#pwForm').waitFor({state: 'visible'});
    assert.ok(await page.locator('body').evaluate(el => el.classList.contains('mustchange')));
    assert.equal(await page.evaluate(async () => (await fetch('/api/overview')).status), 403);
    // A reload must preserve the server-side restriction, not just a UI flag.
    await page.reload();
    await page.locator('#pwForm').waitFor({state: 'visible'});
    assert.equal(await page.evaluate(async () => (await fetch('/api/overview')).status), 403);
    const password = crypto.randomBytes(24).toString('base64url');
    await page.locator('#pwForm input[name=new]').fill(password);
    await page.locator('#pwForm input[name=again]').fill(password);
    await page.locator('#pwForm button[type=submit]').click();
    await page.locator('#v-overview .panel').first().waitFor({state: 'visible'});
    assert.equal(await page.evaluate(async () => (await fetch('/api/overview')).status), 200);
    const old = await fetch(base + '/api/overview', {headers: {Authorization: 'Basic ' + Buffer.from('admin:tryvyossflow66').toString('base64')}});
    assert.equal(old.status, 401, 'default credential remains valid after change');
    assert.deepEqual(errors, []);
    await page.screenshot({path: 'evidence/first-login-complete.png', fullPage: true});
    console.log('PASS Chromium: default login restricted before and after reload; password change unlocks analytics; old password rejected');
  } finally {
    if (browser) await browser.close();
    if (server.exitCode === null) {
      server.kill('SIGTERM');
      for (let i = 0; i < 300 && server.exitCode === null; i++) await delay(100);
      if (server.exitCode === null) server.kill('SIGKILL');
      if (server.exitCode === null) await new Promise(resolve => server.once('exit', resolve));
    }
    fs.closeSync(log);
    fs.rmSync(data, {recursive: true, force: true});
  }
})().catch(err => { console.error(err); process.exitCode = 1; });
