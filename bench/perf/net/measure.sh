#!/bin/sh
# Measures bench/perf/net (decision 0206 stage 3) on Linux: builds the
# program with the xo1 binary given for each backend and each variant
# (xo: the Xo path; c: XO_LLRT_C=net, the C library's sockets, epoll, and
# getaddrinfo; nolibc: the Xo path linked with XO_NOLIBC=1), runs every
# workload 5 times, and prints "<workload> <backend> <variant> <min user
# instructions> <min kernel instructions> <min ms> <checksum>" (perf
# stat; run as a user who may count kernel events, e.g. with sudo or
# kernel.perf_event_paranoid <= 1).
#
# usage: bench/perf/net/measure.sh <xo1 binary> <std dir> <out dir> <backend...>
# WORKLOADS overrides the list; VARIANTS the variants.
set -e
XO=$1
STD=$2
OUT=$3
shift 3
SRC=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$OUT"
for be in "$@"; do
	for v in ${VARIANTS:-xo c nolibc}; do
		rm -rf "$OUT/src-$be-$v"
		mkdir -p "$OUT/src-$be-$v"
		cp "$SRC/main.xo" "$SRC/xo.mod" "$OUT/src-$be-$v/"
		case $v in
		c) env="XO_LLRT_C=net" ;;
		nolibc) env="XO_NOLIBC=1" ;;
		*) env="XO_LLRT_C=" ;;
		esac
		(cd "$OUT/src-$be-$v" && env $env "$XO" build --std="$STD" --backend="$be" -o "$OUT/t-$be-$v" . >/dev/null)
	done
done
for w in ${WORKLOADS:-none echo dial resolve}; do
	for be in "$@"; do
		for v in ${VARIANTS:-xo c nolibc}; do
			bu=""
			bk=""
			bt=""
			ck=""
			for k in 1 2 3 4 5; do
				(cd "$OUT" && perf stat -x, -e instructions:u,instructions:k,task-clock -o "$OUT/p.txt" "$OUT/t-$be-$v" "$w" >"$OUT/o.txt")
				u=$(awk -F, '/instructions:u/{print $1}' "$OUT/p.txt")
				kk=$(awk -F, '/instructions:k/{print $1}' "$OUT/p.txt")
				t=$(awk -F, '/task-clock/{printf "%d", $1}' "$OUT/p.txt")
				ck=$(awk '{print $2}' "$OUT/o.txt")
				if [ -z "$bu" ] || [ "$u" -lt "$bu" ]; then bu=$u; fi
				if [ -z "$bk" ] || [ "$kk" -lt "$bk" ]; then bk=$kk; fi
				if [ -z "$bt" ] || [ "$t" -lt "$bt" ]; then bt=$t; fi
			done
			echo "$w $be $v $bu $bk $bt $ck"
		done
	done
done
