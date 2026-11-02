#!/bin/bash
# alloc-measure-remote.sh NAME ROUNDS TAR WORK (the FIRST host, OFF_EXCLUSIVE=1): the measure half of
# alloc-points/own/env-remote.sh. TAR is alloc-build-remote.sh's tar (put under /work/in). No git, no build:
# the lease holds only the quiet wait and the rounds.
set -u
name=$1 rounds=$2 tar=$3 work=$4
W=$work/rvm$name; rm -rf "$W"; mkdir -p "$W/bin"; B=$W/bin
trap 'rm -rf "$W"; rm -f "$tar"' EXIT
tar -C "$B" -xf "$tar" || { echo "alloc-measure: cannot unpack $tar"; exit 2; }
echo "host load $(cat /proc/loadavg)"
echo "alloc-measure $name on $(uname -sm), $(nproc) CPUs"
grep -v '^base ' "$B/revs.txt"; grep '^base ' "$B/revs.txt"
all=(base)
for k in $(seq 0 63); do [ -f "$B/srv-p$k" ] && all+=(p$k); done
echo "loadgen flags: -c C -rate R -d ${DURATION:-10s} -warmup 2s -awake (default on in open loop); servers on CPUs ${SRV_CPUS:-0-5}, loadgen on ${GEN_CPUS:-6-11}"
ulimit -n 65536 2>/dev/null || true
SRV=${SRV_CPUS:-0-5} GEN=${GEN_CPUS:-6-11}
port=18800
one() { # server path conc rate round
	local srv=$1 path=$2 conc=$3 rate=$4 r=$5 res s0 s1 l0
	local url=$path
	[ $srv = go ] && [ $path = /alloc ] && url=/alloc-hand
	port=$((port + 1))
	local ev=""; [ -f "$B/env-$srv" ] && ev=$(cat "$B/env-$srv")
	env $ev PORT=$port taskset -c $SRV "$B/srv-$srv" > /dev/null 2> "$W/srv.err" &
	local p=$!
	sleep 1
	kill -0 $p 2>/dev/null || { echo "server $srv exited: $(tail -3 "$W/srv.err")"; return; }
	s0=$(awk '/^cpu /{print $9; exit}' /proc/stat); l0=$(cut -d' ' -f1 /proc/loadavg)
	res=$(taskset -c $GEN "$B/loadgen" -addr 127.0.0.1:$port -path $url -c $conc -rate $rate -d ${DURATION:-10s} -warmup 2s -awake -pid $p 2>"$W/lg.err" | tail -1)
	s1=$(awk '/^cpu /{print $9; exit}' /proc/stat)
	kill -INT $p 2>/dev/null; sleep 0.5; kill $p 2>/dev/null; wait $p 2>/dev/null
	echo "$srv $path c$conc rate$rate r$r $res"
	echo "  env $srv $path c$conc rate$rate r$r: load_before $l0 steal_ticks $((s1 - s0)) load_after $(cut -d' ' -f1 /proc/loadavg) lgerr $(head -c 200 "$W/lg.err" | tr '\n' ' ')"
}
q0=$(date +%s)
while awk '{exit !($1 >= 1)}' /proc/loadavg && [ $(($(date +%s) - q0)) -lt 300 ]; do sleep 5; done
echo "quiet wait $(($(date +%s) - q0)) s, load $(cut -d' ' -f1-3 /proc/loadavg)"
for r in $(seq 1 $rounds); do
	for cfg in "/alloc 256 0" "/alloc 32 0"; do
		set -- $cfg
		n=${#all[@]}
		for k in $(seq 0 $((n - 1))); do one ${all[$(((k + r) % n))]} $1 $2 $3 $r; done
	done
done
echo POINTS-DONE
