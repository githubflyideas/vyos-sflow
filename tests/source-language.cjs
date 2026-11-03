#!/usr/bin/env node
// Review gate for literal Han text, not a language detector. Keep legal/data names.
const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),{execFileSync}=require('node:child_process');
const ROOT=path.resolve(__dirname,'..'),HAN=/[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff\u{20000}-\u{323af}]/u;
function production(p){return /^(internal|cmd)\//.test(p)&&!p.includes('/static/i18n/')&&!p.includes('/testdata/')&&!/(_test\.go|\.test\.js)$/.test(p)&&/\.(go|js|html|css|sh)$/.test(p);}
function findings(p,text){if(p==='internal/web/static/app.js')text=text.replaceAll("['zh','中文']","['zh','Chinese']").replaceAll("['ja','日本語']","['ja','Japanese']");return text.split(/\r?\n/).map((line,i)=>[i+1,line]).filter(([,line])=>HAN.test(line));}
if(process.argv.includes('--self-test')){
 assert(findings('internal/x.go','// 中文').length);assert(findings('cmd/x.go','println("错误")').length);
 const p='internal/web/static/app.js';assert.equal(findings(p,"['zh','中文'],['ja','日本語']").length,0);assert(findings(p,"['zh','中文'] // 错误").length);assert(findings('internal/other.js',"['zh','中文']").length);
 for(const p of ['internal/web/static/i18n/zh.json','internal/web/static/i18n/ja.json','internal/geo/dbip/country.mmdb.gz','internal/license/vectors_test.go','internal/tui/width_test.go','LICENSE','ADDITIONAL_TERMS.md'])assert(!production(p),p);
 assert(production('internal/settings/settings.go'));assert(production('internal/web/static/app.js'));console.log('PASS: source-language contract self-tests');
}else{
 assert.equal(process.argv.length,2,'Usage: node tests/source-language.cjs [--self-test]');
 const files=execFileSync('git',['ls-files','-z','--','cmd','internal'],{cwd:ROOT,encoding:'utf8'}).split('\0').filter(production), errors=[];
 for(const p of files)for(const [n,line] of findings(p,fs.readFileSync(path.join(ROOT,p),'utf8')))errors.push(`${p}:${n}: ${line}`);
 if(errors.length){console.error(errors.join('\n'));console.error('FAIL: review Han text above; preserve legitimate names/data/legal notices.');process.exitCode=1;}else console.log(`PASS: ${files.length} tracked production files; no unreviewed literal Han text.\nPreserved: i18n catalogs, native selector labels, Unicode test fixtures, geo data and legal documents.`);
}
