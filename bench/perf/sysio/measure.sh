#!/bin/sh
# Measures bench/perf/sysio (decisions 0206 and 0213): builds the program
# with the xo1 binary given for each backend and each variant (xo: the
# Xo path; c: XO_LLRT_C=sysio; nolibc: the Xo path linked with
# XO_NOLIBC=1, Linux only), runs every workload 5 times in a fresh
# scratch directory, and prints "<workload> <backend> <variant> <min user
# instructions> <min kernel instructions> <min ms> <checksum>". On Linux
# the counts are perf stat's (run as a user who may count kernel events,
# e.g. with sudo or kernel.perf_event_paranoid <= 1). On macOS they are
# /usr/bin/time -l's: "instructions retired" of the process in the user
# column, "-" for kernel, and user plus system milliseconds.
# Decision 0198 deleted the C path of sysio (2026-10-13): naming it in
# XO_LLRT_C now builds the Xo path; earlier results files keep the comparisons.
#
# usage: bench/perf/sysio/measure.sh <xo1 binary> <std dir> <out dir> <backend...>
# WORKLOADS overrides the list; VARIANTS the variants.
set -e
XO=$1
STD=$2
OUT=$3
shift 3
SRC=$(cd "$(dirname "$0")" && pwd)
OS=$(uname -s)
DEFV="xo c nolibc"
if [ "$OS" = Darwin ]; then
	DEFV="xo c"
fi
mkdir -p "$OUT"
for be in "$@"; do
	for v in ${VARIANTS:-$DEFV}; do
		rm -rf "$OUT/src-$be-$v"
		mkdir -p "$OUT/src-$be-$v"
		cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be-$v/"
		case $v in
		c) env="XO_LLRT_C=sysio" ;;
		nolibc) env="XO_NOLIBC=1" ;;
		*) env="XO_LLRT_C=" ;;
		esac
		(cd "$OUT/src-$be-$v" && env $env "$XO" build --std="$STD" --backend="$be" -o "$OUT/t-$be-$v" . >/dev/null)
	done
done
for w in ${WORKLOADS:-none read read_big write stat clock chan sleep}; do
	for be in "$@"; do
		for v in ${VARIANTS:-$DEFV}; do
			bu=""
			bk=""
			bt=""
			ck=""
			for k in 1 2 3 4 5; do
				rm -rf "$OUT/scratch"
				mkdir -p "$OUT/scratch"
				if [ "$OS" = Darwin ]; then
					(cd "$OUT" && /usr/bin/time -l "$OUT/t-$be-$v" "$w" scratch >"$OUT/o.txt" 2>"$OUT/p.txt")
					u=$(awk '/instructions retired/{print $1}' "$OUT/p.txt")
					kk=-
					t=$(awk '/ real /{printf "%d", ($3 + $5) * 1000}' "$OUT/p.txt")
				else
					(cd "$OUT" && perf stat -x, -e instructions:u,instructions:k,task-clock -o "$OUT/p.txt" "$OUT/t-$be-$v" "$w" scratch >"$OUT/o.txt")
					u=$(awk -F, '/instructions:u/{print $1}' "$OUT/p.txt")
					kk=$(awk -F, '/instructions:k/{print $1}' "$OUT/p.txt")
					t=$(awk -F, '/task-clock/{printf "%d", $1}' "$OUT/p.txt")
				fi
				ck=$(awk '{print $2}' "$OUT/o.txt")
				if [ -z "$bu" ] || [ "$u" -lt "$bu" ]; then bu=$u; fi
				if [ "$kk" != - ] && { [ -z "$bk" ] || [ "$kk" -lt "$bk" ]; }; then bk=$kk; fi
				if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
			done
			echo "$w $be $v $bu ${bk:--} $bt $ck"
		done
	done
done
rm -rf "$OUT/scratch"
