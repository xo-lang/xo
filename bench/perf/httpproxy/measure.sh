#!/bin/sh
# Measures bench/perf/httpproxy (decision 0404 stage D): a SIZE byte body
# (default 100 MB) from bench/perf/httpstream's serve/ through Xo's
# http.reverse_proxy (each backend named) and through Go's
# httputil.ReverseProxy, read by httpstream's Go client streaming. Runs
# each 5 times and prints "<proxy> <min wall s of the client> <peak RSS
# bytes of the proxy>" (darwin: the proxy's RSS from ps while serving).
#
# usage: bench/perf/httpproxy/measure.sh <xo binary> <out dir> <backend...>
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
SIZE=${SIZE:-104857600}
UP=${UP:-18097}
PX=${PX:-18099}
mkdir -p "$OUT"
(cd "$SRC/../httpstream/serve" && go build -o "$OUT/serve" .)
(cd "$SRC/../httpstream/gotwin" && go build -o "$OUT/client" .)
(cd "$SRC/goproxy" && go build -o "$OUT/p-go" .)
for be in "$@"; do
	rm -rf "$OUT/src-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/p-$be" . >/dev/null)
done
wait_ready() {
	i=0
	until grep -q ready "$1" 2>/dev/null; do
		i=$((i + 1))
		if [ $i -gt 500 ]; then
			echo "$1: no ready" >&2
			exit 1
		fi
		sleep 0.01
	done
}
"$OUT/serve" -addr 127.0.0.1:$UP >"$OUT/serve.txt" 2>&1 &
spid=$!
ppid=""
trap 'kill $spid 2>/dev/null || true; [ -n "$ppid" ] && kill $ppid 2>/dev/null || true' EXIT
wait_ready "$OUT/serve.txt"
for p in "$@" go; do
	if [ "$p" = go ]; then
		"$OUT/p-go" -addr 127.0.0.1:$PX -upstream 127.0.0.1:$UP >"$OUT/proxy.txt" 2>&1 &
	else
		ADDR=127.0.0.1:$PX UPSTREAM=127.0.0.1:$UP "$OUT/p-$p" >"$OUT/proxy.txt" 2>&1 &
	fi
	ppid=$!
	wait_ready "$OUT/proxy.txt"
	best_w=""
	peak=0
	for _ in 1 2 3 4 5; do
		ADDR=127.0.0.1:$PX SIZE=$SIZE MODE=stream /usr/bin/time -p "$OUT/client" >"$OUT/run.txt" 2>"$OUT/time.txt" &
		cpid=$!
		while kill -0 $cpid 2>/dev/null; do
			r=$(ps -o rss= -p $ppid | tr -d ' ')
			if [ -n "$r" ] && [ "$r" -gt "$peak" ]; then peak=$r; fi
			sleep 0.005
		done
		wait $cpid
		got=$(cat "$OUT/run.txt")
		if [ "$got" != "$SIZE" ]; then
			echo "$p: read $got bytes, want $SIZE" >&2
			exit 1
		fi
		w=$(awk '/^real/{print $2}' "$OUT/time.txt")
		if [ -z "$best_w" ] || awk "BEGIN{exit !($w < $best_w)}"; then best_w=$w; fi
	done
	kill $ppid 2>/dev/null || true
	wait $ppid 2>/dev/null || true
	ppid=""
	echo "$p $best_w $((peak * 1024))"
done
