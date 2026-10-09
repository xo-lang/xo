#!/usr/bin/env python3
"""Summarize an xctrace CPU Profiler trace by function.

Usage: profile.py TRACE [TOP [BINARY]]

Runs `xctrace export` on the cpu-profile table and prints the functions
with the most cycles, self (the sampled frame) and total (anywhere in the
backtrace, counted once per sample), as percentages of all samples.
With BINARY, frames xctrace left as addresses are named from `nm -n
BINARY` (static functions of the runtime often are), with the load slide
taken from a frame it did name. Record with:
  xcrun xctrace record --template 'CPU Profiler' --output p.trace --launch -- ./bin
"""
import subprocess
import sys
import xml.etree.ElementTree as ET
from collections import Counter

trace = sys.argv[1]
top = int(sys.argv[2]) if len(sys.argv) > 2 else 30
binary = sys.argv[3] if len(sys.argv) > 3 else None
syms = []
if binary:
    for line in subprocess.run(["nm", "-n", binary], capture_output=True, text=True).stdout.splitlines():
        parts = line.split()
        if len(parts) == 3 and parts[1] in "tT":
            syms.append((int(parts[0], 16), parts[2][1:] if parts[2].startswith("_") else parts[2]))
symaddr = {n: a for a, n in syms}
xml = subprocess.run(
    ["xcrun", "xctrace", "export", "--input", trace, "--xpath",
     '/trace-toc/run[@number="1"]/data/table[@schema="cpu-profile"]'],
    capture_output=True, check=True).stdout
root = ET.fromstring(xml)
ids = {}


def resolve(e):
    r = e.get("ref")
    if r is not None:
        return ids[r]
    if e.get("id") is not None:
        ids[e.get("id")] = e
    for c in e:
        resolve(c)
    return e


rows = []
slide = None
for row in root.iter("row"):
    w, frames = 0, []
    for c in row:
        c = resolve(c)
        if c.tag == "cycle-weight":
            w = int(c.text or 0)
        elif c.tag in ("backtrace", "tagged-backtrace"):
            if c.tag == "tagged-backtrace":
                c = resolve(c.find("backtrace") if c.find("backtrace") is not None else c)
            for f in c:
                f = resolve(f)
                if f.tag == "frame":
                    name, addr = f.get("name") or "?", f.get("addr")
                    if slide is None and addr and name in symaddr:
                        slide = (int(addr, 16) - symaddr[name]) // 0x4000 * 0x4000
                    frames.append((name, addr))
    rows.append((w, frames))

import bisect
keys = [a for a, _ in syms]


def label(name, addr):
    if name.startswith("0x") and slide is not None and keys:
        a = int(addr or name, 16) - slide
        i = bisect.bisect_right(keys, a) - 1
        if i >= 0 and a - keys[i] < 1 << 16:
            return syms[i][1]
    return name


selfc, total = Counter(), Counter()
all_w = 0
for w, fr in rows:
    frames = [label(n, a) for n, a in fr]
    if not frames:
        continue
    all_w += w
    selfc[frames[0]] += w
    for n in set(frames):
        total[n] += w
print("samples weight %d" % all_w)
print("\nself")
for n, w in selfc.most_common(top):
    print("%6.2f%%  %s" % (100.0 * w / all_w, n))
print("\ntotal")
for n, w in total.most_common(top):
    print("%6.2f%%  %s" % (100.0 * w / all_w, n))

# PROFILE_FUNC=name: the sampled addresses inside that function (offsets
# from its start in `nm`), to read against `otool -tvV BINARY`
import os
fn = os.environ.get("PROFILE_FUNC")
if fn and fn in symaddr and slide is not None:
    pcs = Counter()
    for w, fr in rows:
        if fr and label(*fr[0]) == fn:
            pcs[int(fr[0][1] or fr[0][0], 16) - slide] += w
    print("\n%s at 0x%x" % (fn, symaddr[fn]))
    for a, w in sorted(pcs.items(), key=lambda x: -x[1])[:top]:
        print("%6.2f%%  0x%x (+0x%x)" % (100.0 * w / all_w, a, a - symaddr[fn]))
