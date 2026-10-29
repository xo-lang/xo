#!/bin/sh
# One round of bench/perf/httpproxy for `xo bench --ab` (decision 0340):
# starts serve/ and the proxy binary $1 (Xo's or the Go twin, already
# built), reads SIZE bytes (default 1 GiB) through it with httpstream's Go
# client, stops both (by the PIDs it started) and prints the client's
# wall time as the last JSON line ({"client_ms": N}).
#
# usage: abrun.sh PROXY SERVE CLIENT   (binaries from measure.sh's OUT)
set -e
PROXY=$1
SERVE=$2
CLIENT=$3
SIZE=${SIZE:-1073741824}
UP=${UP:-18197}
PX=${PX:-18199}
T=$(mktemp -d)
"$SERVE" -addr 127.0.0.1:$UP >"$T/serve.txt" 2>&1 &
spid=$!
case "$PROXY" in
*gotwin*) "$PROXY" -addr 127.0.0.1:$PX -upstream 127.0.0.1:$UP >"$T/proxy.txt" 2>&1 & ;;
*) ADDR=127.0.0.1:$PX UPSTREAM=127.0.0.1:$UP "$PROXY" >"$T/proxy.txt" 2>&1 & ;;
esac
ppid=$!
trap 'kill $spid $ppid 2>/dev/null || true; rm -rf "$T"' EXIT
for f in serve proxy; do
	i=0
	until grep -q ready "$T/$f.txt" 2>/dev/null; do
		i=$((i + 1))
		[ $i -gt 500 ] && { echo "$f: no ready" >&2; exit 1; }
		sleep 0.01
	done
done
t0=$(perl -MTime::HiRes=time -e 'printf "%.0f", time*1000')
got=$(ADDR=127.0.0.1:$PX SIZE=$SIZE MODE=stream "$CLIENT")
t1=$(perl -MTime::HiRes=time -e 'printf "%.0f", time*1000')
[ "$got" = "$SIZE" ] || { echo "read $got bytes, want $SIZE" >&2; exit 1; }
echo "{\"client_ms\": $((t1 - t0))}"
