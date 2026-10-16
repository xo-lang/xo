#!/usr/bin/env python3
"""The read only data of an Xo arm64 assembly file (xo build
--backend=arm64 --targets=linux/arm64 prints its path) by kind of label
(bench/results/binary-size-native-2026-10-16.md).

    rodata.py PROG.s

A data directive's bytes go to the label before it: .Lxo_<unit>_<kind>N
counts as <kind> (str: string literals, loc: fault locations, and so on),
other labels by name with trailing digits dropped."""
import collections, re, sys

SIZES = {".byte": 1, ".hword": 2, ".short": 2, ".word": 4, ".long": 4, ".4byte": 4, ".quad": 8, ".xword": 8, ".8byte": 8}

def main():
    sect, label = "", ""
    kinds = collections.Counter()
    sizes = collections.Counter()
    cur = None
    for line in open(sys.argv[1], encoding="utf-8", errors="replace"):
        s = line.strip()
        if s.startswith(".section"):
            sect = s.split()[1].rstrip(",")
            continue
        if s in (".text", ".data", ".bss"):
            sect = s
            continue
        if not sect.startswith(".rodata"):
            continue
        m = re.match(r"^([.\w$]+):", s)
        if m:
            name = m.group(1)
            k = re.match(r"^\.Lxo_\w+?_([a-z]+)\d+$", name)
            label = k.group(1) if k else re.sub(r"\d+$", "", name)
            cur = (label, name)
            continue
        parts = s.split(None, 1)
        if not parts:
            continue
        d, arg = parts[0], parts[1] if len(parts) > 1 else ""
        if d in SIZES:
            n = SIZES[d] * len(arg.split(","))
        elif d in (".ascii", ".asciz", ".string"):
            body = arg.strip()[1:-1]
            n = len(bytes(body, "utf-8").decode("unicode_escape").encode("latin-1", "replace")) + (d != ".ascii")
        elif d in (".zero", ".space", ".skip"):
            n = int(arg.split(",")[0], 0)
        else:
            continue
        sizes[cur] += n
        kinds[label] += n
    # string literals of 256 bytes or more are tables (crypto constants,
    # hpack, Unicode); fault locations print where a check failed
    for (lab, _), n in sizes.items():
        if lab == "str" and n >= 256:
            kinds["str"] -= n
            kinds["str (256 bytes or more: tables)"] += n
    total = sum(kinds.values())
    for k, n in kinds.most_common(25):
        print(f"{k:40} {n:>10}")
    print(f"{'total':40} {total:>10}")

main()
