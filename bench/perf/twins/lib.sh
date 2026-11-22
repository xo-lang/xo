# twins/lib.sh (sourced by rust/run.sh, node/run.sh, python/run.sh): the
# measurement helpers of the language comparison runs. The caller sets
# root, xo, rounds, maxload, out and result first.
#
# `xo bench --ab` runs A and B alternately for ROUNDS rounds and reports the
# median wall, cpu, instructions and max RSS of each, and B / A. A is the other
# language, B is Xo: a ratio below 1 means Xo is faster or smaller. Wall,
# instructions and RSS include the process start (Node about 40 ms, CPython
# about 15 ms). Every measurement waits for the 1 minute load average to
# drop under MAXLOAD.

# load: the 1 minute load average, from /proc/loadavg on linux (LOADAVG_FILE
# for tests), else macOS's vm.loadavg sysctl. A pipeline's status is its
# last command's, so sysctl ... | awk || fallback never fell back (#687).
load() {
	f=${LOADAVG_FILE:-/proc/loadavg}
	if [ -r "$f" ]; then awk '{print $1}' "$f"; else sysctl -n vm.loadavg 2>/dev/null | awk '{print $2}'; fi
}
# ncpu: the number of CPUs (NCPU for tests)
ncpu() { echo "${NCPU:-$(sysctl -n hw.ncpu 2>/dev/null || nproc)}"; }
wait_load() { while [ "$(load | awk '{print int($1)}')" -ge "$maxload" ]; do sleep 20; done; }

# reps ROW: the REPEAT count every program of ROW takes as its second
# argument (task #687): each runs the same work that many times in one
# process, so the faster side takes about 1 s and `xo bench --ab` gives a
# wall verdict (it reads "too short" under 100 ms). The compute and
# concurrency counts are scripts/release-vs-go.sh's (Go and Xo on host 1,
# 0.55.0); the JSON counts make Xo and Rust about 1 s. TWIN_REPEAT=N runs
# every row N times (1: the runs before #687).
reps() {
	if [ -n "${TWIN_REPEAT:-}" ]; then
		echo "$TWIN_REPEAT"
		return
	fi
	case $1 in
	fib) echo 5 ;; sieve | nbody) echo 6 ;; sort) echo 4 ;; interp) echo 24 ;; collatz) echo 3 ;; strings) echo 18 ;;
	spawn) echo 58 ;; fanout) echo 70 ;; pingpong) echo 65 ;; mutex) echo 278 ;; rwmutex) echo 202 ;; pmap) echo 19 ;;
	decode) echo 45 ;; encode) echo 85 ;;
	*) echo 1 ;;
	esac
}

# ab NAME A B ROW [REPEAT]: one `xo bench --ab` table, both programs run as
# `PROGRAM ROW REPEAT` (reps above unless REPEAT is given)
ab() {
	name=$1; a=$2; b=$3; row=$4
	n=${5:-$(reps "$row")}
	wait_load
	# rwmutex and pmap keep every core busy, so their own rounds lift the
	# 1 minute load past xo bench's default limit (the baseline plus 3) and
	# every round is left out ("no round was usable"; host 1, 12 cores,
	# task #687): those rows take --max-load at the core count plus maxload
	ml=
	case $row in rwmutex | pmap) ml=$(( $(ncpu) + ${maxload%.*} )) ;; esac
	echo "| $name, repeat $n | load $(load)${ml:+, max load $ml} |"
	r=$("$xo" bench --ab --rounds="$rounds" ${ml:+--max-load=$ml} "$a" "$b" -- "$row" "$n" 2>&1)
	t=$(printf '%s\n' "$r" | grep -E '^(wall_ms|cpu_ms|instructions|max_rss_kb)')
	# no table: print what xo bench said, so the row is not silently empty
	# (rust rwmutex on host 1, task #687)
	[ -n "$t" ] || t=$(printf 'no table; xo bench --ab said:\n%s' "$(printf '%s\n' "$r" | tail -5)")
	printf '%s\n' "$t" | sed 's/^/    /'
}

# agree NAME A B [ARG]: both programs print the same first line
agree() {
	name=$1; a=$2; b=$3; shift 3
	ra=$("$a" "$@" | head -1); rb=$("$b" "$@" | head -1)
	[ "$ra" = "$rb" ] || { echo "checksums differ for $name: '$ra' against '$rb'" >&2; exit 1; }
}

# wrapper FILE CMD...: an executable that execs CMD with its arguments, so a
# benchmark binary can be an interpreter and a script
wrapper() {
	f=$1; shift
	printf '#!/bin/sh\nexec' > "$f"
	for w in "$@"; do printf " '%s'" "$w" >> "$f"; done
	printf ' "$@"\n' >> "$f"
	chmod +x "$f"
}

# cpu: the CPU's name: darwin's brand string, linux's cpuinfo model name, or
# on linux/arm64, whose cpuinfo has none, HOSTLABEL or the implementer and part
cpu() {
	c=$(sysctl -n machdep.cpu.brand_string 2>/dev/null) || c=""
	[ -n "$c" ] || c=$(grep -m1 'model name' "${CPUINFO_FILE:-/proc/cpuinfo}" 2>/dev/null | cut -d: -f2 | sed 's/^ *//')
	[ -n "$c" ] || c=${HOSTLABEL:-}
	[ -n "$c" ] || c=$(awk -F': *' '/^CPU implementer/{i=$2} /^CPU part/{p=$2} END{if(i!="")printf "CPU implementer %s part %s", i, p; else printf "unknown CPU"}' "${CPUINFO_FILE:-/proc/cpuinfo}" 2>/dev/null)
	echo "$c"
}

# header LANGUAGE VERSIONS...: the machine and versions block
header() {
	lang=$1; shift
	echo "# $lang against Xo: $date"
	echo
	echo "Written by bench/perf/$(echo "$lang" | tr 'A-Z' 'a-z' | sed 's/ .*//')/run.sh (twins/lib.sh). Ratios are Xo over $lang (below 1: Xo is faster or smaller)."
	echo
	echo "## Machine and versions"
	echo
	echo "- $(uname -srm), $(cpu), $(ncpu) CPUs, load average at start $(load)"
	echo "- $XO_STAMP"
	echo "- $(go version)"
	for v in "$@"; do echo "- $v"; done
	echo
}

# xobuild, gobuild, rustbuild NAME: the build command of twins/build.sh's
# NAME row, every cache in its own empty directory under $work/cache-SIDE-NAME
# so the cold build is cold. xo keeps a run cache of whole binaries (decision
# 0360) apart from the back-end cache; without XO_BIN_CACHE the user's own one
# answered the "cold" build in 100 ms on host 1 (task #687, 2026-11-22)
xobuild() { c=$work/cache-xo-$1; rm -rf "${c:?}"; mkdir -p "$c"; echo env XO_BACKEND_CACHE_DIR="$c/backend" XO_BIN_CACHE="$c/bin" "$xo" build -o prog .; }
gobuild() { c=$work/cache-go-$1; rm -rf "${c:?}"; mkdir -p "$c"; echo env GOCACHE="$c/go" GOFLAGS=-trimpath go build -o prog .; }
rustbuild() { c=$work/cache-rust-$1; rm -rf "${c:?}"; echo env CARGO_TARGET_DIR="$c" "$cargo" build --release --locked --offline -q; }
