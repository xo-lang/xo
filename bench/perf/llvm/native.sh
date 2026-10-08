#!/bin/sh
# Compare the native arm64 backend (`xo build --backend=arm64`, decision
# 0049) with the LLVM backend (`--backend=llvm`) and hand written Go on the
# workloads of this directory. Each binary runs REPS times (default 5)
# under /usr/bin/time -l, interleaved (Go, LLVM, arm64, Go, ...); the
# table shows the best wall time in milliseconds and the peak resident
# memory in MB (STAT=median shows the median wall time instead of the
# best). Checksums must agree. Needs an arm64 host, .tools/bin/xo
# (or $XO), go, clang, cc, python3. WORKLOADS selects some of: fib sieve
# nbody sort interp collatz strings. PERF_OUT keeps the binaries.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
xo=${XO:-$root/.tools/bin/xo}
works=${WORKLOADS:-fib sieve nbody sort interp collatz strings}
(cd "$here/go" && go build -o "$out/llperf-go" .)
for w in $works; do
	"$xo" build --backend=llvm -o "$out/ll-$w" "$here/xo/$w" >/dev/null
	"$xo" build --backend=arm64 -o "$out/a64-$w" "$here/xo/$w" >/dev/null
done
python3 - "$out" "${REPS:-5}" "${STAT:-best}" $works <<'PY'
import re, statistics, subprocess, sys, time
out, reps, stat, works = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4:]
def once(cmd):
    t = time.perf_counter()
    p = subprocess.run(["/usr/bin/time", "-l"] + cmd, capture_output=True, text=True)
    dt = (time.perf_counter() - t) * 1000
    m = re.search(r"(\d+)\s+maximum resident set size", p.stderr)
    if p.returncode != 0:
        sys.exit("%s failed: %s" % (cmd, p.stderr.strip()[:200]))
    return dt, (int(m.group(1)) / 1e6 if m else 0), p.stdout.strip()
print("| Workload | Go ms | LLVM ms | arm64 ms | arm64/LLVM | arm64/Go | Go MB | LLVM MB | arm64 MB |")
print("|---|---|---|---|---|---|---|---|---|")
for w in works:
    cmds = [[out + "/llperf-go", w], [out + "/ll-" + w], [out + "/a64-" + w]]
    times, rss, sums = [[], [], []], [0.0] * 3, [None] * 3
    for _ in range(reps):
        for i, c in enumerate(cmds):
            dt, r, s = once(c)
            times[i].append(dt)
            rss[i], sums[i] = max(rss[i], r), s
    pick = statistics.median if stat == "median" else min
    best = [pick(t) for t in times]
    if len(set(sums)) != 1:
        sys.exit("checksum mismatch in %s: %s" % (w, sums))
    g, l, a = best
    print("| %s | %.1f | %.1f | %.1f | %.2fx | %.2fx | %.1f | %.1f | %.1f |" % (w, g, l, a, a / l, a / g, rss[0], rss[1], rss[2]))
PY
