const boundary = require('./browser-product-boundary.cjs');
// Real browser regression for removal of advertising and product registration.
// Run: node tests/browser-no-registration.cjs /absolute/path/to/vyos-sflow
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
  const data = fs.mkdtempSync(path.join(os.tmpdir(), 'vyos-no-registration-'));
  fs.mkdirSync('evidence', {recursive: true});
  const log = fs.openSync('evidence/no-registration-server.log', 'w');
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
    const verifyNetwork = boundary.observe(page, base);
    const errors = [], requests = [], failedAssets = [];
    page.on('request', request => requests.push(request.url()));
    page.on('response', response => {
      if (response.status() >= 400 && /\.(?:js|css|json)(?:\?|$)/.test(response.url())) failedAssets.push(response.url());
    });
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
    const absent = async () => {
      assert.equal(await page.locator('#sponsor, #lic, #licText, #licFile, #licSave, #markChip, #markNote, .markchip, .marknote').count(), 0);
      assert.equal(await page.locator('body.marked').count(), 0);
      assert.doesNotMatch(await page.locator('body').innerText(), /EVALUATION COPY|not registered|How to register/);
    };
    await absent();
    await boundary.verify(page);
    // This server owns a fresh disposable data directory: safe to exercise clear-all.
    await page.locator('#lang').selectOption('en');
    await page.waitForFunction(()=>document.documentElement.lang==='en');
    await page.locator('#nav [data-v=cleanup]').click();
    const clear = page.locator('[data-cl-all]'); await clear.waitFor({state:'visible'});
    assert.equal(await page.locator('#v-cleanup button').count(),1);
    assert.equal(await page.locator('#v-cleanup select, #v-cleanup input, [data-cl]').count(),0);
    const beforeStream = await page.evaluate(async ()=>(await (await fetch('/api/status')).json()).stream);
    assert.equal(beforeStream.enforced,false);
    assert.equal(beforeStream.configured,false);
    const posted = [];
    page.on('request',r=>{if(new URL(r.url()).pathname==='/api/cleanup' && r.method()==='POST') posted.push(r.postDataJSON());});
    page.once('dialog',d=>d.dismiss()); await clear.click();
    assert.equal(posted.length,0);
    let cancelledDialogs=0;
    const cancelSecond=async d=>{if(++cancelledDialogs===1) await d.accept(); else await d.dismiss();};
    page.on('dialog',cancelSecond); await clear.click(); page.off('dialog',cancelSecond);
    assert.equal(cancelledDialogs,2); assert.equal(posted.length,0);
    let confirmations=0;
    const accept = async d=>{confirmations++;await d.accept();}; page.on('dialog',accept);
    const cleared = page.waitForResponse(r=>new URL(r.url()).pathname==='/api/cleanup' && r.request().method()==='POST');
    await clear.click(); assert.equal((await cleared).status(),200);
    await page.waitForFunction(()=>document.querySelector('#clMsg')?.textContent.includes('Deleted'));
    page.off('dialog',accept);
    assert.equal(confirmations,2); assert.deepEqual(posted,[{all:true,confirm:true}]);
    const afterStream = await page.evaluate(async ()=>(await (await fetch('/api/status')).json()).stream);
    assert.deepEqual(afterStream,beforeStream);
    assert.equal((await page.evaluate(async ()=>(await (await fetch('/api/me')).json()).user)),'admin');
    verifyNetwork();
    await absent();
    await page.emulateMedia({media: 'print'});
    await absent();
    assert.equal(await page.evaluate(() => {
      const selection = getSelection(), range = document.createRange();
      range.selectNodeContents(document.querySelector('#main')); selection.removeAllRanges(); selection.addRange(range);
      const event = new ClipboardEvent('copy', {bubbles: true, cancelable: true, clipboardData: new DataTransfer()});
      document.dispatchEvent(event);
      return event.defaultPrevented;
    }), false, 'copy remains unmodified');
    assert.deepEqual(requests.filter(url => /sponsor|ads-sponsor|\/api\/license|\/api\/registration|\/api\/activation/i.test(url)), []);
    assert.deepEqual(requests.filter(url => !url.startsWith(base + '/') && !url.startsWith('data:')), []);
    assert.deepEqual(failedAssets, []);
    assert.deepEqual(errors, []);
    await page.screenshot({path: 'evidence/no-registration-complete.png', fullPage: true});
    console.log('PASS Chromium: first-login security, 15 languages, basic pages and bounded Top/history, no advertising/registration DOM or requests, clean copy/print and absent advanced resources');
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
