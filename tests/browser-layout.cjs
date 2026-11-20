// Layout and accessibility of the web UI over synthetic API data, in Chromium:
// no page ever scrolls sideways (phone to ultrawide, every language), the
// traffic chart follows the window, the theme buttons work in the page
// toolbar, navigation labels take one line each, the device card closes
// the overview, and axe finds no serious or
// critical WCAG 2.1 AA violation on any page in any theme.
// Run with NODE_PATH pointing to Playwright and axe-core: node tests/browser-layout.cjs
'use strict';
const {chromium} = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.join(__dirname, '../internal/web/static');
const axeSource = fs.readFileSync(require.resolve('axe-core/axe.min.js'), 'utf8');
const LANGS = ['en', 'zh', 'hi', 'es', 'ar', 'fr', 'bn', 'pt', 'ru', 'id', 'ur', 'de', 'ja', 'vi', 'ko'];
const WIDTHS = [320, 390, 768, 1280, 1366, 1920, 2560];
const THEMES = ['light', 'gray', 'slate', 'navy', 'black'];
const NAV = ['overview', 'topn', 'records', 'sources', 'cleanup', 'help'];
const catalog = l => JSON.parse(fs.readFileSync(path.join(root, 'i18n', l + '.json'), 'utf8'));

function payloads() {
  const now = Date.now(), step = 60000, n = 60, times = Array.from({length: n}, (_, i) => now - (n - i) * step);
  const names = ['https', 'quic', 'dns', 'ssh', 'other'];
  const series = {times, names, values: names.map((_, k) => times.map((_, i) => Math.round(5e7 / (k + 1) * (1 + Math.sin(i / 6 + k))))), settled_to: now - 2 * step, data_from: times[0]};
  const stream = {enforced: false, configured: false, agent: '', sub_agent: 0, rejected_streams: 0};
  const ifaces = [1, 2, 3, 4].map(i => ({exporter: '192.0.2.1', ifindex: i, speed: 1e9, dev_in_bps: 2e8 * i, dev_out_bps: 1e8 * i, est_in_bps: 1.9e8 * i, est_out_bps: 1.05e8 * i, seconds: 300}));
  return {
    '/api/status': {stream, version: 'layout-fixture', now, records_per_sec: 15.7, dropped: 0, counter_samples: 1234, dropped_counters: 0, write_errors: 0, disk_bytes: 123456789, disk_free: 9876543210, disk_need: -1, retention_days: 2, source_warnings: 1},
    '/api/me': {user: 'fixture-admin', admin: true, must_change: false, fixed: false},
    '/api/users': {users: [{user: 'fixture-admin', admin: true}]},
    '/api/cleanup': {hot_rows: 100, segment_rows: 2000, disk_bytes: 123456789, oldest: now - 864e5, exporters: [{exporter: '192.0.2.1', rows: 100, wire: 1e9, last: now}]},
    '/api/overview': {totals: {wire: 9.8e11, pkts: 7.7e8, flows: 123456}, now_bps: 3.2e8, series, dir_series: series, dir_iface: '192.0.2.1/2'},
    '/api/series': series,
    '/api/topn': {rows: Array.from({length: 12}, (_, i) => ({key: '203.0.113.' + i, wire: 1e9 / (i + 1), pkts: 1e6, flows: 100}))},
    '/api/records': {rows: Array.from({length: 20}, (_, i) => ({ts: now - i * 1000, client: '192.0.2.' + i, cport: 50000 + i, server: '203.0.113.9', port: 443, proto: 6, app: 'https', wire: 1500 * i, pkts: i, in_if: 1, out_if: 2, mult: 1000})), from: new Date(now - 3600e3).toISOString(), to: new Date(now).toISOString(), limit: 500, more: false},
    '/api/sources': {stream, sources: [{exporter: '192.0.2.1', proto: 'sFlow v5', rec_per_sec: 12.5, sampling: '1:1000', effective: 1000, lost_pct: 0.1, status: 'ok', issues: []}, {exporter: '198.51.100.7', proto: 'sFlow v5', rec_per_sec: 3.2, sampling: '1:2000', lost_pct: 2.4, status: 'warn', issues: ['loss']}], listeners: [{addr: '0.0.0.0:6343', proto: 'sflow', packets: 9999}], stale: [], ifaces},
    '/api/loginhint': {default: true, user: 'admin'},
  };
}

(async () => {
  const server = require('node:http').createServer((req, res) => { res.writeHead(404); res.end('Not found'); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = 'http://127.0.0.1:' + server.address().port;
  const browser = await chromium.launch({headless: true});
  const data = payloads();
  let signedIn = true;
  const errors = [];
  async function open(lang, theme, width = 1366) {
    const page = await browser.newPage({viewport: {width, height: 900}});
    page.on('pageerror', e => errors.push(e.message));
    await page.route('**/*', async route => {
      const u = new URL(route.request().url());
      if (u.origin !== base) return route.abort();
      if (u.pathname === '/api/status' && !signedIn) return route.fulfill({status: 401, json: {error: 'login'}});
      if (u.pathname === '/api/manual') return route.fulfill({contentType: 'text/markdown', body: '# Manual\n\n## Install\n\nText.\n\n## Use\n\nMore text.'});
      if (u.pathname === '/logo') return route.fulfill({path: path.join(root, /wide=1/.test(u.search) ? 'logo-wide.svg' : 'logo.svg'), contentType: 'image/svg+xml'});
      if (data[u.pathname]) return route.fulfill({json: data[u.pathname]});
      if (u.pathname.startsWith('/api/')) return route.fulfill({json: {}});
      const file = path.join(root, u.pathname === '/' ? 'index.html' : u.pathname.slice(1));
      if (!file.startsWith(root + path.sep) || !fs.existsSync(file)) return route.fulfill({status: 404, body: 'Not found'});
      return route.fulfill({path: file});
    });
    // each page has its own fresh storage; a reload keeps what the page chose
    await page.addInitScript(([l, t]) => { if (!localStorage.getItem('t66.theme')) { localStorage.setItem('t66.lang', l); localStorage.setItem('t66.theme', t); } }, [lang, theme]);
    return page;
  }
  const show = async (page, v) => {
    await page.goto(base + '/#v=' + v);
    await page.locator('#v-' + v + '.on .panel').first().waitFor({state: 'visible'});
    if (v === 'overview') await page.locator('#chStack svg').waitFor();
  };
  const overflow = page => page.evaluate(() => {
    const d = document.documentElement, out = [];
    if (d.scrollWidth > d.clientWidth) out.push('page ' + d.scrollWidth + '>' + d.clientWidth);
    for (const e of document.querySelectorAll('header.nms *, .view.on .panel')) {
      const r = e.getBoundingClientRect();
      if (r.width && (r.right > d.clientWidth + 0.5 || r.left < -0.5)) out.push('outside: ' + (e.className || e.tagName));
      // a card never scrolls sideways; wide tables scroll inside their own box
      if (e.classList.contains('panel') && e.scrollWidth > e.clientWidth + 1) out.push('panel scrolls: ' + e.querySelector('h2')?.textContent);
    }
    return out;
  });
  try {
    // 1. no sideways scrolling: 15 languages x 7 widths x overview, collection status
    let checked = 0;
    for (const lang of LANGS) {
      const page = await open(lang, 'gray');
      for (const v of ['overview', 'sources']) {
        await show(page, v);
        for (const w of WIDTHS) {
          await page.setViewportSize({width: w, height: 900});
          await page.waitForTimeout(60);
          assert.deepEqual(await overflow(page), [], `${lang} ${v} ${w}px`);
          if (v === 'overview') {
            // the chart is redrawn to the width of its card, all of it visible
            const fit = await page.evaluate(() => { const c = document.querySelector('#chStack'), s = c.querySelector('svg'), p = c.closest('.panel').getBoundingClientRect(), r = s.getBoundingClientRect(); return {svg: Math.round(r.width), box: c.clientWidth, inside: r.left >= p.left - 0.5 && r.right <= p.right + 0.5, vb: +s.getAttribute('viewBox').split(' ')[2]}; });
            assert.ok(fit.inside && Math.abs(fit.svg - fit.box) <= 1 && fit.vb === fit.box, `${lang} ${w}px chart ${JSON.stringify(fit)}`);
          }
          checked++;
        }
      }
      // 2. the header stays on one row from 1280px up
      for (const w of [1280, 1366, 1440, 1501, 1600, 1701, 1920]) {
        await page.setViewportSize({width: w, height: 900}); await page.waitForTimeout(60);
        assert.equal(await page.evaluate(() => Math.round(document.querySelector('header.nms').getBoundingClientRect().height)), 80, `${lang} header wraps at ${w}px`);
      }
      // 3. navigation labels: one line each, the full catalog name, whole
      // and never wrapped by the browser
      const cat = catalog(lang);
      for (const w of [1366, 390]) {
        await page.setViewportSize({width: w, height: 900}); await page.waitForTimeout(60);
        const labels = await page.evaluate(() => [...document.querySelectorAll('#nav [data-n]')].map(el => ({k: el.dataset.n, text: el.textContent, boxes: Array.from(el.getClientRects()).length})));
        assert.deepEqual(labels.map(l => l.k), NAV);
        for (const {k, text, boxes} of labels) {
          assert.equal(text, cat['nav.' + k], `${lang} ${k} name`);
          assert.equal(boxes, 1, `${lang} ${k} "${text}" wrapped to more than one line`);
        }
        if (lang === 'zh') assert.equal(labels.find(l => l.k === 'records').text, '流记录');
      }
      await page.close();
    }
    console.log(`ok   no sideways scrolling: ${checked} language/page/width combinations; header one row at 1280+; nav labels`);

    // 4. theme buttons: five round buttons in the page toolbar, one per
    // theme, a click switches, kept after reload
    {
      const page = await open('en', 'gray');
      await show(page, 'overview');
      const geometry = () => page.evaluate(() => {
        const bs = [...document.querySelectorAll('#themes .tdot')].map(b => { const r = b.getBoundingClientRect(); return {w: r.width, h: r.height, x: r.left + r.width / 2, y: r.top + r.height / 2}; });
        return {bs, inBar: !!document.querySelector('.topbar #themes'), inHeader: !!document.querySelector('header #themes')};
      });
      for (const w of [320, 390, 620, 621, 768, 1280, 1366, 2560]) {
        await page.setViewportSize({width: w, height: 900}); await page.waitForTimeout(60);
        const g = await geometry();
        assert.ok(g.inBar && !g.inHeader, `${w}px theme buttons are not in the page toolbar`);
        // identical round buttons, all on one row
        assert.equal(new Set(g.bs.map(b => `${b.w}x${b.h}`)).size, 1, `${w}px button sizes ${JSON.stringify(g.bs)}`);
        assert.equal(g.bs[0].w, g.bs[0].h);
        assert.ok(Math.max(...g.bs.map(b => b.y)) - Math.min(...g.bs.map(b => b.y)) < 1, `${w}px buttons not on one row ${JSON.stringify(g.bs)}`);
      }
      await page.setViewportSize({width: 1366, height: 900});
      assert.equal(await page.locator('#themes').getAttribute('role'), 'radiogroup');
      assert.equal(await page.locator('#themes [role=radio]').count(), 5);
      const en = catalog('en');
      const before = JSON.stringify((await geometry()).bs);
      for (const t of THEMES) {
        const btn = page.locator(`#themes [data-th="${t}"]`);
        assert.equal(await btn.getAttribute('aria-label'), en['theme.' + t]);
        await btn.click();
        await page.waitForFunction(t => (document.documentElement.getAttribute('data-theme') || 'gray') === t, t);
        assert.equal(await page.locator('#themes [aria-checked=true]').getAttribute('data-th'), t);
        assert.equal(await page.evaluate(() => localStorage.getItem('t66.theme')), t);
        assert.equal(await page.locator('#themes [tabindex="0"]').getAttribute('data-th'), t);
        // the chosen button is filled with its theme colour and outlined
        assert.notEqual(await btn.evaluate(e => getComputedStyle(e).backgroundColor), 'rgba(0, 0, 0, 0)');
        assert.notEqual(await btn.evaluate(e => getComputedStyle(e).boxShadow), 'none');
        assert.equal(JSON.stringify((await geometry()).bs), before, `${t}: buttons moved or resized`);
      }
      await page.reload(); await page.locator('#themes [aria-checked=true]').waitFor();
      assert.equal(await page.locator('#themes [aria-checked=true]').getAttribute('data-th'), 'black', 'theme not kept');
      // keyboard: arrows move through the buttons and switch at once
      await page.locator('#themes [aria-checked=true]').focus();
      await page.keyboard.press('ArrowRight');
      await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'light');
      assert.equal(await page.evaluate(() => document.activeElement.dataset.th), 'light');
      await page.keyboard.press('ArrowLeft');
      await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'black');
      await page.keyboard.press('Home');
      await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'light');
      await page.keyboard.press('End');
      await page.waitForFunction(() => document.documentElement.getAttribute('data-theme') === 'black');
      await page.close();
      console.log('ok   theme buttons: 5 round, one row in the toolbar, click and keyboard switch, kept after reload');
    }

    // 5. the device card is the last section of the overview
    for (const lang of ['en', 'zh', 'ar']) {
      const page = await open(lang, 'gray');
      await show(page, 'overview');
      assert.equal(await page.locator('#v-overview .grid > .panel').last().locator('h2').innerText(), catalog(lang)['ov.device'], lang);
      await page.close();
    }
    console.log('ok   device information is the last section of the overview');

    // 6. axe-core WCAG 2.1 AA: every page x 5 themes x 5 languages, plus sign-in
    const PAGES = ['overview', 'topn', 'records', 'sources', 'cleanup', 'account', 'help'];
    const found = [];
    let runs = 0;
    const scan = async (page, where) => {
      await page.addScriptTag({content: axeSource});
      const r = await page.evaluate(() => axe.run(document, {runOnly: {type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']}}));
      runs++;
      for (const v of r.violations.filter(v => v.impact === 'serious' || v.impact === 'critical')) found.push(`${where}: ${v.id} (${v.impact}) ${v.nodes.slice(0, 3).map(n => n.target.join(' ')).join(', ')}`);
    };
    for (const lang of ['en', 'zh', 'hi', 'ar', 'de']) {
      for (const theme of THEMES) {
        const page = await open(lang, theme);
        for (const v of PAGES) { await show(page, v); await page.waitForTimeout(80); await scan(page, `${lang}/${theme}/${v}`); }
        await page.close();
        signedIn = false;
        const login = await open(lang, theme);
        await login.goto(base + '/'); await login.locator('#login:not([hidden])').waitFor();
        await scan(login, `${lang}/${theme}/login`);
        await login.close();
        signedIn = true;
      }
    }
    assert.deepEqual(found, [], 'axe serious/critical violations');
    console.log(`ok   axe-core WCAG 2.1 AA: ${runs} scans, 0 serious or critical`);
    assert.deepEqual(errors, [], 'JavaScript errors');
    console.log('PASS browser layout: synthetic API data over the production assets');
  } finally {
    await browser.close();
    await new Promise(resolve => server.close(resolve));
  }
})().catch(e => { console.error(e); process.exitCode = 1; });
