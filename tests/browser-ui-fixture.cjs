// Synthetic API fixtures over production assets: UI evidence, NOT backend acceptance.
// Run with NODE_PATH pointing to a Playwright installation, node tests/browser-ui-fixture.cjs.
const {chromium} = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const boundary = require('./browser-product-boundary.cjs');
const root = path.join(__dirname, '../internal/web/static');
(async () => {
 const server = require('node:http').createServer((req,res)=>{res.writeHead(404);res.end('Not found');});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const base = 'http://127.0.0.1:'+server.address().port;
 const browser = await chromium.launch({headless:true});
 try {
  const page = await browser.newPage({viewport:{width:1440,height:1000}});
  const check = boundary.observe(page, base);
  const stream = {enforced:true,configured:true,agent:'127.0.0.1',sub_agent:0,rejected_streams:19};
  const series = {times:[],names:[],values:[]};
  const cleanupPosts = [];
  const payloads = {
   '/api/status': {stream,version:'synthetic-fixture',now:Date.now(),records_per_sec:0,dropped:0,counter_samples:0,dropped_counters:0,write_errors:0,disk_bytes:0,disk_free:1024,disk_need:-1,retention_days:2,source_warnings:0},
   '/api/me':{user:'fixture-admin',admin:true,must_change:false,fixed:false},
   '/api/users':{users:[]},
   '/api/cleanup':{hot_rows:0,segment_rows:0,disk_bytes:0,oldest:0},
   '/api/overview':{totals:{wire:0,pkts:0,flows:0},now_bps:0,series},
   '/api/series':series,
   '/api/topn':{rows:[{key:'SYNTHETIC-ROW',wire:100,pkts:1,flows:1}]},
   '/api/sources':{stream,sources:[],listeners:[],stale:[]}
  };
  await page.route('**/*',async route => {
   const u = new URL(route.request().url());
   if(u.origin !== base) return route.abort();
   if(u.pathname === '/api/manual') return route.fulfill({contentType:'text/markdown',body:'# Synthetic manual fixture'});
   if(u.pathname === '/api/cleanup' && route.request().method() === 'POST') {cleanupPosts.push(route.request().postDataJSON()); return route.fulfill({json:{rows:100,bytes:1024}});}
   if(payloads[u.pathname]) return route.fulfill({json:payloads[u.pathname]});
   if(u.pathname.startsWith('/api/')) throw new Error('Unexpected fixture API: '+u.pathname);
   const relative = u.pathname === '/' ? 'index.html' : u.pathname.slice(1);
   const file = path.join(root,relative);
   if(!file.startsWith(root+path.sep) || !fs.existsSync(file)) return route.fulfill({status:404,body:'Not found'});
   return route.fulfill({path:file});
  });
  await page.goto(base);
  await page.locator('#v-overview .panel').first().waitFor({state:'visible'});
  await boundary.verify(page);
  await page.locator('#lang').selectOption('en');
  await page.waitForFunction(()=>document.documentElement.lang==='en');
  await page.locator('#nav [data-v=sources]').click();
  await page.waitForFunction(()=>document.querySelector('#v-sources .stream-policy')?.textContent.includes('sFlow agent 127.0.0.1 / sub-agent 0'));
  assert.match(await page.locator('#v-sources').innerText(),/sFlow agent 127.0.0.1 \/ sub-agent 0/);
  stream.enforced=false; stream.configured=false; stream.agent='';
  await page.locator('#nav [data-v=topn]').click();
  await page.locator('#topDim').waitFor({state:'visible'});
  await page.locator('#nav [data-v=sources]').click();
  await page.waitForFunction(()=>document.querySelector('#v-sources table') && !document.querySelector('#v-sources .stream-policy'));
  assert.equal(await page.locator('#v-sources .stream-policy').count(),0);
  assert.match(await page.locator('#v-sources').innerText(),/All sFlow sources/);
  assert.doesNotMatch(await page.locator('#v-sources').innerText(),/No samples are accepted|Configure -agent/);
  await page.locator('#nav [data-v=cleanup]').click();
  const clear = page.locator('[data-cl-all]'); await clear.waitFor({state:'visible'});
  assert.equal(await page.locator('#v-cleanup button').count(),1);
  assert.equal(await page.locator('#v-cleanup select, #v-cleanup input, [data-cl]').count(),0);
  page.once('dialog',d=>d.dismiss()); await clear.click();
  assert.equal(cleanupPosts.length,0,'cancel sent destructive request');
  let cancelledDialogs=0;
  const cancelSecond=async d=>{if(++cancelledDialogs===1) await d.accept(); else await d.dismiss();};
  page.on('dialog',cancelSecond); await clear.click(); page.off('dialog',cancelSecond);
  assert.equal(cancelledDialogs,2); assert.equal(cleanupPosts.length,0);
  let dialogs=0;
  const accept = async d=>{dialogs++;await d.accept();}; page.on('dialog',accept);
  await clear.click();
  await page.waitForFunction(()=>document.querySelector('#clMsg')?.textContent.includes('Deleted'));
  page.off('dialog',accept);
  assert.equal(dialogs,2);
  assert.deepEqual(cleanupPosts,[{all:true,confirm:true}]);
  payloads['/api/me'].admin=false;
  await page.reload();
  await page.waitForFunction(()=>document.querySelector('#v-cleanup')?.textContent.includes('Only the administrator'));
  assert.equal(await page.locator('[data-cl-all]').count(),0);
  check();
  console.log('PASS synthetic Chromium fixture: production assets, retained pages, 48h ranges, 15 languages; NOT real backend acceptance');
 } finally {await browser.close(); await new Promise(resolve=>server.close(resolve));}
})().catch(e=>{console.error(e);process.exitCode=1;});
