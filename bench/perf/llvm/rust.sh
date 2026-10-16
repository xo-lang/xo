#!/bin/sh
# Compare Xo's native backends with hand written Rust and Go on the
# workloads of this directory: rust/ is the Rust version of go/main.go
# (the same algorithms and sizes, std only, cargo --release with thin LTO
# and one codegen unit), behind one binary (`llperf <name>`).
#
# Sides: go, rust, xo-llvm (`xo build --backend=llvm`), and xo-arm64
# (`--backend=arm64`, on an arm64 host). Every binary runs REPS times
# (default 5), interleaved (go, rust, xo-llvm, xo-arm64, go, ...); the
# table shows the best wall time in milliseconds, the fewest instructions
# retired in millions (`/usr/bin/time -l`, macOS), and the peak resident
# memory in MB, with each Xo side against Rust. CALLGRIND=1 instead runs
# every binary once under valgrind and shows its exact instruction count
# (Ir; Linux with valgrind): cachegrind with --cache-sim=no, which counts
# what callgrind does (within 0.5% here) without callgrind's call graph,
# whose memory grew past 8 GB on the arm64 interp binary and was killed.
# Checksums must agree.
#
# Needs .tools/bin/xo (or $XO), go, cargo (or $CARGO; ~/.cargo/bin/cargo
# is tried), clang, python3. WORKLOADS selects some of: fib sieve nbody
# sort interp collatz strings; SIDES some of the sides. PERF_OUT keeps the
# binaries; PREBUILT=1 takes them from PERF_OUT without building. bench/perf/llvm/rust-docker.sh runs this under callgrind on
# linux/arm64 in Docker.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
out=${PERF_OUT:-$(mktemp -d)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
xo=${XO:-$root/.tools/bin/xo}
works=${WORKLOADS:-fib sieve nbody sort interp collatz strings}
cargo=${CARGO:-}
if [ -z "$cargo" ]; then
	if command -v cargo >/dev/null 2>&1; then
		cargo=cargo
	else
		cargo=$HOME/.cargo/bin/cargo
	fi
fi
sides=${SIDES:-}
if [ -z "$sides" ]; then
	sides="go rust xo-llvm"
	case $(uname -m) in
	arm64 | aarch64) sides="$sides xo-arm64" ;;
	esac
fi
# PREBUILT=1: the binaries are in PERF_OUT already (rust-docker.sh)
[ "${PREBUILT:-0}" = 1 ] || for s in $sides; do
	case $s in
	go) (cd "$here/go" && go build -o "$out/llperf-go" .) ;;
	rust)
		(cd "$here/rust" && CARGO_TARGET_DIR="$out/rust-target" "$cargo" build --release -q)
		cp "$out/rust-target/release/llperf" "$out/llperf-rust"
		;;
	xo-llvm) for w in $works; do "$xo" build --backend=llvm -o "$out/ll-$w" "$here/xo/$w" >/dev/null; done ;;
	xo-arm64) for w in $works; do "$xo" build --backend=arm64 -o "$out/a64-$w" "$here/xo/$w" >/dev/null; done ;;
	*)
		echo "unknown side $s" >&2
		exit 2
		;;
	esac
done
python3 - "$out" "${REPS:-5}" "${CALLGRIND:-0}" "$sides" $works <<'PY'
import os, re, subprocess, sys, time
out, reps, cg, sides, works = sys.argv[1], int(sys.argv[2]), sys.argv[3] == "1", sys.argv[4].split(), sys.argv[5:]
def cmd(side, w):
    return {"go": [out + "/llperf-go", w], "rust": [out + "/llperf-rust", w],
            "xo-llvm": [out + "/ll-" + w], "xo-arm64": [out + "/a64-" + w]}[side]
def once(c):
    t = time.perf_counter()
    p = subprocess.run(["/usr/bin/time", "-l"] + c, capture_output=True, text=True)
    dt = (time.perf_counter() - t) * 1000
    if p.returncode != 0:
        sys.exit("%s failed: %s" % (c, p.stderr.strip()[:200]))
    m = re.search(r"(\d+)\s+maximum resident set size", p.stderr)
    i = re.search(r"(\d+)\s+instructions retired", p.stderr)
    return dt, (int(m.group(1)) / 1e6 if m else 0), (int(i.group(1)) / 1e6 if i else 0), p.stdout.strip()
def callgrind(c):
    f = out + "/cg.out"
    p = subprocess.run(["valgrind", "--tool=cachegrind", "--cache-sim=no", "--cachegrind-out-file=" + f] + c, capture_output=True, text=True)
    if p.returncode != 0:
        sys.exit("%s failed (status %d): %s" % (c, p.returncode, p.stderr.strip()[-300:]))
    m = re.search(r"I\s+refs:\s+([\d,]+)", p.stderr)
    os.remove(f)
    return int(m.group(1).replace(",", "")) / 1e6, p.stdout.strip()
xs = [s for s in sides if s.startswith("xo-")]
if cg:
    print("| Workload | " + " | ".join("%s M instr" % s for s in sides) + "".join(" | %s/rust" % s for s in xs if "rust" in sides) + " |")
    print("|---|" + "---|" * (len(sides) + (len(xs) if "rust" in sides else 0)))
    for w in works:
        r = [callgrind(cmd(s, w)) for s in sides]
        if len(set(x[1] for x in r)) != 1:
            sys.exit("checksum mismatch in %s: %s" % (w, [x[1] for x in r]))
        v = dict(zip(sides, [x[0] for x in r]))
        row = ["%.1f" % v[s] for s in sides]
        if "rust" in sides:
            row += ["%.2fx" % (v[s] / v["rust"]) for s in xs]
        print("| %s | %s |" % (w, " | ".join(row)))
    sys.exit(0)
print("| Workload | " + " | ".join("%s ms" % s for s in sides) + " | " + " | ".join("%s M instr" % s for s in sides)
      + "".join(" | %s/rust ms" % s for s in xs if "rust" in sides) + " | " + " | ".join("%s MB" % s for s in sides) + " |")
print("|---|" + "---|" * (3 * len(sides) + (len(xs) if "rust" in sides else 0)))
for w in works:
    t = {s: [] for s in sides}
    ins = {s: None for s in sides}
    rss = {s: 0.0 for s in sides}
    sums = set()
    for _ in range(reps):
        for s in sides:
            dt, r, i, out_ = once(cmd(s, w))
            t[s].append(dt)
            rss[s] = max(rss[s], r)
            ins[s] = i if ins[s] is None else min(ins[s], i)
            sums.add(out_)
    if len(sums) != 1:
        sys.exit("checksum mismatch in %s: %s" % (w, sums))
    best = {s: min(t[s]) for s in sides}
    row = ["%.1f" % best[s] for s in sides] + ["%.0f" % ins[s] for s in sides]
    if "rust" in sides:
        row += ["%.2fx" % (best[s] / best["rust"]) for s in xs]
    row += ["%.1f" % rss[s] for s in sides]
    print("| %s | %s |" % (w, " | ".join(row)))
PY
