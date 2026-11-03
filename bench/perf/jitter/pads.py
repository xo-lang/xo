#!/usr/bin/env python3
"""pads.py LOG [REF]: the verdict of a pad sweep (alloc-split.sh ALLOC_PADS,
release-vs-go-http.sh PADS; Lead, 2026-11-03).

A revision's /alloc throughput moves about 4 percent with where its code
lands (XO_ARM64_PAD: 0.958 to 1.003 of v0.35.0 for one revision), so one
build of each side can show a regression that is only placement
(gotchas/arm64-loop-alignment-stable-not-fast.md). Each group (a revision
with its other settings, or a server of the HTTP section) is built at
several pads; per row this prints the median over its pads of each pad's
median over rounds, the range of those pad medians, and the ratio to the
reference group (REF, default "base"). A group is called slower or faster
than the reference only when their pad ranges do not overlap; otherwise
the difference is placement.

LOG lines read: the run lines "SERVER PATH cC rateR rN {loadgen json}";
alloc-measure's "pK REV DATE built env [ENV]" and "pK base VER built env
[ENV]" lines, which name server pK's group and pad; and HTTP server names
NAME.pK (pad K of group NAME). Any other server is a group of one pad.
"""
import json
import re
import statistics as st
import sys

PAD = re.compile(r"(^|\s)XO_ARM64_PAD=(\d+)(\s|$)")


def main():
    lines = open(sys.argv[1], errors="replace").read().splitlines()
    ref = sys.argv[2] if len(sys.argv) > 2 else "base"
    who = {}
    for l in lines:
        m = re.match(r"(p\d+) (\S+) \S+ built(?: env \[(.*)\])?", l)
        if m:
            env = m[3] or ""
            p = PAD.search(env)
            rest = " ".join(PAD.sub(" ", env).split())
            who[m[1]] = (m[2] + (f" [{rest}]" if rest else ""), int(p[2]) if p else None)
    runs = {}
    for l in lines:
        m = re.match(r"(\S+) (/\S+) c(\d+) rate(\d+) r(\d+) (\{.*\})", l)
        if not m:
            continue
        srv = m[1]
        g = re.fullmatch(r"(\w+)\.p(\d+)", srv)
        group, pad = (g[1], int(g[2])) if g else who.get(srv, (srv, None))
        if pad is None and srv not in who and any(w[0] == srv for w in who.values()):
            # alloc-measure's own srv-base beside the padded base builds
            group = f"{srv} (no pad)"
        row = (m[2], int(m[3]), int(m[4]))
        runs.setdefault(row, {}).setdefault(group, {}).setdefault(pad, []).append(json.loads(m[6]))
    if not runs:
        print("pads: no runs")
        return
    print(f"pad sweep verdict (reference {ref}): each cell is the median over pads of each pad's median over rounds, the pad medians' range in brackets; slower or faster only when the ranges do not overlap\n")
    print("| row | group | pads | req/s | CPU us/req | req/s vs ref | verdict |")
    print("|---|---|---|---|---|---|---|")
    for row in sorted(runs):
        name = f"{row[0]} c{row[1]}" + (f" open {row[2]}/s" if row[2] else "")
        stat = {}
        for group, pads in runs[row].items():
            rps = [st.median([a.get("rps", 0) for a in v]) for v in pads.values()]
            cpu = [st.median([a.get("cpu_us_per_req", 0) for a in v]) for v in pads.values()]
            stat[group] = (len(pads), st.median(rps), min(rps), max(rps), st.median(cpu), min(cpu), max(cpu))
        r = stat.get(ref)
        for group in sorted(stat, key=lambda g: (g != ref, g)):
            n, m, lo, hi, c, clo, chi = stat[group]
            vs = verdict = ""
            if r and group != ref:
                vs = f"{m / r[1]:.3f}" if r[1] else ""
                if n < 2 or r[0] < 2:
                    verdict = "one pad: not judged"
                elif hi < r[2]:
                    verdict = "slower"
                elif lo > r[3]:
                    verdict = "faster"
                else:
                    verdict = "placement"
            print(f"| {name} | {group} | {n} | {m:,.0f} [{lo:,.0f} to {hi:,.0f}] | {c:.2f} [{clo:.2f} to {chi:.2f}] | {vs} | {verdict} |")


main()
