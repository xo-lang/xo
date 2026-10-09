"""Measure habit diagnostics (decision 0087).

For each case, run `xo check --json`, count the error diagnostics and how
many carry a machine fix, apply `xo fix`, and check again. Each case is
checked in its own process: `xo check a b` skips a directory whose module
path repeats (knowledgebase/gotchas/xo-check-same-module-path.md).

Usage: python3 -I measure.py <xo binary> <cases dir> <mode>
  mode "snap":   each subdirectory is a module tree (bench/habits/reconstruct.py)
  mode "golden": each subdirectory holds in.xo (internal/fix/testdata/habits)
  mode "drafts": each <name>.xo.txt file is one module (bench/habits/drafts)
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile


def check(xo, d):
    out = subprocess.run([xo, "check", "--json", d], capture_output=True, text=True).stdout
    try:
        res = json.loads(out)
    except ValueError:
        return None
    return [x for x in res.get("diagnostics", []) if x["severity"] == "error"]


def main():
    xo, cases, mode = os.path.abspath(sys.argv[1]), sys.argv[2], sys.argv[3]
    work = tempfile.mkdtemp(prefix="habits-")
    rows = []
    for name in sorted(os.listdir(cases)):
        src = os.path.join(cases, name)
        dst = os.path.join(work, name.replace(".xo.txt", ""))
        if mode == "golden" and os.path.isdir(src):
            os.makedirs(os.path.join(dst, "m"))
            shutil.copy(os.path.join(src, "in.xo"), os.path.join(dst, "m", "main.xo"))
        elif mode == "snap" and os.path.isdir(src):
            shutil.copytree(src, dst)
        elif mode == "drafts" and name.endswith(".xo.txt"):
            os.makedirs(dst)
            shutil.copy(src, os.path.join(dst, "main.xo"))
        else:
            continue
        before = check(xo, dst)
        if before is None:
            rows.append((name, -1, -1, -1))
            continue
        fixed = sum(1 for d in before if d["fixes"])
        subprocess.run([xo, "fix", dst], capture_output=True, text=True)
        after = check(xo, dst)
        rows.append((name, len(before), fixed, len(after) if after is not None else -1))
    shutil.rmtree(work)
    for r in rows:
        print("%-28s errors=%-3d with_fix=%-3d after_fix=%d" % r)
    print("cases=%d errors=%d with_fix=%d all_errors_fixable=%d green_after_fix=%d" % (
        len(rows),
        sum(max(r[1], 0) for r in rows),
        sum(max(r[2], 0) for r in rows),
        sum(1 for r in rows if r[1] > 0 and r[1] == r[2]),
        sum(1 for r in rows if r[3] == 0),
    ))


if __name__ == "__main__":
    main()
