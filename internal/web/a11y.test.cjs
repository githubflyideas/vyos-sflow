'use strict';
// Accessibility points that once regressed: icon buttons must keep a name
// when narrow layouts hide their text, theme swatches meet the 24px target
// size, and the page has one banner and a main landmark on every screen.
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

test('theme swatches are at least 24px everywhere', () => {
  for (const m of css.matchAll(/\.themes button\{width:(\d+)px;height:(\d+)px/g)) {
    assert.ok(Number(m[1]) >= 24 && Number(m[2]) >= 24, m[0]);
  }
});

test('one banner; sign-in page is main content', () => {
  assert.equal((html.match(/<header\b/g) || []).length, 1);
  assert.match(html, /<main class="login" id="login"/);
});
