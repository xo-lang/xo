#!/bin/sh
# Measures bench/perf/stdio (the zero-C audit's work item B03): builds the
# program with the xo given against each std dir given (an earlier
# commit's std is the C baseline: its prelude has no llrt_print, so the
# same compiler calls the C runtime's xo_print), for each backend, runs
# every workload 5 times with standard output and error sent to
# /dev/null, and prints "<workload> <backend> <std label> <min user
# instructions> <min ms>". On macOS the counts are /usr/bin/time -l's
# "instructions retired"; on Linux perf stat's instructions:u.
#
# usage: bench/perf/stdio/measure.sh <xo> <out dir> <label=std dir>... -- <backend...>
# WORKLOADS overrides the list.
set -e
XO=$1
OUT=$2
shift 2
STDS=""
while [ "$1" != "--" ]; do
	STDS="$STDS $1"
	shift
done
shift
SRC=$(cd "$(dirname "$0")" && pwd)
OS=$(uname -s)
mkdir -p "$OUT"
for be in "$@"; do
	for ls in $STDS; do
		l=${ls%%=*}
		std=${ls#*=}
		rm -rf "$OUT/src-$be-$l"
		mkdir -p "$OUT/src-$be-$l"
		cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be-$l/"
		(cd "$OUT/src-$be-$l" && "$XO" build --std="$std" --backend="$be" -o "$OUT/t-$be-$l" . >/dev/null)
	done
done
for w in ${WORKLOADS:-none lines ints small big err mixed}; do
	for be in "$@"; do
		for ls in $STDS; do
			l=${ls%%=*}
			bu=""
			bt=""
			for k in 1 2 3 4 5; do
				if [ "$OS" = Darwin ]; then
					/usr/bin/time -l "$OUT/t-$be-$l" "$w" >/dev/null 2>"$OUT/p.txt"
					u=$(awk '/instructions retired/{print $1}' "$OUT/p.txt")
					t=$(awk '/ real /{printf "%d", ($3 + $5) * 1000}' "$OUT/p.txt")
				else
					perf stat -x, -e instructions:u,task-clock -o "$OUT/p.txt" "$OUT/t-$be-$l" "$w" >/dev/null 2>/dev/null
					u=$(awk -F, '/instructions:u/{print $1}' "$OUT/p.txt")
					t=$(awk -F, '/task-clock/{printf "%d", $1}' "$OUT/p.txt")
				fi
				if [ -z "$bu" ] || [ "$u" -lt "$bu" ]; then bu=$u; fi
				if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
			done
			echo "$w $be $l $bu $bt"
		done
	done
done
