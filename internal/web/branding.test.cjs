'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
test('embedded Web logo is the unchanged vyos-sflow project artwork', () => {
  const svg=fs.readFileSync(path.join(__dirname,'static/logo.svg'),'utf8');
  const image=svg.match(/href="data:image\/png;base64,([A-Za-z0-9+/=]+)"/);
  assert.ok(image,'Web logo must embed the approved project PNG');
  assert.deepEqual(Buffer.from(image[1],'base64'),fs.readFileSync(path.join(__dirname,'../../docs/images/vyos-sflow-logo.png')));
  assert.match(svg,/<title>vyos-sflow<\/title>/);
  assert.doesNotMatch(svg,/traffic66/i);
});
