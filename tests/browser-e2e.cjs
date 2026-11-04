const boundary = require('./browser-product-boundary.cjs');
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
(async () => {
  const base = process.env.E2E_URL;
  assert(base && process.env.E2E_PASSWORD, 'real service URL and credential required');
  const browser = await chromium.launch({headless: true});
  try {
    const page = await browser.newPage({viewport: {width: 1440, height: 1000}});
    const verifyNetwork = boundary.observe(page, base);
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    await page.route('**/*', route => {
      const url = new URL(route.request().url());
      return url.origin === new URL(base).origin || url.protocol === 'data:' ? route.continue() : route.abort();
    });
    await page.goto(base);
    for (const dark of [0,1]) {
      const response = await page.request.get(base + '/logo?dark=' + dark);
      assert.equal(response.status(),200);
      const svg = await response.text();
      const image = svg.match(/href="data:image\/png;base64,([A-Za-z0-9+/=]+)"/);
      assert.ok(image,'Real server did not return the vyos-sflow project logo');
      assert.deepEqual(Buffer.from(image[1],'base64'),fs.readFileSync(require('node:path').join(__dirname,'../docs/images/vyos-sflow-logo.png')));
    }
    await page.waitForFunction(() => {const i=document.querySelector('#login .logo');return i?.complete && i.naturalWidth>0;});
    await page.locator('#loginForm input[name=user]').fill('admin');
    await page.locator('#loginForm input[name=password]').fill(process.env.E2E_PASSWORD);
    await page.locator('#loginForm button').click();
    await page.locator('#app').waitFor({state: 'visible'});
    await page.waitForFunction(() => {const i=document.querySelector('.brand .logo');return i?.complete && i.naturalWidth>0;});
    await page.locator('#v-overview .panel').first().waitFor({state: 'visible'});
    assert.match(await page.title(), /vyos-sflow/);
    assert.equal(await page.locator('#nav [data-v=sandbox]').count(), 0, 'PCAP import still offered');
    fs.mkdirSync('evidence', {recursive: true});
    await page.screenshot({path: 'evidence/browser-overview.png', fullPage: true});
    await page.locator('#nav [data-v=topn]').click();
    await page.locator('#v-topn [data-tm=talkers][aria-pressed=true]').waitFor({state:'visible'});
    await page.waitForFunction(() => /192\.0\.2\.10|198\.51\.100\.20/.test(document.querySelector('#v-topn')?.textContent || ''));
    await page.locator('#v-topn [data-tm=table]').click();
    await page.locator('#topDim').waitFor({state:'visible'});
    await page.waitForFunction(() => /192\.0\.2\.10|198\.51\.100\.20/.test(document.querySelector('#v-topn')?.textContent || ''));
    await page.screenshot({path:'evidence/browser-top.png', fullPage:true});
    await boundary.verify(page);
    verifyNetwork();
    assert.deepEqual(errors, [], 'browser JavaScript errors');
    console.log('PASS Chromium login, real overview/host history, five basic page classes, bounded Top/ranges, 15 languages and no advanced requests');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
