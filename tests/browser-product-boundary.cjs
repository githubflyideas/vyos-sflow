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
// Real-service semantics: responses are not mocked; await their matching render.
async function verifySemantics(page) {
  await page.locator('#lang').selectOption('en');
  await page.waitForFunction(() => document.documentElement.lang === 'en');
  await page.locator('#nav [data-v=overview]').click();
  await page.locator('#kpiScope').waitFor({state:'visible'});
  for (const dir of ['in','out']) {
    const response = page.waitForResponse(r => new URL(r.url()).pathname === '/api/overview' && new URL(r.url()).searchParams.get('dir') === dir);
    await page.locator(`[data-od="${dir}"]`).click();
    const res = await response;
    assert.equal(res.status(), 200);
    const data = await res.json();
    await page.waitForFunction(({dir, iface}) => document.querySelector(`[data-od="${dir}"]`)?.getAttribute('aria-pressed') === 'true' && document.querySelector('#directionIface')?.textContent === iface, {dir, iface:data.dir_iface || 'No sampled interface available'});
    assert.match(await page.locator('#kpiScope').innerText(), /All traffic/);
    assert.match(await page.locator('#directionScope').innerText(), /exporter\/interface/);
    assert.doesNotMatch(await page.locator('#directionScope').innerText(), /WAN/);
  }
  await page.locator('#nav [data-v=topn]').click();
  await page.locator('[data-tm=talkers]').waitFor({state:'visible'});
  await page.locator('[data-tm=talkers]').click();
  await page.locator('#topPerspectives').waitFor({state:'visible'});
  assert.match(await page.locator('#topPerspectives').innerText(), /inferred.*same traffic.*do not add.*not just the Top 30/);
  assert.equal(await page.locator('#v-topn .sum').count(), 2);
  for (const text of await page.locator('#v-topn .sum').allTextContents()) assert.match(text, /not Top 30 subtotal/);
  await page.locator('#nav [data-v=records]').click();
  await page.locator('#recordWindow').waitFor({state:'visible'});
  const response = page.waitForResponse(r => new URL(r.url()).pathname === '/api/records' && new URL(r.url()).searchParams.get('range') === '48h');
  await page.locator('#range [data-r="48h"]').click();
  const res = await response;
  assert.equal(res.status(),200);
  const data = await res.json();
  assert.equal(data.limit,500);
  assert.equal(Date.parse(data.to)-Date.parse(data.from),24*60*60*1000);
  await page.waitForFunction(d => {const el=document.querySelector('#recordWindow');return el?.dataset.from===d.from && el?.dataset.to===d.to && el?.dataset.limit===String(d.limit);}, data);
  assert.match(await page.locator('#recordWindow').innerText(), /Maximum 24 hours/);
  assert.match(await page.locator('#recordEstimates').innerText(), /not the configured sampling ratio/);
  const multipliers = await page.locator('.rec tbody tr:not(:has(.empty)) td:last-child').allTextContents();
  assert.equal(multipliers.length,data.rows.length);
  data.rows.forEach((row,i) => assert.equal(multipliers[i], Number.isFinite(row.mult) && row.mult>0 ? 'Estimated ×'+new Intl.NumberFormat('en',{maximumFractionDigits:2,minimumFractionDigits:2}).format(row.mult) : '—'));
  assert.equal(await page.locator('#range [data-r="48h"]').getAttribute('aria-pressed'),'true');
  await page.locator('#range [data-r="1h"]').click();
}
async function verify(page) {
  await verifySemantics(page);
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
    const catalogResponse = await page.request.get(new URL(`i18n/${code}.json`, page.url()).href);
    assert.equal(catalogResponse.status(), 200);
    const catalog = await catalogResponse.json();
    await page.locator('#lang').selectOption(code);
    await page.waitForFunction(c => document.documentElement.lang === c, code);
    assert.equal(await page.locator('html').getAttribute('dir'), ['ar','ur'].includes(code)?'rtl':'ltr');
    for (const [view, selector, key] of [['overview','#kpiScope','ov.kpi_scope'], ['topn','#topPerspectives','topn.perspectives'], ['records','#recordEstimates','rec.estimates']]) {
      await page.locator(`#nav [data-v=${view}]`).click();
      if (view === 'topn') {
        await page.locator('[data-tm=talkers]').waitFor({state:'visible'});
        await page.locator('[data-tm=talkers]').click();
      }
      // Await translated business content, not the old view or the shell language.
      await page.waitForFunction(({selector,text}) => document.querySelector(selector)?.textContent === text, {selector,text:catalog[key]});
      if (view === 'overview') assert.ok((await page.locator('#directionScope').innerText()).includes(catalog['ov.direction_scope']));
      if (view === 'topn') for (const text of await page.locator('#v-topn .sum').allTextContents()) assert.ok(text.includes(catalog['topn.global_total']));
      if (view === 'records') {
        assert.doesNotMatch(await page.locator('#recordWindow').innerText(), /\{(?:from|to|limit)\}/);
        assert.equal(await page.locator('#recordWindow').getAttribute('data-limit'), '500');
      }
      await page.setViewportSize({width:390,height:844});
      assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth <= innerWidth),true,`${code}/${view}: mobile horizontal overflow`);
      await page.setViewportSize({width:1440,height:1000});
    }
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
