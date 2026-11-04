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
const changelog = fs.readFileSync(path.join(root, 'CHANGELOG.md'), 'utf8');
const release = changelog.match(/^## (\d+\.\d+\.\d+)\s*$/m)?.[1];
assert.ok(release, 'newest changelog version is missing');
const archiveRoot = `vyos-sflow-${release}-linux-amd64`;
const quickStart = readme.split('### 2. Download and run')[1]?.match(/```sh\r?\n([\s\S]*?)```/)?.[1];
assert.ok(quickStart, 'beginner download block is missing');
function verifyDownloads(text, artifact) {
  const downloads = [...text.matchAll(/https:\/\/github\.com\/githubflyideas\/vyos-sflow(?:-new)?\/releases\/download\/v([^/\s]+)\/([^\s]+)/g)];
  assert.equal(downloads.length, 2, 'expected explicit artifact and checksum downloads');
  for (const [, tag] of downloads) assert.equal(tag, release, 'download version differs from newest changelog');
  assert.deepEqual(downloads.map(m => m[2]), [artifact, 'SHA256SUMS']);
  assert.match(text, /sha256sum --ignore-missing -c SHA256SUMS/);
}
verifyDownloads(quickStart, `${archiveRoot}.tar.gz`);
assert.ok(quickStart.includes(`tar -xzf ${archiveRoot}.tar.gz &&`), 'extraction filename differs from download');
assert.ok(quickStart.includes(`cd ${archiveRoot} &&`), 'working directory differs from archive root');
assert.match(quickStart, /\.\/vyos-sflow serve\s*$/);
const installGuide = fs.readFileSync(path.join(root, 'docs/VYOS.md'), 'utf8');
const installBlock = installGuide.match(/```(?:sh|bash)\r?\n([\s\S]*?)```/)?.[1];
assert.ok(installBlock, 'service installation block is missing');
verifyDownloads(installBlock, `${archiveRoot}.run`);
assert.ok(installBlock.includes(`sudo bash ${archiveRoot}.run`), 'installer invocation differs from download');
const startup = fs.readFileSync(path.join(root, 'cmd/vyos-sflow/startup.go'), 'utf8');
assert.equal(startup.match(/var version = "([^"]+)"/)?.[1], release, 'default binary version differs from changelog');
const builder = fs.readFileSync(path.join(root, 'scripts/build-self-installer.sh'), 'utf8');
assert.ok(builder.includes(`ROOT=${archiveRoot}`), 'self-installer payload root differs from release');
for (const file of ['packaging/setup.sh', 'packaging/install.sh', 'packaging/vyos-sflow.service', '.github/workflows/ci.yml', '.github/workflows/released-installer-vyos.yml', '.github/workflows/readme-screenshots.yml']) {
  const text = fs.readFileSync(path.join(root, file), 'utf8');
  assert.ok(text.includes(release), `active release version missing from ${file}`);
  for (const match of text.matchAll(/vyos-sflow(?:-| v?)(\d+\.\d+\.\d+)/g)) {
    assert.equal(match[1], release, `stale package version in ${file}`);
  }
}
for (const f of ['README.md', ...fs.readdirSync(path.join(root, 'docs')).filter(f => f.endsWith('.md')).map(f => `docs/${f}`)]) {
  const text = fs.readFileSync(path.join(root, f), 'utf8');
  for (const m of text.matchAll(/\]\(([^)]+)\)/g)) {
    if (/^(?:https?:|#)/.test(m[1])) continue;
    assert.ok(fs.existsSync(path.resolve(root, path.dirname(f), m[1].split('#')[0])), `broken link in ${f}: ${m[1]}`);
  }
}
assert.ok(fs.existsSync(path.join(root, 'docs/images/vyos-sflow-logo.png')));
console.log('PASS release source layout, runnable quick start, logo and documentation links');
