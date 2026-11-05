#!/usr/bin/env bash
# Wrap a trusted CI archive without repacking. Digest proves integrity, not origin.
set -euo pipefail
umask 077
ROOT=vyos-sflow-1.2.5-linux-amd64
fail() { printf '%s\n' "$*" >&2; exit 1; }
archive= output=
while (($#)); do
    case $1 in
        --archive|--output) (($# >= 2)) || fail 'Missing option value'; key=$1; value=$2; shift 2; if [[ $key == --archive ]]; then archive=$value; else output=$value; fi ;;
        *) fail 'Usage: build-self-installer.sh --archive FILE.tar.gz --output FILE.run' ;;
    esac
done
[[ -n $archive && -n $output && -f $archive ]] || fail 'Archive and output are required'
for cmd in realpath mktemp cp gzip od awk sha256sum sed wc cat chmod mkdir mv rm; do command -v "$cmd" >/dev/null || fail "Required command missing: $cmd"; done
[[ $(realpath -m -- "$archive") != "$(realpath -m -- "$output")" && ! $archive -ef $output ]] || fail 'output must differ from archive'
tmp=$(mktemp -d "${TMPDIR:-/tmp}/self-builder.XXXXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Copy once: validation, digest and append all consume this private snapshot.
cp -- "$archive" "$tmp/payload"
gzip -t -- "$tmp/payload" || fail 'Invalid archive'
# Parse raw 512-byte tar records, not tar listing text (which can normalize unsafe
# names). Reject extensions, links, devices, special modes and duplicate names.
# Only plain ustar regular files/directories are needed by the release package.
gzip -dc -- "$tmp/payload" | od -An -v -tu1 | awk -v root="$ROOT" '
function bad(msg) { print msg > "/dev/stderr"; failed=1; exit 1 }
function text(start,len, i,s,ended) { s=""; ended=0; for(i=start;i<start+len;i++) { if(b[i]==0) ended=1; else { if(ended) bad("Unsafe archive member: embedded NUL"); s=s sprintf("%c",b[i]) } } return s }
function oct(start,len, i,s,n) { s=""; for(i=start;i<start+len;i++) { if(b[i]==0 || b[i]==32) s=s " "; else if(b[i]>=48 && b[i]<=55) s=s sprintf("%c",b[i]); else bad("Invalid archive numeric field") } sub(/^ +/,"",s); sub(/ +$/,"",s); if(s !~ /^[0-7]+$/) bad("Invalid archive numeric field"); n=0; for(i=1;i<=length(s);i++) n=n*8+substr(s,i,1); if(n>9007199254740991) bad("Archive size too large"); return n }
function header( i,sum,check,name,prefix,parts,n,type,mode,size) {
 sum=0; for(i=1;i<=512;i++) sum+=b[i]; if(sum==0) { zeros++; return }
 if(zeros) bad("Invalid archive: data after end marker")
 check=oct(149,8); sum=0; for(i=1;i<=512;i++) sum+=(i>=149 && i<=156)?32:b[i]; if(sum!=check) bad("Invalid archive checksum")
 if(text(258,6)!="ustar" && text(258,6)!="ustar ") bad("Unsupported archive format")
 name=text(1,100); prefix=text(346,155); if(prefix!="") name=prefix "/" name
 sub(/\/+$/,"",name); n=split(name,parts,"/")
 if(name !~ /^[A-Za-z0-9_.\/+\-]+$/ || parts[1]!=root || seen[name]++) bad("Unsafe archive member: " name)
 for(i=1;i<=n;i++) if(parts[i]=="" || parts[i]=="." || parts[i]=="..") bad("Unsafe archive member: " name)
 type=b[157]; mode=oct(101,8); size=oct(125,12)
 if((type!=0 && type!=48 && type!=53) || int(mode/512)%8!=0 || mode>4095) bad("Unsafe archive member: " name)
 if(type==53 && size!=0) bad("Unsafe directory size")
 if(type!=53) regular[name]=1
 skip=int((size+511)/512)*512
}
{ for(j=1;j<=NF;j++) { if(skip>0) { skip--; continue } b[++offset]=$j; if(offset==512) { header(); offset=0 } } }
END {
 if(failed) exit 1
 if(offset || skip || zeros<2) bad("Invalid archive: truncated tar")
 count=split("vyos-sflow packaging/install.sh packaging/common.sh packaging/uninstall.sh packaging/service.env.example packaging/vyos-sflow.service packaging/setup.sh packaging/vyos-sflow-ready.service",required," ")
 for(i=1;i<=count;i++) if(!regular[root "/" required[i]]) bad("Incomplete release archive")
}' || fail 'Archive validation failed'
digest=$(sha256sum "$tmp/payload"); digest=${digest%% *}
template=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/packaging/self-installer.sh
lines=$(wc -l < "$template"); lines=$((lines + 1))
sed -e "s/@SHA256@/$digest/g" -e "s/@ROOT@/$ROOT/g" -e "s/@PAYLOAD_LINE@/$lines/g" "$template" > "$tmp/result"
cat "$tmp/payload" >> "$tmp/result"
chmod 755 "$tmp/result"
mkdir -p -- "$(dirname -- "$output")"
mv -- "$tmp/result" "$output"
printf 'Created %s; embedded archive SHA256 %s\n' "$output" "$digest"
