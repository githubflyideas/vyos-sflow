'use strict';
// Capture real release UI, using synthetic sFlow datagrams through the real collector.
// No API mocks, database inserts, DOM edits, or generated terminal frames.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {chromium} = require('playwright');
const h = require('./runtime-helpers.cjs');

async function main() {
  assert.equal(process.platform, 'linux');
  const binary = h.binaryArg([process.argv[2]]);
  const output = path.resolve(process.argv[3] || 'evidence/readme-screenshots');
  fs.mkdirSync(output, {recursive:true});
  const version = h.run(binary, ['version']).trim();
  assert.match(version, /1\.1\.1/);
  const password = h.password();
  const webport = await h.port(), udpport = await h.port('udp');
  const base = `http://127.0.0.1:${webport}`;
  const env = {...h.cleanEnv(), VYOS_SFLOW_PASSWORD:password, LANG:'C.UTF-8'};
  await h.withTemp('vyos-readme-', async root => {
    const data = path.join(root, 'data');
    const service = new h.Service(binary, ['serve','-data',data,'-addr',`127.0.0.1:${webport}`,'-listen',`127.0.0.1:${udpport}`], env, path.join(root,'service.log'));
    let browser, terminal, sampling = true, producer, sequence = 0, sent = 0;
    try {
      service.start(); await h.waitHealth(base);
      producer = (async () => {
        while (sampling) {
          const batch = [];
          // Several actual elapsed-minute buckets, without backdating records.
          const count = 8 + Math.floor((Math.sin(Date.now()/17000)+1)*8);
          for (let i=0; i<count; i++) {
            const packet = h.sflow(++sequence, '192.0.2.1', 0, 40000+i);
            const frame = packet.length-44;
            // Documentation-only IPv4 ranges, distinct real decoded endpoints.
            Buffer.from([192,0,2,10+i%8]).copy(packet,frame+26);
            Buffer.from([198,51,100,20+i%4]).copy(packet,frame+30);
            packet.writeUInt16BE([443,53,123,8080][i%4],frame+36);
            batch.push(packet);
          }
          await h.send(batch,udpport); sent += batch.length;
          await h.sleep(750);
        }
      })();
      await h.sleep(150000);
      await h.waitFor(async () => (await h.request(base+'/api/overview?range=15m',{auth:h.basic(password)})).totals.wire>0,'No real captured data');
      browser = await chromium.launch({headless:true});
      const page = await browser.newPage({viewport:{width:1360,height:820}});
      const errors=[]; page.on('pageerror', e=>errors.push(e.message));
      await page.goto(base);
      await page.locator('#loginForm input[name=user]').fill('admin');
      await page.locator('#loginForm input[name=password]').fill(password);
      await page.locator('#loginForm button').click();
      await page.locator('#app').waitFor({state:'visible'});
      await page.locator('#range [data-r="15m"]').click();
      await page.locator('#v-overview .panel').first().waitFor({state:'visible'});
      await page.waitForTimeout(1500);
      await page.screenshot({path:path.join(output,'web-overview.png')});
      await page.locator('#nav [data-v=topn]').click();
      await page.waitForFunction(()=>document.querySelectorAll('#v-topn tbody tr').length>=8);
      await page.screenshot({path:path.join(output,'web-top.png')});
      assert.deepEqual(errors,[]);

      // A real X11 xterm process with a real PTY; screenshot its native window.
      terminal = spawn('xterm',['-geometry','128x36','-fa','DejaVu Sans Mono','-fs','12','-bg','#10151c','-fg','#dce4ee','-title','vyos-sflow v1.1.1','-e',binary,'tui','-url',base,'-data',data],{env:{...h.cleanEnv(),LANG:'C.UTF-8'},stdio:'ignore'});
      let window;
      await h.waitFor(()=>{
        window=h.run('xdotool',['search','--onlyvisible','--pid',String(terminal.pid)]).trim().split('\n')[0];
        return /^\d+$/.test(window);
      },'Native terminal window unavailable',{timeout:15000});
      await h.sleep(4000);
      assert.equal(terminal.exitCode,null);
      h.run('import',['-window',window,path.join(output,'tui-overview.png')]);
      h.run('xdotool',['windowfocus','--sync',window,'key','q']);
      await h.waitFor(()=>terminal.exitCode!==null,'TUI did not quit',{timeout:10000});
      assert.equal(terminal.exitCode,0);
      sampling=false; await producer;
      await h.waitFor(async () => (await h.request(base+'/api/overview?range=15m',{auth:h.basic(password)})).totals.wire===sent*7800,'Final screenshot fixture totals mismatch');
      const overview=await h.request(base+'/api/overview?range=15m',{auth:h.basic(password)});
      assert.equal(overview.totals.wire,sent*7800);
      fs.writeFileSync(path.join(output,'manifest.json'),JSON.stringify({release:'v1.1.1',version,binarySHA256:h.hash(binary),data:'Synthetic documentation-range sFlow sent by UDP; real collector, storage and API; actual elapsed timestamps.',terminal:'Native X11 xterm window and PTY, attached to same real service.',screenshots:['web-overview.png','web-top.png','tui-overview.png'].map(name=>({name,sha256:h.hash(path.join(output,name))})),packetsSent:sent,wireBytes:overview.totals.wire,capturedAt:new Date().toISOString()},null,2)+'\n');
      console.log('PASS real WebUI and native TUI screenshot capture; byte totals verified');
    } finally {
      sampling=false; if(producer) await producer.catch(()=>{});
      if(terminal && terminal.exitCode===null) terminal.kill('SIGTERM');
      if(browser) await browser.close();
      await service.cleanup();
    }
  });
}
h.cli(main);
