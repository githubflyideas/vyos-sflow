// Shared real-service browser acceptance; imported by both browser entry points.
const assert = require('node:assert/strict');
const removed = /\/(?:api\/(?:ifaces|recon|findings|threats|geo|geolines|rings|sankey|racks|inventory|names|resolve|report|export|syslog|embeds|license|registration|activation)|world\.json|embed\.(?:js|css)|sponsor\.js)(?:[/?]|$)/;
function observe(page, base) {
  const requests = [], errors = [], failedAssets = [];
  page.on('request', request => requests.push(request.url()));
  page.on('pageerror', error => errors.push(error.message));
  page.on('response', response => {
    if (response.status() >= 400 && /\.(?:js|css|json)(?:\?|$)/.test(response.url())) failedAssets.push(response.url());
  });
  return () => {
    assert.deepEqual(requests.filter(url => removed.test(new URL(url).pathname)), [], 'removed business/API request');
    assert.deepEqual(requests.filter(url => !url.startsWith(base + '/') && !url.startsWith('data:')), [], 'external request');
    assert.deepEqual(errors, [], 'JavaScript errors');
    assert.deepEqual(failedAssets, [], 'failed static resources');
  };
}
async function verify(page) {
  assert.deepEqual(await page.locator('#nav [data-v]').evaluateAll(xs => xs.map(x => x.dataset.v).sort()), ['cleanup','help','overview','records','sources','topn']);
  assert.equal(await page.locator('#lang option').count(), 15);
  assert.equal(await page.locator('#range [data-r="1h"]').getAttribute('aria-pressed'), 'true');
  assert.equal(await page.locator('[type=datetime-local], #embNew, #slSave, #ifsel, #sponsor, #lic').count(), 0);
  for (const view of ['overview', 'topn', 'records', 'sources', 'cleanup', 'help']) {
    await page.locator(`#nav [data-v=${view}]`).click();
    await page.locator(`#v-${view}.on`).waitFor({state:'visible'});
    await page.waitForFunction(v => document.querySelector('#v-' + v)?.textContent.trim().length > 0, view);
    assert.equal(await page.locator(`#v-${view} .empty`).filter({hasText:/TypeError|ReferenceError|database error/i}).count(),0);
  }
  await page.locator('#acct').click();
  await page.locator('#pwForm').waitFor({state:'visible'});
  await page.locator('#nav [data-v=topn]').click();
  await page.locator('#v-topn [data-tm=table]').click();
  await page.locator('#topDim').waitFor({state:'visible'});
  assert.deepEqual(await page.locator('#topDim option').evaluateAll(xs => xs.map(x=>x.value)), ['host','app','proto']);
  for (const dim of ['app','proto','host']) {
    const response = page.waitForResponse(r => new URL(r.url()).pathname === '/api/topn' && new URL(r.url()).searchParams.get('dim') === dim);
    await page.locator('#topDim').selectOption(dim);
    assert.equal((await response).status(),200);
    await page.locator('#topDim').waitFor({state:'visible'});
  }
  for (const [view,endpoint] of [['overview','overview'],['topn','topn']]) {
    await page.locator(`#nav [data-v=${view}]`).click();
    await page.locator(`#v-${view}.on .panel`).first().waitFor({state:'visible'});
    for (const range of ['15m','1h','6h','24h','48h']) {
      const response = page.waitForResponse(r => new URL(r.url()).pathname === '/api/'+endpoint && new URL(r.url()).searchParams.get('range') === range);
      await page.locator(`#range [data-r="${range}"]`).click();
      assert.equal((await response).status(),200);
    }
  }
  assert.deepEqual(await page.locator('#range [data-r]').evaluateAll(xs=>xs.map(x=>x.dataset.r)),['15m','1h','6h','24h','48h']);
  await page.locator('#range [data-r="1h"]').click();
  for (const code of await page.locator('#lang option').evaluateAll(xs=>xs.map(x=>x.value))) {
    await page.locator('#lang').selectOption(code);
    await page.waitForFunction(c => document.documentElement.lang === c, code);
    assert.equal(await page.locator('html').getAttribute('dir'), ['ar','ur'].includes(code)?'rtl':'ltr');
    await page.locator('#topDim').waitFor({state:'visible'});
    assert.equal(await page.locator('#sponsor, #licText, #markChip, #markNote, body.marked').count(),0);
  }
  await page.setViewportSize({width:390,height:844});
  assert.equal(await page.locator('#range [data-r]').count(),5);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth <= innerWidth),true,'mobile horizontal overflow');
  await page.setViewportSize({width:1440,height:1000});
  for (const asset of ['/world.json','/embed.js','/embed.css','/sponsor.js']) {
    assert.equal((await page.request.get(new URL(asset,page.url()).href)).status(),404,asset+' must not ship');
  }
}
module.exports = {observe, verify};
