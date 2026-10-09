#!/usr/bin/env python3
"""Summarize an xctrace CPU Profiler trace by function.

Usage: profile.py TRACE [TOP]

Runs `xctrace export` on the cpu-profile table and prints the functions
with the most cycles, self (the sampled frame) and total (anywhere in the
backtrace, counted once per sample), as percentages of all samples.
Record with:
  xcrun xctrace record --template 'CPU Profiler' --output p.trace --launch -- ./bin
"""
import subprocess
import sys
import xml.etree.ElementTree as ET
from collections import Counter

trace = sys.argv[1]
top = int(sys.argv[2]) if len(sys.argv) > 2 else 30
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


selfc, total = Counter(), Counter()
all_w = 0
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
                    frames.append(f.get("name") or f.get("addr") or "?")
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
