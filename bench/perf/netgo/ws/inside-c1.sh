#!/bin/bash
ulimit -n 65536
port=23200
for r in 1 2 3 4 5; do
 for s in xo go; do
  port=$((port+1))
  if [ $s = xo ]; then bin=/work/xosrv; else bin=/work/gosrv.bin; fi
  PORT=$port $bin > /dev/null 2>&1 < <(sleep 60) &
  p=$!
  sleep 1
  echo -n "$s: "; timeout 30 /work/cli.bin -url ws://127.0.0.1:$port/ -c 1 -size 64 -d 3s | tail -1
  kill $p; wait $p 2>/dev/null
 done
done
