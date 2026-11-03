// Offline boot-readiness regression; not a substitute for a systemd reboot.
const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),{spawn,spawnSync}=require('node:child_process');
const ROOT=path.resolve(__dirname,'..'),pos=p=>p.replaceAll('\\','/');
function ini(file){const result={};let section;for(const line of fs.readFileSync(path.join(ROOT,'packaging',file),'utf8').split(/\r?\n/)){const m=line.match(/^\[(.+)\]$/);if(m)section=result[m[1]]={};else if(section&&/^[A-Za-z]/.test(line)){const i=line.indexOf('=');if(i>=0)section[line.slice(0,i)]=line.slice(i+1);}}return result;}
function script(){const c=ini('vyos-sflow-ready.service').Service.ExecStart;assert(c.startsWith('/bin/bash -c \'')&&c.endsWith("'"));return c.slice(14,-1);}
function temp(t){const p=fs.mkdtempSync(path.join(ROOT,'.lifecycle-'));t.after(()=>fs.rmSync(p,{recursive:true,force:true}));return p;}
test('persistent paths ready before environment and sandbox',()=>{
 const service=ini('vyos-sflow.service'),ready=ini('vyos-sflow-ready.service');for(const key of ['Requires','After'])assert(service.Unit[key].split(/\s+/).includes('vyos-sflow-ready.service'));
 assert.equal(ready.Service.Type,'oneshot');assert.equal(ready.Service.Restart||'no','no');assert.equal(ready.Service.TimeoutStartSec,'310s');for(const key of ['EnvironmentFile','WorkingDirectory','RootDirectory'])assert(!(key in ready.Service));
 assert.equal(service.Service.User,'vyos-sflow');assert.equal(service.Service.Restart,'on-failure');assert.equal(service.Unit.StartLimitBurst,'3');assert.equal(service.Unit.StartLimitIntervalSec,'60');
});
test('readiness waits for delayed persistent files without evaluation',async t=>{
 const root=temp(t),product=path.join(root,'persistent'),code=script().replaceAll('/config/vyos-sflow',pos(product));
 const child=spawn('bash',['-c',code]);let stdout='',stderr='';child.stdout.on('data',x=>stdout+=x);child.stderr.on('data',x=>stderr+=x);
 const done=new Promise((resolve,reject)=>{child.on('error',reject);child.on('close',resolve);});const timer=setTimeout(()=>child.kill(),6000);
 try{await new Promise(r=>setTimeout(r,200));assert.equal(child.exitCode,null,'gate passed without persistent files');fs.mkdirSync(path.join(product,'bin'),{recursive:true});fs.mkdirSync(path.join(product,'data'));fs.writeFileSync(path.join(product,'service.env'),'EVIL=$(touch "'+pos(path.join(root,'executed'))+'")\n');await new Promise(r=>setTimeout(r,100));assert.equal(child.exitCode,null,'gate passed without executable');fs.writeFileSync(path.join(product,'bin/vyos-sflow'),'#!/bin/bash\nexit 97\n',{mode:0o755});assert.equal(await done,0,stderr);assert.equal(stdout,'');assert(!fs.existsSync(path.join(root,'executed')));}finally{clearTimeout(timer);if(child.exitCode===null)child.kill();await done;}
});
test('missing persistent files fail bounded without restart',t=>{const root=temp(t),p=path.join(root,'absent');const r=spawnSync('bash',['-c','sleep() { :; }; '+script().replaceAll('/config/vyos-sflow',pos(p))],{encoding:'utf8',timeout:5000});assert.equal(r.error,undefined);assert.notEqual(r.status,0);assert.match(r.stderr,/Persistent installation unavailable after 300 seconds/);assert(!fs.existsSync(p));});
