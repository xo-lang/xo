#!/bin/sh
# Hybrid builds (decision 0084): the Go backend (`xo build`), the Go
# backend with hot leaf functions in Go assembler (`xo build --hybrid`),
# and the native arm64 backend (`xo build --backend=arm64`) on the compute
# workloads of bench/perf/llvm/xo and the hybrid friendly variants of
# xo/ here (sieve_split, interp_flat: the same work with the allocation
# moved out of the hot function). Each binary runs REPS times (default
# 7) under /usr/bin/time -l, interleaved (Go, hybrid, arm64, Go, ...);
# the table shows the median wall time (ms, process start included) and
# the median cycles (millions, "cycles elapsed"). Outputs must agree.
# Needs an arm64 host, .tools/bin/xo (or $XO), go, cc, python3.
# WORKLOADS selects some of: sort nbody sieve interp fib collatz
# sieve_split interp_flat. PERF_OUT keeps the binaries.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
xo=${XO:-$root/.tools/bin/xo}
works=${WORKLOADS:-sort nbody sieve interp fib collatz sieve_split interp_flat}
for w in $works; do
	src="$root/bench/perf/llvm/xo/$w"
	[ -d "$src" ] || src="$here/xo/$w"
	"$xo" build -o "$out/go-$w" "$src" >/dev/null
	"$xo" build --hybrid -o "$out/hy-$w" "$src" 2>"$out/hy-$w.txt" >/dev/null
	"$xo" build --backend=arm64 -o "$out/a64-$w" "$src" >/dev/null
done
python3 - "$out" "${REPS:-7}" $works <<'PY'
import re, statistics, subprocess, sys, time
out, reps, works = sys.argv[1], int(sys.argv[2]), sys.argv[3:]
def once(cmd):
    t = time.perf_counter()
    p = subprocess.run(["/usr/bin/time", "-l"] + cmd, capture_output=True, text=True)
    dt = (time.perf_counter() - t) * 1000
    if p.returncode != 0:
        sys.exit("%s failed: %s" % (cmd, p.stderr.strip()[:200]))
    m = re.search(r"(\d+)\s+cycles elapsed", p.stderr)
    return dt, (int(m.group(1)) / 1e6 if m else 0), p.stdout.strip()
print("| Workload | in assembly | Go ms | hybrid ms | arm64 ms | hybrid/Go | hybrid/arm64 | Go Mcyc | hybrid Mcyc | arm64 Mcyc |")
print("|---|---|---|---|---|---|---|---|---|---|")
for w in works:
    note = open(out + "/hy-" + w + ".txt").read().strip().replace("hybrid: ", "").replace(" in Go assembler", "").split(" (")[0]
    cmds = [[out + "/go-" + w], [out + "/hy-" + w], [out + "/a64-" + w]]
    ms, cyc, sums = [[], [], []], [[], [], []], [None] * 3
    for _ in range(reps):
        for i, c in enumerate(cmds):
            dt, cy, s = once(c)
            ms[i].append(dt)
            cyc[i].append(cy)
            sums[i] = s
    if len(set(sums)) != 1:
        sys.exit("output mismatch in %s: %s" % (w, sums))
    g, h, a = [statistics.median(x) for x in ms]
    gc, hc, ac = [statistics.median(x) for x in cyc]
    print("| %s | %s | %.1f | %.1f | %.1f | %.2fx | %.2fx | %.0f | %.0f | %.0f |" % (w, note, g, h, a, h / g, h / a, gc, hc, ac))
PY
