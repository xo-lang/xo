#!/bin/sh
# Compare the LLVM backend prototype (`xo build --backend=llvm`, clang -O2)
# with the Go backend (`xo build`) and hand written Go on compute heavy
# workloads in the LLVM subset. Each Xo workload is its own program under
# xo/<name>; go/ has the Go versions behind one binary (`llperf <name>`).
#
# Every binary runs REPS times (default 5); the table shows the best wall
# clock time in milliseconds, process start included. A run longer than
# TIMEOUT seconds (default 60) is stopped and shown as ">TIMEOUT s".
# Checksums must agree. Needs .tools/bin/xo (or $XO), go, clang, python3.
# WORKLOADS selects some of: fib sieve nbody sort interp collatz.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
reps=${REPS:-5}
timeout=${TIMEOUT:-60}
xo=${XO:-$root/.tools/bin/xo}
(cd "$here/go" && go build -o "$out/llperf-go" .)
# best BIN [ARG]: "checksum best_ms", or "timeout -" when a run times out
best() {
	python3 - "$reps" "$timeout" "$@" <<'PY'
import subprocess, sys, time
reps, limit, cmd = int(sys.argv[1]), float(sys.argv[2]), sys.argv[3:]
best, out = None, None
for _ in range(reps):
    t = time.perf_counter()
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=limit)
    except subprocess.TimeoutExpired:
        print("timeout -")
        sys.exit(0)
    dt = (time.perf_counter() - t) * 1000
    if p.returncode != 0:
        print("failed(%d):%s -" % (p.returncode, p.stderr.strip().splitlines()[0] if p.stderr.strip() else ""))
        sys.exit(0)
    out = p.stdout.strip().replace(" ", "_")
    best = dt if best is None or dt < best else best
print("%s %.1f" % (out, best))
PY
}
fmt() { # ms -> text
	if [ "$1" = "-" ]; then echo ">${timeout}000"; else echo "$1"; fi
}
printf '| Workload | Go ms | Xo Go backend ms | Xo LLVM ms | LLVM/Go | Go backend/Go | Go backend/LLVM |\n|---|---|---|---|---|---|---|\n'
for w in ${WORKLOADS:-fib sieve nbody sort interp collatz}; do
	"$xo" build -o "$out/xo-go-$w" "$here/xo/$w" >/dev/null
	"$xo" build --backend=llvm -o "$out/xo-ll-$w" "$here/xo/$w" >/dev/null
	set -- $(best "$out/llperf-go" "$w")
	gs=$1 gt=$2
	set -- $(best "$out/xo-ll-$w")
	ls_=$1 lt=$2
	set -- $(best "$out/xo-go-$w")
	xs=$1 xt=$2
	if [ "$gs" != "$ls_" ]; then echo "checksum mismatch in $w: go $gs llvm $ls_" >&2; exit 1; fi
	if [ "$xt" != "-" ] && [ "$gs" != "$xs" ]; then echo "checksum mismatch in $w: go $gs xo $xs" >&2; exit 1; fi
	awk -v w="$w" -v g="$gt" -v x="$xt" -v l="$lt" -v to="$timeout" 'BEGIN {
		xs = (x == "-") ? sprintf(">%d000", to) : sprintf("%.1f", x)
		r1 = sprintf("%.2fx", l / g)
		r2 = (x == "-") ? sprintf(">%.0fx", to * 1000 / g) : sprintf("%.2fx", x / g)
		r3 = (x == "-") ? sprintf(">%.0fx", to * 1000 / l) : sprintf("%.2fx", x / l)
		printf "| %s | %.1f | %s | %.1f | %s | %s | %s |\n", w, g, xs, l, r1, r2, r3 }'
done
