#!/usr/bin/env python3
"""Mutation probe for the Xo checker over bench/xo/<version>/.

Applies one mechanical mutation at a time to a copy of each task, runs
`xo check`, and records whether any error is reported. Mutations model the
bug classes the checker is meant to catch (and one it is not).
Usage: mutate.py <version dir> <xo binary>. Writes mutations.jsonl there.
"""
import json, os, re, shutil, subprocess, sys, tempfile

OPS = {
    # name: (regex on a line, replacement or None to delete the line, class)
    "drop_try": (re.compile(r"\)\?(?=[\s.)\],])"), ")", "ignored error"),
    "drop_match_arm": (re.compile(r"^\s*[A-Z][A-Za-z.]*(\(.*\))?\s*=>.*$"), None, "missing case"),
    "var_to_let": (re.compile(r"^(\s*)var (\w+)"), r"\1let \2", "mutation rule"),
    "drop_store_back": (re.compile(r"^\s*[\w.]+\.(put|push|set|insert)\(.*\)\s*$"), None, "lost update"),
    "drop_uses": (re.compile(r"^(\s*)uses [^{]*$"), None, "effects"),
    "some_to_raw": (re.compile(r"\bNone\b"), "0", "nil or optional"),
    "flip_compare": (re.compile(r" < "), " <= ", "logic (not a target)"),
}

def check(xo, d):
    p = subprocess.run([xo, "check", d], capture_output=True, text=True, timeout=120)
    m = re.search(r"(\d+) error", p.stdout + p.stderr)
    return int(m.group(1)) if m else -1

def main():
    root, xo = sys.argv[1], sys.argv[2]
    out = open(os.path.join(root, "mutations.jsonl"), "w")
    for task in sorted(t for t in os.listdir(root) if t[:2].isdigit()):
        src = os.path.join(root, task)
        files = [os.path.join(dp, f) for dp, _, fs in os.walk(src) for f in fs if f.endswith(".xo")]
        for f in files:
            lines = open(f).read().split("\n")
            for op, (rx, rep, cls) in OPS.items():
                for i, ln in enumerate(lines):
                    if ln.lstrip().startswith("//") or not rx.search(ln):
                        continue
                    with tempfile.TemporaryDirectory() as w:
                        dst = os.path.join(w, task)
                        shutil.copytree(src, dst)
                        new = list(lines)
                        if rep is None:
                            del new[i]
                        else:
                            new[i] = rx.sub(rep, ln, count=1)
                        open(os.path.join(dst, os.path.relpath(f, src)), "w").write("\n".join(new))
                        errs = check(xo, dst)
                    rec = {"task": task, "file": os.path.relpath(f, src), "line": i + 1, "op": op,
                           "class": cls, "caught": errs > 0, "errors": errs}
                    out.write(json.dumps(rec) + "\n"); out.flush()
    out.close()

if __name__ == "__main__":
    main()
