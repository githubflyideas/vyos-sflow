(() => {
'use strict';

// ------------------------------------------------------------ i18n
// Ordered by total number of speakers (Ethnologue).
const LANGS = [['en','English'],['zh','中文'],['hi','हिन्दी'],['es','Español'],['ar','العربية'],['fr','Français'],
  ['bn','বাংলা'],['pt','Português'],['ru','Русский'],['id','Bahasa Indonesia'],['ur','اردو'],['de','Deutsch'],['ja','日本語'],['vi','Tiếng Việt'],['ko','한국어']];
const RTL = new Set(['ar', 'ur']);
let LANG = 'en', DICT = {}, EN = {};
const t = (k, p) => {
  let s = DICT[k] ?? EN[k] ?? k;
  if (p) for (const [a, b] of Object.entries(p)) s = s.split('{' + a + '}').join(b);
  return s;
};
// the key for n of something: k_one, k_few, ... where the language has
// that form, else k itself
const plural = (k, n) => {
  let f = '';
  try { f = new Intl.PluralRules(LANG).select(n); } catch (e) {}
  return f && f !== 'other' && (DICT[k + '_' + f] ?? (LANG === 'en' ? EN[k + '_' + f] : undefined)) != null ? k + '_' + f : k;
};
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
async function loadLang(code) {
  const get = async c => (await fetch('i18n/' + c + '.json')).json();
  if (!Object.keys(EN).length) EN = await get('en');
  DICT = code === 'en' ? EN : await get(code).catch(() => EN);
  LANG = code;
  document.documentElement.lang = code;
  document.documentElement.dir = RTL.has(code) ? 'rtl' : 'ltr';
  try { localStorage.setItem('t66.lang', code); } catch (e) {}
  document.querySelectorAll('[data-i]').forEach(el => { el.textContent = t(el.dataset.i); });
  refreshLabel();
  themeLabel();
}
// the theme button shows the current theme; a click goes to the next one
function themeLabel() {
  const b = $('#theme'), cur = window.t66Theme ? t66Theme.get() : 'black';
  b.querySelector('span').textContent = t('theme.' + cur);
  b.title = t('theme.label') + ': ' + t('theme.' + cur) + ' · ' + t('theme.next');
  b.setAttribute('aria-label', b.title);
}
function pickLang() {
  let saved = null;
  try { saved = localStorage.getItem('t66.lang'); } catch (e) {}
  if (saved && LANGS.some(l => l[0] === saved)) return saved;
  for (const l of navigator.languages || [navigator.language || 'en']) {
    const c = l.toLowerCase().split('-')[0];
    if (LANGS.some(x => x[0] === c)) return c;
  }
  return 'en';
}

// ------------------------------------------------------------ helpers
const $ = s => document.querySelector(s);
const nf = (v, d = 0) => new Intl.NumberFormat(LANG, {maximumFractionDigits: d, minimumFractionDigits: d}).format(v);
function fmtBps(v) {
  if (v == null || !isFinite(v)) return '—';
  const u = [['Tbps', 1e12], ['Gbps', 1e9], ['Mbps', 1e6], ['Kbps', 1e3]];
  for (const [n, m] of u) if (Math.abs(v) >= m) return nf(v / m, Math.abs(v / m) < 10 ? 2 : Math.abs(v / m) < 100 ? 1 : 0) + ' ' + n;
  return nf(v) + ' bps';
}
function fmtBytes(v) {
  if (v == null) return '—';
  const u = [['TB', 1e12], ['GB', 1e9], ['MB', 1e6], ['KB', 1e3]];
  for (const [n, m] of u) if (v >= m) return nf(v / m, v / m < 10 ? 2 : v / m < 100 ? 1 : 0) + ' ' + n;
  return nf(v) + ' B';
}
const fmtPct = (v, d = 1) => (v > 0 ? '+' : '') + nf(v * 100, d) + '%';
function fmtAxisBps(v) {
  if (v === 0) return '0';
  for (const [n, m] of [['T', 1e12], ['G', 1e9], ['M', 1e6], ['K', 1e3]]) if (v >= m) return nf(v / m, v / m < 10 && v % m ? 1 : 0) + n;
  return nf(v);
}
function fmtPps(v) {
  if (v == null || !isFinite(v)) return '—';
  for (const [n, m] of [['M', 1e6], ['k', 1e3]]) if (Math.abs(v) >= m) return nf(v / m, Math.abs(v / m) < 10 ? 2 : Math.abs(v / m) < 100 ? 1 : 0) + ' ' + n + 'p/s';
  return nf(v, v < 10 ? 1 : 0) + ' p/s';
}
let regionNames = null;
const PROTO = {1: 'ICMP', 2: 'IGMP', 4: 'IPIP', 6: 'TCP', 17: 'UDP', 41: 'IPv6', 47: 'GRE', 50: 'ESP', 51: 'AH', 58: 'ICMPv6', 89: 'OSPF', 132: 'SCTP'};
const proto = p => PROTO[p] || String(p);
const COLORS = ['var(--c1)', 'var(--c2)', 'var(--c3)', 'var(--c4)', 'var(--c5)', 'var(--c6)', 'var(--c7)', 'var(--c8)'];
const color = (i, key) => key === '__other__' || i >= COLORS.length ? OTHER : COLORS[i];
const OTHER = 'var(--other)';
const appLabel = a => a === '__other__' ? t('other') : a;
function fmtTime(ms, span) {
  const d = new Date(ms);
  if (span > 36 * 3600e3) return new Intl.DateTimeFormat(LANG, {month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit'}).format(d);
  return new Intl.DateTimeFormat(LANG, {hour: '2-digit', minute: '2-digit'}).format(d);
}
function fmtAxisTime(ms, span) {
  const d = new Date(ms);
  if (span > 36 * 3600e3) return new Intl.DateTimeFormat(LANG, {month: 'numeric', day: 'numeric'}).format(d);
  return new Intl.DateTimeFormat(LANG, {hour: '2-digit', minute: '2-digit'}).format(d);
}
function ago(ms) {
  const s = Math.max(0, (Date.now() - ms) / 1000);
  const rtf = new Intl.RelativeTimeFormat(LANG, {numeric: 'auto'});
  if (s < 60) return rtf.format(-Math.round(s), 'second');
  if (s < 3600) return rtf.format(-Math.round(s / 60), 'minute');
  if (s < 86400) return rtf.format(-Math.round(s / 3600), 'hour');
  return rtf.format(-Math.round(s / 86400), 'day');
}
// emptyText is what a page with nothing to show says. For the first
// several minutes of a new installation the honest answer is not "no
// data" but "not yet": the collector is waiting for a template, for a
// sampling rate, or for the most recent minutes to settle. Saying only
// "no data" there reads as a fault, and sends somebody to change ports
// and firewall rules while the collector is about to finish.
// waitedFor says how long the collector has been at this stage, from the
// server's elapsed_sec, so "up to about 15 minutes" can be read against it.
function waitedFor(w) {
  return w.elapsed_sec > 0 ? ' ' + t('wait.elapsed', {m: Math.max(1, Math.floor(w.elapsed_sec / 60))}) : '';
}
// waitHint wraps a first-run waiting message in a prominent callout, so a
// new installation reads "not yet, and here is why" at a glance rather than
// a grey "no data". Ordinary empty periods keep the plain muted text.
function waitHint(msg, elapsed) {
  return `<span class="waithint" role="status"><span class="waithint-ic" aria-hidden="true">⏳</span><span>${msg}${elapsed ? `<span class="waithint-el">${elapsed}</span>` : ''}</span></span>`;
}
function emptyText() {

  const w = waiting;
  if (!w || !w.reason) return t('empty.nodata');
  if (w.reason === 'nothing') return waitHint(firstText());
  if (w.reason === 'templates') return waitHint(t('wait.templates', {n: nf(w.records || 0)}), waitedFor(w));
  if (w.reason === 'sampling') return waitHint(t('wait.sampling', {n: nf(w.records || 0)}), waitedFor(w));
  if (w.reason === 'settling') { const n = Math.max(1, Math.ceil((w.seconds || 0) / 60)); return t(plural('wait.settling', n), {n}); }
  return t('empty.nodata');
}

function toast(msg) {
  const el = $('#toast'); el.textContent = msg; el.style.display = 'block';
  clearTimeout(toast.h); toast.h = setTimeout(() => el.style.display = 'none', 1800);
}
const ICON = {
  ok: '<svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="currentColor"/><path d="M4.5 8.2l2.2 2.2 4.8-4.8" stroke="#fff" stroke-width="1.8" fill="none"/></svg>',
  warn: '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.5l7 12.5H1z" fill="var(--warn)"/><path d="M8 6v4M8 11.6v.4" stroke="#1b2432" stroke-width="1.6"/></svg>',
  bad: '<svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="7" fill="currentColor"/><path d="M5.5 5.5l5 5M10.5 5.5l-5 5" stroke="#fff" stroke-width="1.8"/></svg>'
};
const status = (kind, text) => `<span class="st ${kind}">${ICON[kind]}${esc(text)}</span>`;
const bar = (x, max, col) => `<div class="sb"><span style="width:${Math.max(1.5, max > 0 ? x / max * 100 : 0)}%${col ? ';background:' + col : ''}"></span></div>`;


const VIEWS = ['overview', 'topn', 'records', 'sources', 'account', 'cleanup', 'help'];
const DIMS = ['host', 'app', 'proto'];
const RANGES = ['15m', '1h', '6h', '24h', '48h'];
const REFRESH_MS = {'15m': 20000, '1h': 30000, '6h': 60000, '24h': 60000, '48h': 300000};
const LIVE_VIEWS = new Set(['overview', 'topn', 'records', 'sources']);
const state = {v: 'overview', r: '1h', dim: 'host', f: [], od: '', tm: 'talkers', ip: ''};
function readHash() {
 const p = new URLSearchParams(location.hash.slice(1));
 state.v = VIEWS.includes(p.get('v')) ? p.get('v') : 'overview';
 state.r = RANGES.includes(p.get('r')) ? p.get('r') : '1h';
 state.dim = DIMS.includes(p.get('dim')) ? p.get('dim') : 'host';
 state.od = ['in', 'out'].includes(p.get('od')) ? p.get('od') : '';
 state.tm = p.get('tm') === 'table' ? 'table' : 'talkers';  // Top 66 opens on the two ends
 state.ip = p.get('ip') || '';
 state.f = [];
}
function writeHash(push) {
 const h = '#' + new URLSearchParams({v: state.v, r: state.r, dim: state.dim, ...(state.od ? {od: state.od} : {}), ...(state.tm === 'table' ? {tm: 'table'} : {}), ...(state.ip ? {ip: state.ip} : {})});
 if (push && h !== location.hash) history.pushState(null, '', h); else history.replaceState(null, '', h);
}
let clockSkew = 0, waiting = null, setup = null;
const spanMs = () => ({'15m': 9e5, '1h': 36e5, '6h': 216e5, '24h': 864e5, '48h': 1728e5})[state.r];
const rangeLabel = () => t('range.' + state.r);
function renderRange() {
 $('#range').innerHTML = RANGES.map(r => `<button data-r="${r}" aria-pressed="${r === state.r}">${t('range.' + r)}</button>`).join('');
}
function tickClock() {
  try { $('#clock').textContent = new Intl.DateTimeFormat(LANG, {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'}).format(new Date(Date.now() + clockSkew)); } catch (e) {}
}
setInterval(tickClock, 1000);
// Reject obsolete responses before view functions can write their HTML.
function checkLoad(seq) { if (seq !== loadSeq) throw new Error('stale'); }
async function api(path, extra = {}, filters = state.f) {
  const seq = loadSeq;
  const p = new URLSearchParams({range: state.r, ...extra});
  // Which clock the day and six-hour columns are cut on. Without it they
  // were cut on UTC, so a day in Tokyo arrived split across two of them
  // and neither column was a day's traffic. getTimezoneOffset counts
  // minutes behind UTC, so the sign is turned round here.
  p.set('tz', String(-new Date().getTimezoneOffset()));
  if (filters.length) p.set('f', JSON.stringify(filters.map(x => ({f: x.f, v: x.v, neg: x.neg}))));
  const res = await fetch('/api/' + path + '?' + p);
  checkLoad(seq);
  if (res.status === 401) { showLogin(); throw new Error('login'); }
  if (res.status === 403 && me) { const j = await res.clone().json().catch(() => ({})); if (j.error === 'change_password') { me.must_change = true; go('account'); throw new Error('login'); } }
  const j = await res.json().catch(() => ({}));
  checkLoad(seq);
  if (!res.ok) { const e = new Error(j.error || res.statusText); e.kind = j.kind; throw e; }
  return j;
}
// ------------------------------------------------------------ tooltip & charts
const tip = $('#tip');
function showTip(e, html) {
  tip.innerHTML = html; tip.style.display = 'block';
  const r = tip.getBoundingClientRect();
  let l = e.clientX + 14; if (l + r.width > innerWidth - 8) l = e.clientX - r.width - 14;
  let tp = e.clientY - r.height - 12; if (tp < 8) tp = e.clientY + 16;
  tip.style.left = l + 'px'; tip.style.top = tp + 'px';
}
const hideTip = () => tip.style.display = 'none';
function niceMax(m) {
  if (m <= 0) return 1;
  const p = Math.pow(10, Math.floor(Math.log10(m / 4)));
  for (const s of [1, 2, 2.5, 5, 10]) if (s * p * 4 >= m) return s * p * 4;
  return m;
}
// axisTicks returns round local times between from and to for the time
// axis: the smallest interval that gives at most max labels.
function axisTicks(from, to, max, span) {
  const M = 6e4, D = 864e5;
  const iv = [M, 2 * M, 5 * M, 10 * M, 15 * M, 30 * M, 60 * M, 120 * M, 180 * M, 360 * M, 720 * M, D, 2 * D, 5 * D, 7 * D, 10 * D, 15 * D];
  const fine = span > 36 * 3600e3 ? iv.filter(v => v >= D) : iv; // day labels only once a day
  const I = fine.find(v => (to - from) / v <= max) || fine[fine.length - 1];
  const off = ms => new Date(ms).getTimezoneOffset() * M; // local = utc - off
  const out = [];
  let t = Math.ceil((from - off(from)) / I) * I + off(from);
  for (; t <= to && out.length < 50; t += I) { const a = Math.round((t - off(t)) / I) * I + off(t); out.push(a); }
  return out;
}
// settledTo marks where the data stops being complete: buckets after it
// are drawn, because leaving the newest data off a live chart is its own
// kind of lie, but they are still filling. Without the mark a minute that
// is half counted looks like a fall in traffic.
// dataFrom is when the stored data begins: the buckets that end before it
// are left out (a gap), not drawn as traffic of zero. Lines marked keep
// (a baseline from another day, device counters) are not cut.
function tsChart(el, {times, areas = [], lines = [], fmtY = fmtAxisBps, fmtV = fmtBps, settledTo = 0, dataFrom = 0, empty = ''}) {
  const stepMs = times && times.length > 1 ? times[1] - times[0] : 0;
  if (dataFrom && times) {
    const cut = d => (d || []).map((v, i) => times[i] + stepMs <= dataFrom ? null : v);
    areas = areas.map(a => ({...a, data: cut(a.data)}));
    lines = lines.map(l => l.keep ? l : {...l, data: cut(l.data)});
  }
  const draw = () => {
    const W = el.clientWidth, H = el.clientHeight;
    if (!times || !times.length) { el.innerHTML = `<div class="empty">${empty || emptyText()}</div>`; return; }
    if (W < 120 || H < 60) { el.innerHTML = ''; return; }
    // nothing at all to draw: say so instead of an axis from 0 to 1
    const has = d => (d || []).some(v => v != null && v >= 0);
    if (!areas.some(a => has(a.data)) && !lines.some(l => has(l.data))) { el.innerHTML = `<div class="empty">${empty || emptyText()}</div>`; return; }
    const rtl = document.documentElement.dir === 'rtl';
    const L = rtl ? 8 : 50, R = rtl ? 50 : 8, T = 8, B = 22, n = times.length;
    const x = i => rtl ? W - R - (W - L - R) * i / Math.max(1, n - 1) : L + (W - L - R) * i / Math.max(1, n - 1);
    const cum = areas.map(() => new Array(n).fill(null));
    for (let i = 0; i < n; i++) { let s = 0; areas.forEach((a, k) => { const v = a.data[i]; if (v == null || v < 0) return; s += v; cum[k][i] = s; }); }
    let top = 0;
    cum.forEach(c => c.forEach(v => { if (v != null && v > top) top = v; }));
    lines.forEach(l => l.data.forEach(v => { if (v != null && v > top) top = v; }));
    // all zero: a scale up to 1K reads as "nothing", 0..1 rounds to 0,0,1,1,1
    const Y = top > 0 ? niceMax(top * 1.05) : 1000, y = v => T + (H - T - B) * (1 - v / Y);
    const span = times[n - 1] - times[0];
    let s = `<svg viewBox="0 0 ${W} ${H}" height="${H}" role="img" aria-label="${esc(el.getAttribute('aria-label') || '')}">`;
    for (let k = 0; k <= 4; k++) {
      const yy = y(Y * k / 4);
      s += `<line class="gl" x1="${L}" x2="${W - R}" y1="${yy}" y2="${yy}"/><text class="ax" x="${rtl ? W - R + 8 : L - 8}" y="${yy + 4}" text-anchor="${rtl ? 'start' : 'end'}">${fmtY(Y * k / 4)}</text>`;
    }
    // time labels on round times (every 5, 10, 15, 30 minutes, hours,
    // days at local midnight), placed where that time falls
    const xt = ms => { const f = stepMs ? (ms - times[0]) / stepMs : 0; return rtl ? W - R - (W - L - R) * f / Math.max(1, n - 1) : L + (W - L - R) * f / Math.max(1, n - 1); };
    for (const ms of axisTicks(times[0], times[n - 1], W < 600 ? 4 : 7, span)) s += `<text class="ax" x="${xt(ms).toFixed(1)}" y="${H - 5}" text-anchor="middle">${fmtAxisTime(ms, span)}</text>`;
    // The buckets still filling are not stacked: a stack that falls off a
    // cliff at the right edge reads as traffic stopping. They stay in the
    // tooltip and under the hatching; lines draw them dashed.
    const firstUnsettled = settledTo ? times.findIndex(v => v + stepMs > settledTo) : -1;
    const stackTo = firstUnsettled > 0 ? firstUnsettled : n;
    areas.forEach((a, k) => {
      let seg = [], paths = '';
      const flush = () => {
        if (seg.length < 1) { seg = []; return; }
        let up = '', lo = '';
        seg.forEach(i => { up += (up ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(cum[k][i]).toFixed(1); });
        for (let j = seg.length - 1; j >= 0; j--) { const i = seg[j]; const b = k ? (cum[k - 1][i] || 0) : 0; lo += 'L' + x(i).toFixed(1) + ' ' + y(b).toFixed(1); }
        paths += up + lo + 'Z'; seg = [];
      };
      for (let i = 0; i < stackTo; i++) { if (cum[k][i] == null) flush(); else seg.push(i); }
      flush();
      s += `<path d="${paths}" fill="${a.color}" fill-opacity=".88" stroke="var(--surface)" stroke-width="1"/>`;
    });
    lines.forEach(l => {
      let d = '', pen = false, area = '', seg = [];
      const close = () => { if (seg.length > 1) area += 'M' + seg.map(i => x(i).toFixed(1) + ' ' + y(l.data[i]).toFixed(1)).join('L') + `L${x(seg[seg.length - 1]).toFixed(1)} ${y(0)}L${x(seg[0]).toFixed(1)} ${y(0)}Z`; seg = []; };
      const lineTo = l.keep ? n : stackTo;
      l.data.forEach((v, i) => { if (i >= lineTo) return; if (v == null || v < 0) { pen = false; close(); return; } d += (pen ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(v).toFixed(1); pen = true; seg.push(i); });
      close();
      // the unsettled tail, from the last settled point, thin and dashed
      let tail = '';
      if (lineTo < n) { let tp = false; for (let i = lineTo - 1; i < n; i++) { const v = l.data[i]; if (v == null || v < 0) { tp = false; continue; } tail += (tp ? 'L' : 'M') + x(i).toFixed(1) + ' ' + y(v).toFixed(1); tp = true; } }
      if (l.fill && area) s += `<path d="${area}" fill="${l.color}" fill-opacity=".14" stroke="none"/>`;
      s += `<path d="${d}" fill="none" stroke="${l.color}" stroke-width="2" ${l.dash ? 'stroke-dasharray="4 3"' : ''}/>`;
      if (tail) s += `<path class="tail" d="${tail}" fill="none" stroke="${l.color}" stroke-width="1.5" stroke-opacity=".6" stroke-dasharray="2 3"/>`;
    });
    // The tail that is still filling: a hatched wash and a line where the
    // complete data ends, rather than a chart that stops short of now.
    // A bucket is complete only when the whole of it is behind the
    // boundary, so it is the bucket's end that is compared, not its start:
    // at 10:58 with a settle of a minute, the 10:55 bucket still has two
    // minutes to go.
    // starting half a bucket back, so a single filling bucket is visible
    if (firstUnsettled >= 0 && firstUnsettled < n) {
      const xs = n > 1 ? (x(firstUnsettled) + x(Math.max(0, firstUnsettled - 1))) / 2 : x(0);
      const xa = Math.min(xs, x(n - 1)), xb = Math.max(xs, x(n - 1));
      const xEdge = rtl ? xb : xa;
      s += `<defs><pattern id="uns" width="7" height="7" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">` +
        `<line x1="0" y1="0" x2="0" y2="7" stroke="var(--ink-3)" stroke-width="1" opacity=".5"/></pattern></defs>` +
        `<rect x="${xa.toFixed(1)}" y="${T}" width="${Math.max(0, xb - xa).toFixed(1)}" height="${H - T - B}" fill="url(#uns)" opacity=".5" pointer-events="none"/>` +
        `<line x1="${xEdge.toFixed(1)}" x2="${xEdge.toFixed(1)}" y1="${T}" y2="${H - B}" stroke="var(--ink-3)" stroke-width="1" stroke-dasharray="3 3"/>`;
    }
    s += `<line class="xh" x1="0" x2="0" y1="${T}" y2="${H - B}" stroke="var(--ink-2)" stroke-width="1" style="display:none"/><rect class="hit" x="${L}" y="${T}" width="${W - L - R}" height="${H - T - B}" fill="transparent"/></svg>`;
    el.innerHTML = s;
    const svg = el.querySelector('svg'), xh = svg.querySelector('.xh'), hit = svg.querySelector('.hit');
    hit.onmousemove = e => {
      const r = svg.getBoundingClientRect();
      let fx = (e.clientX - r.left - L) / (W - L - R); if (rtl) fx = (W - R - (e.clientX - r.left)) / (W - L - R);
      const i = Math.max(0, Math.min(n - 1, Math.round(fx * (n - 1))));
      xh.setAttribute('x1', x(i)); xh.setAttribute('x2', x(i)); xh.style.display = '';
      let h = `<div class="t">${fmtTime(times[i], span)}${settledTo && times[i] + (n > 1 ? times[1] - times[0] : 0) > settledTo ? ` <span class="muted">· ${esc(t('ch.unsettled'))}</span>` : ''}</div>`, tot = 0, any = false;
      [...areas].reverse().forEach(a => { const v = a.data[i]; if (v == null || v < 0) return; any = true; tot += v; h += `<div class="r"><span><i style="background:${a.color}"></i>${esc(a.name)}</span><b>${fmtV(v)}</b></div>`; });
      if (any && areas.length > 1) h += `<div class="r tot"><span>${t('total')}</span><span>${fmtV(tot)}</span></div>`;
      lines.forEach(l => { const v = l.data[i]; if (v == null || v < 0) return; h += `<div class="r"><span><i style="background:${l.color}"></i>${esc(l.name)}</span><b>${fmtV(v)}</b></div>`; });
      showTip(e, h);
    };
    hit.onmouseleave = () => { xh.style.display = 'none'; hideTip(); };
  };
  el._draw = draw; draw();
  if (!el._ro) { el._ro = new ResizeObserver(() => el._draw && el._draw()); el._ro.observe(el); }
}
const views = {};
let loadSeq = 0;
function panel(cls, title, sub, body) {
  return `<div class="panel ${cls}"><div class="ph"><h2>${title}</h2>${sub ? `<span class="sub">${sub}</span>` : ''}</div>${body}</div>`;
}
// Database failures get a plain explanation; the details are in the log.
function errMsg(err) {
  if (err.kind === 'memory') return t('err.db_memory');
  if (err.kind === 'storage') return t('err.db_busy');
  return t('err.load', {e: err.message});
}
function errorBox(err) { return `<div class="panel"><div class="empty">${esc(errMsg(err))}</div></div>`; }
function firstText() {
  const l = setup?.listen || [], ports = l.map(x => x.port).sort().join(',');
  if (!l.length || ports === '6343') return t('empty.first');
  return t('ob.step1') + ' ' + t('ob.ports') + ' ' + l.map(x => x.name + ' ' + x.port).join(', ');
}
// ---- Help: the user manual (docs/MANUAL*.md, built into the program) ----
// A small markdown renderer for exactly what the manual uses: headings,
// paragraphs, lists, tables, fenced code, images, links, **bold**, `code`
// and <a id> anchors. Everything is escaped first and only these tags are
// made, so nothing in the text can run or load anything else.
const GH = 'https://github.com/githubflyideas/traffic66/blob/main/';
let helpLang = null;  // a language picked on the page; else the UI language
const mdSlug = (s, seen) => {
  let b = s.toLowerCase().replace(/<[^>]*>/g, '').replace(/[^\p{L}\p{N}\s_-]/gu, '').trim().replace(/\s/g, '-'), k = b, n = 0;
  while (seen.has(k)) k = b + '-' + (++n);
  seen.add(k);
  return k;
};
function mdHref(u) {
  let m;
  if (u.startsWith('#')) return {href: u, cls: 'mdin'};
  if ((m = /^MANUAL(?:\.([a-z]{2}))?\.md(#.*)?$/.exec(u))) return {href: '#v=help', lang: m[1] || 'en'};
  if (/^https?:\/\//.test(u)) return {href: u, ext: true};
  if ((m = /^\.\.\/([\w.-]+\.md)$/.exec(u))) return {href: GH + m[1], ext: true};
  return null;
}
function mdInline(s) {
  // code spans first, kept aside so nothing inside them is formatted
  const codes = [];
  s = s.replace(/`([^`]+)`/g, (_, c) => { codes.push(c); return '\u0000' + (codes.length - 1) + '\u0000'; });
  s = esc(s.replace(/<\/?sub>/g, ''));
  s = s.replace(/!\[([^\]]*)\]\(images\/([\w.-]+\.png)\)/g, (_, alt, f) => `<img src="/api/manual/images/${f}" alt="${alt}">`);
  s = s.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (all, txt, u) => {
    const h = mdHref(u.replace(/&amp;/g, '&'));
    if (!h) return txt;
    return `<a href="${esc(h.href)}"${h.cls ? ` class="${h.cls}"` : ''}${h.lang ? ` data-hlang="${h.lang}"` : ''}${h.ext ? ' target="_blank" rel="noopener"' : ''}>${txt}</a>`;
  });
  s = s.replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>').replace(/(^|[\s(])\*([^*\s][^*]*)\*(?=[\s).,;:]|$)/g, '$1<i>$2</i>');
  return s.replace(/\u0000(\d+)\u0000/g, (_, i) => `<code>${esc(codes[+i])}</code>`);
}
function mdRender(md) {
  const lines = md.replace(/\r/g, '').split('\n'), out = [], toc = [], seen = new Set();
  let i = 0, para = [];
  const flush = () => { if (para.length) { out.push(`<p>${mdInline(para.join(' '))}</p>`); para = []; } };
  while (i < lines.length) {
    const l = lines[i];
    let m;
    if (/^```/.test(l)) {
      flush();
      const code = [];
      for (i++; i < lines.length && !/^```/.test(lines[i]); i++) code.push(lines[i]);
      i++;
      out.push(`<div class="mdcode"><button class="mdcopy" type="button">${esc(t('ob.copy'))}</button><pre><code>${esc(code.join('\n'))}</code></pre></div>`);
      continue;
    }
    if ((m = /^<a id="([\w-]+)"><\/a>$/.exec(l.trim()))) { flush(); seen.add(m[1]); out.push(`<span class="mdanchor" id="h-${esc(m[1])}"></span>`); i++; continue; }
    if (/^<\/?div[^>]*>$/.test(l.trim())) { i++; continue; }
    if ((m = /^(#{1,4}) (.*)$/.exec(l))) {
      flush();
      const n = m[1].length, id = mdSlug(m[2], seen);
      if (n === 2) toc.push({id, text: m[2]});
      out.push(`<h${n} id="h-${esc(id)}">${mdInline(m[2])}</h${n}>`);
      i++; continue;
    }
    if (/^\|.*\|\s*$/.test(l) && /^\|[\s:|-]+\|\s*$/.test(lines[i + 1] || '')) {
      flush();
      const cells = r => r.trim().replace(/^\||\|$/g, '').split('|').map(c => c.trim());
      const head = cells(l);
      let html = `<div class="mdtable"><table><tr>${head.map(c => `<th>${mdInline(c)}</th>`).join('')}</tr>`;
      for (i += 2; i < lines.length && /^\|.*\|\s*$/.test(lines[i]); i++) html += `<tr>${cells(lines[i]).map(c => `<td>${mdInline(c)}</td>`).join('')}</tr>`;
      out.push(html + '</table></div>');
      continue;
    }
    if ((m = /^\s*([-*]|\d+\.) (.*)$/.exec(l))) {
      flush();
      const ol = /\d/.test(m[1]), items = [];
      while (i < lines.length && (m = /^\s*([-*]|\d+\.) (.*)$/.exec(lines[i]))) {
        let it = m[2];
        for (i++; i < lines.length && /^\s{2,}\S/.test(lines[i]) && !/^\s*([-*]|\d+\.) /.test(lines[i]); i++) it += ' ' + lines[i].trim();
        items.push(`<li>${mdInline(it)}</li>`);
      }
      out.push(`<${ol ? 'ol' : 'ul'}>${items.join('')}</${ol ? 'ol' : 'ul'}>`);
      continue;
    }
    if (!l.trim()) { flush(); i++; continue; }
    para.push(l.trim());
    i++;
  }
  flush();
  return {html: out.join('\n'), toc};
}
views.help = async (el) => {
  const seq = loadSeq;
  const lang = helpLang || LANG;
  const res = await fetch('/api/manual?lang=' + encodeURIComponent(lang));
  if (!res.ok) throw new Error('manual: ' + res.status);
  const {html, toc} = mdRender(await res.text());
  checkLoad(seq);
  const rtl = ['ar', 'ur'].includes(lang);
  el.innerHTML = `<div class="help">
    <nav class="helptoc" aria-label="${esc(t('nav.help'))}">${toc.map(h => `<a href="#h-${esc(h.id)}" class="mdin">${mdInline(h.text)}</a>`).join('')}</nav>
    <article class="manual panel"${rtl ? ' dir="rtl"' : ''}>${html}</article></div>`;
  el.querySelectorAll('.mdcopy').forEach(b => b.onclick = () => {
    const txt = b.nextElementSibling.textContent;
    navigator.clipboard?.writeText(txt).then(() => toast(t('top.copied')), () => toast(txt.slice(0, 80)));
  });
  el.querySelectorAll('a.mdin').forEach(a => a.onclick = e => {
    e.preventDefault();
    const id = a.getAttribute('href').slice(1), tg = document.getElementById(id.startsWith('h-') ? id : 'h-' + id);
    tg?.scrollIntoView({block: 'start'});
  });
  el.querySelectorAll('a[data-hlang]').forEach(a => a.onclick = e => { e.preventDefault(); helpLang = a.dataset.hlang; render(); window.scrollTo(0, 0); });
};

const dimLabel = d => t(d === 'app' ? 'col.service' : d === 'host' ? 'kpi.hosts' : 'col.proto');
function drawSeries(el, series) {
 series = series || {times: [], names: [], values: []};
 tsChart(el, {times: series.times || [], areas: (series.names || []).map((n,i) => ({name: appLabel(n), color: color(i,n), data: series.values[i]})), settledTo: series.settled_to, dataFrom: series.data_from});
}
// the series of the overview: everything, or only what came in or only
// what went out through the interface sFlow samples on
const dirSeg = () => `<div class="seg" role="group">${[['', 'ov.dir_total'], ['in', 'if.in'], ['out', 'if.out']].map(([k, l]) =>
  `<button data-od="${k}" aria-pressed="${state.od === k}">${esc(t(l))}</button>`).join('')}</div>`;
views.overview = async el => {
 const d = await api('overview', state.od ? {dir: state.od} : {}), total = d.totals || {};
 const title = state.od ? t(state.od === 'in' ? 'ov.bw_in' : 'ov.bw_out') : t('ov.bw_title');
 el.innerHTML = `<div class="grid"><div class="panel c12"><p class="muted" id="kpiScope">${esc(t('ov.kpi_scope'))}</p><div class="kpis">
 <div class="kpi"><div class="lab">${t('kpi.now')}</div><div class="n">${fmtBps(d.now_bps)}</div></div>
 <div class="kpi"><div class="lab">${t('kpi.total', {r: rangeLabel()})}</div><div class="n">${fmtBytes(total.wire)}</div></div>
 <div class="kpi"><div class="lab">${t('col.pkts')}</div><div class="n">${nf(total.pkts || 0)}</div></div>
 <div class="kpi"><div class="lab">${t('col.flows')}</div><div class="n">${nf(total.flows || 0)}</div></div>
 </div></div><div class="panel c12"><div class="ph"><h2>${esc(title)}</h2><div class="tools" style="margin-inline-start:auto">${dirSeg()}</div></div>
 ${state.od ? `<p class="muted" id="directionScope">${esc(t('ov.direction_scope'))} · <span id="directionIface">${d.dir_iface ? esc(d.dir_iface) : esc(t('ov.direction_empty'))}</span></p>` : ''}
 <div class="chart" id="chStack"></div></div>
 ${panel('c12', t('self.title'), '', '<div class="self" id="self"></div>')}</div>`;
 drawSeries($('#chStack'), state.od ? d.dir_series : d.series); fillSelf();
 el.querySelectorAll('[data-od]').forEach(b => b.onclick = () => { state.od = b.dataset.od; render(true); });
};
const modeSeg = () => `<div class="seg" role="group"><button data-tm="talkers" aria-pressed="${state.tm === 'talkers'}">${esc(t('topn.talkers'))}</button><button data-tm="table" aria-pressed="${state.tm !== 'talkers'}">${esc(t('topn.table'))}</button></div>`;
const bindMode = el => el.querySelectorAll('[data-tm]').forEach(b => b.onclick = () => { state.tm = b.dataset.tm; render(true); });
views.topn = el => state.tm === 'talkers' ? talkers(el) : topTable(el, state.dim);
// the two ends: who sent and fetched the most, and who served the most
async function talkers(el) {
 const [cl, sv, ov] = await Promise.all([api('topn', {dim: 'client', limit: 30}), api('topn', {dim: 'server', limit: 30}), api('overview')]);
 const tot = ov.totals || {};
 const table = (rows, head) => `<table><thead><tr><th></th><th>${t(head)}</th><th>${t('col.traffic')}</th><th>${t('col.pkts')}</th><th>${t('col.flows')}</th></tr></thead><tbody>
  ${(rows || []).map((r, i) => `<tr><td class="rank">${i + 1}</td><td>${esc(r.key)}</td><td>${fmtBytes(r.wire)}</td><td>${nf(r.pkts || 0)}</td><td>${nf(r.flows || 0)}</td></tr>`).join('') || `<tr><td colspan="5" class="empty">${emptyText()}</td></tr>`}
  <tr class="sum"><td></td><td>${t('topn.global_total')}</td><td>${fmtBytes(tot.wire)}</td><td>${nf(tot.pkts || 0)}</td><td>${nf(tot.flows || 0)}</td></tr></tbody></table>`;
 el.innerHTML = `<div class="grid"><div class="c12">${modeSeg()}<p class="muted" id="topPerspectives">${esc(t('topn.perspectives'))}</p></div>
  ${panel('c6', t('ov.top_clients'), esc(t('topn.top_n', {n: 30})), table(cl.rows, 'col.client'))}
  ${panel('c6', t('topn.top_servers'), esc(t('topn.top_n', {n: 30})), table(sv.rows, 'col.server'))}</div>`;
 bindMode(el);
}
async function topTable(el, dim) {
 dim = DIMS.includes(dim) ? dim : 'host';
 const d = await api('topn', {dim, limit: 66}), rows = d.rows || [];
 el.innerHTML = `<div class="grid"><div class="c12">${modeSeg()}</div>` + panel('c12', t('nav.topn'), '', `<label>${t('nav.topn')} <select id="topDim">${DIMS.map(k => `<option value="${k}" ${dim === k ? 'selected' : ''}>${dimLabel(k)}</option>`).join('')}</select></label>
 <table><thead><tr><th>${dimLabel(dim)}</th><th>${t('col.traffic')}</th><th>${t('col.pkts')}</th><th>${t('col.flows')}</th></tr></thead><tbody>
 ${rows.map(r => `<tr><td>${esc(dim === 'proto' ? proto(r.key) : r.key)}</td><td>${fmtBytes(r.wire)}</td><td>${nf(r.pkts || 0)}</td><td>${nf(r.flows || 0)}</td></tr>`).join('') || `<tr><td colspan="4" class="empty">${emptyText()}</td></tr>`}</tbody></table>`) + '</div>';
 $('#topDim').onchange = e => { state.dim = e.target.value; render(true); };
 bindMode(el);
}
// The newest flow records, at most 500, over at most a day; one address
// can be picked out.
const REC_MAX = 500;
// sFlow's interface number for the router itself
const ifLabel = i => i === 0x3FFFFFFF ? esc(t('rc.local')) : String(i);
views.records = async el => {
 const d = await api('records', {limit: REC_MAX}, state.ip ? [{f: 'ip', v: state.ip, neg: false}] : []), rows = d.rows || [];
 const tf = new Intl.DateTimeFormat(LANG, {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'});
 el.innerHTML = panel('c12', t('rec.title'), esc(t('rec.latest', {n: nf(rows.length)})) + ' · ' + esc(t('rec.sorted')), `<p class="muted" id="recordWindow" data-from="${esc(d.from || '')}" data-to="${esc(d.to || '')}" data-limit="${esc(d.limit ?? '')}">${esc(t('rec.window', {from: d.from ? new Date(d.from).toLocaleString(LANG, {timeZone: 'UTC'}) + ' UTC' : '—', to: d.to ? new Date(d.to).toLocaleString(LANG, {timeZone: 'UTC'}) + ' UTC' : '—', limit: d.limit == null ? '—' : nf(d.limit)}))}</p><p class="muted" id="recordEstimates">${esc(t('rec.estimates'))}</p><form class="recf" id="recF"><input id="recIP" value="${esc(state.ip)}" placeholder="${esc(t('field.ip'))} (192.0.2.10)" spellcheck="false" autocomplete="off"><button class="btn">${esc(t('rec.filter'))}</button>${state.ip ? `<button type="button" class="btn" id="recClr">${esc(t('rec.all'))}</button>` : ''}</form>
 <div class="tablewrap"><table class="rec"><thead><tr><th>${t('rc.time')}</th><th>${t('rc.client')}</th><th>${t('rc.server')}</th><th>${t('rc.proto')}</th><th>${t('rc.app')}</th><th>${t('rc.traffic')}</th><th>${t('rc.packets')}</th><th>${t('rc.in_if')} → ${t('rc.out_if')}</th><th>${t('rc.sampling')}</th></tr></thead><tbody>
 ${rows.map(r => `<tr><td class="nw">${esc(tf.format(new Date(r.ts)))}</td><td class="mono">${esc(r.client)}<span class="muted">:${r.cport}</span></td><td class="mono">${esc(r.server)}<span class="muted">:${r.port}</span></td><td>${esc(proto(r.proto))}</td><td>${esc(appLabel(r.app))}</td><td>${fmtBytes(r.wire)}</td><td>${nf(r.pkts || 0)}</td><td class="nw">${ifLabel(r.in_if)} → ${ifLabel(r.out_if)}</td><td>${Number.isFinite(r.mult) && r.mult > 0 ? esc(t('rec.multiplier', {n: nf(r.mult, 2)})) : '—'}</td></tr>`).join('') || `<tr><td colspan="9" class="empty">${emptyText()}</td></tr>`}</tbody></table></div>`);
 $('#recF').onsubmit = e => { e.preventDefault(); state.ip = $('#recIP').value.trim(); render(true); };
 if ($('#recClr')) $('#recClr').onclick = () => { state.ip = ''; render(true); };
};
function streamHint(s) {
 if (!s?.enforced || !s.configured) return '';
 const text = t('stream.identity', {a:s.agent, s:nf(s.sub_agent)});
 return `<p class="stream-policy" role="status">${esc(text)} · ${esc(t('stream.rejected', {n:nf(s.rejected_streams || 0)}))}</p>`;
}
views.sources = async el => {
 const d = await api('sources'), rows = d.sources || [];
 el.innerHTML = `<div class="grid">${panel('c12', t('self.title'), '', '<div class="self" id="self"></div>')}
 ${panel('c12', t('src.title'), t('src.sub'), `${streamHint(d.stream)}<table><tr><th>${t('col.device')}</th><th>${t('col.proto')}</th><th>${t('col.rate')}</th><th>${t('col.sampling')}</th><th>${t('col.lost')}</th><th>${t('col.status')}</th></tr>
 ${rows.map(s => `<tr><td>${esc(s.exporter)}</td><td>${esc(s.proto)}</td><td>${nf(s.rec_per_sec || 0,1)}</td><td>${esc(s.sampling || s.sampling_state || '—')}${s.effective ? ' (1:' + nf(s.effective) + ')' : ''}</td><td>${nf(s.lost_pct || 0,2)}%</td><td>${esc(t('st.' + s.status))}${(s.issues || []).map(k => `<div class="issue">${esc(t('issue.' + k,{a:s.exporter,n:nf(s.pending||0),p:nf((k === 'agent_drops' ? s.drop_pct : k === 'loss' ? (s.transit_pct ?? s.lost_pct) : s.lost_pct) || 0,1)+'%',s:Math.round(Math.abs(s.clock_skew_ns||0)/1e9)+' s'}))}</div>`).join('')}</td></tr>`).join('') || `<tr><td colspan="6" class="empty">${emptyText()}</td></tr>`}</table>
 <p>${t('src.listeners')} ${(d.listeners || []).map(l => `<span class="tag">UDP ${esc(l.addr)} · ${esc(l.proto)} · ${nf(l.packets || 0)}</span>`).join(' ')}</p>`)}
 ${panel('c12', t('ic.title'), t('ic.sub'), ifaceCheck(d.ifaces || []))}</div>`;
 fillSelf();
};
// Each interface as the device counts it against what the samples add up
// to; far apart in one direction usually means it is not sampled there.
const icPct = (est, dev) => {
  if (!(dev > 0)) return '<td>—</td>';
  const p = est / dev * 100, off = Math.abs(p - 100);
  return `<td class="${off > 20 ? 'bad' : off > 8 ? 'warn' : 'good'}">${nf(p, 1)}%</td>`;
};
const ifaceCheck = rows => rows.length ? `<div class="tablewrap"><table class="ifcheck"><tr><th>${t('ic.iface')}</th><th>${t('ic.dev_in')}</th><th>${t('ic.est_in')}</th><th></th><th>${t('ic.dev_out')}</th><th>${t('ic.est_out')}</th><th></th></tr>
 ${rows.map(r => `<tr><td class="nw">${esc(r.exporter)} / ${r.ifindex}${r.speed ? ` <span class="muted">${fmtBps(r.speed)}</span>` : ''}</td><td>${fmtBps(r.dev_in_bps)}</td><td>${fmtBps(r.est_in_bps)}</td>${icPct(r.est_in_bps, r.dev_in_bps)}<td>${fmtBps(r.dev_out_bps)}</td><td>${fmtBps(r.est_out_bps)}</td>${icPct(r.est_out_bps, r.dev_out_bps)}</tr>`).join('')}</table></div>
 <p class="muted" style="font-size:12.5px">${esc(t('ic.note', {m: nf(Math.round((rows[0].seconds || 0) / 60))}))}</p>` : `<div class="empty">${esc(t('ic.none'))}</div>`;
views.cleanup = async (el) => {
  // Every control here deletes stored data, so there is nothing of this
  // page to show someone who may not.
  if (!adm()) { el.innerHTML = `<div class="panel"><div class="empty">${esc(t('cl.admin_only'))}</div></div>`; return; }
  const d = await (await fetch('/api/cleanup')).json();
  el.innerHTML = `<div class="grid">${panel('c12', t('cl.title'), t('cl.sub'), `
    <div class="kpis" style="grid-template-columns:repeat(2,1fr);margin-bottom:14px"><div class="kpi"><div class="lab">${t('cl.disk')}</div><div class="n">${fmtBytes(d.disk_bytes)}</div></div>
      <div class="kpi"><div class="lab">${t('cl.oldest')}</div><div class="n" style="font-size:20px">${d.oldest && !d.oldest.startsWith('0001') ? esc(new Intl.DateTimeFormat(LANG, {dateStyle: 'medium', timeStyle: 'short'}).format(new Date(d.oldest))) : '—'}</div></div></div>
    <p>${esc(t('cl.all'))}</p><button class="btn danger" data-cl-all>${esc(t('cl.run'))}</button>
    <div id="clMsg" style="margin-top:10px;font-size:13px"></div>`)}</div>`;
  const b = el.querySelector('[data-cl-all]');
  let busy = false;
  b.onclick = async () => {
    if (busy) return;
    if (!confirm(t('cl.confirm_all')) || !confirm(t('cl.confirm_all2'))) return;
    busy = true; b.disabled = true;
    try {
      const res = await fetch('/api/cleanup', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({all:true, confirm:true})});
      const j = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(j.error || res.statusText);
      clDone = t('cl.done', {n: nf(j.rows || 0), s: fmtBytes(j.bytes || 0)});
      loadStatus(); render();
    } catch (error) {
      const m = $('#clMsg'); m.style.color = 'var(--crit)'; m.textContent = error.message;
    } finally { busy = false; b.disabled = false; }
  };
  if (clDone) { const m = $('#clMsg'); m.style.color = 'var(--good)'; m.textContent = clDone; clDone = ''; }
};
let clDone = '';


// Controls only an administrator gets. The calls behind them are gated
// server-side too (adminAPI, in internal/api/server.go) — this only keeps
// the page from offering a button that would answer 403. Tables and
// everything else worth reading stay visible to everyone.
const adm = () => !!(me && me.admin);
const admOnly = html => adm() ? html : '';
views.account = async (el) => {
  await loadMe();
  const m = me || {};
  const pwForm = m.fixed ? `<p class="muted">${esc(t('acct.fixed'))}</p>` : `
    ${m.must_change ? `<p class="acctmust">${esc(t('acct.must'))}</p>` : ''}
    <form class="acctform" id="pwForm" autocomplete="off">
      <label ${m.must_change ? 'hidden' : ''}><span>${t('acct.old')}</span><input type="password" name="old" autocomplete="current-password"></label>
      <label><span>${t('acct.new')}</span><input type="password" name="new" autocomplete="new-password" required></label>
      <label><span>${t('acct.confirm')}</span><input type="password" name="again" autocomplete="new-password" required></label>
      <p class="muted" style="font-size:12.5px;margin:0">${esc(t('acct.rule'))}</p>
      <div><button class="primary" type="submit">${t('acct.save')}</button> <span id="pwMsg"></span></div>
    </form>`;
  let users = null;
  if (m.admin && !m.fixed && !m.must_change) {
    const r = await fetch('/api/users');
    if (r.ok) users = await r.json();
  }
  const userRows = users ? users.users.map(u => `<tr><td><b>${esc(u)}</b>${u === users.admin ? ` <span class="tag">${t('acct.admin_tag')}</span>` : ''}</td>
      <td class="nw"><button class="btn" data-reset="${esc(u)}">${t('acct.reset')}</button>${u === users.admin ? '' : ` <button class="btn" data-udel="${esc(u)}">${t('acct.delete')}</button>`}</td></tr>
      <tr data-resetrow="${esc(u)}" hidden><td colspan="2"><form class="acctinline" data-resetform="${esc(u)}"><input type="password" name="pw" placeholder="${esc(t('acct.new'))}" autocomplete="new-password" required> <button class="primary" type="submit">${t('acct.save')}</button></form></td></tr>`).join('') : '';
  el.innerHTML = `<div class="grid">
    ${panel(users ? 'c6' : 'c12', t('acct.title_pw'), esc(m.user || ''), pwForm)}
    ${users ? panel('c6', t('acct.users'), t('acct.users_sub'), `<table class="accttable">${userRows}</table>
      <form class="acctinline" id="addUser" autocomplete="off"><input name="user" placeholder="${esc(t('acct.user'))}" required> <input type="password" name="pw" placeholder="${esc(t('acct.new'))}" autocomplete="new-password" required> <button class="primary" type="submit">${t('acct.add')}</button></form>
      ${users.limit > 0 ? `<p class="muted" style="font-size:13px;margin:8px 0 0">${esc(t('acct.users_count', {used: users.users.length, n: users.limit}))}</p>` : ''}
      <p id="userMsg" style="font-size:13px;margin:8px 0 0"></p>`) : ''}
  </div>`;
  const errText = c => t('acct.err_' + c) !== 'acct.err_' + c ? t('acct.err_' + c) : c;
  const say = (id, ok, text) => { const n = $(id); if (n) { n.style.color = ok ? 'var(--good)' : 'var(--crit)'; n.textContent = text; } };
  const pf = $('#pwForm');
  if (pf) pf.onsubmit = async e => {
    e.preventDefault();
    const old = m.must_change ? (sessionStorage.getItem('t66.old') || 'tryvyossflow66') : pf.old.value;
    if (pf.new.value !== pf.again.value) return say('#pwMsg', false, t('acct.err_mismatch'));
    const r = await fetch('/api/password', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({old, new: pf.new.value})});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) return say('#pwMsg', false, errText(j.error));
    sessionStorage.removeItem('t66.old');
    toast(t('acct.saved'));
    if (m.must_change) { me.must_change = false; go('overview'); } else render();
  };
  const putUser = async (user, password) => {
    const r = await fetch('/api/users', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({user, password})});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) { say('#userMsg', false, errText(j.error)); return false; }
    toast(t('acct.added')); render(); return true;
  };
  const au = $('#addUser');
  if (au) au.onsubmit = e => { e.preventDefault(); putUser(au.user.value.trim(), au.pw.value); };
  el.querySelectorAll('[data-reset]').forEach(b => b.onclick = () => { const row = el.querySelector(`[data-resetrow="${CSS.escape(b.dataset.reset)}"]`); row.hidden = !row.hidden; });
  el.querySelectorAll('[data-resetform]').forEach(f => f.onsubmit = e => { e.preventDefault(); putUser(f.dataset.resetform, f.pw.value); });
  el.querySelectorAll('[data-udel]').forEach(b => b.onclick = async () => {
    if (!confirm(t('acct.confirm_del', {u: b.dataset.udel}))) return;
    const r = await fetch('/api/users?user=' + encodeURIComponent(b.dataset.udel), {method: 'DELETE'});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) return say('#userMsg', false, errText(j.error));
    render();
  });
};
const NOBAR_VIEWS = new Set(['help', 'sources', 'cleanup', 'account']);
function renderBar() {
  const off = NOBAR_VIEWS.has(state.v);
  if (off) $('#range').hidden = true;
  $('#refresh').hidden = off || $('#range').hidden || !REFRESH_MS[state.r];
  refreshLabel();
  $('#share').hidden = off;
}
function refreshLabel() {
  const beat = REFRESH_MS[state.r];
  $('#refresh').textContent = beat ? t('top.refresh', {n: beat / 1000}) : '';
}


let drawnAt = 0;
async function render(push) {
  const seq = ++loadSeq;
  if (me?.must_change) state.v = 'account';
  document.body.classList.toggle('mustchange', !!me?.must_change);
  $('#range').hidden = false;
  renderBar();
  writeHash(push);
  const navV = state.v;
  $('#title').textContent = t('nav.' + navV);  // the detail view refines it
  document.title = t('nav.' + state.v) + ' · vyos-sflow';
  document.querySelectorAll('#nav button').forEach(b => b.dataset.v === navV ? b.setAttribute('aria-current', 'page') : b.removeAttribute('aria-current'));
  renderRange();
  if (seq !== loadSeq) return;
  writeHash(false);
  document.querySelectorAll('.view').forEach(s => s.classList.toggle('on', s.id === 'v-' + state.v));
  const el = $('#v-' + state.v);
  try {
    await views[state.v](el);
  } catch (e) {
    if (e.message !== 'login' && e.message !== 'stale' && seq === loadSeq) el.innerHTML = errorBox(e);
  }
  if (seq === loadSeq) { drawnAt = Date.now(); }
}
function go(v) { state.v = VIEWS.includes(v) ? v : 'overview'; render(true); window.scrollTo(0, 0); }

async function loadStatus() {
  try {
    const res = await fetch('/api/status');
    if (res.status === 401) { showLogin(); return; }
    const s = await res.json();
    waiting = s.waiting || null;
    setup = s;
    $('#verNo').textContent = 'v' + s.version + (s.build && s.build !== 'official' ? ' · ' + s.build : '');
    if (s.now) clockSkew = s.now - Date.now();
    selfStatus = s;
    fillSelf();
    $('#srcBadge').hidden = !s.source_warnings; $('#srcBadge').textContent = s.source_warnings;
  } catch (e) {}
}

let selfStatus = null;
function fillSelf() {
  const el = $('#self'), s = selfStatus;
  if (!el || !s) return;
  const live = s.records_per_sec > 0.2;
  const it = (k, v, attrs = '') => `<div class="it"><span class="k">${k}</span><span class="val" ${attrs}>${v}</span></div>`;
  el.innerHTML = `${streamHint(s.stream)}<div class="live ${live ? 'ok' : 'idle'}"><i></i>${t(live ? 'self.receiving' : 'self.idle')}</div>
    ${it(t('self.ingest'), esc(t('self.per_sec', {n: nf(s.records_per_sec, s.records_per_sec < 10 ? 1 : 0)})))}
    ${it(t('self.dropped'), nf(s.dropped), s.dropped ? 'style="color:var(--crit)"' : '')}
    ${it(t('self.counter_samples'), nf(s.counter_samples || 0))}
    ${it(t('self.dropped_counters'), nf(s.dropped_counters || 0))}
    ${it(t('self.write_errors'), nf(s.write_errors || 0))}
    ${it(t('self.disk'), fmtBytes(s.disk_bytes))}
    ${it(t('self.free'), fmtBytes(s.disk_free), `${s.disk_need > s.disk_free ? 'style="color:var(--crit)"' : ''} title="${esc(s.disk_need >= 0 ? t('self.need', {d: nf(s.retention_days), n: fmtBytes(s.disk_need)}) : t('self.need_later', {d: nf(s.retention_days)}))}"`)}
    ${s.write_errors ? it('!', nf(s.write_errors), `style="color:var(--crit)" title="${esc(s.last_error)}"`) : ''}`;
}

function showLogin() {
  $('#app').hidden = true; $('#login').hidden = false;
  $('#loginForm [name=user]').focus();
  fetch('/api/loginhint').then(r => r.json()).then(h => {
    const el = $('#loginHint');
    el.hidden = !h.default;
    if (h.default) {
      el.textContent = h.demo ? t('login.hint_demo', {u: h.user, p: h.password}) : t('login.hint');
      if (h.user && !$('#loginForm [name=user]').value) $('#loginForm [name=user]').value = h.user;
    }
  }).catch(() => {});
}
let me = null;
async function loadMe() {
  try { const r = await fetch('/api/me'); if (r.ok) me = await r.json(); } catch (e) {}
  $('#acctName').textContent = me?.user ? me.user : t('nav.account');
}
$('#loginForm').addEventListener('submit', async e => {
  e.preventDefault();
  const f = new FormData(e.target);
  const res = await fetch('/api/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({user: f.get('user'), password: f.get('password')})});
  if (!res.ok) { $('#loginErr').hidden = false; $('#loginErr').textContent = t('login.bad'); return; }
  const j = await res.json().catch(() => ({}));
  $('#login').hidden = true; $('#app').hidden = false;
  if (j.must_change) sessionStorage.setItem('t66.old', f.get('password'));
  await Promise.all([loadStatus(), loadMe()]);
  render();
  startTimers();
});
let timers = false;
function startTimers() {
  if (timers) return;
  timers = true;
  setInterval(loadStatus, 10000);
  setInterval(() => {
    if (document.hidden || ['TEXTAREA', 'INPUT', 'SELECT'].includes(document.activeElement?.tagName)) return;
    if (!LIVE_VIEWS.has(state.v)) return;
    const beat = REFRESH_MS[state.r];
    if (!beat || Date.now() - drawnAt < beat) return;  // a range in the past never moves
    render();
  }, 5000);
}
$('#logout').onclick = async () => { await fetch('/api/logout', {method: 'POST'}); me = null; showLogin(); };
$('#acct').onclick = () => go('account');

async function init() {
  $('#lang').innerHTML = LANGS.map(([c, n]) => `<option value="${c}">${n}</option>`).join('');
  const lang = pickLang();
  await loadLang(lang);
  $('#lang').value = lang;
  $('#theme').onclick = () => { if (window.t66Theme) t66Theme.next(); themeLabel(); render(); };
  $('#lang').onchange = async () => {
    await loadLang($('#lang').value);
    loadStatus(); render();
  };
  $('#range').onclick = e => {
    const b = e.target.closest('[data-r]'); if (!b) return;
    state.r = b.dataset.r; render();
  };
  $('#nav').onclick = e => {
    const b = e.target.closest('[data-v]');
    if (!b) return;
    go(b.dataset.v);
  };
  $('#share').onclick = () => { navigator.clipboard?.writeText(location.href).then(() => toast(t('top.copied')), () => toast(location.href)); };
  readHash();
  window.addEventListener('hashchange', () => { const before = location.hash; readHash(); if (before) render(); });
  window.addEventListener('popstate', () => { state.f = []; readHash(); render(); });
  const res = await fetch('/api/status');
  if (res.status === 401) { showLogin(); return; }
  $('#app').hidden = false;
  await Promise.all([loadStatus(), loadMe()]);
  render();
  startTimers();
}
init();
})();
