#!/bin/sh
# Server performance rows for std/http's Proven evidence (decision 0616,
# "Servers"), read by `go run ./scripts/stdproven serverperf http`. Runs
# three twins against Go on the native backends (arm64, llvm):
#
#   crud    bench/perf/http/run.sh, the CRUD API server under loadgen
#           (CONFIGS "go xo-arm64 xo-llvm", REPS runs, medians):
#           requests per second and the server's CPU microseconds per
#           request, p99 latency shown beside them
#   proxy   bench/perf/httpproxy/measure.sh: a 1 GiB body through Xo's
#           reverse_proxy and Go's httputil.ReverseProxy (best wall of 5)
#   stream  bench/perf/httpstream/measure.sh: a body read by Xo's client
#           and Go's, 2 GiB streamed and 1 GiB whole (best wall of 5)
#
# Prints one line per measure:
#   row WORKLOAD BACKEND METRIC GO XO
# METRIC is rps (more is better), cpu_us or wall_s (less is better), or
# p99_us (shown, not judged). Environment: XO (the xo binary, default
# bin/xo of the repository), REPS (5), OUT (a scratch directory).
set -e
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
XO=${XO:-$root/bin/xo}
REPS=${REPS:-5}
OUT=${OUT:-$(mktemp -d)}
mkdir -p "$OUT"

# crud: run.sh's Markdown table, | Config | Req/s | p50 | p99 | ... | CPU us/req |
XO=$XO REPS=$REPS CONFIGS="go xo-arm64 xo-llvm" PERF_OUT="$OUT/crud" sh "$here/run.sh" >"$OUT/crud.txt" 2>"$OUT/crud.err" || {
	tail -20 "$OUT/crud.err" >&2
	exit 1
}
awk -F'|' '
	function t(s) { gsub(/^ +| +$/, "", s); return s }
	NF >= 14 && t($2) ~ /^(go|xo-arm64|xo-llvm)$/ {
		c = t($2); rps[c] = t($3); p99[c] = t($5); cpu[c] = t($14)
	}
	END {
		if (!("go" in rps)) { print "crud: no go row" > "/dev/stderr"; exit 1 }
		n = split("xo-arm64 xo-llvm", cs, " ")
		for (i = 1; i <= n; i++) {
			c = cs[i]; be = substr(c, 4)
			if (!(c in rps)) { print "crud: no " c " row" > "/dev/stderr"; exit 1 }
			print "row crud", be, "rps", rps["go"], rps[c]
			print "row crud", be, "cpu_us", cpu["go"], cpu[c]
			print "row crud", be, "p99_us", p99["go"], p99[c]
		}
	}' "$OUT/crud.txt"

# proxy: "<proxy> <min wall s> <peak RSS bytes>", gotwin is Go's
# (1 GiB: 100 MB crosses loopback in about 25 ms, too short to judge)
SIZE=${PROXY_SIZE:-1073741824} sh "$here/../httpproxy/measure.sh" "$XO" "$OUT/proxy" arm64 llvm >"$OUT/proxy.txt"
awk '{ w[$1] = $2 } END { print "row proxy arm64 wall_s", w["gotwin"], w["arm64"]; print "row proxy llvm wall_s", w["gotwin"], w["llvm"] }' "$OUT/proxy.txt"

# stream: "<client> <mode> <min wall s> <min peak RSS bytes>"
# (2 GiB streamed, 1 GiB whole: 100 MB streams in about 11 ms, and
# 512 MiB whole took 42 ms once the client read into one allocation)
STREAM_SIZE=${STREAM_SIZE:-2147483648} SEND_SIZE=${SEND_SIZE:-1073741824} sh "$here/../httpstream/measure.sh" "$XO" "$OUT/stream" arm64 llvm >"$OUT/stream.txt"
awk '{ w[$1 " " $2] = $3 }
	END {
		split("stream send", ms, " ")
		for (i = 1; i <= 2; i++) {
			m = ms[i]
			print "row stream-" m, "arm64 wall_s", w["gotwin " m], w["arm64 " m]
			print "row stream-" m, "llvm wall_s", w["gotwin " m], w["llvm " m]
		}
	}' "$OUT/stream.txt"
