#!/usr/bin/env python3
"""serial-hold.py GO_TEST_JSON: the serial top-level tests of a `go test
-json` log (a scripts/gate.sh step's $logs/NAME.json) and how many tests
ran at once in each 100 s of the run.

go test resumes a top-level test that called t.Parallel only after every
serial top-level test of its package has finished, and a serial test's
parallel subtests start when its function returns; so serial tests run one
at a time, often with nothing beside them
(knowledgebase/gotchas/go-serial-tests-hold-parallel.md). A serial test is
one with no "pause" event. Prints the step's length, the serial tests of 3 s
or more (start, seconds, name; the longest first), then START:RUNNING for
each 100 s window; a run of small counts is time the machine sat idle.
"""
import collections
import json
import re
import sys
from datetime import datetime


def ts(s):
    # Go writes up to nine fractional digits, which fromisoformat refuses
    m = re.match(r"(.*T\d\d:\d\d:\d\d)(\.\d+)?(.*)", s)
    return datetime.fromisoformat(m.group(1) + m.group(3)).timestamp() + float(m.group(2) or 0)


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: serial-hold.py GO_TEST_JSON")
    start, end, cont, elapsed, paused, t0 = {}, {}, {}, {}, set(), None
    for line in open(sys.argv[1], errors="replace"):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if "Test" not in e or "Time" not in e:
            continue
        t, n, a = ts(e["Time"]), e["Test"], e["Action"]
        t0 = t if t0 is None else min(t0, t)
        if a == "run":
            start.setdefault(n, t)
        elif a == "cont":
            cont.setdefault(n, t)
        elif a == "pause":
            paused.add(n)
        elif a in ("pass", "fail", "skip"):
            end[n], elapsed[n] = t, e.get("Elapsed", 0)
    if not end:
        sys.exit("no test results in " + sys.argv[1])
    serial = [n for n in end if "/" not in n and n not in paused and n in start and elapsed[n] >= 3]
    print("step %d s; serial top-level tests of 3 s or more: %d, %d s" % (
        max(end.values()) - t0, len(serial), sum(elapsed[n] for n in serial)))
    for n in sorted(serial, key=lambda n: -elapsed[n]):
        print("  %6d %5d %s" % (start[n] - t0, elapsed[n], n))
    running = collections.Counter()
    for n in end:
        if any(m.startswith(n + "/") for m in end):
            continue  # a parent; its leaves are counted
        s = cont.get(n, start.get(n))
        if s is None:
            continue
        for k in range(int((s - t0) // 100), int((end[n] - t0) // 100) + 1):
            running[k] += 1
    print("running per 100 s: " + " ".join("%d:%d" % (k * 100, running[k]) for k in sorted(running)))


main()
