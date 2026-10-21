#!/bin/sh
# listlock-docker.sh: GET /users encoded under the read lock against the
# shape before it (owner decision 2026-10-21): the CRUD servers of BASE
# (default origin/main; GET /users copied the users out, sorted them, and
# encoded after the lock) and of this checkout (encoded inside the read
# lock), Go and Xo (arm64), in scripts/docker-linux.sh's linux/arm64
# container on this Mac.
#
#   bench/perf/http/listlock-docker.sh XO OUT [BASE]
#
# XO is a darwin xo whose std is the checkout's; it cross builds both Xo
# servers for linux/arm64 with the checkout's std (only the bench code
# differs), and Go cross builds both Go servers, listload, and loadgen.
# In the container the servers run on the first half of the CPUs and the
# client on the second (taskset); LOADS "list mixed" (list: every
# connection loops GET /users over 1,032 preloaded users, listload; mixed:
# loadgen's CRUD cycle with its 64 preloaded users), CONC 1024, REPS 3
# rounds of DURATION 10s, the four servers interleaved in each round.
# Prints raw lines (requests or loadgen's JSON, and the server's CPU
# seconds) into OUT/results.txt.
set -eu
xo=$1
out=$2
base=${3:-origin/main}
root=$(cd "$(dirname "$0")/../../.." && pwd)
mkdir -p "$out/old" "$out/bin"
rm -rf "${out:?}/old/bench"
git -C "$root" archive "$base" bench/perf/http/xo bench/perf/http/go | tar -x -C "$out/old"
(cd "$root/bench/perf/http/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$out/bin/xo-new" .)
(cd "$out/old/bench/perf/http/xo" && XO_STD="$root/std" XO_BIN_CACHE=off "$xo" build --backend=arm64 --targets=linux/arm64 -o "$out/bin/xo-old" .)
(cd "$root/bench/perf/http/go" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/bin/go-new" .)
(cd "$out/old/bench/perf/http/go" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/bin/go-old" .)
(cd "$root/bench/perf/http/listload" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/bin/listload" .)
(cd "$root/bench/perf/http/loadgen" && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o "$out/bin/loadgen" .)
echo "base $(git -C "$root" rev-parse "$base") new $(git -C "$root" rev-parse HEAD)$(git -C "$root" diff --quiet HEAD -- bench/perf/http || echo ' dirty')" > "$out/stamp.txt"
cat > "$out/inside.sh" <<INSIDE
#!/bin/bash
set -uo pipefail
ncpu=\$(nproc)
half=\$((ncpu / 2))
srv=0-\$((half - 1))
gen=\$half-\$((ncpu - 1))
ulimit -n 65536 2>/dev/null || true
echo "machine: \$(uname -m) \$ncpu CPUs; servers on \$srv, client on \$gen; CONC ${CONC:-1024}; ${REPS:-3} rounds of ${DURATION:-10s}"
port=18400
for load in ${LOADS:-list mixed}; do
	for r in \$(seq 1 ${REPS:-3}); do
		for s in go-old go-new xo-old xo-new; do
			port=\$((port + 1))
			PORT=\$port taskset -c "\$srv" /work/bin/\$s > /work/\$s.log 2>&1 &
			p=\$!
			sleep 2
			kill -0 "\$p" 2>/dev/null || { echo "server \$s exited at start: \$(tail -3 /work/\$s.log)"; exit 1; }
			t0=\$(awk '{print \$14 + \$15}' /proc/\$p/stat)
			if [ "\$load" = list ]; then
				res=\$(taskset -c "\$gen" /work/bin/listload -url "http://127.0.0.1:\$port" -c ${CONC:-1024} -d ${DURATION:-10s} -preload 1032 2>&1 | tail -1)
			else
				res=\$(taskset -c "\$gen" /work/bin/loadgen -url "http://127.0.0.1:\$port" -c ${CONC:-1024} -d ${DURATION:-10s} -warmup 3s -pid "\$p" 2>/dev/null | tail -1)
			fi
			t1=\$(awk '{print \$14 + \$15}' /proc/\$p/stat)
			kill "\$p" 2>/dev/null; wait "\$p" 2>/dev/null
			echo "\$load round \$r \$s: \$res server_cpu_s \$(awk -v a="\$t0" -v b="\$t1" 'BEGIN{printf "%.1f", (b - a) / 100}')"
		done
	done
done
INSIDE
chmod +x "$out/inside.sh"
st=0
nice -n 10 "$root/scripts/docker-linux.sh" arm64 "$out" -- ./inside.sh > "$out/results.txt" 2>&1 || st=$?
cat "$out/results.txt"
exit $st
