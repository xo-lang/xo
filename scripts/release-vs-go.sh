#!/bin/bash
# release-vs-go.sh XO OUT: the release's Xo against Go comparison (owner,
# 2026-10-22; a standing release step, toolchain.md "Cutting a release").
# XO is the released xo (its std beside it), OUT an empty directory for the
# report (OUT/report.md) and the raw files. Go is the `go` on PATH.
#
# Pairs: the Go-twin programs of bench/perf: llvm/xo/<w> against llvm/go,
# xo against go (collections), conc/xo/<w> against conc/go. Per pair:
#   - compile: cold (empty caches), second (again, unchanged), warm (a
#     third, unchanged: xo writes a directory's parts pack on its second
#     build and reads it from the third, bcache.xo pack_dir) and edit (one
#     new function appended to a copy of the main file), wall time and
#     peak RSS of the build (/usr/bin/time -l);
#   - binary size (default build; Go's default keeps its symbols too);
#   - run: `xo bench --ab GO XO -- WORKLOAD` (decision 0340): wall, CPU,
#     peak RSS, ratio XO/GO with its interval and verdict.
# ROUNDS (default 10) sets the --ab rounds; WORKLOADS limits the run
# workloads (names as below). Run it through heavy.sh (HEAVY_CLASS=bulk).
# MAX_LOAD (default 6): each build and run waits, up to WAIT_MIN minutes
# (default 30), for a 1-minute load average under MAX_LOAD, and the report
# gives the load each run started at (lead, 2026-10-23: the 0.26.0 report
# ran at load 14, and its three slower rows needed a rerun to trust).
set -euo pipefail
XO=${1:?usage: release-vs-go.sh XO OUT}
OUT=${2:?usage: release-vs-go.sh XO OUT}
ROUNDS=${ROUNDS:-10}
MAX_LOAD=${MAX_LOAD:-6}
WAIT_MIN=${WAIT_MIN:-30}
root=$(cd "$(dirname "$0")/.." && pwd)
perf=$root/bench/perf
mkdir -p "$OUT"
if [ -n "$(ls -A "$OUT")" ]; then
	echo "release-vs-go: $OUT is not empty" >&2
	exit 2
fi
XO=$(cd "$(dirname "$XO")" && pwd)/$(basename "$XO")
work=$OUT/work
mkdir -p "$work"
report=$OUT/report.md

# timed LOG CMD...: runs CMD under /usr/bin/time -l; prints "ms peak_mb"
timed() {
	local log=$1
	shift
	/usr/bin/time -l "$@" >"$log" 2>&1
	# time's own lines are the last of the log: "N.NN real ..." and the
	# largest resident set in bytes
	local ms rss
	ms=$(awk '$2 == "real" {printf "%d", $1 * 1000}' "$log" | tail -1)
	rss=$(awk '/maximum resident set size/ {print $1}' "$log" | tail -1)
	echo "${ms:-0} $((${rss:-0} / 1048576))"
}

load1() { sysctl -n vm.loadavg | awk '{print int($2)}'; }
# settle: waits up to WAIT_MIN minutes for a load under MAX_LOAD; prints
# the load it starts at
settle() {
	local end=$(($(date +%s) + WAIT_MIN * 60))
	while [ "$(load1)" -ge "$MAX_LOAD" ] && [ "$(date +%s)" -lt "$end" ]; do
		sleep 15
	done
	load1
}

# compile SIDE NAME SRCDIR MAINFILE: cold, second, warm and edit builds of a copy
# of SRCDIR; prints a table row and leaves the binary at $work/NAME.SIDE
compile() {
	local side=$1 name=$2 src=$3 mainf=$4
	local d=$work/src-$side-$name
	rm -rf "${d:?}"
	cp -R "$src" "$d"
	local bin=$work/$name.$side
	local cache=$work/cache-$side-$name
	mkdir -p "$cache"
	local build
	if [ "$side" = xo ]; then
		# the cache statistics name the path the build took (its last lines
		# in the log): "cache std ... reused" is separate compilation against
		# std's variants, "cache stdx record loaded" the std records
		build=(env XO_BACKEND_CACHE_DIR="$cache/backend" XO_CACHE_STATS=1 "$XO" build -o "$bin" .)
	else
		build=(env GOCACHE="$cache/go" GOFLAGS=-trimpath go build -o "$bin" .)
	fi
	local cold second warm edit
	settle >/dev/null
	cold=$(cd "$d" && timed "$OUT/build-$side-$name-cold.log" "${build[@]}")
	if [ "$side" = xo ]; then
		# std's variants (the store and facts of separate compilation,
		# decision 0388): a fresh xo has none and its first builds start
		# them in the background while building the whole program, so the
		# second, warm and edit rows of an unprepared run were whole-program
		# builds (gotchas/sepcomp-default-check-needs-std-variants.md);
		# built here, after the cold row, as a user's second build would
		# have them after a few seconds
		for conc in 0 1; do
			(cd "$d" && env XO_BACKEND_CACHE_DIR="$cache/backend" XO_BACKEND_CACHE=on "$XO" dev stdcache --conc=$conc >"$OUT/stdcache-$name-$conc.log" 2>&1) || echo "stdcache --conc=$conc failed for $name (see $OUT/stdcache-$name-$conc.log)" >&2
		done
	fi
	second=$(cd "$d" && timed "$OUT/build-$side-$name-second.log" "${build[@]}")
	warm=$(cd "$d" && timed "$OUT/build-$side-$name-warm.log" "${build[@]}")
	if [ "$side" = xo ]; then
		printf '\nfn release_vs_go_edit() -> Int {\n  7\n}\n' >>"$d/$mainf"
	else
		printf '\nfunc releaseVsGoEdit() int {\n\treturn 7\n}\n' >>"$d/$mainf"
	fi
	edit=$(cd "$d" && timed "$OUT/build-$side-$name-edit.log" "${build[@]}")
	# rebuild the unedited program for the size and run columns
	cp "$src/$mainf" "$d/$mainf"
	(cd "$d" && "${build[@]}" >/dev/null 2>&1)
	local size
	size=$(stat -f %z "$bin")
	# the path the edit build took, from its cache statistics
	local path=-
	if [ "$side" = xo ]; then
		path=whole
		if grep -q '^cache std .* reused' "$OUT/build-$side-$name-edit.log"; then
			path=separate
			grep -q '^cache stdx record loaded' "$OUT/build-$side-$name-edit.log" && path="separate, std records"
		fi
	fi
	set -- $cold $second $warm $edit
	echo "| $name | $side | $1 | $3 | $5 | $7 | $2 / $6 / $8 | $(python3 -c "print(f'{$size/1048576:.2f}')") | $path |"
}

{
	echo "# Xo against Go: $("$XO" version 2>&1 | head -1) ($(date +%Y-%m-%d))"
	echo
	echo "scripts/release-vs-go.sh on $(uname -sm), $(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%d GB", $1/1073741824}'),"
	echo "Go $(go env GOVERSION). Build times are wall milliseconds of one build; peak MB is the"
	echo "build's largest resident set (cold / warm / edit). Warm is the third unchanged build"
	echo "(xo's parts pack is written on the second and read from the third); second is the second."
	echo "The edit's path is what xo's cache statistics say the edit build did: separate (against"
	echo "std's variants, built after the cold row as a user's second build would have them),"
	echo "with the std records, or whole (the whole program)."
	echo "Size is the default build's binary."
	echo
	echo "## Compile and size"
	echo
	echo "| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |"
	echo "|---|---|---|---|---|---|---|---|---|"
} >"$report"

# the pairs: NAME XO_SRC GO_SRC
compile go llvm "$perf/llvm/go" main.go >>"$report"
compile go coll "$perf/go" main.go >>"$report"
compile go conc "$perf/conc/go" main.go >>"$report"
for w in fib sieve nbody sort interp collatz strings; do
	compile xo "llvm-$w" "$perf/llvm/xo/$w" main.xo >>"$report"
done
compile xo coll "$perf/xo" main.xo >>"$report"
for w in spawn fanout pingpong mutex rwmutex pmap; do
	compile xo "conc-$w" "$perf/conc/xo/$w" main.xo >>"$report"
done

{
	echo
	echo "The Go column builds one binary per group (llvm, coll, conc); the Xo"
	echo "programs are one per workload except coll."
	echo
	echo "## Run (xo bench --ab, A = Go, B = Xo, $ROUNDS rounds)"
	echo
} >>"$report"

want() { [ -z "${WORKLOADS:-}" ] || [[ " $WORKLOADS " == *" $1 "* ]]; }
ab() { # NAME GOBIN XOBIN ARG
	want "$4" || return 0
	local l
	l=$(settle)
	echo "### $1 $4 (load $l at start)" >>"$report"
	echo >>"$report"
	echo '```' >>"$report"
	"$XO" bench --ab --rounds="$ROUNDS" --metrics=wall_ms,cpu_ms,max_rss_kb "$2" "$3" -- "$4" 2>&1 | grep -v '^round ' | sed "s#$work/##g" >>"$report" || echo "(failed)" >>"$report"
	echo '```' >>"$report"
	echo >>"$report"
}
for w in fib sieve nbody sort interp collatz strings; do
	ab llvm "$work/llvm.go" "$work/llvm-$w.xo" "$w"
done
for w in putget get push update iter copymut branch; do
	ab coll "$work/coll.go" "$work/coll.xo" "$w"
done
for w in spawn fanout pingpong mutex rwmutex pmap; do
	ab conc "$work/conc.go" "$work/conc-$w.xo" "$w"
done
rm -rf "${work:?}"
echo "release-vs-go: $report"
