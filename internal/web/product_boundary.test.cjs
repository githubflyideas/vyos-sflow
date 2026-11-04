const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.join(__dirname, 'static');
test('only basic product views, dimensions and assets ship', () => {
 const js = fs.readFileSync(path.join(root,'app.js'),'utf8');
 const html = fs.readFileSync(path.join(root,'index.html'),'utf8');
 assert.deepEqual([...html.matchAll(/id="v-([^"]+)"/g)].map(m=>m[1]).sort(), ['account','cleanup','help','overview','records','sources','topn']);
 assert.doesNotMatch(js, /api\(['"](?:ifaces|findings|geo|rings|sankey|threats|recon)|\/api\/(?:resolve|embeds|syslog|report)|views\.(?:detail|names|racks)/);
 assert.match(js, /const DIMS = \['host', 'app', 'proto'\]/);
 assert.doesNotMatch(js, /7d|30d|datetime-local|state\.r = 'custom'/);
 assert.match(js, /const RANGES = \['15m', '1h', '6h', '24h', '48h'\]/);
 assert.match(js, /'48h': 1728e5/);
 for (const f of ['world.json','embed.js','embed.css']) assert.equal(fs.existsSync(path.join(root,f)),false,f);
 for (const f of fs.readdirSync(path.join(root,'i18n'))) {
   const d = JSON.parse(fs.readFileSync(path.join(root,'i18n',f),'utf8'));
   assert.equal(Object.keys(d).some(k => /^(fd|sankey|rg|sev|cause)\./.test(k)),false,f);
   assert.doesNotMatch(d['tui.h.pages'], /Interfaces|Search/);
   assert.doesNotMatch(d['empty.first'], /NetFlow|IPFIX|2055|4739/,f);
   assert.equal('range.7d' in d,false,f);
   assert.equal('cl.days' in d,false,f);
   assert.equal('cl.confirm_days' in d,false,f);
 }
});
