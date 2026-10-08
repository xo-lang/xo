#!/usr/bin/env python3
"""Run planted bug variants through the plain and strict Go loops.

A variant counts as caught by the plain loop if build, vet, or test fails.
It counts as caught by a strict tool only if that tool reports a finding that
the unmodified base does not (line and column numbers are ignored so edits do
not shift matches). Usage: run_detection.py [task ...]. Writes results.jsonl.
"""
import json, os, re, shutil, subprocess, sys, tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
TOOLS = os.path.join(HERE, "..", "..", ".tools", "bin")
STRICT = [
    ("vet", ["go", "vet", "./..."]),
    ("staticcheck", [os.path.join(TOOLS, "staticcheck"), "./..."]),
    ("errcheck", [os.path.join(TOOLS, "errcheck"), "./..."]),
    ("nilaway", [os.path.join(TOOLS, "nilaway"), "-pretty-print=false", "./..."]),
    ("exhaustive", [os.path.join(TOOLS, "exhaustive"), "./..."]),
    ("race", ["go", "test", "-race", "-count=1", "-timeout", "180s", "./..."]),
]

def run(cmd, cwd):
    p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr

def norm(out, root):
    lines = set()
    out = out.replace(os.path.realpath(root), "").replace(root, "")
    for ln in out.splitlines():
        ln = re.sub(r":\d+(:\d+)?", "", ln).strip()
        if ln and not ln.startswith(("ok ", "?", "#", "FAIL\t", "--- ", "PASS", "=== ")):
            lines.add(ln)
    return lines

def strict_findings(root):
    res = {}
    for name, cmd in STRICT:
        rc, out = run(cmd, root)
        if name == "race":
            # Test failures are already counted by the plain loop; only a
            # reported data race counts for the race detector.
            races = {ln.strip() for ln in out.splitlines() if "DATA RACE" in ln}
            res[name] = (1 if races else 0, races)
        else:
            res[name] = (rc, norm(out, root))
    return res

def plain(root):
    for step, cmd in [("build", ["go", "build", "./..."]), ("vet", ["go", "vet", "./..."]),
                      ("test", ["go", "test", "-count=1", "-timeout", "180s", "./..."])]:
        rc, _ = run(cmd, root)
        if rc != 0:
            return step
    return "none"

def copy_tree(base, variant, dst):
    shutil.copytree(base, dst, dirs_exist_ok=True)
    if variant:
        shutil.copytree(variant, dst, dirs_exist_ok=True)

WORK = os.path.join(tempfile.gettempdir(), "xo-planted-work", "m")

def fresh_workdir():
    shutil.rmtree(WORK, ignore_errors=True)
    os.makedirs(WORK)
    return WORK

def main():
    tasks = sys.argv[1:] or sorted(d for d in os.listdir(HERE) if d[:2].isdigit())
    out_path = os.path.join(HERE, "results.jsonl")
    for task in tasks:
        tdir = os.path.join(HERE, task)
        manifest = {b["id"]: b for b in json.load(open(os.path.join(tdir, "manifest.json")))}
        w = fresh_workdir()
        copy_tree(os.path.join(tdir, "base"), None, w)
        base = strict_findings(w)
        for vid in sorted(os.listdir(os.path.join(tdir, "variants"))):
            w = fresh_workdir()
            copy_tree(os.path.join(tdir, "base"), os.path.join(tdir, "variants", vid), w)
            p = plain(w)
            caught = []
            for name, (rc, found) in strict_findings(w).items():
                brc, bfound = base[name]
                if (rc != 0 and brc == 0) or (found - bfound):
                    caught.append(name)
            rec = {"task": task, "bug": vid, "class": manifest.get(vid, {}).get("class", "?"),
                   "plain_caught_by": p, "strict_new_findings": caught}
            with open(out_path, "a") as f:
                f.write(json.dumps(rec) + "\n")
            print(json.dumps(rec), flush=True)

if __name__ == "__main__":
    main()
