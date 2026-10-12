#!/usr/bin/env python3
# The self hosted ELF assembler (selfhost/macho `asm --elf`, decision 0182
# stage 3) against LLVM's: scripts/elf-compare.py <macho> <file.s...>
# assembles each linux/arm64 assembly file with `<macho> asm --elf` and
# with `$ELF_CC -c` (default `clang --target=aarch64-linux-gnu`), and
# counts byte identical objects, and per section and per function (the
# .text bytes between consecutive function symbols) identical and
# differing ones, and relocation tables compared with symbol numbers
# replaced by names. ELF_KEEP=1 keeps the objects next to the inputs
# (.my.o, .ref.o).
import os, shlex, struct, subprocess, sys, tempfile, time


def parse(path):
    # name -> (bytes, [(offset, type, symbol name, addend)]); the symbols
    # (section index, value, name, info)
    b = open(path, 'rb').read()
    shoff = struct.unpack_from('<Q', b, 40)[0]
    shnum, shstrndx = struct.unpack_from('<HH', b, 60)
    hdrs = [struct.unpack_from('<IIQQQQIIQQ', b, shoff + 64 * i) for i in range(shnum)]
    strsec = hdrs[shstrndx]

    def name(tab, o):
        e = b.index(b'\0', tab + o)
        return b[tab + o:e].decode()

    names = [name(strsec[4], h[0]) for h in hdrs]
    syms = []
    for h in hdrs:
        if h[1] == 2:
            tab = hdrs[h[6]][4]
            for k in range(h[5] // 24):
                n, info, other, shn, val, size = struct.unpack_from('<IBBHQQ', b, h[4] + 24 * k)
                nm = name(tab, n) if n else (names[shn] if info & 15 == 3 else '')
                syms.append((shn, val, nm, info))
    secs = {}
    for i, h in enumerate(hdrs):
        if h[1] in (2, 3, 4, 0):
            continue
        data = b'' if h[1] == 8 else b[h[4]:h[4] + h[5]]
        secs[names[i]] = [data, [], i]
    for i, h in enumerate(hdrs):
        if h[1] != 4:
            continue
        target = names[h[7]]
        rel = []
        for k in range(h[5] // 24):
            off, info, add = struct.unpack_from('<QQq', b, h[4] + 24 * k)
            rel.append((off, info & 0xffffffff, syms[info >> 32][2], add))
        secs[target][1] = rel
    return b, secs, syms


def functions(secs, syms):
    text = secs.get('.text')
    if not text:
        return {}
    data, _, idx = text
    starts = sorted(set((v, n) for s, v, n, info in syms if s == idx and info & 15 == 2))
    out = {}
    for i, (v, n) in enumerate(starts):
        end = starts[i + 1][0] if i + 1 < len(starts) else len(data)
        out[n] = data[v:end]
    return out


def main():
    macho = os.path.abspath(sys.argv[1])
    files = sys.argv[2:]
    cc = shlex.split(os.environ.get('ELF_CC', 'clang --target=aarch64-linux-gnu'))
    tmp = tempfile.mkdtemp(prefix='elf-compare-')
    keep = os.environ.get('ELF_KEEP') == '1'
    n = same = 0
    sec_same = {}
    sec_diff = {}
    rel_same = rel_diff = 0
    fn_same = fn_diff = 0
    fails = []
    t_my = t_ref = 0.0
    size_my = size_ref = 0
    for f in files:
        n += 1
        base = os.path.join(os.path.dirname(f) if keep else tmp, os.path.basename(f)[:-2])
        my, ref = base + '.my.o', base + '.ref.o'
        t0 = time.time()
        r = subprocess.run([macho, 'asm', '--elf', '-o', my, f], capture_output=True, text=True)
        t_my += time.time() - t0
        if r.returncode != 0:
            fails.append((f, r.stderr.strip()))
            continue
        t0 = time.time()
        subprocess.run(cc + ['-c', '-o', ref, f], check=True)
        t_ref += time.time() - t0
        mb, ms, msy = parse(my)
        rb, rs, rsy = parse(ref)
        size_my += len(mb)
        size_ref += len(rb)
        if mb == rb:
            same += 1
        elif os.environ.get('ELF_VERBOSE'):
            print('differs:', f)
        for k in sorted(set(ms) | set(rs)):
            a = ms.get(k)
            b = rs.get(k)
            if a and b and a[0] == b[0]:
                sec_same[k] = sec_same.get(k, 0) + 1
            else:
                sec_diff[k] = sec_diff.get(k, 0) + 1
                if os.environ.get('ELF_VERBOSE'):
                    print('  section', k, 'differs', len(a[0]) if a else None, len(b[0]) if b else None)
            if a and b and a[1] == b[1]:
                rel_same += 1
            else:
                rel_diff += 1
                if os.environ.get('ELF_VERBOSE') and a and b:
                    for x, y in zip(a[1], b[1]):
                        if x != y:
                            print('  reloc', k, x, y)
                            break
                    if len(a[1]) != len(b[1]):
                        print('  reloc count', k, len(a[1]), len(b[1]))
        mf, rf = functions(ms, msy), functions(rs, rsy)
        for k in set(mf) | set(rf):
            if mf.get(k) == rf.get(k):
                fn_same += 1
            else:
                fn_diff += 1
    print('files: %d, objects byte identical: %d' % (n, same))
    print('functions identical: %d, differing: %d' % (fn_same, fn_diff))
    print('sections identical: %d, differing: %d' % (sum(sec_same.values()), sum(sec_diff.values())))
    for k in sorted(sec_diff):
        print('  differing %s: %d' % (k, sec_diff[k]))
    print('relocation tables identical: %d, differing: %d' % (rel_same, rel_diff))
    print('object bytes: macho %d, cc %d' % (size_my, size_ref))
    print('time: macho %.2f s, cc %.2f s' % (t_my, t_ref))
    for f, e in fails:
        print('FAIL', f, e)
    sys.exit(1 if fails or same != n else 0)


main()
