#!/usr/bin/env python3
"""cgcompare.py BASE NEW [SCENARIO...]: per function instructions a request
of two callgrind.sh runs made with ANN=1 (OUT directories), base against
new: each function's self count of the 2N run minus the N run, over the
request difference (startup cancels), the totals and the largest changes.
After Xo lang 5's cgdiff.py (2026-10-18). --xo-names (2026-10-19, first
argument) matches names across backends: the native backends' xo_f_
symbols (`.` as _2e, `_` doubled) and LLVM's `x.` names both become the
Xo name, so a native run compares with an LLVM one (callgrind-docker.sh
with one BACKEND per OUT, since ann files are named by scenario only).
Without it the output is xo dev compare's (cmd/xo TestDevCompare)."""
import os
import re
import sys

SELF = re.compile(r"^\s*([\d,]+)\s+\(\s*[\d.]+%\)\s+(.*)$")


XO_NAMES = False


def xo_name(k):
    if k.startswith("xo_f_"):
        k = k[len("xo_f_"):]
        out = []
        i = 0
        while i < len(k):
            if k.startswith("_2e", i):
                out.append(".")
                i += 3
            elif k.startswith("__", i):
                out.append("_")
                i += 2
            else:
                out.append(k[i])
                i += 1
        return "".join(out)
    if k.startswith("x."):
        return k[2:]
    return k


def load(path):
    d = {}
    for line in open(path):
        m = SELF.match(line.rstrip("\n"))
        if not m:
            continue
        name = m.group(2)
        if "PROGRAM TOTALS" in name or "events annotated" in name or re.search(r"\(\d+x\)\s*$", name):
            continue
        # "file:function'2 [object]": the function, without the object or
        # callgrind's recursion suffix
        name = re.sub(r"\s*\[[^\]]*\]\s*$", "", name)
        name = re.sub(r"'\d+$", "", name.split(":")[-1].strip())
        if " " in name or not name:
            continue
        if XO_NAMES:
            name = xo_name(name)
        d[name] = d.get(name, 0) + int(m.group(1).replace(",", ""))
    return d


def per_request(out, sc):
    runs = sorted(int(f.rsplit(".", 1)[1]) for f in os.listdir(out) if f.startswith(f"ann.{sc}."))
    if len(runs) < 2:
        return None
    n1, n2 = runs[0], runs[-1]
    d1, d2 = load(f"{out}/ann.{sc}.{n1}"), load(f"{out}/ann.{sc}.{n2}")
    q = int(open(f"{out}/req.{sc}.{n2}").read()) - int(open(f"{out}/req.{sc}.{n1}").read())
    return {k: (d2.get(k, 0) - d1.get(k, 0)) / q for k in set(d1) | set(d2)}


def main():
    global XO_NAMES
    if len(sys.argv) > 1 and sys.argv[1] == "--xo-names":
        XO_NAMES = True
        del sys.argv[1]
    if len(sys.argv) < 3:
        sys.exit("usage: cgcompare.py BASE NEW [SCENARIO...]")
    base, new = sys.argv[1], sys.argv[2]
    scs = sys.argv[3:] or ["plain", "plain-newconn", "tls", "tls-newconn", "h2"]
    for sc in scs:
        b, n = per_request(base, sc), per_request(new, sc)
        if b is None or n is None:
            continue
        tb, tn = sum(b.values()), sum(n.values())
        print(f"== {sc}: base {tb:.0f} new {tn:.0f} ratio {tn / tb:.3f}")
        keys = set(b) | set(n)
        diff = sorted(((n.get(k, 0) - b.get(k, 0), k) for k in keys), key=lambda x: abs(x[0]), reverse=True)
        for dv, k in diff[:15]:
            print(f"{dv:8.0f} {b.get(k, 0):8.0f} {n.get(k, 0):8.0f}  {k[:70]}")


main()
