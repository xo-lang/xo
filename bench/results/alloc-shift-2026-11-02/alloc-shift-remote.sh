#!/bin/bash
# alloc-shift-remote.sh REVS NAME SEED PRIV WORK BUNDLE ROUNDS SHIFTS (first host, OFF_EXCLUSIVE=1): each
# revision's jitter server linked at each XO_DATA_SHIFT (decision 0459), /alloc c256 and c32, ROUNDS rounds.
set -u
a=$1 name=$2 seed=$3 P=$4 work=$5 bundle=$6 rounds=$7 shifts=$8
W=$work/ash$name
L=$P.lockdir boot=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null)
hp= hb=
until mkdir "$L" 2>/dev/null; do
	read -r hp hb < "$L/holder" 2>/dev/null
	if [ -n "$hp" ] && { [ "$hb" != "$boot" ] || ! kill -0 "$hp" 2>/dev/null; }; then
		rm -f "$L/holder"; rmdir "$L" 2>/dev/null
	fi
	hp= hb=; sleep 1
done
echo "$$ $boot" > "$L/holder"
trap 'rm -f "$L/holder"; rmdir "$L" 2>/dev/null' EXIT
cd "$P" || exit 2
git for-each-ref --format='%(refname)' refs/ab 2>/dev/null | while read -r r; do git update-ref -d "$r"; done
rm -rf .git/refs/ab .git/objects/pack/tmp_*
git fetch -q "$bundle" "+refs/heads/*:refs/ab/*" || { echo "release-vs-go-http: the bundle did not fetch"; exit 2; }
rm -f "$bundle" 2>/dev/null
rm -rf "$W"; git worktree prune
revs=(${a//,/ })
for k in "${!revs[@]}"; do git worktree add -q --detach "$W/w$k" "${revs[$k]}" || exit 2; done
rm -f "$L/holder"; rmdir "$L"; trap - EXIT
cleanup() { cd "$work"; for k in "${!revs[@]}"; do git -C "$P" worktree remove --force "$W/w$k" 2>/dev/null; done; rm -rf "$W"; }
trap cleanup EXIT
B=$W/bin; mkdir -p "$B"
echo "host load $(cat /proc/loadavg)"
echo "alloc-shift $name (shifts $shifts): ${#revs[@]} revisions on $(uname -sm), $(nproc) CPUs, $(go version | cut -d' ' -f3)"
# the bench (server source and loadgen) of the last revision, for every build
J=$W/w$((${#revs[@]} - 1))/bench/perf/jitter
( cd "$J/loadgen" && go build -o "$B/loadgen" . ) || exit 1
all=()
for k in "${!revs[@]}"; do
	T=$W/w$k
	( cd "$T" && XO_SEED=$seed go run ./scripts/xobuild ) > "$W/xobuild$k.txt" 2>&1 || { echo "p$k ${revs[$k]}: xobuild failed: $(tail -3 "$W/xobuild$k.txt" | tr '\n' ' ')"; continue; }
	# the server source of base's tree, so only the compiler and std differ
	for sh in ${shifts//,/ }; do
		XO_DATA_SHIFT=$sh "$T/bin/xo" build -o "$B/srv-p${k}s$sh" "$J/xo" > "$W/srv$k.txt" 2>&1 || { echo "p$k s$sh: server build failed: $(tail -3 "$W/srv$k.txt" | tr '\n' ' ')"; continue; }
		echo "p${k}s$sh $(git -C "$T" rev-parse --short HEAD) built: $(go tool nm -n "$B/srv-p${k}s$sh" 2>/dev/null | awk '$3=="xo_sys_words"||$3=="xo_static"||$3=="xo_empty_list"{printf "%s=%s ", $3, $1}')"
		all+=(p${k}s$sh)
	done
	# a build cache per revision is not kept: drop the xo build tree's outputs
	rm -rf "$T/bin"
done
echo "loadgen flags: -c C -rate R -d ${DURATION:-10s} -warmup 2s -awake (default on in open loop); servers on CPUs ${SRV_CPUS:-0-5}, loadgen on ${GEN_CPUS:-6-11}"
ulimit -n 65536 2>/dev/null || true
SRV=${SRV_CPUS:-0-5} GEN=${GEN_CPUS:-6-11}
port=18800
one() { # server path conc rate round
	local srv=$1 path=$2 conc=$3 rate=$4 r=$5 res s0 s1 l0
	# Go answers /alloc with its hand encoder (/alloc-hand, Xo's encoding
	# done the same way; jitter-encoder-2026-11-02.md); the line keeps /alloc
	local url=$path
	[ $srv = go ] && [ $path = /alloc ] && url=/alloc-hand
	port=$((port + 1))
	PORT=$port taskset -c $SRV "$B/srv-$srv" > /dev/null 2> "$W/srv.err" &
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
for r in $(seq 1 $rounds); do
	for cfg in "/alloc 256 0" "/alloc 32 0"; do
		set -- $cfg
		n=${#all[@]}
		for k in $(seq 0 $((n - 1))); do one ${all[$(((k + r) % n))]} $1 $2 $3 $r; done
	done
done
echo SHIFT-DONE
