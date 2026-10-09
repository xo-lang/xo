#!/usr/bin/env python3
"""Score agent benchmark outputs with the current toolchain (standard library only).

  score.py solutions <dir>        solutions in <dir>/<arm>/<task>/ (arm: xo, go-plain, go-strict)
  score.py solutions --claude     the existing Claude outputs in bench/ (validation)
  score.py planted <dir>          planted bug sets in <dir>/planted/<set>/<task>/
                                  (set: go, xo-translate, xo-independent)
  score.py planted --claude       the existing planted sets in bench/planted*

Common flags: --tasks 01,02 (default all), --arms or --sets, --out FILE.md
(default stdout), --json FILE.jsonl (one record per cell), --no-hidden.

Solutions: every cell is copied to a temporary directory and rerun through
its arm's feedback loop (xo: `xo check --json`, then `xo test --race`;
go-plain: `go build`, `go vet`, `go test`; go-strict: bench/go/strict-check.sh).
Runs to green and the first try result come from `runs.jsonl` (written by
run_openai_compatible.py), else from the cell's `BUILD_LOG.md` table (the
format of the original Claude runs). Task 01 also gets hidden black box HTTP
checks (bench/harness/hidden/crud_api.py) unless --no-hidden.

Planted: each variant overlays its files on the base (Go:
bench/planted/<task>/base; Xo: bench/xo/v0.4/<task>) and runs the same
detection as bench/planted/run_detection.py and
bench/planted-xo/run_detection_xo.py, except that a test failure counts
as a catch only for a test that passes on the unplanted base.
"""
import argparse
import json
import os
import re
import socket
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common as c  # noqa: E402
from hidden import crud_api  # noqa: E402

# ---------------------------------------------------------------- published numbers
# three-arm-2026-10-08.md (tasks 01 to 08) and stage3-2026-10-08.md (09, 10):
# (plain runs, strict runs, xo runs, go lines, xo lines, go test funcs, xo test blocks)
PUBLISHED = {
    "01-crud-api": (1, 1, 1, 654, 321, 8, 7),
    "02-worker-pool": (1, 2, 1, 639, 241, 7, 6),
    "03-retry-client": (1, 2, 1, 710, 325, 14, 14),
    "04-file-sync": (1, 2, 1, 862, 383, 11, 12),
    "05-rate-limiter": (1, 2, 1, 586, 285, 11, 8),
    "06-job-queue": (2, 2, 1, 1270, 786, 16, 11),
    "07-cache": (1, 2, 1, 1200, 616, 21, 11),
    "08-notes-service": (1, 1, 2, 1782, 1149, 25, 30),
    "09-inventory": (1, 2, 1, 928, 517, None, None),
    "10-log-pipeline": (2, 2, 1, 999, 588, None, None),
}
# planted-and-harder-2026-10-07.md and planted-xo-2026-10-07.md (spec 0.6 update)
PUBLISHED_PLANTED = {
    "go": ("X: plain 9/30, plain + strict 13/30 (14 raw: the 05 X3 staticcheck S1000 style hit was "
           "excluded by hand); N: 17/20 (plain and strict)"),
    "xo-translate": "X: 29/30 prevented or caught (conservative 23 to 24 after faithfulness adjustments); N: 16/20",
    "xo-independent": "X: 23/30 caught, 25/30 caught or warned (spec 0.6 rules)",
}

CLAUDE_SOLUTIONS = {"go-plain": ["bench/go/plain/{t}"], "go-strict": ["bench/go/strict/{t}"],
                    "xo": ["bench/xo/v0.4/{t}", "bench/xo/v0.6/{t}"]}
CLAUDE_PLANTED = {"go": "bench/planted", "xo-translate": "bench/planted-xo", "xo-independent": "bench/planted-xo2"}
ARM_EXT = {"xo": ".xo", "go-plain": ".go", "go-strict": ".go"}


def log(msg):
    print(msg, file=sys.stderr, flush=True)


# ---------------------------------------------------------------- run history

def parse_build_log(path):
    """Runs to green from a BUILD_LOG.md table of the original runs."""
    rows, header = [], None
    with open(path, encoding="utf-8") as f:
        for line in f:
            if not line.startswith("|"):
                continue
            cells = [x.strip() for x in line.strip().strip("|").split("|")]
            if cells[0].lower() == "run":
                header = [x.lower() for x in cells]
            elif header and re.fullmatch(r"\d+", cells[0]):
                rows.append(dict(zip(header, cells)))

    def green(r):
        if "result" in r:
            return r["result"].lower().startswith("pass")
        if "check errors" in r:
            return r["check errors"] == "0" and r.get("test result", "pass").lower().startswith("pass")
        if "errors" in r:
            return r["errors"] == "0"
        return False

    if not rows:
        return None
    flags = [green(r) for r in rows]
    k = flags.index(True) + 1 if True in flags else None
    return {"source": "BUILD_LOG.md", "runs_to_green": k, "first_green": flags[0], "logged_runs": len(rows)}


def parse_runs_jsonl(path):
    runs = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                runs.append(json.loads(line))
    if not runs:
        return None
    flags = [bool(r.get("green")) for r in runs]
    k = flags.index(True) + 1 if True in flags else None
    first = runs[0]
    return {"source": "runs.jsonl", "runs_to_green": k, "first_green": flags[0], "logged_runs": len(runs),
            "first_errors": first.get("errors"), "first_warnings": first.get("warnings"),
            "first_stage": first.get("stage"), "first_tests_failed": first.get("tests_failed")}


# ---------------------------------------------------------------- measures

def strict_findings(output):
    """Counts analyzer findings in strict-check.sh output (file:line lines) plus data races."""
    n, section = 0, None
    for line in output.splitlines():
        m = re.match(r"^== (\w+): (ok|FAIL)", line)
        if m:
            section = m.group(1) if m.group(2) == "FAIL" else None
            continue
        if not section:
            continue
        if section == "test":
            n += line.count("WARNING: DATA RACE")
        elif re.match(r"^\S+\.go:\d+(:\d+)?:", line):  # one finding per unindented position line
            n += 1
    return n


def free_port():
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    p = s.getsockname()[1]
    s.close()
    return p


def hidden_crud(arm, d):
    """Builds the solution root, serves it on a free port, runs the hidden checks."""
    with tempfile.TemporaryDirectory() as tmp:
        exe = os.path.join(tmp, "server")
        if arm == "xo":
            rc, out = c.run([c.xo_bin(), "build", "-o", exe, d], cwd=d)
        else:
            rc, out = c.run(["go", "build", "-o", exe, "."], cwd=d)
        if rc != 0 or not os.path.exists(exe):
            return {"passed": 0, "total": 0, "error": "build failed: " + out.strip()[-500:]}
        port = free_port()
        env = dict(os.environ, PORT=str(port))
        logf = open(os.path.join(tmp, "server.log"), "w")
        proc = subprocess.Popen([exe], cwd=tmp, env=env, stdout=logf, stderr=subprocess.STDOUT)
        try:
            up = False
            for _ in range(200):
                try:
                    socket.create_connection(("127.0.0.1", port), timeout=0.2).close()
                    up = True
                    break
                except OSError:
                    if proc.poll() is not None:
                        break
                    time.sleep(0.1)
            if not up:
                return {"passed": 0, "total": 0, "error": "server did not listen on PORT"}
            res = crud_api.checks("http://127.0.0.1:%d" % port)
        finally:
            # only the server this function started
            proc.terminate()
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
            logf.close()
    failed = [name for name, ok, _ in res if not ok]
    return {"passed": len(res) - len(failed), "total": len(res), "failed": failed}


def first_snapshot(src, task):
    """The sources of a cell's first run, or None.

    The adapter keeps them in <cell>/.attempts/run-1; a run made another
    way (harness path 2b) may keep them in <out>/history/<task>/1, next to
    <out>/<arm>/<task>.
    """
    for p in (os.path.join(src, ".attempts", "run-1"),
              os.path.join(os.path.dirname(os.path.dirname(src)), "history", task, "1")):
        if os.path.isdir(p):
            return p
    return None


def score_cell(arm, task, src, hidden=True):
    rec = {"arm": arm, "task": task, "dir": os.path.relpath(src, c.ROOT) if src.startswith(c.ROOT) else src}
    ext = ARM_EXT[arm]
    rec["lines"] = c.count_lines(src, ext)
    rec["tests"] = c.count_tests(src, ext)
    hist = None
    if os.path.exists(os.path.join(src, "runs.jsonl")):
        hist = parse_runs_jsonl(os.path.join(src, "runs.jsonl"))
    elif os.path.exists(os.path.join(src, "BUILD_LOG.md")):
        hist = parse_build_log(os.path.join(src, "BUILD_LOG.md"))
    rec["history"] = hist
    snap = first_snapshot(src, task)
    if snap:
        # first try now (as written), and after `xo check --fix`, with this toolchain
        rec["first"] = c.first_try(arm, snap)
    with tempfile.TemporaryDirectory() as tmp:
        d = os.path.join(tmp, task)
        c.copy_solution(src, d)
        t0 = time.time()
        fb = c.feedback(arm, d)
        rec["final"] = {k: v for k, v in fb.items() if k != "output"}
        rec["final"]["seconds"] = round(time.time() - t0, 1)
        if arm == "go-strict":
            rec["diagnostics"] = strict_findings(fb["output"])
        elif arm == "go-plain":
            # escaped findings: the strict tools applied to the plain arm
            rec["diagnostics"] = strict_findings(c.loop_go_strict(d)["output"])
        else:
            rec["diagnostics"] = fb.get("errors", 0) + fb.get("warnings", 0)
        if not fb["green"]:
            rec["final_output_tail"] = fb["output"][-3000:]
        if hidden and task == "01-crud-api":
            rec["hidden"] = hidden_crud(arm, d)
    return rec


# ---------------------------------------------------------------- solutions report

def cell_dirs(args):
    out = []
    for arm in args.arms:
        for t in args.tasks:
            if args.claude:
                cands = [os.path.join(c.ROOT, p.format(t=t)) for p in CLAUDE_SOLUTIONS[arm]]
            else:
                cands = [os.path.join(args.dir, arm, t)]
            src = next((p for p in cands if os.path.isdir(p)), None)
            out.append((arm, t, src))
    return out


def fmt_runs(rec):
    if rec is None:
        return "missing"
    h = rec.get("history")
    if not h:
        return "n/a"
    return str(h["runs_to_green"]) if h["runs_to_green"] else "not green in %d" % h["logged_runs"]


def xo_first_tests(rec):
    if rec is None:
        return "missing"
    h = rec.get("history") or {}
    if h.get("source") == "runs.jsonl":
        if h.get("first_stage") == "check":
            return "not run (check errors)"
        return "pass" if h.get("first_green") else "fail"
    f = rec["final"]
    if f.get("stage") == "check":
        return "not run now (check errors)"
    return ("pass" if f["green"] else "fail") + " (rerun now)"


def solutions_report(args, recs):
    by = {(r["arm"], r["task"]): r for r in recs}
    L = []
    L.append("# Agent benchmark score: solutions")
    L.append("")
    L.append("Scored %s with `bench/harness/score.py` on %s (xo built from this checkout)." % (
        "the existing Claude outputs" if args.claude else "`%s`" % args.dir, time.strftime("%Y-%m-%d")))
    L.append("")
    L.append("## Writing the tasks")
    L.append("")
    L.append("| Task | Plain Go runs to green | Strict Go runs to green | Xo runs to green | Xo tests when first run | Go lines | Xo lines | Go test funcs | Xo test blocks |")
    L.append("|---|---|---|---|---|---|---|---|---|")
    for t in args.tasks:
        p, s, x = by.get(("go-plain", t)), by.get(("go-strict", t)), by.get(("xo", t))
        L.append("| %s | %s | %s | %s | %s | %s | %s | %s | %s |" % (
            t.replace("-", " ", 1), fmt_runs(p), fmt_runs(s), fmt_runs(x), xo_first_tests(x),
            p["lines"] if p else "", x["lines"] if x else "", p["tests"] if p else "", x["tests"] if x else ""))
    L.append("")
    L.append("Runs to green come from each cell's `runs.jsonl` (harness runs) or `BUILD_LOG.md` (original runs).")
    L.append("")
    L.append("## Summary")
    L.append("")
    L.append("| Measure | Plain Go | Strict Go | Xo |")
    L.append("|---|---|---|---|")

    def col(fn):
        return " | ".join(fn([by[(a, t)] for t in args.tasks if by.get((a, t))]) for a in ["go-plain", "go-strict", "xo"])

    def first_rate(rs):
        hs = [r["history"] for r in rs if r.get("history")]
        return "%d/%d" % (sum(1 for h in hs if h["first_green"]), len(hs)) if hs else "n/a"

    def total_runs(rs):
        hs = [r["history"] for r in rs if r.get("history")]
        return str(sum(h["runs_to_green"] or h["logged_runs"] for h in hs)) if hs else "n/a"

    def first_now(key):
        def fn(rs):
            fs = [r["first"][key] for r in rs if r.get("first") and r["first"].get(key)]
            return "%d/%d" % (sum(1 for f in fs if f["green"]), len(fs)) if fs else "n/a"
        return fn

    L.append("| First try pass rate | %s |" % col(first_rate))
    L.append("| First try now (first run sources, current toolchain) | %s |" % col(first_now("plain")))
    L.append("| First try after `xo check --fix` (current toolchain) | %s |" % col(first_now("after_fix")))
    L.append("| Runs in total | %s |" % col(total_runs))
    L.append("| Lines (code and tests) | %s |" % col(lambda rs: str(sum(r["lines"] for r in rs))))
    L.append("| Green now (current toolchain) | %s |" % col(lambda rs: "%d/%d" % (sum(1 for r in rs if r["final"]["green"]), len(rs))))
    L.append("| Diagnostics on final code | %s |" % col(lambda rs: str(sum(r["diagnostics"] for r in rs))))
    hid = col(lambda rs: next(("%d/%d" % (r["hidden"]["passed"], r["hidden"]["total"]) if r["hidden"]["total"]
                               else r["hidden"].get("error", "error")[:60]
                               for r in rs if r.get("hidden")), "not run"))
    L.append("| Hidden checks, task 01 | %s |" % hid)
    L.append("")
    L.append("Diagnostics: Xo counts `xo check` errors plus warnings; both Go arms count strict analyzer")
    L.append("findings (vet, staticcheck, errcheck, nilaway, exhaustive lines plus data races), so the")
    L.append("plain Go number is the \"escaped findings\" measure of go-baseline-2026-10-07.md.")
    L.append("")
    L.append("## Final state per cell (current toolchain)")
    L.append("")
    L.append("| Arm | Task | Green | Failing stage | Xo errors/warnings | Diagnostics | Hidden |")
    L.append("|---|---|---|---|---|---|---|")
    for r in recs:
        f = r["final"]
        h = r.get("hidden")
        hs = "" if not h else ("%d/%d" % (h["passed"], h["total"]) if h["total"] else h.get("error", "")[:60])
        if h and h.get("failed"):
            hs += " (failed: %s)" % "; ".join(h["failed"])
        xe = "%s/%s" % (f.get("errors", ""), f.get("warnings", "")) if r["arm"] == "xo" else ""
        if r["arm"] == "xo" and f.get("codes"):
            xe += " " + ",".join(f["codes"])
        L.append("| %s | %s | %s | %s | %s | %s | %s |" % (r["arm"], r["task"], "yes" if f["green"] else "no",
                                                       f.get("stage", ""), xe, r["diagnostics"], hs))
    L.append("")
    firsts = [r for r in recs if r.get("first")]
    if firsts:
        L.append("## First runs (current toolchain)")
        L.append("")
        L.append("The first run's sources rerun as written (read only `xo check`), and after `xo check --fix` (Xo only). Recorded is the run's own result; Xo runs since decision 0096 get `xo check --fix` in their loop, so it counts after fix.")
        L.append("")
        L.append("| Arm | Task | Recorded | Now | After `xo check --fix` |")
        L.append("|---|---|---|---|---|")

        def cell(f):
            if not f:
                return "n/a"
            s = "pass" if f["green"] else "fail (%s%s)" % (f.get("stage", ""), ": " + ",".join(f["codes"]) if f.get("codes") else "")
            if f.get("fixes"):
                s += ", %d fixes" % f["fixes"]
            return s
        for r in firsts:
            h = r.get("history") or {}
            rec_ = "n/a" if "first_green" not in h else ("pass" if h["first_green"] else "fail")
            L.append("| %s | %s | %s | %s | %s |" % (r["arm"], r["task"], rec_, cell(r["first"]["plain"]),
                                                     cell(r["first"]["after_fix"])))
        L.append("")
    if args.claude:
        L.append("## Against the published numbers")
        L.append("")
        L.append("| Task | Published (plain, strict, xo runs; go, xo lines; go, xo tests) | Scored | Match |")
        L.append("|---|---|---|---|")
        for t in args.tasks:
            pub = PUBLISHED.get(t)
            p, s, x = by.get(("go-plain", t)), by.get(("go-strict", t)), by.get(("xo", t))
            if not (pub and p and s and x):
                continue

            def rg(r):
                h = r.get("history")
                return h["runs_to_green"] if h else None
            got = (rg(p), rg(s), rg(x), p["lines"], x["lines"], p["tests"], x["tests"])
            ok = all(a is None or a == b for a, b in zip(pub, got))
            L.append("| %s | %s | %s | %s |" % (t, ", ".join("-" if v is None else str(v) for v in pub),
                                                ", ".join(str(v) for v in got), "yes" if ok else "**no**"))
        L.append("")
    return "\n".join(L) + "\n"


# ---------------------------------------------------------------- planted

GO_STRICT = [("vet", ["go", "vet", "./..."]), ("staticcheck", ["{tools}/staticcheck", "./..."]),
             ("errcheck", ["{tools}/errcheck", "./..."]),
             ("nilaway", ["{tools}/nilaway", "-pretty-print=false", "./..."]),
             ("exhaustive", ["{tools}/exhaustive", "./..."]),
             ("race", ["go", "test", "-race", "-count=1", "-timeout", "180s", "./..."])]


def _norm(out, root):
    lines = set()
    out = out.replace(os.path.realpath(root), "").replace(root, "")
    for ln in out.splitlines():
        ln = re.sub(r":\d+(:\d+)?", "", ln).strip()
        if ln and not ln.startswith(("ok ", "?", "#", "FAIL\t", "--- ", "PASS", "=== ")):
            lines.add(ln)
    return lines


def go_strict_findings(root):
    tools = c.tools_bin()
    res = {}
    for name, cmd in GO_STRICT:
        rc, out = c.run([x.format(tools=tools) for x in cmd], cwd=root, timeout=c.T_TEST)
        if name == "race":
            races = {ln.strip() for ln in out.splitlines() if "DATA RACE" in ln}
            res[name] = (1 if races else 0, races)
        else:
            res[name] = (rc, _norm(out, root))
    return res


GO_TEST = ["go", "test", "-count=1", "-timeout", "180s", "./..."]


def failed_tests(out):
    """The names of the failing tests in `go test` or `xo test` output,
    without the location and seed suffix `(file:line:col, seed N)`, which
    moves when a variant edits the file."""
    return {re.sub(r"\s*\([^()]*\)\s*$", "", x).strip() for x in re.findall(r"--- FAIL: (.*)", out)}


def new_test_failures(rc, out, base):
    """Whether a test run (rc, out) catches a planted bug, and the failing
    tests that count. base is (rc, failed test names) of the unplanted
    code: tests that already fail there do not count, so a variant is
    caught by tests only when a test fails that passes on the base (or
    the base passes and the variant fails). A failing run without test
    names over a failing base is not counted."""
    if rc == 0:
        return False, []
    names = failed_tests(out)
    brc, bnames = base
    if brc == 0:
        return True, sorted(names)
    new = sorted(names - bnames)
    return bool(new), new


def test_base(rc, out):
    """(rc, failed test names) of a run on the unplanted code."""
    return rc, failed_tests(out) if rc != 0 else set()


def go_plain_caught(root, base_tests=(0, set())):
    for step, cmd in [("build", ["go", "build", "./..."]), ("vet", ["go", "vet", "./..."]), ("test", GO_TEST)]:
        rc, out = c.run(cmd, cwd=root, timeout=c.T_TEST)
        if step == "test":
            return "test" if new_test_failures(rc, out, base_tests)[0] else "none"
        if rc != 0:
            return step
    return "none"


def load_manifest(tdir):
    with open(os.path.join(tdir, "manifest.json"), encoding="utf-8") as f:
        return {b["id"]: b for b in json.load(f)}


def overlay(base, variant, dst):
    c.copy_solution(base, dst)
    if variant:
        c.copy_solution(variant, dst)


def planted_go(tdir, task):
    base_dir = os.path.join(c.ROOT, "bench", "planted", task, "base")
    man = load_manifest(tdir)
    recs = []
    with tempfile.TemporaryDirectory() as tmp:
        w = os.path.join(tmp, task)  # same name as the variants: nilaway traces print it
        overlay(base_dir, None, w)
        base = go_strict_findings(w)
        base_tests = test_base(*c.run(GO_TEST, cwd=w, timeout=c.T_TEST))
    for vid in sorted(man):
        vdir = os.path.join(tdir, "variants", vid)
        rec = {"set": "go", "task": task, "bug": vid, "class": vid[:2]}
        if not os.path.isdir(vdir):
            rec["caught_by"] = "missing"
            recs.append(rec)
            continue
        with tempfile.TemporaryDirectory() as tmp:
            w = os.path.join(tmp, task)
            overlay(base_dir, vdir, w)
            rec["plain_caught_by"] = go_plain_caught(w, base_tests)
            caught = []
            for name, (rc, found) in go_strict_findings(w).items():
                brc, bfound = base[name]
                if (rc != 0 and brc == 0) or (found - bfound):
                    caught.append(name)
            rec["strict_new_findings"] = caught
        log(json.dumps(rec))
        recs.append(rec)
    return recs


def planted_xo(tdir, task, set_name):
    base_dir = os.path.join(c.ROOT, "bench", "xo", "v0.4", task)
    man = load_manifest(tdir)
    with tempfile.TemporaryDirectory() as tmp:
        w = os.path.join(tmp, task)
        overlay(base_dir, None, w)
        _, bw, _, _ = c.xo_check(w)
        base_warn = {(x.get("code"), x.get("message")) for x in bw}
        base_tests = test_base(*c.xo_test(w))
    if base_tests[0] != 0:
        log("%s %s: the base already fails %d tests; they do not count as catches" % (set_name, task, len(base_tests[1])))
    recs = []
    for vid in sorted(man):
        b = man[vid]
        rec = {"set": set_name, "task": task, "bug": vid, "class": vid[:2]}
        vdir = os.path.join(tdir, "variants", vid)
        status = b.get("status", "variant")
        if status == "inexpressible":
            rec["caught_by"] = "inexpressible"
        elif not os.path.isdir(vdir):
            rec["caught_by"] = "missing"
        else:
            with tempfile.TemporaryDirectory() as tmp:
                w = os.path.join(tmp, task)
                overlay(base_dir, vdir, w)
                errs, warns, _, _ = c.xo_check(w)
                rec["warnings"] = sorted({x.get("code") for x in warns
                                          if (x.get("code"), x.get("message")) not in base_warn})
                if errs:
                    rec["caught_by"], rec["codes"] = "check", sorted({x.get("code") for x in errs})
                else:
                    rc, out = c.xo_test(w)
                    caught, new = new_test_failures(rc, out, base_tests)
                    rec["caught_by"] = "test" if caught else "none"
                    if rc != 0:
                        rec["failed"] = new[:3]
                    if rc != 0 and not caught:
                        rec["base_failures_only"] = True
        log(json.dumps(rec))
        recs.append(rec)
    return recs


CLASS_NAMES = {"X1": "ignored error", "X2": "nil dereference", "X3": "missing case", "X4": "data race",
               "X5": "goroutine or resource leak", "X6": "lost update", "N1": "off by one",
               "N2": "wrong status or kind", "N3": "wrong arithmetic", "N4": "wrong condition"}


def planted_report(args, recs):
    L = ["# Agent benchmark score: planted bugs", ""]
    L.append("Scored %s with `bench/harness/score.py planted` on %s (xo built from this checkout)." % (
        "the existing planted sets" if args.claude else "`%s`" % args.dir, time.strftime("%Y-%m-%d")))
    L.append("")
    go = [r for r in recs if r["set"] == "go"]
    if go:
        L.append("## Go (bugs planted in the plain Go solutions)")
        L.append("")
        L.append("| Class | Plain caught | Plain + strict caught | Escaped both |")
        L.append("|---|---|---|---|")
        tot = {"X": [0, 0, 0, 0], "N": [0, 0, 0, 0]}
        for cl in CLASS_NAMES:
            rs = [r for r in go if r["class"] == cl and r.get("caught_by") != "missing"]
            if not rs:
                continue
            p = sum(1 for r in rs if r["plain_caught_by"] != "none")
            ps = sum(1 for r in rs if r["plain_caught_by"] != "none" or r["strict_new_findings"])
            L.append("| %s %s | %d/%d | %d/%d | %d |" % (cl, CLASS_NAMES[cl], p, len(rs), ps, len(rs), len(rs) - ps))
            t = tot[cl[0]]
            t[0] += p
            t[1] += ps
            t[2] += len(rs) - ps
            t[3] += len(rs)
        for k, label in (("X", "X total (Xo targets)"), ("N", "N total (not Xo targets)")):
            t = tot[k]
            if t[3]:
                L.append("| **%s** | **%d/%d** | **%d/%d** | **%d** |" % (label, t[0], t[3], t[1], t[3], t[2]))
        L.append("")
        L.append("A strict tool counts only for findings the unmodified base does not produce. Published: %s." % PUBLISHED_PLANTED["go"])
        L.append("")
    for set_name in ("xo-translate", "xo-independent"):
        xs = [r for r in recs if r["set"] == set_name]
        if not xs:
            continue
        L.append("## Xo, %s author" % set_name.split("-")[1])
        L.append("")
        L.append("| Class | Inexpressible | Caught by `xo check` | Caught by `xo test` | Escaped (new warning) | Escaped | Missing |")
        L.append("|---|---|---|---|---|---|---|")
        tot = {}
        for cl in CLASS_NAMES:
            rs = [r for r in xs if r["class"] == cl]
            if not rs:
                continue
            ie = sum(1 for r in rs if r["caught_by"] == "inexpressible")
            ch = sum(1 for r in rs if r["caught_by"] == "check")
            te = sum(1 for r in rs if r["caught_by"] == "test")
            wn = sum(1 for r in rs if r["caught_by"] == "none" and r.get("warnings"))
            es = sum(1 for r in rs if r["caught_by"] == "none" and not r.get("warnings"))
            mi = sum(1 for r in rs if r["caught_by"] == "missing")
            L.append("| %s %s | %d | %d | %d | %d | %d | %d |" % (cl, CLASS_NAMES[cl], ie, ch, te, wn, es, mi))
            t = tot.setdefault(cl[0], [0, 0, 0, 0, 0, 0])
            for i, v in enumerate((ie, ch, te, wn, es, mi)):
                t[i] += v
        for k in ("X", "N"):
            if k in tot:
                t = tot[k]
                n = sum(t)
                L.append("| **%s total** | **%d** | **%d** | **%d** | **%d** | **%d** | **%d** |" % (k, *t))
                L.append("")
                L.append("%s: prevented or caught **%d/%d**, caught or warned %d/%d." % (
                    k, t[0] + t[1] + t[2], n, t[0] + t[1] + t[2] + t[3], n))
        L.append("")
        L.append("Caught by `xo test` counts only tests that pass on the unplanted base: tests "
                 "that already fail there are subtracted.")
        L.append("")
        L.append("Published: %s." % PUBLISHED_PLANTED[set_name])
        L.append("")
    return "\n".join(L) + "\n"


# ---------------------------------------------------------------- main

def write_out(args, text, recs):
    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            f.write(text)
        log("wrote " + args.out)
    else:
        sys.stdout.write(text)
    if args.json:
        with open(args.json, "w", encoding="utf-8") as f:
            for r in recs:
                f.write(json.dumps(r) + "\n")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("mode", choices=["solutions", "planted"])
    ap.add_argument("dir", nargs="?", help="output directory of a model run")
    ap.add_argument("--claude", action="store_true", help="score the existing Claude outputs in bench/")
    ap.add_argument("--tasks", default="", help="comma separated, default all")
    ap.add_argument("--arms", default=",".join(c.ARMS))
    ap.add_argument("--sets", default=",".join(c.PLANTED_SETS))
    ap.add_argument("--out")
    ap.add_argument("--json")
    ap.add_argument("--no-hidden", action="store_true")
    ap.add_argument("--nice", type=int, default=10, help="lower this process's priority (default 10)")
    args = ap.parse_args()
    if not args.claude and not args.dir:
        ap.error("give a directory or --claude")
    if args.nice:
        os.nice(args.nice)
    if args.mode == "solutions":
        args.tasks = [c.task_name(x) for x in args.tasks.split(",")] if args.tasks else list(c.TASKS)
        args.arms = [a for a in args.arms.split(",") if a]
        recs = []
        for arm, t, src in cell_dirs(args):
            if not src:
                log("missing: %s %s" % (arm, t))
                continue
            log("scoring %s %s (%s)" % (arm, t, src))
            r = score_cell(arm, t, src, hidden=not args.no_hidden)
            log("  green=%s stage=%s diagnostics=%s runs=%s" % (r["final"]["green"], r["final"].get("stage"),
                                                               r["diagnostics"], fmt_runs(r)))
            recs.append(r)
        write_out(args, solutions_report(args, recs), recs)
    else:
        args.tasks = [c.task_name(x) for x in args.tasks.split(",")] if args.tasks else list(c.PLANTED_TASKS)
        recs = []
        for s in [x for x in args.sets.split(",") if x]:
            root = os.path.join(c.ROOT, CLAUDE_PLANTED[s]) if args.claude else os.path.join(args.dir, "planted", s)
            for t in args.tasks:
                tdir = os.path.join(root, t)
                if not os.path.exists(os.path.join(tdir, "manifest.json")):
                    log("missing: %s %s" % (s, t))
                    continue
                log("planted %s %s" % (s, t))
                recs += planted_go(tdir, t) if s == "go" else planted_xo(tdir, t, s)
        write_out(args, planted_report(args, recs), recs)


if __name__ == "__main__":
    main()
