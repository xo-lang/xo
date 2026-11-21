#!/usr/bin/env python3
"""golden-rt-strings.py GOLDEN_ROOT < DIFF: the internal/llvm goldens whose
want.txt or want_go.txt holds a string a std/rt/*.go change adds or removes.

A std/rt change (the Go backend's runtime) is not an xo build input, so the
gate's LLVM selection names no golden for it and TestGoldenGoParity ran
only its every-16th sample: #540's BytesBuilder.put fault text reached main
with its four goldens unrun on the Go backend (task #552). scripts/gate.sh
runs these goldens on the Go backend too.

DIFF is `git diff -U0 BASE HEAD -- 'std/rt/*.go'`. Each Go string literal on
an added or removed line is split at its format verbs (%d, %v, %q, ...);
a piece of at least MIN characters (after trimming) that a want file holds
selects that golden. Prints the golden names comma separated, sorted, at
most CAP of them (the sample covers the rest), or nothing.
"""
import os
import re
import sys

MIN = 8
CAP = 64

LIT = re.compile(r'"((?:[^"\\]|\\.)*)"|`([^`]*)`')
VERB = re.compile(r'%[-+# 0-9.*]*[a-zA-Z%]')


def pieces(diff):
    out = set()
    for line in diff.splitlines():
        if line.startswith(('+++', '---')) or not line.startswith(('+', '-')):
            continue
        for m in LIT.finditer(line[1:]):
            s = m.group(1) if m.group(1) is not None else m.group(2)
            s = s.replace('\\n', '\n').replace('\\t', '\t').replace('\\"', '"').replace('\\\\', '\\')
            for p in VERB.split(s):
                for q in p.split('\n'):
                    q = q.strip()
                    if len(q) >= MIN:
                        out.add(q)
    return out


def main():
    if len(sys.argv) != 2:
        print("usage: golden-rt-strings.py GOLDEN_ROOT < DIFF", file=sys.stderr)
        return 2
    root = sys.argv[1]
    ps = pieces(sys.stdin.read())
    if not ps:
        return 0
    hits = []
    for name in sorted(os.listdir(root)):
        for w in ("want.txt", "want_go.txt"):
            f = os.path.join(root, name, w)
            try:
                with open(f, encoding="utf-8", errors="replace") as fh:
                    text = fh.read()
            except OSError:
                continue
            if any(p in text for p in ps):
                hits.append(name)
                break
    print(",".join(hits[:CAP]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
