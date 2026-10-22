#!/bin/bash
ulimit -n 65536
apt-get update >/dev/null 2>&1; apt-get install -y --no-install-recommends strace >/dev/null 2>&1
port=23000
for cfg in "1 64" "16 64" "16 4096" "4 65536"; do
  set -- $cfg
  for s in xo go; do
    port=$((port+1))
    if [ $s = xo ]; then bin=/work/xosrv; else bin=/work/gosrv.bin; fi
    PORT=$port $bin > /work/srv.$s.log 2>&1 < <(sleep 600) &
    p=$!
    sleep 1.5
    strace -f -c -o /work/st.$s.$1.$2.txt -p $p 2>/dev/null &
    sp=$!
    sleep 0.5
    echo -n "$s c=$1 size=$2: "
    timeout 60 /work/cli.bin -url ws://127.0.0.1:$port/ -c $1 -size $2 -d 5s 2>&1 | tail -1
    kill -INT $sp; wait $sp 2>/dev/null
    kill $p; wait $p 2>/dev/null
  done
done
