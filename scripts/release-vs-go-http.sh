#!/bin/bash
# release-vs-go-http.sh REV NAME SEED PRIV WORK BUNDLE ROUNDS: the http section of release-vs-go.sh
# (Lead, 2026-11-02), run on the first linux/arm64 offload host by offload.sh with OFF_EXCLUSIVE=1
# (knowledgebase/infrastructure/offload.md). bench/perf/jitter's server built three ways: Xo at REV
# (xo, built with xobuild from SEED), Xo the host's seed, the release before (base in the lines, the
# previous release), and Go with the hand encoder twice: with Go's defaults (go) and with a memory
# limit in place of GOGC (go-tuned: GO_TUNED_ENV, default "GOGC=off GOMEMLIMIT=1GiB"; at the
# defaults Go's tiny live heap collects about 1,250 times a second on /alloc, 2026-11-09). Each server is pinned to SRV_CPUS (default
# 0-5), the loadgen to GEN_CPUS (6-11), with -awake (f7bd0cb5a: open loop sends on time). Rows:
# closed loop /hello and /alloc at 32 and 256 connections, open loop /alloc with 64 connections at
# 10k and 20k req/s (Go's /alloc row is its /alloc-hand); DURATION (10s) after a 2 s warm-up, servers interleaved in a rotating order per
# round. Prints one line per run: "SERVER PATH cC rateR rN {loadgen json}", the json with the
# server's peak resident set (rss_kb, VmHWM).
# BASE_XO (the 9th argument, default SEED; release-vs-go.sh HTTP_BASE_XO): the xo of the release the report
# compares against, which builds the base servers; SEED only builds Xo at REV.
# PADS (the 8th argument, a space list; default none): both Xo servers are built at each XO_ARM64_PAD
# and named xo.pK and base.pK, so pads.py judges xo against base by the median over pads (a revision's
# /alloc moves about 4 percent with where its code lands; Lead, 2026-11-03).
set -u
a=$1 name=$2 seed=$3 P=$4 work=$5 bundle=$6 rounds=$7 pads=${8:-} basexo=${9:-$3}
W=$work/rvg$name
L=$P.lockdir boot=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null)
hp= hb=
# the bundle goes on every exit, a stop while waiting for the lock
# included (two 80 MB bundles of stopped runs stayed in /work/in, 2026-11-07)
trap 'rm -f "$bundle" 2>/dev/null' EXIT
until mkdir "$L" 2>/dev/null; do
	read -r hp hb < "$L/holder" 2>/dev/null
	if [ -n "$hp" ] && { [ "$hb" != "$boot" ] || ! kill -0 "$hp" 2>/dev/null; }; then
		rm -f "$L/holder"; rmdir "$L" 2>/dev/null
	fi
	hp= hb=; sleep 1
done
echo "$$ $boot" > "$L/holder"
trap 'rm -f "$L/holder" "$bundle"; rmdir "$L" 2>/dev/null' EXIT
cd "$P" || exit 2
git for-each-ref --format='%(refname)' refs/ab 2>/dev/null | while read -r r; do git update-ref -d "$r"; done
rm -rf .git/refs/ab .git/objects/pack/tmp_*
git fetch -q "$bundle" "+refs/heads/*:refs/ab/*" || { echo "release-vs-go-http: the bundle did not fetch"; exit 2; }
rm -f "$bundle" 2>/dev/null
rm -rf "$W"; git worktree prune
git worktree add -q --detach "$W/a" "$a" || exit 2
rm -f "$L/holder"; rmdir "$L"; trap - EXIT
cleanup() { cd "$work"; git -C "$P" worktree remove --force "$W/a" 2>/dev/null; rm -rf "$W"; }
trap cleanup EXIT
T=$W/a; B=$W/bin; mkdir -p "$B"
echo "host load $(cat /proc/loadavg)"
echo "release-vs-go-http $name: $(git -C "$T" rev-parse --short HEAD) on $(uname -sm), $(nproc) CPUs, $(go version | cut -d' ' -f3)"
( cd "$T" && XO_SEED=$seed go run ./scripts/xobuild ) > "$W/xobuild.txt" 2>&1 || { tail -5 "$W/xobuild.txt"; exit 1; }
J=$T/bench/perf/jitter
( cd "$J/loadgen" && go build -o "$B/loadgen" . ) || exit 1
( cd "$J/go" && go build -o "$B/srv-go" . ) || exit 1
servers="xo base go go-tuned"
if [ -n "$pads" ]; then
	servers="go go-tuned"
	for k in $pads; do
		XO_ARM64_PAD=$k "$T/bin/xo" build -o "$B/srv-xo.p$k" "$J/xo" > "$W/xobuild-srv.txt" 2>&1 || { tail -5 "$W/xobuild-srv.txt"; exit 1; }
		XO_ARM64_PAD=$k "$basexo" build -o "$B/srv-base.p$k" "$J/xo" > "$W/xobuild-srv35.txt" 2>&1 || { tail -5 "$W/xobuild-srv35.txt"; exit 1; }
		servers="$servers xo.p$k base.p$k"
	done
else
	"$T/bin/xo" build -o "$B/srv-xo" "$J/xo" > "$W/xobuild-srv.txt" 2>&1 || { tail -5 "$W/xobuild-srv.txt"; exit 1; }
	"$basexo" build -o "$B/srv-base" "$J/xo" > "$W/xobuild-srv35.txt" 2>&1 || { tail -5 "$W/xobuild-srv35.txt"; exit 1; }
fi
echo "xo $("$T/bin/xo" version); base $("$basexo" version)"
echo "go-tuned env: ${GO_TUNED_ENV:-GOGC=off GOMEMLIMIT=1GiB}"
echo "loadgen flags: -c C -rate R -d ${DURATION:-10s} -warmup 2s -awake (default on in open loop); servers on CPUs ${SRV_CPUS:-0-5}, loadgen on ${GEN_CPUS:-6-11}"
ulimit -n 65536 2>/dev/null || true
SRV=${SRV_CPUS:-0-5} GEN=${GEN_CPUS:-6-11}
port=18800
one() { # server path conc rate round
	local srv=$1 path=$2 conc=$3 rate=$4 r=$5 res s0 s1 l0
	# Go answers /alloc with its hand encoder (/alloc-hand, Xo's encoding
	# done the same way; jitter-encoder-2026-11-02.md); the line keeps /alloc
	local url=$path
	case $srv in go*) [ $path = /alloc ] && url=/alloc-hand ;; esac
	local bin=$srv envs=
	[ $srv = go-tuned ] && bin=go envs=${GO_TUNED_ENV:-GOGC=off GOMEMLIMIT=1GiB}
	port=$((port + 1))
	env PORT=$port $envs taskset -c $SRV "$B/srv-$bin" > /dev/null 2> "$W/srv.err" &
	local p=$!
	sleep 1
	kill -0 $p 2>/dev/null || { echo "server $srv exited: $(tail -3 "$W/srv.err")"; return; }
	s0=$(awk '/^cpu /{print $9; exit}' /proc/stat); l0=$(cut -d' ' -f1 /proc/loadavg)
	res=$(taskset -c $GEN "$B/loadgen" -addr 127.0.0.1:$port -path $url -c $conc -rate $rate -d ${DURATION:-10s} -warmup 2s -awake -pid $p 2>"$W/lg.err" | tail -1)
	s1=$(awk '/^cpu /{print $9; exit}' /proc/stat)
	local hwm=$(awk '/^VmHWM:/{print $2}' /proc/$p/status 2>/dev/null)
	[ -n "$hwm" ] && [ "${res%\}}" != "$res" ] && res="${res%\}},\"rss_kb\":$hwm}"
	kill -INT $p 2>/dev/null; sleep 0.5; kill $p 2>/dev/null; wait $p 2>/dev/null
	echo "$srv $path c$conc rate$rate r$r $res"
	echo "  env $srv $path c$conc rate$rate r$r: load_before $l0 steal_ticks $((s1 - s0)) load_after $(cut -d' ' -f1 /proc/loadavg) lgerr $(head -c 200 "$W/lg.err" | tr '\n' ' ')"
}
for r in $(seq 1 $rounds); do
	for cfg in "/hello 32 0" "/hello 256 0" "/alloc 32 0" "/alloc 256 0" "/alloc 64 10000" "/alloc 64 20000"; do
		set -- $cfg
		# a rotating order: each server starts the row in turn
		o=($servers); n=${#o[@]}
		for k in $(seq 0 $((n - 1))); do one ${o[$(((k + r) % n))]} $1 $2 $3 $r; done
	done
done
echo VSGO-HTTP-DONE
