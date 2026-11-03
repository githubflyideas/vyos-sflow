#!/usr/bin/env node
'use strict';
// Real Linux PTY + candidate binary acceptance. No simulated product process.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const dgram = require('node:dgram');
const {spawnSync} = require('node:child_process');
const {ExpectBridge} = require('./vyos_vm.cjs');
const h = require('./runtime-helpers.cjs');

function alive(pid) { try { process.kill(pid, 0); return true; } catch { return false; } }
function sockets(pid, kind) {
  const inodes = new Set(fs.readdirSync(`/proc/${pid}/fd`).flatMap(name => {
    try { const m = fs.readlinkSync(`/proc/${pid}/fd/${name}`).match(/^socket:\[(\d+)\]$/); return m ? [m[1]] : []; } catch { return []; }
  }));
  return fs.readFileSync(`/proc/${pid}/net/${kind}`, 'utf8').trim().split('\n').slice(1).flatMap(line => {
    const f = line.trim().split(/\s+/);
    if (!inodes.has(f[9]) || (kind === 'tcp' && f[3] !== '0A')) return [];
    const [address, port] = f[1].split(':'); return [{address, port: parseInt(port, 16)}];
  });
}
function tokenFiles(dir) {
  return fs.readdirSync(dir, {withFileTypes:true}).flatMap(e => e.isDirectory() ? tokenFiles(path.join(dir,e.name)) : e.name === '.tui-token' ? [path.join(dir,e.name)] : []);
}
const wrapper = 'cd "$1" || exit 90; shift; before=$(stty -g); bash -c \'printf "%s" "$$" > "$1"; shift; exec "$@"\' _ "$@"; rc=$?; after=$(stty -g); if [ "$before" = "$after" ]; then printf "\\n__TTY_RESTORED__\\n"; else printf "\\n__TTY_CHANGED__\\n"; fi; printf "__TUI_EXIT_%s__\\n" "$rc"';
async function launch(root, binary, args = []) {
  const cwd = path.join(root,'cwd'), tmp = path.join(root,'tmp'), pidfile = path.join(root,'pid');
  fs.mkdirSync(cwd,{recursive:true}); fs.mkdirSync(tmp,{recursive:true});
  const env = {...h.cleanEnv(), TMPDIR:tmp, TERM:'xterm', LANG:'en_US.UTF-8'};
  const bridge = new ExpectBridge(['bash','-c',wrapper,'_',cwd,pidfile,binary,'tui',...args],env);
  await h.waitFor(() => fs.existsSync(pidfile),'PTY child did not start',{timeout:5000});
  return {bridge,pid:Number(fs.readFileSync(pidfile,'utf8')),cwd,tmp};
}
async function stopped(session, expected = 0) {
  const result = await session.bridge.expect('__TUI_EXIT_([0-9]+)__',20);
  assert.equal(Number(result.match.match(/__TUI_EXIT_([0-9]+)__/)[1]),expected,'TUI returned unexpected exit status');
  assert.match(result.before,/__TTY_RESTORED__/,'TUI did not restore terminal state');
  await h.waitFor(() => !alive(session.pid),'TUI process remained after exit',{timeout:5000});
  assert.deepEqual(fs.readdirSync(session.tmp),[],'Owned temporary data remained after exit');
}
async function standalone(binary, ending) {
  await h.withTemp('vyos-tui-pty-',async root => {
    const cwd=path.join(root,'cwd'); fs.mkdirSync(path.join(cwd,'vyso_sflow_data'),{recursive:true});
    const sentinel=path.join(cwd,'vyso_sflow_data','keep-existing-history'); fs.writeFileSync(sentinel,'untouched');
    const s=await launch(root,binary);
    try {
      await s.bridge.expect('\\x1b\\[\\?1049h',30);
      const tcp=sockets(s.pid,'tcp'), udp=sockets(s.pid,'udp');
      assert.equal(tcp.length,1); assert.equal(tcp[0].address,'0100007F','Temporary API exposed outside loopback');
      assert.ok(udp.some(x=>x.address==='00000000'&&x.port===6343),'Temporary collector not listening on default UDP');
      const files=tokenFiles(s.tmp); assert.equal(files.length,1);
      const token=fs.readFileSync(files[0],'utf8').trim(), base=`http://127.0.0.1:${tcp[0].port}`;
      await h.request(base+'/api/overview?range=1h',{expected:401});
      await h.send([h.sflow(1)],6343);
      await h.waitFor(async()=> (await h.request(base+'/api/overview?range=1h',{auth:'Bearer '+token})).totals.wire===7800,'Temporary TUI did not collect real sFlow',{timeout:10000});
      if (ending==='q') await s.bridge.sendline('q');
      else if (ending==='ctrl-c') await s.bridge.sendline('\x03');
      else process.kill(s.pid,ending);
      await stopped(s);
      assert.equal(fs.readFileSync(sentinel,'utf8'),'untouched');
      const probe=dgram.createSocket('udp4');
      try { await new Promise((resolve,reject)=>{probe.once('error',reject);probe.bind(6343,'0.0.0.0',resolve);}); }
      finally { probe.close(); }
    } finally { if(alive(s.pid)) process.kill(s.pid,'SIGKILL'); await s.bridge.close(); }
  });
  console.log('PASS temporary TUI real sFlow, private API, terminal/data/socket cleanup: '+ending);
}
async function existing(binary) {
  await h.withTemp('vyos-tui-existing-',async root=>{
    const cwd=path.join(root,'cwd'),tmp=path.join(root,'tmp');fs.mkdirSync(cwd);fs.mkdirSync(tmp);
    const data=path.join(cwd,'vyso_sflow_data'), password=h.password();
    const service=new h.Service(binary,['serve','-data',data,'-addr','127.0.0.1:8066','-listen','127.0.0.1:6343'],{...h.cleanEnv(),VYOS_SFLOW_PASSWORD:password},path.join(root,'server.log'));
    let s;
    try {
      service.start(); await h.waitHealth('http://127.0.0.1:8066');
      await h.send([h.sflow(1)],6343);
      const get=()=>h.request('http://127.0.0.1:8066/api/overview?range=1h',{auth:h.basic(password)});
      await h.waitFor(async()=>(await get()).totals.wire===7800,'Existing service did not persist fixture');
      const token=fs.readFileSync(path.join(data,'.tui-token'));
      s=await launch(root,binary);await s.bridge.expect('\\x1b\\[\\?1049h',20);await s.bridge.sendline('q');await stopped(s);
      assert.ok(service.alive(),'TUI stopped an existing service');assert.equal((await get()).totals.wire,7800);
      assert.deepEqual(fs.readFileSync(path.join(data,'.tui-token')),token,'TUI replaced existing credentials');
    } finally { if(s){if(alive(s.pid))process.kill(s.pid,'SIGKILL');await s.bridge.close();}await service.cleanup(); }
  });
  console.log('PASS attach and quit preserves existing service, history and token');
}
async function failures(binary) {
  await h.withTemp('vyos-tui-errors-',async root=>{
    const tmp=path.join(root,'tmp');fs.mkdirSync(tmp);
    const r=spawnSync(binary,['tui'],{cwd:root,env:{...h.cleanEnv(),TMPDIR:tmp},encoding:'utf8',timeout:10000});
    assert.notEqual(r.status,0);assert.match(r.stderr,/interactive terminal/);assert.deepEqual(fs.readdirSync(tmp),[]);
  });
  for(const mode of ['explicit-url','udp-busy']) await h.withTemp('vyos-tui-errors-',async root=>{
    let socket;
    if(mode==='udp-busy') {socket=dgram.createSocket('udp4');await new Promise((resolve,reject)=>{socket.once('error',reject);socket.bind(6343,'0.0.0.0',resolve);});}
    const args=mode==='explicit-url'?['-url',`http://127.0.0.1:${await h.port()}`]:[];
    let s;
    try {s=await launch(root,binary,args);await stopped(s,1);}
    finally {if(s){if(alive(s.pid))process.kill(s.pid,'SIGKILL');await s.bridge.close();}if(socket)socket.close();}
  });
  console.log('PASS non-TTY, explicit unreachable URL and busy UDP fail without leaked state');
}
async function main(){
  assert.equal(process.platform,'linux','Real TUI acceptance requires Linux');
  const binary=h.binaryArg(process.argv.slice(2));
  assert.equal(spawnSync('expect',['-v']).status,0,'Expect is required');
  for(const ending of ['q','ctrl-c','SIGTERM','SIGHUP']) await standalone(binary,ending);
  await existing(binary);await failures(binary);
}
if(require.main===module) h.cli(main);
