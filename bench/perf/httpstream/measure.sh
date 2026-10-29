#!/bin/sh
# Measures bench/perf/httpstream (decision 0404 stage B): a 100 MB body
# (SIZE) read by Xo's client as a stream and whole (send), on each backend
# named, beside the Go twin's two modes. Runs each 5 times under
# /usr/bin/time -l and prints "<client> <mode> <min wall s> <min peak
# RSS bytes>" (darwin).
#
# usage: bench/perf/httpstream/measure.sh <xo binary> <out dir> <backend...>
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
SIZE=${SIZE:-104857600}
PORT=${PORT:-18097}
mkdir -p "$OUT"
(cd "$SRC/serve" && go build -o "$OUT/serve" .)
(cd "$SRC/gotwin" && go build -o "$OUT/c-gotwin" .)
for be in "$@"; do
	rm -rf "$OUT/src-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/c-$be" . >/dev/null)
done
"$OUT/serve" -addr 127.0.0.1:$PORT >"$OUT/serve.txt" 2>&1 &
spid=$!
trap 'kill $spid 2>/dev/null || true' EXIT
i=0
until grep -q ready "$OUT/serve.txt" 2>/dev/null; do
	i=$((i + 1))
	if [ $i -gt 500 ]; then
		echo "serve did not start" >&2
		exit 1
	fi
	sleep 0.01
done
for c in "$@" gotwin; do
	for mode in stream send; do
		best_w=""
		best_r=""
		for _ in 1 2 3 4 5; do
			ADDR=127.0.0.1:$PORT SIZE=$SIZE MODE=$mode /usr/bin/time -l "$OUT/c-$c" >"$OUT/run.txt" 2>"$OUT/time.txt"
			got=$(cat "$OUT/run.txt")
			if [ "$got" != "$SIZE" ]; then
				echo "$c $mode: read $got bytes, want $SIZE" >&2
				exit 1
			fi
			w=$(awk '/ real /{print $1}' "$OUT/time.txt")
			r=$(awk '/maximum resident set size/{print $1}' "$OUT/time.txt")
			if [ -z "$best_w" ] || awk "BEGIN{exit !($w < $best_w)}"; then best_w=$w; fi
			if [ -z "$best_r" ] || [ "$r" -lt "$best_r" ]; then best_r=$r; fi
		done
		echo "$c $mode $best_w $best_r"
	done
done
