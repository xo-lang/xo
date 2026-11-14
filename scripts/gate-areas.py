#!/usr/bin/env python3
# gate-areas.py PKG AREAS NOTE < changed paths: prints the go test -skip
# pattern for PKG's tests that AREAS (scripts/gate-areas.txt) scopes to
# areas none of the changed paths touch, and writes a one line summary to
# NOTE. Called by scripts/gate.sh gate_skip (lead, 2026-11-14). With
# --check it validates AREAS against the repository instead (every named
# test exists in its package, every area is defined, every regex compiles)
# and exits 1 on a problem: internal/repotest TestGateAreasValid runs it.
import os
import re
import sys


def parse(path):
    areas, rules = {}, []
    for n, line in enumerate(open(path), 1):
        line = line.split("#", 1)[0].strip() if not line.lstrip().startswith("@") else line.strip()
        if not line:
            continue
        f = line.split()
        if f[0].startswith("@"):
            areas[f[0]] = " ".join(f[1:])
        elif len(f) >= 3:
            rules.append((n, f[0], f[1], f[2:]))
        else:
            raise SystemExit(f"{path}:{n}: want PKG TEST AREA...: {line}")
    return areas, rules


def area_regexes(areas, words, where):
    out = []
    for w in words:
        if w == "tier":
            continue
        if w.startswith("@"):
            if w not in areas:
                raise SystemExit(f"{where}: area {w} is not defined")
            out.append(areas[w])
        else:
            out.append(w)
    return [re.compile(r) for r in out]


def test_files(pkg):
    # top-level test name -> its _test.go file, from the package's sources
    m = {}
    for fn in sorted(os.listdir(pkg)):
        if not fn.endswith("_test.go"):
            continue
        for line in open(os.path.join(pkg, fn), errors="replace"):
            g = re.match(r"func (Test[A-Za-z0-9_]*)\(t \*testing\.T\)", line)
            if g:
                m[g.group(1)] = f"{pkg}/{fn}"
    return m


def check(path):
    areas, rules = parse(path)
    bad = 0
    for n, pkg, test, words in rules:
        try:
            area_regexes(areas, words, f"{path}:{n}")
        except (SystemExit, re.error) as e:
            print(e)
            bad = 1
        if test == "*":
            continue
        if not os.path.isdir(pkg):
            print(f"{path}:{n}: no package {pkg}")
            bad = 1
            continue
        if test not in test_files(pkg):
            print(f"{path}:{n}: {pkg} has no top-level test {test} (renamed or removed: update the line)")
            bad = 1
    return bad


def main():
    if sys.argv[1] == "--check":
        sys.exit(check(sys.argv[2]))
    pkg, path, note = sys.argv[1], sys.argv[2], sys.argv[3]
    changed = [l.strip() for l in sys.stdin if l.strip()]
    areas, rules = parse(path)
    files = test_files(pkg) if os.path.isdir(pkg) else {}
    for n, p, test, words in rules:
        if p == pkg and test == "*":
            if any(r.search(c) for r in area_regexes(areas, words, f"{path}:{n}") for c in changed):
                with open(note, "w") as f:
                    f.write("every test (the change touches an area that runs them all)\n")
                return
    skip, ran = [], 0
    for n, p, test, words in rules:
        if p != pkg or test == "*":
            continue
        if files.get(test) in changed:
            ran += 1
            continue
        if "tier" in words and len(words) == 1:
            skip.append(test)
            continue
        if any(r.search(c) for r in area_regexes(areas, words, f"{path}:{n}") for c in changed):
            ran += 1
        else:
            skip.append(test)
    with open(note, "w") as f:
        f.write(f"{len(skip)} scoped tests skipped, {ran} scoped tests run (scripts/gate-areas.txt; the full tier runs every test)\n")
    if skip:
        print("^(" + "|".join(sorted(skip)) + ")$")


main()
