#!/bin/bash
apt-get update >/dev/null 2>&1; apt-get install -y --no-install-recommends strace >/dev/null 2>&1
/work/serve.bin -addr 127.0.0.1:18095 > /work/serve.log 2>&1 < <(sleep 900) &
sp=$!
sleep 1
TIMEFORMAT='%R real %U user %S sys'
for w in len chunked big redirect; do
  for c in xoc goc; do
    for k in 1 2 3; do
      echo -n "$w $c: "
      { time ADDR=127.0.0.1:18095 W=$w N=20000 /work/$c.bin >/dev/null; } 2>&1
    done
    ADDR=127.0.0.1:18095 W=$w N=5000 strace -f -c -o /work/hs.$w.$c.txt /work/$c.bin >/dev/null
  done
done
kill $sp
