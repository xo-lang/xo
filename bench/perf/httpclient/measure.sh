#!/bin/sh
# Measures bench/perf/httpclient (audit item B10): builds the client with
# each backend named, starts serve/ (its own PID, stopped at the end), and
# for every workload runs the client 5 times under /usr/bin/time -l with
# N requests. Prints "<workload> <backend> <min instructions retired of
# the client>". Instructions come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/httpclient/measure.sh <xo binary> <out dir> <backend...>
# WORKLOADS overrides the list ("none" is the start and the end alone,
# N=0); N the requests per workload (default 3000); PORT (default 18095).
# The C client (xo_cl_do, cl_read_response) was deleted with B10: an xo
# from before it (main 1018327e) measures the C path.
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
N=${N:-3000}
PORT=${PORT:-18095}
mkdir -p "$OUT"
(cd "$SRC/serve" && go build -o "$OUT/serve" .)
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
		echo "server did not start" >&2
		exit 1
	fi
	sleep 0.02
done
for w in ${WORKLOADS:-none len chunked big redirect}; do
	for be in "$@"; do
		best=""
		n=$N
		if [ "$w" = none ]; then n=0; fi
		for k in 1 2 3 4 5; do
			ADDR=127.0.0.1:$PORT W=$w N=$n /usr/bin/time -l "$OUT/c-$be" >"$OUT/out.txt" 2>"$OUT/time.txt"
			if grep -q error "$OUT/out.txt"; then
				cat "$OUT/out.txt" >&2
				exit 1
			fi
			c=$(awk '/instructions retired/{print $1}' "$OUT/time.txt")
			if [ -z "$best" ] || [ "$c" -lt "$best" ]; then best=$c; fi
		done
		echo "$w $be $best"
	done
done
