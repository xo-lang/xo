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

load() { sysctl -n vm.loadavg 2>/dev/null | awk '{print $2}' || awk '{print $1}' /proc/loadavg; }
wait_load() { while [ "$(load | awk '{print int($1)}')" -ge "$maxload" ]; do sleep 20; done; }

# ab NAME A B [ARG]: one `xo bench --ab` table
ab() {
	name=$1; a=$2; b=$3; shift 3
	wait_load
	echo "| $name | load $(load) |"
	"$xo" bench --ab --rounds="$rounds" "$a" "$b" ${1:+-- "$@"} 2>&1 | grep -E '^(wall_ms|cpu_ms|instructions|max_rss_kb)' | sed 's/^/    /'
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

# header LANGUAGE VERSIONS...: the machine and versions block
header() {
	lang=$1; shift
	echo "# $lang against Xo: $date"
	echo
	echo "Written by bench/perf/$(echo "$lang" | tr 'A-Z' 'a-z' | sed 's/ .*//')/run.sh (twins/lib.sh). Ratios are Xo over $lang (below 1: Xo is faster or smaller)."
	echo
	echo "## Machine and versions"
	echo
	echo "- $(uname -srm), $(sysctl -n machdep.cpu.brand_string 2>/dev/null || grep -m1 'model name' /proc/cpuinfo | cut -d: -f2), $(sysctl -n hw.ncpu 2>/dev/null || nproc) CPUs, load average at start $(load)"
	echo "- $XO_STAMP"
	echo "- $(go version)"
	for v in "$@"; do echo "- $v"; done
	echo
}
