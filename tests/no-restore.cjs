// Restore is not a supported product feature; restart persistence remains required.
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const ROOT=path.resolve(__dirname,'..');
test('restore script is not shipped',()=>assert(!fs.existsSync(path.join(ROOT,'packaging/restore.sh'))));
test('native acceptance retains restart, not deleted-unit restoration',()=>{
 const text=fs.readFileSync(path.join(ROOT,'tests/vyos_vm.cjs'),'utf8');
 for(const token of ['--restore-command','args.restore_command','restore-service-definition'])assert(!text.includes(token),'obsolete restore acceptance: '+token);
 assert.doesNotMatch(text,/checks\s*(?:\.restore|\[['"]restore['"]\])/);
 assert(text.includes('restart-persistence'),'missing restart-persistence stage');
 assert.match(text,/checks\s*(?:\.restart|\[['"]restart['"]\])/,'missing restart check');
});
