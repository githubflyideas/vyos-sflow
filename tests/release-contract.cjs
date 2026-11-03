const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {execFileSync} = require('node:child_process');
const root = path.resolve(__dirname, '..');
const files = [...new Set(execFileSync('git', ['ls-files', '-c', '-o', '--exclude-standard', '-z'], {cwd: root, encoding: 'utf8'}).split('\0').filter(f => f && fs.existsSync(path.join(root, f))))];
assert.deepEqual(files.filter(f => /\.(py|pyc|pyo|pyw)$/i.test(f)), [], 'obsolete interpreter files remain');
for (const f of files.filter(f => /^(packaging|scripts|\.github\/workflows)\//.test(f))) {
  if (!fs.statSync(path.join(root, f)).isFile()) continue;
  assert.doesNotMatch(fs.readFileSync(path.join(root, f), 'utf8'), /\bpython(?:2|3)?\b/i, `obsolete interpreter call in ${f}`);
}
const readme = fs.readFileSync(path.join(root, 'README.md'), 'utf8');
assert.deepEqual([...readme.matchAll(/^### (\d)\./gm)].map(m => m[1]), ['1', '2', '3']);
assert.match(readme, /^\.\/vyos-sflow serve$/m);
assert.match(readme, /\.\/vyso_sflow_data/);
assert.doesNotMatch(readme, /-agent\b|-sub-agent\b|v1\.0\.[01]/);
assert.match(readme, /v1\.1\.2/);
for (const f of ['README.md', ...fs.readdirSync(path.join(root, 'docs')).filter(f => f.endsWith('.md')).map(f => `docs/${f}`)]) {
  const text = fs.readFileSync(path.join(root, f), 'utf8');
  for (const m of text.matchAll(/\]\(([^)]+)\)/g)) {
    if (/^(?:https?:|#)/.test(m[1])) continue;
    assert.ok(fs.existsSync(path.resolve(root, path.dirname(f), m[1].split('#')[0])), `broken link in ${f}: ${m[1]}`);
  }
}
assert.ok(fs.existsSync(path.join(root, 'docs/images/vyos-sflow-logo.png')));
console.log('PASS release source layout, runnable quick start, logo and documentation links');
