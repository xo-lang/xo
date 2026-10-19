#!/bin/bash
# burst-ci.sh: is the native Xo server more sensitive to connection bursts
# than Go's, on a quiet linux/arm64 machine (owner, 2026-10-19: one CI run
# to settle knowledgebase/gotchas/docker-burst-stalls-on-loaded-mac.md,
# whose stalls were the Docker VM descheduled on a loaded Mac).
#
#   gh workflow run ci.yml -f ref=BRANCH -f suite=script \
#     -f packages="bench/perf/http/burst-ci.sh [BLOCKS] [ROUNDS]"
#
# (knowledgebase/infrastructure/ci.md, the script suite). It builds the
# plain HTTP server (bench/perf/http/xo, native arm64, $XO_CLI with
# $XO_CI_STD) and the Go comparison server (bench/perf/http/go), starts
# both on CPUs of their own (the first half of the machine; loadgen the
# second half), and runs BLOCKS (default 6) alternating blocks of
# ROUNDS (default 50) rounds of `loadgen -burst 2000` against each: go,
# xo, go, xo, ... Each block records the server's minor faults
# (/proc/PID/stat). $XO_CI_OUT/burst.txt gets the summary per server
# (round wall p50, p99, max; rounds over 150 and 300 ms; latency p99;
# failed connections; minor faults) and every block's loadgen line;
# $XO_CI_OUT/rounds-SERVER.txt every round. About 10 minutes.
set -uo pipefail
BLOCKS=${1:-6}
ROUNDS=${2:-50}
N=${BURST_N:-2000}
: "${XO_CLI:?}" "${XO_CI_STD:?}" "${XO_CI_OUT:?}"
SRC=$(cd "$(dirname "$0")" && pwd)
W=$(mktemp -d)
mkdir -p "$XO_CI_OUT"
out=$XO_CI_OUT/burst.txt
: >"$out"
log() { echo "$@" | tee -a "$out"; }

(cd "$SRC/xo" && "$XO_CLI" build --std="$XO_CI_STD" --backend=arm64 -o "$W/srv-xo" .) || { log "build: xo server failed"; exit 1; }
(cd "$SRC/go" && CGO_ENABLED=0 go build -o "$W/srv-go" .) || { log "build: go server failed"; exit 1; }
(cd "$SRC/loadgen" && CGO_ENABLED=0 go build -o "$W/loadgen" .) || { log "build: loadgen failed"; exit 1; }

ncpu=$(nproc)
half=$((ncpu / 2))
srvcpus=0-$((half - 1))
gencpus=$half-$((ncpu - 1))
ulimit -n 65536 2>/dev/null || ulimit -n "$(ulimit -Hn)"
log "machine: $(uname -m) $ncpu CPUs, $(grep -m1 MemTotal /proc/meminfo | tr -s ' '), load $(cut -d' ' -f1-3 /proc/loadavg)"
log "somaxconn $(cat /proc/sys/net/core/somaxconn), tcp_max_syn_backlog $(cat /proc/sys/net/ipv4/tcp_max_syn_backlog), nofile $(ulimit -n)"
log "servers on CPUs $srvcpus, loadgen on $gencpus; $BLOCKS blocks of $ROUNDS rounds of $N connections, alternating go and xo"
log "xo: $("$XO_CLI" version 2>&1 | head -1)"

declare -A port pid
port[go]=18181
port[xo]=18182
for s in go xo; do
	PORT=${port[$s]} taskset -c "$srvcpus" "$W/srv-$s" >"$W/$s.log" 2>&1 &
	pid[$s]=$!
done
sleep 2
for s in go xo; do
	kill -0 "${pid[$s]}" 2>/dev/null || { log "server $s exited at start: $(tail -3 "$W/$s.log")"; exit 1; }
done
minflt() { awk '{print $10}' "/proc/$1/stat" 2>/dev/null || echo 0; }

: >"$W/faults.txt"
for b in $(seq 1 "$BLOCKS"); do
	for s in go xo; do
		pre=0
		[ "$b" = 1 ] && pre=1
		f0=$(minflt "${pid[$s]}")
		taskset -c "$gencpus" "$W/loadgen" -url "http://127.0.0.1:${port[$s]}" -preload "$pre" -burst "$N" -reps "$ROUNDS" >"$W/$s-$b.txt" 2>&1
		f1=$(minflt "${pid[$s]}")
		echo "$s $((f1 - f0))" >>"$W/faults.txt"
		grep '^round ' "$W/$s-$b.txt" | sed "s/^/block $b: /" >>"$XO_CI_OUT/rounds-$s.txt"
		log "block $b $s: $(grep -E '^burst|loadgen:' "$W/$s-$b.txt" | tr '\n' ' ')minor faults $((f1 - f0))"
		kill -0 "${pid[$s]}" 2>/dev/null || { log "server $s exited in block $b: $(tail -3 "$W/$s.log")"; exit 1; }
	done
done
for s in go xo; do kill "${pid[$s]}" 2>/dev/null; done

# the summary: round walls (Go durations: 850µs, 47.7ms, 1.2s) from the
# rounds files, latency p99 the worst block's (awk: no python on every
# runner)
ms() { awk '{ t = 0; s = $0
	while (match(s, /[0-9.]+(ns|µs|us|ms|s|m)/)) {
		x = substr(s, RSTART, RLENGTH); s = substr(s, RSTART + RLENGTH)
		v = x + 0; u = x; sub(/^[0-9.]+/, "", u)
		if (u == "ns") v /= 1e6; else if (u == "µs" || u == "us") v /= 1e3; else if (u == "s") v *= 1e3; else if (u == "m") v *= 6e4
		t += v
	}
	printf "%.3f\n", t }'; }
{
	echo
	echo "| server | rounds | wall p50 | wall p99 | wall max | over 150 ms | over 300 ms | latency p99 (worst block) | failed connections | minor faults |"
	echo "|---|---|---|---|---|---|---|---|---|---|"
	for s in go xo; do
		sed -n 's/.* answered, wall \([^,]*\),.*/\1/p' "$XO_CI_OUT/rounds-$s.txt" | ms | sort -n >"$W/walls-$s"
		failed=$(sed -n 's/.*: \([0-9]*\) of \([0-9]*\) answered.*/\1 \2/p' "$XO_CI_OUT/rounds-$s.txt" | awk '{f += $2 - $1} END {print f + 0}')
		p99=$(cat "$W/$s"-*.txt | sed -n 's/.*latency p50 [^ ]* p99 \([^ ]*\) .*/\1/p' | ms | sort -n | tail -1)
		faults=$(awk -v s="$s" '$1 == s {f += $2} END {print f + 0}' "$W/faults.txt")
		awk -v s="$s" -v failed="$failed" -v p99="${p99:-0}" -v faults="$faults" '{ w[NR] = $1 }
			function p(q) { return NR ? w[int((NR - 1) * q) + 1] : 0 }
			END { o150 = 0; o300 = 0; for (i = 1; i <= NR; i++) { if (w[i] > 150) o150++; if (w[i] > 300) o300++ }
				printf "| %s | %d | %.1f ms | %.1f ms | %.1f ms | %d | %d | %.1f ms | %d | %d |\n", s, NR, p(0.5), p(0.99), p(1), o150, o300, p99, failed, faults }' "$W/walls-$s"
	done
} >>"$out"
cat "$out"
rm -rf "$W"
