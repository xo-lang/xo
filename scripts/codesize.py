#!/usr/bin/env python3
"""codesize.py BINARY: where the __text bytes of a native (darwin/arm64)
xo build go (bench/results/xo-code-size-2026-10-18.md).

Prints, from the symbol table and one objdump pass:
  - functions by size, by module, generic instantiations (name[...]);
  - instructions by kind (stack loads and stores, other memory, constant
    addresses, branches, release calls, ...);
  - word copies through the x16 scratch (ldr x16, [a]; str x16, [b]) by
    run length: a value split into words, each spilled and moved;
  - runs of consecutive release calls (a large value dropped field by
    field) and the most loaded constant addresses.

Needs objdump and nm (Xcode command line tools). Release functions are
recognized by their code (the shared refcount decrement shape), not by
name, since long names are hashed.
"""
import collections
import re
import subprocess
import sys


def demangle(n):
    n = n.strip("<>:")
    if n.startswith("_xo_f_"):
        n = n[6:]
    elif n.startswith("_"):
        n = n[1:]
    out, i = "", 0
    while i < len(n):
        if n[i] == "_" and n[i + 1:i + 2] == "_":
            out += "_"
            i += 2
        elif n[i] == "_" and re.match("[0-9a-f]{2}$", n[i + 1:i + 3]):
            out += chr(int(n[i + 1:i + 3], 16))
            i += 3
        else:
            out += n[i]
            i += 1
    return out


def module_of(d):
    if d.startswith("xo_h"):
        return "<hashed>"
    base = d.split("[")[0]
    return base.split(".")[0] if "." in base else d


def text_bounds(binary):
    out = subprocess.run(["otool", "-l", binary], capture_output=True, text=True).stdout
    sect = out.split("sectname __text", 1)[1]
    addr = int(re.search(r"addr (0x[0-9a-f]+)", sect).group(1), 16)
    size = int(re.search(r"size (0x[0-9a-f]+)", sect).group(1), 16)
    return addr, addr + size


def symbols(binary):
    lo, hi = text_bounds(binary)
    nm = subprocess.run(["nm", "-n", binary], capture_output=True, text=True).stdout
    syms = []
    for l in nm.splitlines():
        p = l.split()
        if len(p) >= 3 and p[1] in "tT":
            a = int(p[0], 16)
            if lo <= a < hi:
                syms.append((a, p[2]))
    syms.sort()
    rows = []
    for i, (a, n) in enumerate(syms):
        nx = syms[i + 1][0] if i + 1 < len(syms) else hi
        rows.append((nx - a, demangle(n), n))
    return rows, hi - lo


def report_symbols(rows, total):
    rows = sorted(rows, reverse=True)
    print(f"__text {total / 1e6:.2f} MB, {len(rows)} functions")
    for n in (10, 100, 500, 1000):
        print(f"  top {n}: {sum(r[0] for r in rows[:n]) / total * 100:.1f}%")
    print("largest functions:")
    for s, d, _ in rows[:15]:
        print(f"  {s / 1e3:8.1f} KB  {d[:100]}")
    mods, cnt = collections.Counter(), collections.Counter()
    for s, d, _ in rows:
        mods[module_of(d)] += s
        cnt[module_of(d)] += 1
    print("by module:")
    for m, s in mods.most_common(16):
        print(f"  {s / 1e6:6.2f} MB {s / total * 100:5.1f}% {cnt[m]:5d} functions  {m}")
    gen, gc = collections.Counter(), collections.Counter()
    for s, d, _ in rows:
        if "[" in d:
            gen[d.split("[")[0]] += s
            gc[d.split("[")[0]] += 1
    print(f"generic instantiations: {sum(gen.values()) / 1e6:.2f} MB in {sum(gc.values())} copies")
    for b, s in gen.most_common(6):
        print(f"  {s / 1e3:8.0f} KB {gc[b]:4d} copies  {b}")


RELEASE_SHAPE = ["ldr x1, [x0]", "cmp x1, #0x1", "b.le"]


def release_funcs(binary, rows):
    """Names of functions whose first instructions are the refcount
    decrement (obj_rc_dec and its per element size copies)."""
    small = [r[2] for r in rows if 40 <= r[0] <= 80]
    found = set()
    for i in range(0, len(small), 400):
        args = ["objdump", "-d", "--no-show-raw-insn", "--no-leading-addr"]
        args += [f"--disassemble-symbols={n}" for n in small[i:i + 400]] + [binary]
        out = subprocess.run(args, capture_output=True, text=True).stdout
        cur, body = None, []
        for l in out.splitlines() + ["<end>:"]:
            l = l.strip()
            if l.endswith(":") and l.startswith("<"):
                if cur and all(body[k].startswith(RELEASE_SHAPE[k]) for k in range(min(3, len(body)))) and len(body) >= 3:
                    found.add(cur)
                cur, body = l.strip("<>:"), []
            elif l and cur:
                body.append(" ".join(l.split()))
    return found


def report_insns(binary, rel):
    kinds = collections.Counter()
    copy_runs = collections.Counter()
    rel_runs = collections.Counter()
    adrp = collections.Counter()
    n = 0
    run = rrun = 0
    ld16 = False
    prev = ("", "")
    proc = subprocess.Popen(["objdump", "-d", "--no-show-raw-insn", "--no-leading-addr", binary],
                            stdout=subprocess.PIPE, text=True)

    def flush_copy():
        nonlocal run
        if run:
            copy_runs[run] += 1
        run = 0

    def flush_rel():
        nonlocal rrun
        if rrun:
            rel_runs[rrun] += 1
        rrun = 0

    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        if line.endswith(":"):
            flush_copy()
            flush_rel()
            ld16 = False
            continue
        p = line.split(None, 1)
        op, a = p[0], (p[1] if len(p) > 1 else "")
        n += 1
        if op == "bl":
            m = re.search(r"<(\S+?)(\+0x[0-9a-f]+)?>", a)
            t = m.group(1) if m else ""
            if t in rel:
                kinds["release call"] += 1
                rrun += 1
            elif "fault_" in t:
                kinds["fault call"] += 1
            else:
                kinds["other call"] += 1
                flush_rel()
        elif op in ("ldr", "str", "ldp", "stp", "ldur", "stur") and "[sp" in a:
            kinds["stack load/store"] += 1
        elif op.startswith(("ldr", "str", "ldp", "stp", "ldur", "stur")):
            kinds["other load/store"] += 1
        elif op == "adrp" or (op == "add" and prev[0] == "adrp"):
            kinds["constant address (adrp+add)"] += 1
            if op == "adrp":
                m = re.search(r"<([^+>]+)", a)
                adrp[m.group(1) if m else "?"] += 1
        elif op in ("stadd", "ldadd", "ldaddal"):
            kinds["atomic refcount"] += 1
        elif op.startswith("b.") or op in ("b", "cbz", "cbnz", "tbz", "tbnz", "ret", "br", "blr"):
            kinds["branch"] += 1
        elif op.startswith("mov"):
            kinds["mov"] += 1
        elif op in ("cmp", "cmn", "tst", "ccmp"):
            kinds["compare"] += 1
        else:
            kinds["other"] += 1
        # word copies through x16
        if op == "ldr" and a.startswith("x16, ["):
            ld16 = True
        elif ld16 and op == "str" and a.startswith("x16, ["):
            run += 1
            ld16 = False
        else:
            ld16 = False
            if op not in ("ldp", "stp", "ldr", "str"):
                flush_copy()
        if not (op == "bl" or op in ("ldr", "mov", "ldp", "add", "cbz")):
            flush_rel()
        prev = (op, a)
    flush_copy()
    flush_rel()
    print(f"instructions: {n} ({n * 4 / 1e6:.2f} MB)")
    for k, v in kinds.most_common():
        print(f"  {v * 4 / 1e6:6.2f} MB {v / n * 100:5.1f}%  {k}")
    pairs = sum(k * v for k, v in copy_runs.items())
    print(f"x16 word copies: {pairs} words, {pairs * 8 / 1e6:.2f} MB")
    for lo, hi in ((1, 3), (4, 7), (8, 15), (16, 63), (64, 10**9)):
        c = sum(k * v for k, v in copy_runs.items() if lo <= k <= hi)
        r = sum(v for k, v in copy_runs.items() if lo <= k <= hi)
        print(f"  runs of {lo}-{hi if hi < 10**9 else 'more'} words: {r} runs, {c * 8 / 1e6:.2f} MB")
    print("release calls in runs:")
    for lo, hi in ((1, 1), (2, 7), (8, 31), (32, 10**9)):
        c = sum(k * v for k, v in rel_runs.items() if lo <= k <= hi)
        r = sum(v for k, v in rel_runs.items() if lo <= k <= hi)
        print(f"  runs of {lo}-{hi if hi < 10**9 else 'more'}: {r} runs, {c} calls")
    print("constant addresses (adrp targets):")
    for t, c in adrp.most_common(5):
        print(f"  {c:8d}  {t}")


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: codesize.py BINARY")
    binary = sys.argv[1]
    rows, total = symbols(binary)
    report_symbols(rows, total)
    rel = release_funcs(binary, rows)
    print(f"release functions recognized: {len(rel)}")
    report_insns(binary, rel)


if __name__ == "__main__":
    main()
