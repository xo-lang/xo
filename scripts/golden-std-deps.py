#!/usr/bin/env python3
"""golden-std-deps.py ROOT DEPS|inputs:DIR: which LLVM goldens a std only change can
reach (scripts/gate.sh's llvm step, lead 2026-11-15: build HEAD's IR only
for the goldens whose inputs the change touches).

Reads the changed files (repository paths, one a line) on stdin and prints
"all", "none", or the comma separated golden names whose IR the change can
reach. DEPS is the base manifest's deps file (TestGoldenIRManifest,
XO_LLVM_IR_DEPS: a golden's name and the std files its IR's debug
information names), or "-" for none.

A std module's key is its file's first path part under std/ (or
std/native/), with ".xo" and a "_native" twin's suffix dropped: std/csv.xo,
std/native/csv.xo and std/sqlite/x.xo are csv, csv and sqlite. A golden's
keys are the closure, over the `use` lines of every std file of a key, of
its own sources' std imports and the keys of the files its base IR names
(which holds the modules the compiler adds by itself, a derive's codec).
"all" when the change touches anything else that makes IR (the compiler
but its wasm and Mach-O parts, std/rt, a std file that is not a module's
.xo source), a key the prelude's or the runtime's closure reaches (every
program has them), or a golden that imports a module from outside std.

inputs:DIR in DEPS's place (lead, 2026-11-15, (B) of the gate-time
plan): DIR holds NAME.txt, `xo build --list-inputs` of each golden at
HEAD (gate.sh golden_inputs), and a golden is hit when one of its input
files changed: exact files in place of the key closure, with no base
manifest's deps needed (the compiler's own additions, a derive's codec,
are in the list). A golden with no NAME.txt, a std .xo file that no
longer exists (a removal) and the "all" rules above still give it or
"all".
"""
import os
import re
import sys

USE = re.compile(r'^\s*(?:pub\s+)?use\s+("?)([A-Za-z0-9_./-]+)\1')
# every program: the prelude, the runtimes, and what the compiler names
# for every build (compiler/: std/prelude, native/rt, native/llrt,
# native/sys_*, std/internal)
ROOTS = ["prelude", "rt", "internal", "sys_c", "sys_darwin", "sys_windows"]


def key_of_std(rel):
    """rel: a path under std/ ("native/csv.xo"); its module key"""
    parts = rel.split("/")
    if parts[0] == "native" and len(parts) > 1:
        parts = parts[1:]
    k = parts[0]
    if k == "llrt" and len(parts) > 1:
        # each runtime module its own key (a change under llrt/ is "all")
        n = parts[1][:-3] if parts[1].endswith(".xo") else parts[1]
        return "llrt/" + n
    if k.endswith(".xo"):
        k = k[:-3]
    if k.endswith("_native"):
        k = k[:-7]
    return k


def key_of_use(path):
    """a use line's path ("std/csv", "native/llrt/atomic"); its key or
    None for one outside std ("c", "github.com/...")"""
    if path.startswith("std/"):
        path = path[4:]
    elif not path.startswith("native/"):
        return None
    return key_of_std(path + ".xo")


def uses_of(path):
    out = []
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                m = USE.match(line)
                if m:
                    out.append(m.group(2))
    except OSError:
        pass
    return out


def by_inputs(root, d):
    changed = []
    for p in sys.stdin.read().split():
        if p.endswith(".md") or p.endswith("_test.xo"):
            continue
        if p.startswith("std/changes/") or p.startswith("std/evidence/"):
            continue
        if p.startswith("compiler/wasm/") or p.startswith("compiler/macho/"):
            continue
        if p.startswith("compiler/") or p.startswith("std/rt/"):
            print("all"); return
        if p.startswith("std/"):
            if not p.endswith(".xo") or not os.path.exists(os.path.join(root, p)):
                print("all"); return
            changed.append(p)
    if not changed:
        print("none"); return
    gdir = os.path.join(root, "internal", "llvm", "testdata", "golden")
    want = set(changed)
    hit = []
    for g in sorted(os.listdir(gdir)):
        if not os.path.isdir(os.path.join(gdir, g)):
            continue
        try:
            with open(os.path.join(d, g + ".txt")) as f:
                ins = set(f.read().split())
        except OSError:
            hit.append(g); continue
        if not ins or ins & want:
            hit.append(g)
    print(",".join(hit) if hit else "none")


def main():
    root, depsf = sys.argv[1], sys.argv[2]
    if depsf.startswith("inputs:"):
        return by_inputs(root, depsf[len("inputs:"):])
    std = os.path.join(root, "std")
    # key -> the std .xo sources of that key (tests left out)
    files = {}
    for d, dirs, names in os.walk(std):
        rel_d = os.path.relpath(d, std)
        if rel_d.split("/")[0] in ("rt", "changes", "evidence"):
            dirs[:] = []
            continue
        for n in names:
            if n.endswith(".xo") and not n.endswith("_test.xo"):
                rel = os.path.normpath(os.path.join(rel_d, n))
                files.setdefault(key_of_std(rel), []).append(os.path.join(d, n))
    edges = {}
    for k, fs in files.items():
        s = set()
        for f in fs:
            for u in uses_of(f):
                uk = key_of_use(u)
                if uk and uk != k:
                    s.add(uk)
        edges[k] = s

    def closure(keys):
        seen, todo = set(), list(keys)
        while todo:
            k = todo.pop()
            if k in seen:
                continue
            seen.add(k)
            todo.extend(edges.get(k, ()))
        return seen

    always = closure([k for k in ROOTS if k in files])
    changed = set()
    for p in sys.stdin.read().split():
        if p.endswith(".md") or p.endswith("_test.xo"):
            continue
        if p.startswith("std/changes/") or p.startswith("std/evidence/"):
            continue
        if p.startswith("compiler/wasm/") or p.startswith("compiler/macho/"):
            continue
        if p.startswith("compiler/") or p.startswith("std/rt/"):
            print("all"); return
        if p.startswith("std/"):
            if not p.endswith(".xo"):
                print("all"); return
            k = key_of_std(p[4:])
            if k in always or k.startswith("llrt"):
                print("all"); return
            changed.add(k)
    if not changed:
        print("none"); return
    base = {}
    if depsf != "-":
        with open(depsf) as f:
            for line in f:
                w = line.split()
                if w:
                    base[w[0]] = w[1:]
    gdir = os.path.join(root, "internal", "llvm", "testdata", "golden")
    hit = []
    for g in sorted(os.listdir(gdir)):
        gd = os.path.join(gdir, g)
        if not os.path.isdir(gd):
            continue
        keys, outside = set(), False
        for d, _, names in os.walk(gd):
            for n in names:
                if n.endswith(".xo"):
                    for u in uses_of(os.path.join(d, n)):
                        uk = key_of_use(u)
                        if uk:
                            keys.add(uk)
                        elif u != "c" and not u.startswith("c/"):
                            outside = True
        if outside:
            hit.append(g); continue
        for f in base.get(g, ()):
            if f.startswith("std/"):
                keys.add(key_of_std(f[4:]))
        if g not in base:
            # no deps for it (a new golden): build it
            hit.append(g); continue
        if closure(keys) & changed:
            hit.append(g)
    print(",".join(hit) if hit else "none")


if __name__ == "__main__":
    main()
