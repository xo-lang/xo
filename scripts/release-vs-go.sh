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
# Test pairs (2026-10-30, the lead's approval after 0.35.0): `xo test
# --std-module=native/M` against `go test PKG` (the Go tests M was ported
# from) for base64 and json, under `xo bench --ab` (A = Go, B = Xo): cold
# (every cache emptied before each round, Go with -count=1) and edit (one
# function appended to the module under test before each round, in copies
# of GOROOT and xo's std).
# HTTP (2026-11-02, Lead): bench/perf/jitter's server (Xo at HTTP_REV, default
# the checkout's HEAD; Xo the offload host's seed, the release before; Go with
# the hand encoder) under its loadgen with -awake, on the first linux/arm64
# offload host with OFF_EXCLUSIVE=1 (loopback HTTP on the Mac runs out of
# ports, gotchas/loopback-time-wait-exhausts-ports.md): scripts/release-vs-go-
# http.sh, HTTP_ROUNDS rounds (default 5); the report gives each row's
# medians with their ranges. Run it before seed-hosts.sh moves the host's seed
# to the new release, or the base is the release itself.
# SECTIONS (default "compile run test http") limits the report's sections.
# ROUNDS (default 3, owner 2026-11-01) sets the --ab rounds; WORKLOADS limits the run
# workloads (names as below). Run it through heavy.sh as HEAVY_CLASS=measure:
# xo bench --ab refuses to run under a bulk or gate job (lead, 2026-10-30:
# a bulk run for 0.35.0 reported every run row as failed).
# MAX_LOAD (default 6): each build and run waits, up to WAIT_MIN minutes
# (default 30), for a 1-minute load average under MAX_LOAD, and the report
# gives the load each run started at (lead, 2026-10-23: the 0.26.0 report
# ran at load 14, and its three slower rows needed a rerun to trust).
set -euo pipefail
XO=${1:?usage: release-vs-go.sh XO OUT}
OUT=${2:?usage: release-vs-go.sh XO OUT}
ROUNDS=${ROUNDS:-3}
SECTIONS=${SECTIONS:-compile run test http}
section() { [[ " $SECTIONS " == *" $1 "* ]]; }
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
	local start=$(date +%s) end=$(($(date +%s) + WAIT_MIN * 60)) said=0
	while [ "$(load1)" -ge "$MAX_LOAD" ] && [ "$(date +%s)" -lt "$end" ]; do
		# progress on standard error (the callers capture standard output),
		# at the start and every 3 minutes: a quiet log looked hung to the
		# lead's hang check (2026-10-29, 29 quiet minutes behind load 6)
		local now=$(date +%s)
		if [ $((now - said)) -ge 180 ]; then
			echo "release-vs-go: waiting for load under $MAX_LOAD (load $(load1), $(((now - start) / 60)) min of $WAIT_MIN)" >&2
			said=$now
		fi
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
		# XO_BIN_CACHE=off: no whole-build run cache, which lives in the user
		# cache and served a second run of this script with the same OUT its
		# cold row in 70 ms (2026-10-29)
		build=(env XO_BACKEND_CACHE_DIR="$cache/backend" XO_BIN_CACHE=off "$XO" build -o "$bin" .)
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
		# the first separately compiled build writes the std records (stage
		# 2) and parses std for them: an untimed build of a one-function
		# edit (an unchanged build could be served from a cache), restored
		# after, so the edit row is the loader, a user's steady state
		printf '\nfn release_vs_go_records() -> Int {\n  1\n}\n' >>"$d/$mainf"
		(cd "$d" && "${build[@]}" >"$OUT/build-$side-$name-records.log" 2>&1) || true
		cp "$src/$mainf" "$d/$mainf"
	fi
	second=$(cd "$d" && timed "$OUT/build-$side-$name-second.log" "${build[@]}")
	warm=$(cd "$d" && timed "$OUT/build-$side-$name-warm.log" "${build[@]}")
	if [ "$side" = xo ]; then
		printf '\nfn release_vs_go_edit() -> Int {\n  7\n}\n' >>"$d/$mainf"
	else
		printf '\nfunc releaseVsGoEdit() int {\n\treturn 7\n}\n' >>"$d/$mainf"
	fi
	edit=$(cd "$d" && timed "$OUT/build-$side-$name-edit.log" "${build[@]}")
	# the path an edit takes, from an untimed build of another edit under
	# the cache statistics, which cost the timed rows about 28 MB of peak
	# RSS and 3% of the instructions (coll warm, 2026-11-01;
	# gotchas/cache-stats-raise-rss.md)
	if [ "$side" = xo ]; then
		printf '\nfn release_vs_go_path() -> Int {\n  8\n}\n' >>"$d/$mainf"
		(cd "$d" && env XO_CACHE_STATS=1 "${build[@]}" >"$OUT/build-$side-$name-path.log" 2>&1) || true
	fi
	# rebuild the unedited program for the size and run columns
	cp "$src/$mainf" "$d/$mainf"
	(cd "$d" && "${build[@]}" >/dev/null 2>&1)
	local size
	size=$(stat -f %z "$bin")
	# the path the edit build took, from its cache statistics
	# "cache std ... reused" prints with --phase-times only; the "cache
	# stdx" lookup line prints with the statistics and only on a separately
	# compiled build (the records' default)
	local path=-
	if [ "$side" = xo ]; then
		path=whole
		local elog=$OUT/build-$side-$name-path.log
		if grep -q '^cache std .* reused' "$elog" || grep -q '^cache stdx' "$elog"; then
			path=separate
			grep -q '^cache stdx record loaded' "$elog" && path="separate, std records"
			grep -q '^cache stdx no entry' "$elog" && path="separate, std records written"
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
	echo "Reports through v0.35.0 ran xo's timed builds under XO_CACHE_STATS=1, which added about"
	echo "28 MB to coll's warm peak (116 against 88 MB, 2026-11-01), so xo's peak MB steps down"
	echo "from those reports to this one with no change in the compiler."
	echo "The edit's path is what xo's cache statistics say an untimed build of another edit did: separate (against"
	echo "std's variants, built after the cold row as a user's second build would have them),"
	echo "with the std records, or whole (the whole program)."
	echo "xo's builds run with the whole-build run cache off (XO_BIN_CACHE=off), so the second and"
	echo "warm rows are compiles through the back-end caches, not a cached result (before 2026-10-29"
	echo "they could be the run cache's 10 to 20 ms)."
	echo "Size is the default build's binary."
	echo
	echo "## Compile and size"
	echo
	echo "| program | side | cold ms | second ms | warm ms | edit ms | peak MB (c/w/e) | size MB | edit's path |"
	echo "|---|---|---|---|---|---|---|---|---|"
} >"$report"

# the pairs: NAME XO_SRC GO_SRC
if section compile; then
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
fi

{
	echo
	echo "The Go column builds one binary per group (llvm, coll, conc); the Xo"
	echo "programs are one per workload except coll."
	echo
	echo "## Run (xo bench --ab, A = Go, B = Xo, $ROUNDS rounds)"
	echo
} >>"$report"

want() { section run && { [ -z "${WORKLOADS:-}" ] || [[ " $WORKLOADS " == *" $1 "* ]]; }; }
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

# the test pairs: xo test --std-module=native/M against go test PKG, in
# copies of GOROOT and of the release's std, which the edit rows change
if section test; then
	xstd=$(cd "$(dirname "$XO")/.." && pwd)/lib/xo/std
	[ -d "$xstd" ] || xstd=$(cd "$(dirname "$XO")/.." && pwd)/std
	cp -R "$xstd" "$work/xostd"
	cp -R "$(go env GOROOT)" "$work/goroot"
	tgo=$work/goroot/bin/go
	tx=$work/test-cache
	{
		echo "## Test (xo bench --ab, A = go test, B = xo test --std-module, $ROUNDS rounds)"
		echo
		echo "Cold empties every cache before each round (Go with -count=1); edit appends one"
		echo "function to the module under test before each round (Go -count=1). Copies of GOROOT"
		echo "and the release's std."
		echo
	} >>"$report"
	for p in base64:encoding/base64:base64.go json:encoding/json:encode.go; do
		m=${p%%:*}; r=${p#*:}; gp=${r%%:*}; gf=${r#*:}
		tenv=(env GOROOT="$work/goroot" GOCACHE="$tx/go" GOTOOLCHAIN=local GOFLAGS= XO_STD="$work/xostd" XO_BIN_CACHE="$tx/bin" XO_BACKEND_CACHE_DIR="$tx/backend" XO_SEPCOMP_BG=0)
		for row in cold edit; do
			if [ $row = cold ]; then
				guard="rm -rf $tx"
			else
				"${tenv[@]}" "$tgo" test -count=1 "$gp" >/dev/null 2>&1 || true
				# std's test variant, as a user's first `xo test` builds it in
				# the background: XO_SEPCOMP_BG=0 (no build racing the
				# rounds) starts none, so without these every edit round
				# built the whole program (2026-10-30: base64's edit row
				# read 1.267 of go test's wall)
				"${tenv[@]}" "$XO" dev stdcache --test --conc=0 >/dev/null 2>&1 || true
				"${tenv[@]}" "$XO" dev stdcache --test --conc=1 >/dev/null 2>&1 || true
				"${tenv[@]}" "$XO" test --std-module=native/$m >/dev/null 2>&1 || true
				guard="echo 'func releaseVsGoEdit'\$(date +%s)_\$\$'() int { return 1 }' >> $work/goroot/src/$gp/$gf; echo 'fn release_vs_go_edit_'\$(date +%s)_\$\$'() -> Int { 1 }' >> $work/xostd/native/$m.xo"
			fi
			l=$(settle)
			{
				echo "### $m ($gp) $row (load $l at start)"
				echo
				echo '```'
				"${tenv[@]}" "$XO" bench --ab --no-guard --keep-loaded --rounds="$ROUNDS" --warmup=1 --metrics=wall_ms,cpu_ms,max_rss_kb --args-a="test -count=1 $gp" --args-b="test --std-module=native/$m" --guard="$guard" "$tgo" "$XO" 2>&1 | grep -v '^round ' | sed "s#$work/##g" || echo "(failed)"
				echo '```'
				echo
			} >>"$report"
		done
	done
fi
# the http section: on the offload host (its seed is the base release)
if section http; then
	{
		echo "## HTTP server (bench/perf/jitter, linux/arm64 offload host)"
		echo
	} >>"$report"
	agents=$HOME/xo-lead/agents
	if [ ! -x "$agents/offload.sh" ]; then
		echo "(skipped: no $agents/offload.sh)" >>"$report"
	else
		rev=$(git -C "$root" rev-parse --verify "${HTTP_REV:-HEAD}^{commit}")
		(
			. "$HOME/xo-lead/offload.env"
			b=rvg-http-$$
			git -C "$root" branch -q -f "$b" "$rev"
			bundle=$(mktemp "${TMPDIR:-/tmp}/rvg-http.XXXXXX")
			git -C "$root" bundle create -q "$bundle" "$b"
			git -C "$root" branch -q -D "$b"
			put=/work/in/rvg-http-$$.bundle
			"$agents/offload.sh" --put "$bundle" "$put" || { rm -f "$bundle"; exit 4; }
			rm -f "$bundle"
			OFF_EXCLUSIVE=1 "$agents/offload.sh" "$root/scripts/release-vs-go-http.sh" "$rev" "rvg$$" "$OFF_SEED" /work/priv /work "$put" "${HTTP_ROUNDS:-5}"
		) >"$OUT/http.txt" 2>&1 || true
		python3 - "$OUT/http.txt" >>"$report" <<'PY'
import re, json, statistics as st, sys
lines = open(sys.argv[1], errors="replace").read().splitlines()
R = {}
for l in lines:
    m = re.match(r"(xo|base|go) (\S+) c(\d+) rate(\d+) r(\d+) (\{.*\})", l)
    if m:
        R.setdefault((m[2], int(m[3]), int(m[4])), {}).setdefault(m[1], []).append(json.loads(m[6]))
for l in lines:
    if l.startswith(("host load", "release-vs-go-http ", "xo ", "loadgen flags")) and not re.match(r"(xo|base|go) \S+ c\d+ ", l):
        print("- " + l)
if not R:
    print("\n(no runs: see http.txt)\n" + "\n".join("    " + l for l in lines[-15:]))
    sys.exit()
print("- Go's /alloc rows are its /alloc-hand (json.Marshal written by hand, as Xo's derive encodes).")
print("- Rounds: %d per row; each cell is the median, its range in brackets; CPU is the server's CPU microseconds per request.\n" % max(len(v) for x in R.values() for v in x.values()))
F = [("rps", "req/s", 0), ("p50_us", "p50 us", 0), ("p99_us", "p99 us", 0), ("p999_us", "p99.9 us", 0), ("max_us", "max us", 0), ("cpu_us_per_req", "CPU us/req", 1), ("errors", "errors", 0)]
print("| row | server | " + " | ".join(f[1] for f in F) + " |")
print("|---|---|" + "---|" * len(F))
for k in sorted(R):
    row = f"{k[0]} c{k[1]}" + (f" open {k[2]}/s" if k[2] else "")
    for s in ("xo", "base", "go"):
        v = R[k].get(s)
        if not v:
            continue
        cells = []
        for n, _, d in F:
            x = [a.get(n, 0) for a in v]
            cells.append(f"{st.median(x):,.{d}f} [{min(x):,.{d}f} to {max(x):,.{d}f}]")
        print(f"| {row} | {s} | " + " | ".join(cells) + " |")
print()
for k in sorted(R):
    med = {(s, n): st.median([a.get(n, 0) for a in v]) for s, v in R[k].items() for n, _, _ in F}
    row = f"{k[0]} c{k[1]}" + (f" open {k[2]}/s" if k[2] else "")
    for b, a in (("xo", "go"), ("xo", "base")):
        if (a, "rps") in med and (b, "rps") in med:
            print(f"- {row}, {b}/{a} medians: " + ", ".join(f"{lab} {med[(b, n)] / med[(a, n)]:.3f}" for n, lab, _ in F if n != "errors" and med[(a, n)]))
PY
	fi
fi
rm -rf "${work:?}"
echo "release-vs-go: $report"
