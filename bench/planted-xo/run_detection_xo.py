#!/usr/bin/env python3
"""Run Xo planted bug variants through `xo check`, then `xo test`.

Base solutions come from bench/xo/v0.4/<task>; each variant overlays its
changed files. A test failure counts as a catch only for a test that
passes on the unplanted base (bench/harness/score.py new_test_failures).
Writes bench/planted-xo/results.jsonl.
Usage: run_detection_xo.py [task ...]
"""
import json, os, re, shutil, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
XO = os.path.join(ROOT, ".tools", "bin", "xo")
BASES = os.path.join(ROOT, "bench", "xo", "v0.4")

def run(args, cwd=None):
    p = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=600)
    return p.returncode, p.stdout + p.stderr

_base = {}

def base_warnings(task):
    # warnings the unmodified solution already has; only new ones count
    if task not in _base:
        d = os.path.join(BASES, task)
        _, txt = run([XO, "check", d])
        _base[task] = set(re.findall(r"warning (XO\d+: .*)", txt.replace(d, "")))
    return _base[task]

_base_tests = {}

def failed_tests(txt):
    # failing test names without the "(file:line:col, seed N)" suffix
    return {re.sub(r"\s*\([^()]*\)\s*$", "", x).strip() for x in re.findall(r"--- FAIL: (.*)", txt)}

def base_tests(task):
    # (rc, failing tests) of the unmodified solution; those do not count
    if task not in _base_tests:
        rc, txt = run([XO, "test", "--race", os.path.join(BASES, task)])
        _base_tests[task] = (rc, failed_tests(txt) if rc != 0 else set())
    return _base_tests[task]

def main():
    tasks = sys.argv[1:] or sorted(d for d in os.listdir(HERE) if d[:2].isdigit())
    out = open(os.path.join(HERE, "results.jsonl"), "a")
    for task in tasks:
        man = {b["id"]: b for b in json.load(open(os.path.join(HERE, task, "manifest.json")))}
        for vid, b in sorted(man.items()):
            rec = {"task": task, "bug": vid, "class": b.get("class"), "status": b.get("status")}
            if b.get("status") != "variant":
                rec["caught_by"] = "inexpressible"
            else:
                with tempfile.TemporaryDirectory() as w:
                    dst = os.path.join(w, task)
                    shutil.copytree(os.path.join(BASES, task), dst)
                    shutil.copytree(os.path.join(HERE, task, "variants", vid), dst, dirs_exist_ok=True)
                    rc, txt = run([XO, "check", dst])
                    codes = sorted(set(re.findall(r"error (XO\d+)", txt)))
                    found = set(re.findall(r"warning (XO\d+: .*)", txt.replace(dst, "")))
                    rec["warnings"] = sorted({w.split(":")[0] for w in found - base_warnings(task)})
                    if rc != 0 or codes:
                        rec["caught_by"], rec["codes"] = "check", codes
                    else:
                        rc, txt = run([XO, "test", "--race", dst])
                        brc, bnames = base_tests(task)
                        new = sorted(failed_tests(txt) - bnames) if rc != 0 else []
                        caught = rc != 0 and (brc == 0 or bool(new))
                        rec["caught_by"] = "test" if caught else "none"
                        if rc != 0:
                            rec["failed"] = (new if brc != 0 else sorted(failed_tests(txt)))[:3]
            out.write(json.dumps(rec) + "\n"); out.flush()
            print(json.dumps(rec), flush=True)

if __name__ == "__main__":
    main()
