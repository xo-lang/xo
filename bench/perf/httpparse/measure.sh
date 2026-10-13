#!/bin/sh
# Measures bench/perf/httpparse (decision 0193 step 6): builds the server
# with each backend named, and for every workload runs it 5 times under
# /usr/bin/time -l while load/ sends N requests, then stops it (SIGTERM to
# the server, the child of the time process this script started). Prints
# "<workload> <backend> <min instructions retired of the server>".
# Instructions come from /usr/bin/time -l (darwin).
#
# usage: bench/perf/httpparse/measure.sh <xo binary> <out dir> <backend...>
# XO_LLRT_C=http1,hpack in the environment builds the C paths; WORKLOADS
# overrides the list ("none" is the start and the stop alone); N the
# requests per workload (default 20000); PORT (default 18090).
# Decision 0198 deleted the C path of http1 and hpack (2026-10-13): naming it in
# XO_LLRT_C now builds the Xo path; earlier results files keep the comparisons.
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
N=${N:-20000}
PORT=${PORT:-18090}
mkdir -p "$OUT"
(cd "$SRC/load" && go build -o "$OUT/load" .)
"$OUT/load" -gencert "$OUT"
for be in "$@"; do
	rm -rf "$OUT/src-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/t-$be" .)
done
for w in ${WORKLOADS:-none h1_get h1_chunked h1_url h2_get h2_varied}; do
	for be in "$@"; do
		best=""
		for k in 1 2 3 4 5; do
			rm -f "$OUT/ready.txt" "$OUT/time.txt"
			if [ "${w#h2}" != "$w" ] || [ "$w" = none ]; then
				TLS_CERT=$(cat "$OUT/cert.pem") TLS_KEY=$(cat "$OUT/key.pem") ADDR=127.0.0.1:$PORT \
					/usr/bin/time -l "$OUT/t-$be" >"$OUT/ready.txt" 2>"$OUT/time.txt" &
			else
				ADDR=127.0.0.1:$PORT /usr/bin/time -l "$OUT/t-$be" >"$OUT/ready.txt" 2>"$OUT/time.txt" &
			fi
			tpid=$!
			i=0
			until grep -q ready "$OUT/ready.txt" 2>/dev/null; do
				i=$((i + 1))
				if [ $i -gt 500 ]; then
					echo "server did not start" >&2
					exit 1
				fi
				sleep 0.02
			done
			if [ "$w" != none ]; then
				"$OUT/load" -addr 127.0.0.1:$PORT -w "$w" -n "$N"
			fi
			spid=$(pgrep -P "$tpid")
			kill -TERM "$spid"
			wait "$tpid"
			n=$(awk '/instructions retired/{print $1}' "$OUT/time.txt")
			if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		done
		echo "$w $be $best"
	done
done
