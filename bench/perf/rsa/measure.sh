#!/bin/sh
# Measures bench/perf/rsa: builds the program with each backend named
# and runs every workload 5 times with N operations; prints "<workload>
# <backend> <min instructions retired> <instructions per operation>"
# (the process alone, workload none, taken off).
#
# usage: bench/perf/rsa/measure.sh <xo binary> <out dir> <backend...>
set -e
XO=$1
OUT=$2
shift 2
SRC=$(cd "$(dirname "$0")" && pwd)
N=${N:-50}
mkdir -p "$OUT"
for be in "$@"; do
	rm -rf "$OUT/src-$be"
	mkdir -p "$OUT/src-$be"
	cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be/"
	(cd "$OUT/src-$be" && "$XO" build --backend="$be" -o "$OUT/r-$be" .)
done
count() {
	if [ "$(uname)" = Darwin ]; then
		N=$3 /usr/bin/time -l "$2" "$1" >"$OUT/o.txt" 2>"$OUT/t.txt"
		awk '/instructions retired/{print $1}' "$OUT/t.txt"
	else
		N=$3 perf stat -x, -e instructions:u -o "$OUT/t.txt" "$2" "$1" >"$OUT/o.txt" 2>/dev/null
		awk -F, '/instructions/{print $1}' "$OUT/t.txt"
	fi
}
for be in "$@"; do
	base=""
	for k in 1 2 3 4 5; do
		c=$(count none "$OUT/r-$be" 0)
		if [ -z "$base" ] || [ "$c" -lt "$base" ]; then base=$c; fi
	done
	for w in ${WORKLOADS:-sign2048 verify2048 sign4096 verify4096}; do
		best=""
		for k in 1 2 3 4 5; do
			c=$(count "$w" "$OUT/r-$be" "$N")
			grep -q "of $N" "$OUT/o.txt" || { echo "$w $be: $(cat "$OUT/o.txt")"; exit 1; }
			grep -q " $N of $N" "$OUT/o.txt" || { echo "$w $be wrong: $(cat "$OUT/o.txt")"; exit 1; }
			if [ -z "$best" ] || [ "$c" -lt "$best" ]; then best=$c; fi
		done
		echo "$w $be $best $(( (best - base) / N ))"
	done
done
