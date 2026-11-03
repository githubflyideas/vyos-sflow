'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
let h = {};
try { h = require('./runtime-helpers.cjs'); } catch (e) { if (e.code !== 'MODULE_NOT_FOUND') throw e; }

test('readiness retries real HTTP starting responses until database is healthy', async () => {
  assert.equal(typeof h.waitHealth, 'function', 'missing bounded HTTP readiness');
  let calls = 0;
  const server = http.createServer((req, res) => {
    assert.equal(req.url, '/healthz');
    res.end(JSON.stringify({status: ++calls < 3 ? 'starting' : 'ok'}));
  });
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  try { await h.waitHealth(`http://127.0.0.1:${server.address().port}`, {timeout: 1000, interval: 5}); assert.equal(calls, 3); }
  finally { await new Promise(r => server.close(r)); }
});

test('permanent refusal and unhealthy HTTP both fail within a bounded deadline', async () => {
  assert.equal(typeof h.waitHealth, 'function');
  const server = http.createServer((req, res) => res.end('{"status":"starting"}'));
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const url = `http://127.0.0.1:${server.address().port}`;
  try { await assert.rejects(h.waitHealth(url, {timeout: 60, interval: 5}), /health.*ready/i); }
  finally { await new Promise(r => server.close(r)); }
  await assert.rejects(h.waitHealth(url, {timeout: 60, interval: 5}), /health.*ready/i);
});
