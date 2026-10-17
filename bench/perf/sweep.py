#!/usr/bin/env python3
"""Compare two xo binaries across bench/perf on one backend (macOS).

usage: bench/perf/sweep.py BASE_XO NEW_XO OUT [--backend arm64] [--rounds 3] [DIR...]

Runs each bench/perf/<dir>/measure.sh (its default workloads; each run
takes the minimum instructions retired of 5) with BASE_XO and NEW_XO in
alternating rounds, and prints per workload the smaller minimum of each
side, their ratio (new / base), and the spread: the larger max/min over
the rounds of either side. A row is "SLOWER" or "faster" when the ratio
is outside its spread and at least 0.5% from 1 (the same binary on both
sides gave 2 of 9 rows outside a 2 round spread, by 0.1 to 0.2%).
Each xo finds its std at bin/../std.
"""
import os
import subprocess
import sys

DIRS = ["atomic", "ctx", "digest", "fault", "fsproc", "json", "net", "portable",
        "rsa", "stdio", "sysio", "text", "httpclient", "httpparse", "tls"]


def measure(here, d, xo, out, backend):
    """{(workload, variant): instructions} of one measure.sh run."""
    env = dict(os.environ)
    p = subprocess.run([os.path.join(here, d, "measure.sh"), xo, out, backend],
                       capture_output=True, text=True, env=env)
    res = {}
    for line in p.stdout.splitlines():
        f = line.split()
        if len(f) < 3 or f[1] != backend:
            continue
        # the first large integer after the backend is the instruction
        # count (sysio and tls name a variant before it)
        var = ""
        ins = None
        for tok in f[2:]:
            if tok.isdigit() and int(tok) >= 1000:
                ins = int(tok)
                break
            if not tok.isdigit():
                var = tok
        if ins is not None:
            res[(f[0], var)] = ins
    if p.returncode != 0 and not res:
        sys.stderr.write("%s: %s\n" % (d, p.stderr.strip()[-400:]))
    return res


def main():
    args = sys.argv[1:]
    backend, rounds, dirs = "arm64", 3, []
    pos = []
    i = 0
    while i < len(args):
        if args[i] == "--backend":
            backend = args[i + 1]
            i += 2
        elif args[i] == "--rounds":
            rounds = int(args[i + 1])
            i += 2
        else:
            pos.append(args[i])
            i += 1
    if len(pos) < 3:
        sys.exit(__doc__)
    base, new, out = pos[0], pos[1], pos[2]
    dirs = pos[3:] or DIRS
    here = os.path.dirname(os.path.abspath(__file__))
    rows = []
    for d in dirs:
        runs = {"base": [], "new": []}
        for r in range(rounds):
            order = ["base", "new"] if r % 2 == 0 else ["new", "base"]
            for side in order:
                xo = base if side == "base" else new
                runs[side].append(measure(here, d, xo, os.path.join(out, d, side, str(r)), backend))
        keys = sorted(set().union(*[set(m) for m in runs["base"] + runs["new"]]))
        for k in keys:
            b = [m[k] for m in runs["base"] if k in m]
            n = [m[k] for m in runs["new"] if k in m]
            if not b or not n:
                continue
            spread = max(max(b) / min(b), max(n) / min(n))
            ratio = min(n) / min(b)
            off = abs(ratio - 1) >= 0.005
            mark = "SLOWER" if ratio > spread and off else ("faster" if ratio < 1 / spread and off else "")
            rows.append(("%s/%s%s" % (d, k[0], "/" + k[1] if k[1] else ""), min(b), min(n), ratio, spread, mark))
    for name, b, n, ratio, spread, mark in rows:
        print("%-36s base %13d new %13d ratio %6.3f spread %5.3f %s" % (name, b, n, ratio, spread, mark))
    slower = sum(1 for r in rows if r[5] == "SLOWER")
    faster = sum(1 for r in rows if r[5] == "faster")
    print("%d workloads; %d slower, %d faster beyond spread" % (len(rows), slower, faster))


if __name__ == "__main__":
    main()
