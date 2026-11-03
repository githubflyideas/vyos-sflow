// Run with node --test internal/web/no_registration.test.cjs.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.join(__dirname, 'static');
const source = fs.readFileSync(path.join(root, 'app.js'), 'utf8');
function harness(payload) {
  const elements = new Map(), requests = [], listeners = [], mutations = [];
  const element = () => ({innerHTML: '', textContent: '', dataset: {}, style: {setProperty() {}}, classList: {toggle(...args) {mutations.push(args);}, add() {}, remove() {}}, querySelectorAll: () => [], querySelector: () => null, addEventListener() {}, setAttribute() {}, removeAttribute() {}, remove() {}});
  const document = {body: element(), querySelector(s) {if (!elements.has(s)) elements.set(s, element()); return elements.get(s);}, querySelectorAll: () => [], addEventListener(name) {listeners.push(name);}, createElement: element};
  const context = {document, console, URLSearchParams, Intl, Date, Map, Set, Math, JSON, Error, setTimeout() {}, setInterval() {}, clearInterval() {}, clearTimeout() {}, location: {hash: '', hostname: 'localhost', protocol: 'https:'}, history: {replaceState() {}, pushState() {}}, navigator: {language: 'en'}, localStorage: {getItem() {return null;}, setItem() {}}, matchMedia: () => ({matches: false, addEventListener() {}}), fetch: async (url, options) => {requests.push({url, options}); return {ok: true, status: 200, json: async () => payload(url)};}};
  context.window = context; context.addEventListener = () => {};
  context.catalog = JSON.parse(fs.readFileSync(path.join(root,'i18n/en.json'),'utf8'));
  vm.runInNewContext(source.replace(/init\(\);\s*\}\)\(\);\s*$/, 'EN = globalThis.catalog; globalThis.testing = {loadStatus, views, setAdmin: () => me = {admin:true}};\n})();'), context);
  return {context, elements, requests, listeners, mutations, element, testing: context.testing};
}
test('production shell has no advertising, activation or evaluation assets', () => {
  const html = fs.readFileSync(path.join(root, 'index.html'), 'utf8');
  const css = fs.readFileSync(path.join(root, 'app.css'), 'utf8');
  assert.doesNotMatch(html, /sponsor|licText|id="lic"|markChip|markNote/i);
  assert.doesNotMatch(css, /sponsor|\.spc|body\.marked|\.markchip|\.marknote|\.lic\b|\.panel\.locked/i);
  assert.doesNotMatch(source, /t66Sponsor|\/api\/license|\.registered|not_registered|s\.license|markNotice|applyMark|licFile|licSave/);
  assert.equal(fs.existsSync(path.join(root, 'sponsor.js')), false);
  for (const [, asset] of html.matchAll(/(?:src|href)="([^"#?]+\.(?:js|css))"/g)) assert.ok(fs.existsSync(path.join(root, asset)), asset);
  assert.match(html, /id="loginForm"/); assert.match(html, /id="acct"/);
  assert.match(source, /must_change/);
});
test('status fetch never loads advertising or evaluation UI', async () => {
  const h = harness(() => ({version: 'test', license: {registered: false, watermark: true, notice: 'EVALUATION COPY'}}));
  let adCalls = 0; h.context.t66Sponsor = {load() {adCalls++;}, remove() {adCalls++;}};
  await h.testing.loadStatus();
  assert.equal(adCalls, 0);
  assert.deepEqual(h.requests.map(r => r.url), ['/api/status']);
  assert.equal(h.listeners.includes('copy'), false);
  assert.equal(h.mutations.some(([name]) => name === 'marked'), false);
  assert.equal(h.elements.has('#licText'), false);
  assert.match(h.elements.get('#verNo').textContent, /test/);
});
test('obsolete registration-adjacent business controls are removed, not unlocked', () => {
 assert.doesNotMatch(source, /syslogPanel|embedPanel|slSave|embNew|api\/syslog|api\/embeds/);
});
test('status includes queue drops, counter reception and storage diagnostics', async () => {
  const h = harness(() => ({version:'test', dropped:17, counter_samples:23, dropped_counters:31, write_errors:41, last_error:'disk full', disk_bytes:50, disk_free:60}));
  await h.testing.loadStatus();
  const html = h.elements.get('#self').innerHTML;
  for (const value of ['17','23','31','41','disk full']) assert.ok(html.includes(value), value);
});
test('source diagnostics distinguish agent drops from transit loss', async () => {
 const h = harness(() => ({sources:[{exporter:'192.0.2.1',proto:'sFlow v5',status:'warn',lost_pct:2,transit_pct:3,drop_pct:7,issues:['loss','agent_drops']}]})), el = h.element();
 h.context.testing = h.testing;
 // Loading a catalog is unnecessary: placeholder values remain visible through a small real catalog.
 h.context.document.documentElement = {};
 const rendering = h.testing.views.sources(el);
 await rendering;
 assert.match(el.innerHTML, /3\.0%/); assert.match(el.innerHTML, /7\.0%/);
});
test('non-administrator cleanup remains unavailable', async () => {
 const h = harness(() => ({})), el = h.element();
 await h.testing.views.cleanup(el);
 assert.ok(el.innerHTML.includes(h.context.catalog['cl.admin_only']));
 assert.deepEqual(h.requests, []);
});

test('all 15 catalogs remove obsolete mandatory sender configuration guidance', () => {
 const catalogs = fs.readdirSync(path.join(root,'i18n')).filter(f=>f.endsWith('.json'));
 assert.equal(catalogs.length,15);
 for (const file of catalogs) {
  const text = fs.readFileSync(path.join(root,'i18n',file),'utf8');
  assert.doesNotMatch(text, /VYOS_SFLOW_AGENT|Configure -agent|stream\.unconfigured/, file);
 }
});
test('unenforced receiver never demands identity even with unconfigured metadata', async () => {
 for (const configured of [false,true]) {
  const stream = {enforced:false,configured,agent:configured?'2001:db8::1':'',sub_agent:7};
  const h = harness(()=>({stream,sources:[]}));
  await h.testing.loadStatus();
  const el = h.element(); await h.testing.views.sources(el);
  for (const html of [h.elements.get('#self').innerHTML,el.innerHTML]) {
   assert.doesNotMatch(html,/No samples are accepted|Configure -agent|One sFlow agent|stream-policy/);
  }
 }
});
test('legacy configured stream diagnostics remain informational', async () => {
 for (const configured of [false,true]) {
  // Synthetic payload mirrors collector.StreamStatus in status/sources.
  const stream = {enforced:true,configured,agent:configured?'2001:db8::1':'',sub_agent:7,rejected_streams:19};
  const h = harness(()=>({stream,sources:[]}));
  await h.testing.loadStatus();
  const el = h.element(); await h.testing.views.sources(el);
  for (const html of [h.elements.get('#self').innerHTML,el.innerHTML]) {
   if(configured) {assert.match(html,/19/); assert.match(html,/2001:db8::1/); assert.match(html,/7/);}
   else {assert.doesNotMatch(html,/No samples are accepted|Configure -agent|One sFlow agent|stream-policy/);}
  }
 }
});

test('administrator cleanup has only clear-all and requires two confirmations', async () => {
 const h = harness(()=>({disk_bytes:0,oldest:'',options:[{days:1,rows:99},{days:0,rows:100}]})), el = h.element(), button = h.element();
 h.testing.setAdmin(); el.querySelector=()=>button;
 await h.testing.views.cleanup(el);
 assert.doesNotMatch(el.innerHTML,/data-cl="|type="date|Older than/);
 assert.match(el.innerHTML,/data-cl-all/);
 let confirmations=0;
 h.context.confirm=()=>{confirmations++; return false;};
 await button.onclick();
 assert.equal(confirmations,1); assert.equal(h.requests.length,1);
 confirmations=0; h.context.confirm=()=>++confirmations===1;
 await button.onclick(); assert.equal(confirmations,2); assert.equal(h.requests.length,1);
 h.context.confirm=()=>true;
 h.context.fetch=async (url,options)=>{h.requests.push({url,options}); return {ok:false,statusText:'denied',json:async()=>({error:'denied'})};};
 await button.onclick();
 assert.deepEqual(JSON.parse(h.requests[1].options.body),{all:true,confirm:true});
 assert.equal(button.disabled,false);
});
