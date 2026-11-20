'use strict';
// Accessibility points that once regressed: icon buttons must keep a name
// when narrow layouts hide their text, the theme buttons sit in the page
// toolbar as a named radio group, navigation labels take one line, and
// the page has one banner and a main landmark on every screen.
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

test('theme buttons are a named radio group in the page toolbar', () => {
  assert.match(html, /class="tdots" id="themes" role="radiogroup"/);
  // the picker lives in the page toolbar, not the header
  assert.ok(html.indexOf('id="themes"') > html.indexOf('class="topbar"'));
  assert.match(js, /role="radio"[^`]*aria-checked=/);
  assert.match(js, /aria-label="\$\{esc\(t\('theme\.' \+ k\)\)\}"/);
  // round buttons, filled with the theme colour
  assert.match(css, /\.tdot\{[^}]*border-radius:50%/);
  assert.match(css, /\.tdot\{[^}]*background:var\(--c\)/);
});

test('navigation labels are the full name on one line', () => {
  const dir = path.join(__dirname, 'static/i18n');
  for (const file of fs.readdirSync(dir).filter(f => f.endsWith('.json'))) {
    const code = file.slice(0, -5), cat = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    for (const k of ['overview', 'topn', 'records', 'sources', 'cleanup', 'help']) {
      const one = cat['nav.' + k];
      assert.ok(typeof one === 'string' && one.length > 0 && !one.includes('\n'), `${code} ${k}`);
    }
  }
  assert.equal(JSON.parse(fs.readFileSync(path.join(dir, 'zh.json'), 'utf8'))['nav.records'], '流记录');
  // the renderer writes the full name as plain text, on one line
  assert.match(js, /textContent = t\('nav\.' \+ el\.dataset\.n\)/);
  assert.match(css, /\.nl\{[^}]*white-space:nowrap/);
});

test('one banner; sign-in page is main content', () => {
  assert.equal((html.match(/<header\b/g) || []).length, 1);
  assert.match(html, /<main class="login" id="login"/);
});
