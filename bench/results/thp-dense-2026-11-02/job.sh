#!/bin/bash
# job.sh DIR: 2,000 held TLS connections; xo default, XO_THP=off, XO_THP=dense, and go; 3 rounds interleaved
set -u
S=${1:?DIR}; T=$(mktemp -d "$HOME/dense.XXXXXX"); trap 'rm -rf "$T"' EXIT
for f in xosrv gosrv cli density.py; do cp "$S/$f" "$T/$f"; done; chmod +x "$T/xosrv" "$T/gosrv" "$T/cli"
cd "$T" || exit 2; ulimit -n 200000 2>/dev/null || ulimit -n 65536
echo "thp: $(cat /sys/kernel/mm/transparent_hugepage/enabled) defrag: $(cat /sys/kernel/mm/transparent_hugepage/defrag); $(uname -m); nproc $(nproc); free huge $(grep -E '^AnonHugePages' /proc/meminfo)"
./cli -gencert "$T" > /dev/null
flags() { python3 -c "
import sys
hg=nh=0
for l in open('/proc/$1/smaps'):
    if l.startswith('VmFlags:'):
        f=l.split()
        hg+='hg' in f; nh+='nh' in f
print(f'$2:   VmFlags hg {hg}, nh {nh}')"; }
for round in 1 2 3; do
  for impl in xo xo-off xo-dense go; do
    case $impl in
      xo) PORT=18072 TLS_CERT="$(cat cert.pem)" TLS_KEY="$(cat key.pem)" ./xosrv > /dev/null 2>&1 & ;;
      xo-off) XO_THP=off PORT=18072 TLS_CERT="$(cat cert.pem)" TLS_KEY="$(cat key.pem)" ./xosrv > /dev/null 2>&1 & ;;
      xo-dense) XO_THP=dense PORT=18072 TLS_CERT="$(cat cert.pem)" TLS_KEY="$(cat key.pem)" ./xosrv > /dev/null 2>&1 & ;;
      go) PORT=18072 CERT=cert.pem KEY=key.pem ./gosrv > /dev/null 2>&1 & ;;
    esac
    spid=$!; sleep 1
    ./cli -addr 127.0.0.1:18072 -n 2000 -tls -settle 25s -pid $spid > cli.out 2>&1 &
    cpid=$!; sleep 15
    echo "-- round $round $impl"
    python3 density.py $spid "$impl" | grep -v '^.*:   [<>0-9]'
    flags $spid "$impl"
    wait $cpid; grep -E "idle:" cli.out | cut -c1-110
    kill $spid; wait $spid 2>/dev/null
  done
done
true
