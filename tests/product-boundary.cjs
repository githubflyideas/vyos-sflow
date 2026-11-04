const {test}=require('node:test'), assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path');
const ROOT=path.resolve(__dirname,'..'),read=p=>fs.readFileSync(path.join(ROOT,p),'utf8');
test('removed routes and implementations',()=>{
 const server=read('internal/api/server.go');
 for(const name of ['sankey','threats','rings','geolines','report','ifaces','recon','inventory','iface','racks','findings','geo','logo'])assert.doesNotMatch(server,new RegExp('(?:api|adminAPI)\\("(?:GET|POST|DELETE) /api/'+name+'(?:["/])'));
 for(const pkg of ['detect','geo','snmp']){const p=path.join(ROOT,'internal',pkg);assert(!fs.existsSync(p)||!fs.readdirSync(p).some(n=>n.endsWith('.go')),pkg);}
 for(const n of ['report','racks','findings','geo','dbip','threatlists','rings'])assert(!fs.existsSync(path.join(ROOT,'internal/api',n+'.go')),n);
});
test('no advanced pipeline or counter storage',()=>{
 const pipeline=read('internal/pipeline/pipeline.go');
 for(const s of ['ASNDB','Threats','.Match(','.Lookup(','AppendCounter','chan []flow.IfCounters','Dim: "asn"','Dim: "country"','Dim: "segment"','noteObs('])assert(!pipeline.includes(s),s);
 const store=read('internal/store/store.go');for(const s of ['CREATE TABLE IF NOT EXISTS ifc','appIfc','findingSchema','reconSchema'])assert(!store.includes(s),s);
});
test('top and series dimensions',()=>{for(const [name,v,want] of [['query.go','dims',['app','client','host','proto','server']],['series.go','seriesDims',['app','host','proto']]]){const block=read('internal/store/'+name).split('var '+v+' = ')[1].split('\n}')[0];assert.deepEqual([...block.matchAll(/^\s*"([^"\n]+)":/gm)].map(m=>m[1]).sort(),want);}});
