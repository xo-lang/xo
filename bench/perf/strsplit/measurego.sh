#!/bin/sh
# Measures the Go twin of bench/perf/strsplit: builds twin.go.txt and runs
# every workload 5 times, printing "<workload> go <min instructions
# retired> <best us> <checksum>", the shape of measure.sh, so the Go row
# sits beside the arm64 and llvm rows. Instructions come from
# /usr/bin/time -l (darwin).
#
# usage: bench/perf/strsplit/measurego.sh <out dir>
set -e
OUT=$1
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT/twin"
cp "$SRC/twin.go.txt" "$OUT/twin/main.go"
(cd "$OUT/twin" && go build -o "$OUT/u-go" main.go)
for w in split split_one split_multi split_absent split_empty split_after lines bytes_split bytes_fields fields index; do
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
