#!/bin/sh
# Measures the Go twin of bench/perf/unicode: builds go/main.go and runs
# every workload 5 times, printing "<workload> go <min instructions
# retired> <best us> <checksum>", the shape of measure.sh, so the Go row
# sits beside the arm64 and llvm rows. Instructions come from
# /usr/bin/time -l (darwin).
#
# usage: bench/perf/unicode/measurego.sh <out dir>
# example: bench/perf/unicode/measurego.sh /tmp/u
set -e
OUT=$1
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
(cd "$SRC/go" && go build -o "$OUT/u-go" .)
for w in lower upper lower_u upper_u eqlower classify runecase classes latin1 title utf8len quote indexfunc trimfunc fieldsfunc mapfunc; do
	best=""
	bt=""
	ck=""
	for k in 1 2 3 4 5; do
		/usr/bin/time -l "$OUT/u-go" "$w" >"$OUT/o.txt" 2>"$OUT/t.txt"
		n=$(awk '/instructions retired/{print $1}' "$OUT/t.txt")
		t=$(awk '{print $3}' "$OUT/o.txt")
		ck=$(awk '{print $2}' "$OUT/o.txt")
		if [ -z "$best" ] || [ "$n" -lt "$best" ]; then best=$n; fi
		if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
	done
	echo "$w go $best $bt $ck"
done
