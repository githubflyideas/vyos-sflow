'use strict';
// Accessibility points that once regressed: icon buttons must keep a name
// when narrow layouts hide their text, the theme rings stay within the
// logo's height, navigation labels break between words, and the page has
// one banner and a main landmark on every screen.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const dir = path.join(__dirname, 'static');
const html = fs.readFileSync(path.join(dir, 'index.html'), 'utf8');
const css = fs.readFileSync(path.join(dir, 'app.css'), 'utf8');
const js = fs.readFileSync(path.join(dir, 'app.js'), 'utf8');

test('account and sign-out buttons are named whatever the width', () => {
  assert.match(html, /id="acct"[^>]*data-il="nav\.account"/);
  assert.match(html, /id="logout"[^>]*data-il="top\.logout"/);
  assert.match(js, /\[data-il\][^\n]*aria-label/);
});

test('theme rings are a named radio group never taller than the logo', () => {
  assert.match(html, /class="rings" id="themes" role="radiogroup"/);
  assert.match(js, /role="radio"[^`]*aria-checked=/);
  assert.match(js, /aria-label="\$\{esc\(t\('theme\.' \+ k\)\)\}"/);
  // the header logo is 65px tall (40px on phones): ring + lower-row drop
  // (sin 60deg x (ring + gap)) must stay within it
  const sets = [...css.matchAll(/\.rings\{--d:(\d+)px;--g:(\d+)px;/g)].map(m => [Number(m[1]), Number(m[2])]);
  assert.equal(sets.length, 2);
  sets.forEach(([d, g], i) => assert.ok(d + 0.8660254 * (d + g) <= [65, 40][i], `rings ${d}/${g}`));
  assert.match(html, /class="logo" src="\/logo\?wide=1"[^>]*height="65"/);
  assert.match(css, /@media \(max-width:620px\)\{[^\n]*header\.nms \.brand \.logo\{width:auto;height:40px\}/);
});

test('two-line navigation labels break only between words', () => {
  const dir = path.join(__dirname, 'static/i18n');
  // written without spaces: the allowed breaks, word by word
  const cjk = {zh: ['采集\n状态', '数据\n清理'], ja: ['フロー\nレコード', '収集の\n状態', 'データ\n削除']};
  for (const file of fs.readdirSync(dir).filter(f => f.endsWith('.json'))) {
    const code = file.slice(0, -5), cat = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    for (const k of ['overview', 'topn', 'records', 'sources', 'cleanup', 'help']) {
      const two = cat['nav2.' + k], one = cat['nav.' + k];
      const parts = two.split('\n');
      assert.ok(parts.length <= 2 && parts.every(p => p && p === p.trim()), `${code} ${k}`);
      if (parts.length === 1) { assert.equal(two, one, `${code} ${k}`); continue; }
      const ok = parts.join(' ') === one || (parts[0].endsWith('-') && parts.join('') === one) || (cjk[code] || []).includes(two);
      assert.ok(ok, `${code} ${k}: "${two}" is not "${one}" broken between words`);
    }
  }
  assert.equal(JSON.parse(fs.readFileSync(path.join(dir, 'zh.json'), 'utf8'))['nav2.records'], '流记录');
});

test('one banner; sign-in page is main content', () => {
  assert.equal((html.match(/<header\b/g) || []).length, 1);
  assert.match(html, /<main class="login" id="login"/);
});
