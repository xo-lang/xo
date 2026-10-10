#!/bin/sh
# Concurrency benchmarks: the native task runtime (std/llrt/xo_rt.c,
# decisions 0068, 0069, 0077) against the Go backend and idiomatic Go.
# Each Xo workload is its own program under xo/<name>; go/ has the Go
# versions behind one binary (`concperf <name>`).
#
# Configurations: go (plain Go), xo-go (`xo build`), xo-llvm and xo-arm64
# (native), and xo-arm64-1 (arm64 with XO_THREADS=1). Every binary runs
# REPS times (default 7), interleaved: each round runs every workload in
# every configuration once, so load from other processes on the host hits
# all of them alike. The tables show medians of wall time, CPU time (user
# plus system), and cycles (`/usr/bin/time -l`, Darwin) in milliseconds
# and millions. Outputs must agree. Needs .tools/bin/xo (or $XO), go,
# clang, python3. WORKLOADS selects some of: spawn fanout pingpong mutex
# rwmutex pmap; CONFIGS some of the configurations. KEEP=dir keeps the binaries.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${KEEP:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
xo=${XO:-$root/.tools/bin/xo}
workloads=${WORKLOADS:-spawn fanout pingpong mutex rwmutex pmap}
configs=${CONFIGS:-go xo-go xo-llvm xo-arm64 xo-arm64-1}
(cd "$here/go" && go build -o "$out/concperf-go" .)
for w in $workloads; do
	"$xo" build -o "$out/xo-go-$w" "$here/xo/$w" >/dev/null
	"$xo" build --backend=llvm -o "$out/xo-llvm-$w" "$here/xo/$w" >/dev/null
	"$xo" build --backend=arm64 -o "$out/xo-arm64-$w" "$here/xo/$w" >/dev/null
done
echo "uptime: $(uptime)"
python3 - "$out" "${REPS:-7}" "$workloads" "$configs" <<'PY'
import os, re, resource, statistics, subprocess, sys, time
out, reps, workloads, configs = sys.argv[1], int(sys.argv[2]), sys.argv[3].split(), sys.argv[4].split()

def cmd(c, w):
    env = dict(os.environ)
    if c == "go":
        return [out + "/concperf-go", w], env
    if c == "xo-arm64-1":
        env["XO_THREADS"] = "1"
        return [out + "/xo-arm64-" + w], env
    return [out + "/" + c + "-" + w], env

res = {}
for _ in range(reps):
    for w in workloads:
        for c in configs:
            argv, env = cmd(c, w)
            r0, t0 = resource.getrusage(resource.RUSAGE_CHILDREN), time.perf_counter()
            p = subprocess.run(["/usr/bin/time", "-l"] + argv, capture_output=True, text=True, env=env, timeout=120)
            real, r1 = time.perf_counter() - t0, resource.getrusage(resource.RUSAGE_CHILDREN)
            if p.returncode != 0:
                sys.exit("%s %s failed: %s" % (c, w, p.stderr[-400:]))
            e = p.stderr
            cyc = re.search(r"(\d+)\s+cycles elapsed", e)
            cyc = int(cyc.group(1)) if cyc else 0
            r = res.setdefault((w, c), {"out": p.stdout.strip(), "wall": [], "cpu": [], "cyc": []})
            if r["out"] != p.stdout.strip():
                sys.exit("output differs: %s %s" % (c, w))
            r["wall"].append(real * 1000)
            r["cpu"].append((r1.ru_utime - r0.ru_utime + r1.ru_stime - r0.ru_stime) * 1000)
            r["cyc"].append(cyc / 1e6)
    for w in workloads:
        outs = {res[(w, c)]["out"] for c in configs}
        if len(outs) != 1:
            sys.exit("outputs differ in %s: %s" % (w, outs))

med = lambda w, c, k: statistics.median(res[(w, c)][k])
for k, title in (("wall", "wall ms"), ("cpu", "CPU ms (user + sys)"), ("cyc", "Mcycles")):
    print("\nMedian %s of %d runs\n" % (title, reps))
    hdr = "| Workload | " + " | ".join(configs)
    ratios = [c for c in configs if c.startswith("xo-") and c != "xo-go" and "xo-go" in configs]
    hdr += "".join(" | %s/xo-go" % c for c in ratios) + " |"
    print(hdr)
    print("|---" * (1 + len(configs) + len(ratios)) + "|")
    for w in workloads:
        row = "| %s | " % w + " | ".join("%.1f" % med(w, c, k) for c in configs)
        row += "".join(" | %.2fx" % (med(w, c, k) / max(med(w, "xo-go", k), 1e-9)) for c in ratios)
        print(row + " |")
PY
