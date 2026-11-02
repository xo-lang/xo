#!/bin/bash
# thp-job.sh DIR: 2,000 held TLS connections, xo (default and XO_THP=off) and go, 2 rounds; density.py while held
set -u
S=${1:?DIR}; T=$(mktemp -d "$HOME/thpd.XXXXXX"); trap 'rm -rf "$T"' EXIT
for f in xosrv gosrv cli density.py; do cp "$S/$f" "$T/$f"; done; chmod +x "$T/xosrv" "$T/gosrv" "$T/cli"
cd "$T" || exit 2; ulimit -n 200000 2>/dev/null || ulimit -n 65536
echo "thp: $(cat /sys/kernel/mm/transparent_hugepage/enabled) defrag: $(cat /sys/kernel/mm/transparent_hugepage/defrag); $(uname -m); nproc $(nproc)"
./cli -gencert "$T" > /dev/null
for round in 1 2; do
  for impl in xo xo-thpoff go; do
    case $impl in
      xo) PORT=18071 TLS_CERT="$(cat cert.pem)" TLS_KEY="$(cat key.pem)" ./xosrv > /dev/null 2>&1 & ;;
      xo-thpoff) XO_THP=off PORT=18071 TLS_CERT="$(cat cert.pem)" TLS_KEY="$(cat key.pem)" ./xosrv > /dev/null 2>&1 & ;;
      go) PORT=18071 CERT=cert.pem KEY=key.pem ./gosrv > /dev/null 2>&1 & ;;
    esac
    spid=$!; sleep 1
    ./cli -addr 127.0.0.1:18071 -n 2000 -tls -settle 25s -pid $spid > cli.out 2>&1 &
    cpid=$!; sleep 15
    echo "-- round $round $impl"
    python3 density.py $spid "$impl"
    wait $cpid; grep -E "idle:" cli.out | cut -c1-110
    kill $spid; wait $spid 2>/dev/null
  done
done
