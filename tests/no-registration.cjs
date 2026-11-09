const {test}=require('node:test'),assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
const ROOT=path.resolve(__dirname,'..');
test('product dependency graph excludes runtime registration',()=>{
 const r=spawnSync(process.env.GO||'go',['list','-deps','./cmd/vyos-sflow'],{cwd:ROOT,env:{...process.env,GOOS:'linux',GOARCH:'amd64',CGO_ENABLED:'1',GOTOOLCHAIN:'local'},encoding:'utf8'});
 assert.equal(r.status,0,r.stderr||String(r.error));const deps=new Set(r.stdout.trim().split(/\r?\n/));
 for(const p of ['api','auth'])assert(deps.has('github.com/githubflyideas/vyos-sflow/internal/'+p));
 assert(!deps.has('github.com/githubflyideas/vyos-sflow/internal/license'));
});
test('runtime has no registration machinery',()=>{
 assert(!fs.existsSync(path.join(ROOT,'internal/license')));
 const forbidden=/internal\/license|license\.Open|\bLicense\s+\*|\/api\/license|not_registered|user_limit|X-Traffic66-Evaluation|s\.(?:licenseState|userLimit|notice|markHeader|allowed)\(/;
 function walk(dir){for(const e of fs.readdirSync(dir,{withFileTypes:true})){const p=path.join(dir,e.name);if(e.isDirectory())walk(p);else if(p.endsWith('.go')&&!p.endsWith('_test.go'))assert.doesNotMatch(fs.readFileSync(p,'utf8'),forbidden,p);}}
 for(const d of ['cmd','internal'])walk(path.join(ROOT,d));
});
