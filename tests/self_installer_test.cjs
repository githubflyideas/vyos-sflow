// Offline fixtures are inert, never product binaries. No external Node packages.
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'), path=require('node:path');
const {spawnSync}=require('node:child_process');
const {gzipSync,gunzipSync}=require('node:zlib');
const {createHash}=require('node:crypto');
const REPO=path.resolve(__dirname,'..'), ROOT='vyos-sflow-1.2.10-linux-amd64';
const pos=p=>p.replaceAll('\\','/');
function shell(args,opts={}) {return spawnSync('bash',args,{encoding:'utf8',cwd:REPO,...opts});}
function tar(files) {
 const chunks=[];
 for(const f of files){const h=Buffer.alloc(512), data=Buffer.from(f.data||'');
  h.write(f.name,0,100);h.write((f.mode||0o644).toString(8).padStart(7,'0')+'\0',100);
  h.write('0000000\0',108);h.write('0000000\0',116);h.write(data.length.toString(8).padStart(11,'0')+'\0',124);h.write('00000000000\0',136);h.fill(32,148,156);h.write(f.type||'0',156);h.write(f.link||'',157,100);h.write('ustar\0',257);h.write('00',263);
  h.write([...h].reduce((a,b)=>a+b,0).toString(8).padStart(6,'0')+'\0 ',148);
  chunks.push(h,data,Buffer.alloc((512-data.length%512)%512));
 }
 return gzipSync(Buffer.concat([...chunks,Buffer.alloc(1024)]));
}
function fixture(t){
 const dir=fs.mkdtempSync(path.join(REPO,'.self-test-'));t.after(()=>fs.rmSync(dir,{recursive:true,force:true}));
 const archive=path.join(dir,'release.tar.gz'), output=path.join(dir,'installer.run');
 const scratch=path.join(dir,'private temp'), commands=path.join(dir,'commands'), log=path.join(dir,'forbidden');fs.mkdirSync(scratch);fs.mkdirSync(commands);
 for(const cmd of ['systemctl','useradd','groupadd','passwd','chown','getent'])fs.writeFileSync(path.join(commands,cmd),'#!/bin/bash\nprintf forbidden >> "$FORBIDDEN_LOG"\nexit 99\n',{mode:0o755});
 const files=[{name:ROOT+'/vyos-sflow',data:'fixture-not-executable\n'},...fs.readdirSync(path.join(REPO,'packaging')).filter(n=>fs.statSync(path.join(REPO,'packaging',n)).isFile()).map(n=>({name:ROOT+'/packaging/'+n,data:fs.readFileSync(path.join(REPO,'packaging',n))}))];
 const write=(f=files)=>fs.writeFileSync(archive,tar(f));write();
 const build=()=>shell([pos(path.join(REPO,'scripts/build-self-installer.sh')),'--archive',pos(archive),'--output',pos(output)]);
 const run=(args=[],stdin='')=>{const r=shell(['-c','p=$1; if command -v cygpath >/dev/null; then p=$(cygpath -u "$p"); fi; export PATH="$p:$PATH"; shift; exec bash "$@"','test',pos(commands),pos(output),...args],{env:{...process.env,TMPDIR:pos(scratch),FORBIDDEN_LOG:pos(log)},input:stdin});assert(!fs.existsSync(log),'privileged action');assert.deepEqual(fs.readdirSync(scratch),[],'temporary extraction leaked');return r;};
 return {dir,archive,output,scratch,commands,files,write,build,run};
}
test('builder preserves exact archive bytes, digest; offline check/help/cancel',t=>{
 const f=fixture(t),r=f.build();assert.equal(r.status,0,r.stderr);
 const payload=fs.readFileSync(f.archive), out=fs.readFileSync(f.output);assert(out.subarray(-payload.length).equals(payload));assert(out.includes(Buffer.from(createHash('sha256').update(payload).digest('hex'))));
 for(const flag of ['--check','--help']){const r=f.run([flag]);assert.equal(r.status,0,r.stderr);}
 assert.match(f.run(['--check']).stdout,/verified/);
 const c=f.run([],'no\n');assert.equal(c.status,0,c.stderr);assert.match(c.stdout,/Cancelled/);
});
for(const [label,extra] of [
 ['traversal',{name:ROOT+'/../../escape'}],['absolute',{name:'/etc/escape'}],['dot',{name:ROOT+'/./escape'}],['empty component',{name:ROOT+'//escape'}],['space',{name:ROOT+'/bad name'}],
 ['symlink',{name:ROOT+'/link',type:'2',link:'/etc'}],['hardlink',{name:ROOT+'/link',type:'1',link:ROOT+'/vyos-sflow'}],['fifo',{name:ROOT+'/fifo',type:'6'}],['device',{name:ROOT+'/device',type:'3'}],['special mode',{name:ROOT+'/setuid',mode:0o4755}],['duplicate',{name:ROOT+'/vyos-sflow'}],['PAX',{name:ROOT+'/pax',type:'x',data:'override'}]
])test('builder rejects '+label,t=>{const f=fixture(t);f.write([...f.files,extra]);const r=f.build();assert.notEqual(r.status,0);assert(!fs.existsSync(f.output));});
test('builder requires every lifecycle file',t=>{const f=fixture(t);for(const name of ['vyos-sflow','packaging/install.sh','packaging/common.sh','packaging/uninstall.sh','packaging/service.env.example','packaging/vyos-sflow.service','packaging/setup.sh','packaging/vyos-sflow-ready.service']){f.write(f.files.filter(x=>x.name!==ROOT+'/'+name));const r=f.build();assert.notEqual(r.status,0);assert.match(r.stderr,/Incomplete release archive/);assert(!fs.existsSync(f.output));}});
test('corrupt payload fails without side effects',t=>{const f=fixture(t);assert.equal(f.build().status,0);const b=fs.readFileSync(f.output);b[b.length-12]^=1;fs.writeFileSync(f.output,b);const r=f.run(['--check']);assert.notEqual(r.status,0);assert.match(r.stderr,/checksum mismatch/);});
test('invalid gzip and same input/output fail closed',t=>{const f=fixture(t);fs.writeFileSync(f.archive,'not gzip');assert.notEqual(f.build().status,0);assert(!fs.existsSync(f.output));const r=shell([pos(path.join(REPO,'scripts/build-self-installer.sh')),'--archive',pos(f.archive),'--output',pos(f.archive)]);assert.notEqual(r.status,0);assert.equal(fs.readFileSync(f.archive,'utf8'),'not gzip');});
test('live installer rejects non-Linux',t=>{const f=fixture(t);assert.equal(f.build().status,0);fs.writeFileSync(path.join(f.commands,'uname'),'#!/bin/bash\necho Darwin\n',{mode:0o755});const r=f.run([],'yes\n');assert.notEqual(r.status,0);assert.match(r.stderr,/Linux x86_64/);});

test('real non-Linux host is rejected',t=>{
 if(shell(['-c','uname -s']).stdout.trim()==='Linux'){t.skip('real host is Linux; synthetic case runs separately');return;}
 const f=fixture(t);assert.equal(f.build().status,0);const r=f.run([],'yes\n');assert.notEqual(r.status,0);assert.match(r.stderr,/Linux x86_64/);
});
for(const [label,mutate] of [
 ['bad header checksum',b=>{b[0]^=1;return b;}],
 ['truncated tar',b=>b.subarray(0,b.length-513)],
 ['content after end marker',b=>Buffer.concat([b,gunzipSync(tar([{name:ROOT+'/late'}]))])],
])test('builder rejects '+label,t=>{const f=fixture(t);fs.writeFileSync(f.archive,gzipSync(mutate(gunzipSync(fs.readFileSync(f.archive)))));const r=f.build();assert.notEqual(r.status,0);assert(!fs.existsSync(f.output));});

// Frozen historical bytes: do not derive this from today's wizard/template.
const LEGACY_ENV='# systemd EnvironmentFile syntax, NOT a shell script. Never source this file.\nVYOS_SFLOW_ADDR=127.0.0.1:8066\nVYOS_SFLOW_LISTEN=127.0.0.1:6343\nVYOS_SFLOW_DATA=/config/vyos-sflow/data\nVYOS_SFLOW_RETENTION_DAYS=7\nVYOS_SFLOW_MEMORY_FRACTION=0.05\nVYOS_SFLOW_DB_THREADS=1\n# First login requires changing the default password; existing accounts are retained.\n# Do not persist an initial plaintext VYOS_SFLOW_PASSWORD here.\n';
function wizardFixture(t,{health='ok',start=true,active=true}={}){
 const f=fixture(t),base=path.join(f.dir,'config'),pkg=path.join(f.dir,'package'),systemd=path.join(f.dir,'systemd'),overrides=path.join(f.dir,'overrides'),trace=path.join(f.dir,'trace');
 for(const p of [base,pkg,systemd])fs.mkdirSync(p);
 const relocate=s=>s.replaceAll('/config/vyos-sflow',pos(base));
 const template=relocate(fs.readFileSync(path.join(REPO,'packaging/service.env.example'),'utf8'));
 fs.writeFileSync(path.join(pkg,'service.env.example'),template);
 fs.writeFileSync(path.join(pkg,'setup.sh'),relocate(fs.readFileSync(path.join(REPO,'packaging/setup.sh'),'utf8')).replaceAll('/run/systemd/system',pos(systemd)).replaceAll('/etc/systemd/system/vyos-sflow.service.d',pos(overrides)));
 // Only OS/service operations are substituted; the real wizard owns preflight,
 // candidate generation, health decisions and messages. Never execute product binaries.
 fs.writeFileSync(path.join(pkg,'install.sh'),'#!/bin/bash\necho install >> "$TRACE"\nwhile (($#)); do if [[ $1 == --environment ]]; then cp "$2" "$TEST_BASE/service.env"; exit; fi; shift; done\nexit 98\n');
 const scripts={uname:'if [[ $1 == -s ]]; then echo Linux; else echo x86_64; fi',id:'echo 0',sleep:':',
  systemctl:`echo "systemctl $*" >> "$TRACE"\nif [[ $1 == start ]]; then exit ${start?0:1}; fi\nif [[ $1 == is-active ]]; then exit ${active?0:1}; fi`,
  curl:'echo "curl $*" >> "$TRACE"\n'+(health==='transport-failure'?'exit 22':`printf '%s\\n' '${health==='ok'?'{"status":"ok","version":"1.2.10"}':health}'`)};
 for(const [name,body] of Object.entries(scripts))fs.writeFileSync(path.join(f.commands,name),'#!/bin/bash\n'+body+'\n',{mode:0o755});
 const run=(input='y\n')=>{
  const r=shell(['-c','p=$1; if command -v cygpath >/dev/null; then p=$(cygpath -u "$p"); fi; export PATH="$p:$PATH"; shift; exec bash "$@"','test',pos(f.commands),pos(path.join(pkg,'setup.sh'))],{input,env:{...process.env,TMPDIR:pos(f.scratch),FORBIDDEN_LOG:pos(path.join(f.dir,'forbidden')),TRACE:pos(trace),TEST_BASE:pos(base)}});
  assert(!fs.existsSync(path.join(f.dir,'forbidden')),'unexpected privileged action');assert.deepEqual(fs.readdirSync(f.scratch),[],'temporary candidate leaked');return r;
 };
 return {...f,base,pkg,overrides,trace,template,legacy:relocate(LEGACY_ENV),run};
}
function snapshot(dir){
 const files={};for(const n of fs.readdirSync(dir).sort()){const p=path.join(dir,n);files[n]=fs.statSync(p).isDirectory()?snapshot(p):fs.readFileSync(p).toString('base64');}return files;
}
function rejectUnchanged(f,input='y\n',pattern=/advanced/){
 const before=snapshot(f.base),r=f.run(input);assert.notEqual(r.status,0,r.stdout);assert.match(r.stderr,pattern);assert(!fs.existsSync(f.trace),'preflight invoked install or service commands');assert.deepEqual(snapshot(f.base),before,'existing configuration, credentials or history changed');assert.doesNotMatch(r.stdout,/tryvyossflow66/);return r;
}
test('wizard confirmation alone installs, starts, verifies health and refuses subsequent existing data',t=>{
 const f=wizardFixture(t),r=f.run();assert.equal(r.status,0,r.stderr);assert.match(r.stdout,/tryvyossflow66/);assert.match(r.stdout,/http:\/\/ROUTER_IP:8066/);assert.match(r.stdout,/private interface/);assert.match(r.stdout,/loopback \/healthz verified/);
 assert.equal(fs.readFileSync(path.join(f.base,'service.env'),'utf8'),f.template);
 const actions=fs.readFileSync(f.trace,'utf8');assert(actions.indexOf('install\n')<actions.indexOf('systemctl start'));assert.match(actions,/http:\/\/127.0.0.1:8066\/healthz/);
 fs.mkdirSync(path.join(f.base,'data'));fs.writeFileSync(path.join(f.base,'data','accounts.db'),Buffer.from([0,1,2,255]));fs.writeFileSync(path.join(f.base,'data','history.parquet'),'existing history');
 fs.unlinkSync(f.trace);rejectUnchanged(f);
});
for(const mode of ['history','hidden credentials','custom environment','modified legacy','custom override','service drop-in'])test('wizard preserves '+mode+' and refuses before install',t=>{
 const f=wizardFixture(t);
 if(mode==='history'||mode==='hidden credentials'){
  fs.mkdirSync(path.join(f.base,'data'));fs.writeFileSync(path.join(f.base,'data',mode==='history'?'history.parquet':'.accounts'),'existing history/password hash\0');
  fs.mkdirSync(path.join(f.base,'bin'));fs.writeFileSync(path.join(f.base,'bin','vyos-sflow'),'old binary');
 }else if(mode==='custom environment'){
  fs.writeFileSync(path.join(f.base,'service.env'),'VYOS_SFLOW_ADDR=0.0.0.0:9999\nVYOS_SFLOW_PASSWORD=keep-this-password\nEVIL=$(touch "$FORBIDDEN_LOG")\n');
 }else if(mode==='modified legacy')fs.writeFileSync(path.join(f.base,'service.env'),f.legacy+'# edited\n');
 else if(mode==='custom override')fs.writeFileSync(path.join(f.base,'systemd-override.conf'),'[Service]\nEnvironment=VYOS_SFLOW_PASSWORD=existing\n');
 else {fs.mkdirSync(f.overrides);fs.writeFileSync(path.join(f.overrides,'custom.conf'),'unchanged override');}
 rejectUnchanged(f);
 if(mode==='service drop-in')assert.equal(fs.readFileSync(path.join(f.overrides,'custom.conf'),'utf8'),'unchanged override');
});
test('wizard empty legacy layout preserves listeners without identity prompts or retention opt-in',t=>{
 const f=wizardFixture(t);fs.writeFileSync(path.join(f.base,'service.env'),f.legacy);
 const r=f.run();assert.equal(r.status,0,r.stderr);assert.match(r.stdout,/retained loopback/);assert.match(r.stdout,/password not reset/);assert.doesNotMatch(r.stdout,/tryvyossflow66/);
 assert.equal(fs.readFileSync(path.join(f.base,'service.env'),'utf8'),f.legacy.replace('VYOS_SFLOW_RETENTION_DAYS=7\n',''));
});
test('wizard exact current template retains credentials messaging',t=>{
 const f=wizardFixture(t);fs.writeFileSync(path.join(f.base,'service.env'),f.template);const r=f.run();assert.equal(r.status,0,r.stderr);assert.match(r.stdout,/Use your existing credentials/);assert.doesNotMatch(r.stdout,/tryvyossflow66/);assert.equal(fs.readFileSync(path.join(f.base,'service.env'),'utf8'),f.template);
});
for(const input of ['no\n','\n',''])test('wizard cancel or missing confirmation is a no-op '+JSON.stringify(input),t=>{
 const f=wizardFixture(t);fs.writeFileSync(path.join(f.base,'service.env'),'private existing configuration');const before=snapshot(f.base),r=f.run(input);assert.equal(r.status,input===''?1:0,r.stderr);assert.match(r.stdout+r.stderr,input===''?/Confirmation required/:/Cancelled/);assert(!fs.existsSync(f.trace));assert.deepEqual(snapshot(f.base),before);
});
for(const health of ['transport-failure','{"status":"failed"}','{"status":"okay"}','not json'])test('wizard does not claim success after health failure '+health,t=>{
 const f=wizardFixture(t,{health}),r=f.run();assert.notEqual(r.status,0);assert.match(r.stderr,/NOT verified/);assert.doesNotMatch(r.stdout,/loopback \/healthz verified|tryvyossflow66/);assert.equal(fs.readFileSync(path.join(f.base,'service.env'),'utf8'),f.template);
 const actions=fs.readFileSync(f.trace,'utf8');assert.match(actions,/systemctl start/);assert.equal(actions.split('\n').filter(s=>s.startsWith('curl ')).length,30);
});
for(const [label,options] of [['start failure',{start:false}],['inactive service',{active:false}]])test('wizard rejects '+label,t=>{
 const f=wizardFixture(t,options),r=f.run();assert.notEqual(r.status,0);assert.match(r.stderr,label==='start failure'?/Service start failed/:/NOT verified/);assert.doesNotMatch(r.stdout,/loopback \/healthz verified|tryvyossflow66/);assert.doesNotMatch(fs.readFileSync(f.trace,'utf8'),/^curl /m);
});
test('defaults accept all senders without automatic destructive retention consent',()=>{
 const env=fs.readFileSync(path.join(REPO,'packaging/service.env.example'),'utf8');assert.match(env,/VYOS_SFLOW_ADDR=0\.0\.0\.0:8066\r?\n/);assert.match(env,/VYOS_SFLOW_LISTEN=0\.0\.0\.0:6343\r?\n/);assert.doesNotMatch(env,/^VYOS_SFLOW_(?:AGENT|SUB_AGENT|RETENTION_DAYS)=/m);assert.doesNotMatch(env,/^VYOS_SFLOW_ACCEPT_48H_RETENTION=true/m);
 assert.doesNotMatch(env,/Initialize admin with the passwd/);assert.doesNotMatch(fs.readFileSync(path.join(REPO,'packaging/install.sh'),'utf8'),/initialize admin with passwd/);
});
test('self-installer migration leaves no Python test or builder',()=>{
 assert(!fs.existsSync(path.join(REPO,'tests/self_installer_test.py')));
 assert(!fs.existsSync(path.join(REPO,'scripts/build-self-installer.py')));
});
test('restore is neither shipped nor required',()=>{
 assert(!fs.existsSync(path.join(REPO,'packaging/restore.sh')));assert.doesNotMatch(fs.readFileSync(path.join(REPO,'packaging/install.sh'),'utf8'),/install\.sh restore\.sh/);assert.doesNotMatch(fs.readFileSync(path.join(REPO,'scripts/build-self-installer.sh'),'utf8'),/packaging\/restore\.sh/);
});
