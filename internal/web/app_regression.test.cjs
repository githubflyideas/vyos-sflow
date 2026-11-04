// Run with node --test internal/web/app_regression.test.cjs.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, 'static/app.js'), 'utf8');
function harness() {
  const elements = new Map();
  const element = () => ({innerHTML: '', textContent: '', dataset: {}, style: {}, classList: {toggle() {}, add() {}, remove() {}}, querySelectorAll: () => [], querySelector: () => null, addEventListener() {}, setAttribute() {}, removeAttribute() {}});
  const document = {body: element(), querySelector(s) {if (!elements.has(s)) elements.set(s, element()); return elements.get(s);}, querySelectorAll: () => [], addEventListener() {}, createElement: element};
  const pending = [];
  const context = {document, console, URLSearchParams, Intl, Date, Map, Set, Math, JSON, Error, setTimeout() {}, setInterval() {}, clearInterval() {}, clearTimeout() {}, location: {hash: '', hostname: 'localhost'}, history: {replaceState() {}, pushState() {}}, navigator: {language: 'en'}, localStorage: {getItem() {return null;}, setItem() {}}, matchMedia: () => ({matches: false, addEventListener() {}}), fetch: (url, options) => new Promise(resolve => pending.push({url, options, resolve}))};
  context.window = context; context.addEventListener = () => {};
  vm.runInNewContext(source.replace(/init\(\);\s*\}\)\(\);\s*$/, 'globalThis.testing = {setCatalog: d => {DICT = d;}, state, views, topTable, tsChart, api, readHash, LANGS, t, pickLang, loadLang, next: () => ++loadSeq};\n})();'), context);
  return {context, elements, pending, element, testing: context.testing};
}
const reply = (req, key) => req.resolve({status: 200, ok: true, json: async () => ({rows: [{key, wire: 100, pkts: 1, flows: 1}]})});
test('older table response cannot overwrite the newer rendered selection', async () => {
  const h = harness(), el = h.element();
  h.testing.state.dim = 'app'; h.testing.next();
  const old = h.testing.topTable(el, 'app').catch(e => e);
  h.testing.state.r = '1h'; h.testing.next();
  const fresh = h.testing.topTable(el, 'app');
  reply(h.pending[1], 'NEW-ROW'); await fresh;
  assert.match(el.innerHTML, /NEW-ROW/);
  reply(h.pending[0], 'OLD-ROW'); await old;
  assert.match(el.innerHTML, /NEW-ROW/);
  assert.doesNotMatch(el.innerHTML, /OLD-ROW/);
});
test('stale JSON body cannot commit after headers arrived', async () => {
  const h = harness(), el = h.element(); let finish;
  h.testing.next(); const old = h.testing.topTable(el, 'app').catch(e => e);
  h.pending[0].resolve({status: 200, ok: true, json: () => new Promise(r => finish = r)});
  await new Promise(setImmediate);
  h.testing.next(); const fresh = h.testing.topTable(el, 'app');
  reply(h.pending[1], 'NEW-BODY'); await fresh;
  finish({rows: [{key: 'OLD-BODY', wire: 1}]}); await old;
  assert.match(el.innerHTML, /NEW-BODY/); assert.doesNotMatch(el.innerHTML, /OLD-BODY/);
});
test('first-login reload submits the current factory password without session storage', async () => {
  const h = harness();
  h.context.sessionStorage = {getItem() {return null;}, removeItem() {}};
  const rendering = h.testing.views.account(h.element());
  assert.equal(h.pending[0].url, '/api/me');
  h.pending[0].resolve({ok: true, json: async () => ({user: 'admin', must_change: true, fixed: false})});
  await rendering;
  const form = h.elements.get('#pwForm');
  form.new = {value: 'new-private-password'};
  form.again = {value: 'new-private-password'};
  const submission = form.onsubmit({preventDefault() {}});
  const req = h.pending[1];
  assert.equal(req.url, '/api/password');
  assert.equal(req.options.method, 'POST');
  assert.deepEqual(JSON.parse(req.options.body), {old: 'tryvyossflow66', new: 'new-private-password'});
  req.resolve({ok: false, json: async () => ({error: 'test-rejection'})});
  await submission;
});
test('every login translation describes the public factory credential', () => {
  const dir = path.join(__dirname, 'static/i18n');
  for (const file of fs.readdirSync(dir).filter(f => f.endsWith('.json'))) {
    const catalog = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    for (const key of ['acct.rule', 'acct.err_default', 'login.hint']) {
      assert.ok(catalog[key].includes('tryvyossflow66'), `${file}: ${key}`);
      assert.ok(!catalog[key].includes('traffic66'), `${file}: stale ${key}`);
    }
    assert.ok(catalog['login.hint'].includes('admin'), `${file}: missing initial user`);
  }
});
test('language menu matches all 15 complete catalogs', () => {
  const h = harness();
  const codes = Array.from(h.testing.LANGS, ([code]) => code).sort();
  const expected = ['ar', 'bn', 'de', 'en', 'es', 'fr', 'hi', 'id', 'ja', 'ko', 'pt', 'ru', 'ur', 'vi', 'zh'];
  assert.deepEqual(codes, expected);
  const dir = path.join(__dirname, 'static/i18n');
  assert.deepEqual(fs.readdirSync(dir).filter(f => f.endsWith('.json')).sort(), expected.map(c => c + '.json'));
  const en = JSON.parse(fs.readFileSync(path.join(dir, 'en.json'), 'utf8'));
  const placeholders = s => (s.match(/\{\w+\}/g) || []).sort();
  for (const code of codes) {
    const catalog = JSON.parse(fs.readFileSync(path.join(dir, code + '.json'), 'utf8'));
    assert.deepEqual(Object.keys(catalog).sort(), Object.keys(en).sort(), code);
    for (const [key, text] of Object.entries(catalog)) {
      assert.equal(typeof text, 'string', `${code}: ${key}`);
      assert.ok(text.length > 0, `${code}: ${key}`);
      assert.deepEqual(placeholders(text), placeholders(en[key]), `${code}: ${key}`);
    }
    h.context.localStorage.getItem = () => code;
    assert.equal(h.testing.pickLang(), code);
  }
});
test('locale selection honors saved choices, regional locales and English fallback', () => {
  const h = harness();
  h.context.navigator.languages = ['xx-XX', 'ja-JP'];
  assert.equal(h.testing.pickLang(), 'ja');
  h.context.localStorage.getItem = () => 'zh';
  assert.equal(h.testing.pickLang(), 'zh');
  h.context.localStorage.getItem = () => 'unsupported';
  h.context.navigator.languages = ['xx-XX'];
  assert.equal(h.testing.pickLang(), 'en');
  h.context.localStorage.getItem = () => { throw new Error('storage disabled'); };
  h.context.navigator.languages = ['pt-BR'];
  assert.equal(h.testing.pickLang(), 'pt');
});
test('catalog loading updates direction and falls back to English and then the key', async () => {
  const h = harness();
  h.context.document.documentElement = {};
  h.elements.set('#theme', {...h.element(), querySelector: () => h.element()});
  const saved = [];
  h.context.localStorage.setItem = (key, value) => saved.push([key, value]);
  const en = JSON.parse(fs.readFileSync(path.join(__dirname, 'static/i18n/en.json'), 'utf8'));
  h.context.fetch = async url => ({json: async () => {
    if (url === 'i18n/en.json') return en;
    if (url === 'i18n/ar.json') return {'login.hint': 'translated {name}'};
    throw new Error('unavailable catalog');
  }});
  await h.testing.loadLang('ar');
  assert.equal(h.context.document.documentElement.dir, 'rtl');
  assert.equal(h.context.document.documentElement.lang, 'ar');
  assert.equal(h.testing.t('login.hint', {name: 'example'}), 'translated example');
  assert.equal(h.testing.t('acct.rule'), en['acct.rule']);
  assert.equal(h.testing.t('missing.test.key'), 'missing.test.key');
  await h.testing.loadLang('ja');
  assert.equal(h.context.document.documentElement.dir, 'ltr');
  assert.equal(h.testing.t('login.hint'), en['login.hint']);
  assert.deepEqual(saved, [['t66.lang', 'ar'], ['t66.lang', 'ja']]);
});
test('unsupported deep links cannot restore advanced views, ranges or dimensions', () => {
  const h = harness();
  for (const hash of ['#v=sankey&r=30d&dim=country', '#v=ifaces&r=custom&from=1&to=2&dim=conv', '#v=traffic&r=7d']) {
    h.context.location.hash = hash; h.testing.readHash();
    assert.equal(h.testing.state.v, 'overview'); assert.equal(h.testing.state.r, '1h'); assert.equal(h.testing.state.dim, 'host');
  }
});
test('help response cannot overwrite a newer language render', async () => {
  const h = harness(), el = h.element();
  h.testing.next(); const old = h.testing.views.help(el).catch(e => e);
  h.testing.next(); const fresh = h.testing.views.help(el);
  h.pending[1].resolve({ok: true, text: async () => '# New manual'}); await fresh;
  h.pending[0].resolve({ok: true, text: async () => '# Old manual'}); await old;
  assert.match(el.innerHTML, /New manual/); assert.doesNotMatch(el.innerHTML, /Old manual/);
});
test('empty series shows an explicit empty state rather than a blank chart', () => {
  const h = harness(), el = h.element();
  el.clientWidth = 500; el.clientHeight = 280;
  h.context.ResizeObserver = class { observe() {} };
  h.testing.tsChart(el, {times:[], areas:[]});
  assert.match(el.innerHTML, /empty.nodata/);
});
test('direction identifies the selected exporter/interface and leaves KPI scope explicit', async () => {
  for (const dir of ['in', 'out']) for (const iface of ['192.0.2.1/7', '']) {
    const h = harness(), el = h.element();
    h.context.ResizeObserver = class { observe() {} };
    h.testing.state.od = dir;
    const rendering = h.testing.views.overview(el);
    h.pending[0].resolve({ok:true, json:async () => ({totals:{wire:123}, dir_iface:iface, dir_series:{times:[],names:[],values:[]}})});
    await rendering;
    assert.match(el.innerHTML, /ov.kpi_scope/);
    assert.match(el.innerHTML, /ov.direction_scope/);
    assert.ok(el.innerHTML.includes(iface || 'ov.direction_empty'));
  }
});
test('Top explains overlapping inferred roles and global rather than displayed-row totals', async () => {
  const h = harness(), el = h.element();
  h.testing.state.tm = 'talkers';
  const rendering = h.testing.views.topn(el);
  for (const req of h.pending) req.resolve({ok:true, json:async () => req.url.includes('overview') ? {totals:{wire:900}} : {rows:[{key:'192.0.2.10',wire:100}]}});
  await rendering;
  assert.match(el.innerHTML, /topn.perspectives/);
  assert.equal((el.innerHTML.match(/topn.global_total/g) || []).length, 2);
  assert.equal((el.innerHTML.match(/900 B/g) || []).length, 2);
});
test('records show fractional estimated multipliers and actual bounded windows even when filtered empty', async () => {
  for (const empty of [false, true]) {
    const h = harness(), el = h.element();
    h.testing.setCatalog({'rec.window':'Effective {from} — {to}; limit {limit}', 'rec.multiplier':'Estimated ×{n}'});
    h.testing.state.r = '48h'; h.testing.state.ip = empty ? '203.0.113.9' : '';
    const rendering = h.testing.views.records(el);
    const from = '2026-01-02T00:00:00Z', to = '2026-01-03T00:00:00Z';
    h.pending[0].resolve({ok:true, json:async () => ({from,to,limit:500,rows:empty ? [] : [{ts:to,client:'192.0.2.10',server:'198.51.100.20',mult:9.090909,wire:100,pkts:3}]})});
    await rendering;
    assert.match(el.innerHTML, /Effective/); assert.match(el.innerHTML, /limit 500/);
    assert.ok(el.innerHTML.includes(from)); assert.ok(el.innerHTML.includes(to));
    assert.match(el.innerHTML, /rec.estimates/);
    if (empty) {assert.match(el.innerHTML, /203.0.113.9/); assert.match(el.innerHTML, /empty.nodata/);}
    else {assert.match(el.innerHTML, /Estimated ×9.09/); assert.doesNotMatch(el.innerHTML, /1:9/);}
  }
});
test('semantic explanations are translated in every catalog without English placeholders', () => {
  const dir = path.join(__dirname, 'static/i18n');
  const en = JSON.parse(fs.readFileSync(path.join(dir, 'en.json'), 'utf8'));
  const keys = ['ov.kpi_scope','ov.direction_scope','ov.direction_empty','topn.perspectives','topn.global_total','rec.window','rec.estimates','rec.multiplier'];
  for (const file of fs.readdirSync(dir).filter(f => f.endsWith('.json'))) {
    const dict = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    for (const key of keys) {
      assert.ok(dict[key], `${file}: missing ${key}`);
      if (file !== 'en.json') assert.notEqual(dict[key], en[key], `${file}: English fallback ${key}`);
    }
    assert.doesNotMatch(dict['ov.direction_scope'], /WAN/);
  }
});
test('product has no offline or non-sFlow entry points', () => {
  const html = fs.readFileSync(path.join(__dirname, 'static/index.html'), 'utf8');
  assert.doesNotMatch(html, /data-v="sandbox"|id="v-sandbox"/);
  assert.doesNotMatch(source, /\/api\/sandbox|softflowd|traffic-flow target|NetFlow v9/);
  assert.match(html, /<title>vyos-sflow<\/title>/);
  const h = harness(); h.context.location.hash = '#v=sandbox&ds=sb'; h.testing.readHash();
  assert.equal(h.testing.state.v, 'overview'); assert.notEqual(h.testing.state.ds, 'sb');
});
