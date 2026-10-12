#!/usr/bin/env python3
# The self hosted assembler (selfhost/macho, decision 0182) against the
# system assembler: scripts/macho-compare.py <macho> <file.s...> assembles
# each file with `<macho> asm --minos=<host>` and with `cc -c`, and counts
# byte identical objects, and per section and per function (the __text
# bytes between consecutive symbols) identical and differing ones.
# MACHO_KEEP=1 keeps the objects next to the inputs (.my.o, .ref.o).
import os, subprocess, sys, struct, tempfile, time


def sections(path):
    # name -> (addr, bytes, ordinal, relocations by name); the symbols
    # (section, value, name)
    b = open(path, 'rb').read()
    ncmds = struct.unpack_from('<I', b, 16)[0]
    off = 32
    secs = {}
    syms = []
    for _ in range(ncmds):
        cmd, size = struct.unpack_from('<II', b, off)
        if cmd == 0x19:
            nsects = struct.unpack_from('<I', b, off + 64)[0]
            for k in range(nsects):
                s = off + 72 + 80 * k
                name = b[s:s + 16].rstrip(b'\0').decode()
                seg = b[s + 16:s + 32].rstrip(b'\0').decode()
                addr, sz = struct.unpack_from('<QQ', b, s + 32)
                fo, _, reloff, nreloc = struct.unpack_from('<IIII', b, s + 48)
                rel = [struct.unpack_from('<iI', b, reloff + 8 * j) for j in range(nreloc)]
                secs[seg + ',' + name] = (addr, b[fo:fo + sz], k + 1, rel)
        elif cmd == 0x2:
            symoff, nsyms, stroff, strsize = struct.unpack_from('<IIII', b, off + 8)
            for k in range(nsyms):
                strx, typ, sect, desc, val = struct.unpack_from('<IBBHQ', b, symoff + 16 * k)
                e = b.index(b'\0', stroff + strx)
                syms.append((sect, val, b[stroff + strx:e].decode()))
        off += size
    # relocations with extern symbol numbers replaced by names
    for k, (addr, data, ordinal, rel) in list(secs.items()):
        named = []
        for a, w in rel:
            num, rest = w & 0xffffff, w >> 24
            if (w >> 27) & 1:
                num = syms[num][2]
            named.append((a, num, rest))
        secs[k] = (addr, data, ordinal, named)
    return secs, syms


def functions(secs, syms):
    text = secs.get('__TEXT,__text')
    if not text:
        return {}
    addr, data, ordinal, _ = text
    starts = sorted(set((v, n) for s, v, n in syms if s == ordinal and not n.startswith('ltmp')))
    out = {}
    for i, (v, n) in enumerate(starts):
        end = starts[i + 1][0] if i + 1 < len(starts) else addr + len(data)
        out[n] = data[v - addr:end - addr]
    return out


def main():
    macho = os.path.abspath(sys.argv[1])
    files = sys.argv[2:]
    host = subprocess.run(['sw_vers', '-productVersion'], capture_output=True, text=True).stdout.strip()
    minos = '.'.join((host.split('.') + ['0'])[:2])
    tmp = tempfile.mkdtemp(prefix='macho-compare-')
    keep = os.environ.get('MACHO_KEEP') == '1'
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
        r = subprocess.run([macho, 'asm', '--minos=' + minos, '-o', my, f], capture_output=True, text=True)
        t_my += time.time() - t0
        if r.returncode != 0:
            fails.append((f, r.stderr.strip()[:300]))
            continue
        t0 = time.time()
        subprocess.run(['cc', '-c', '-o', ref, f], check=True)
        t_ref += time.time() - t0
        a, b = open(my, 'rb').read(), open(ref, 'rb').read()
        size_my += len(a)
        size_ref += len(b)
        if a == b:
            same += 1
        sa, ya = sections(my)
        sb, yb = sections(ref)
        for k in sorted(set(sa) | set(sb)):
            if sa.get(k, (0, None))[1] == sb.get(k, (0, None))[1]:
                sec_same[k] = sec_same.get(k, 0) + 1
            else:
                sec_diff[k] = sec_diff.get(k, 0) + 1
            if sa.get(k, (0, 0, 0, None))[3] == sb.get(k, (0, 0, 0, None))[3]:
                rel_same += 1
            else:
                rel_diff += 1
                fails.append((f, 'relocations of %s differ' % k))
        fa, fb = functions(sa, ya), functions(sb, yb)
        for k in set(fa) | set(fb):
            if fa.get(k) == fb.get(k):
                fn_same += 1
            else:
                fn_diff += 1
                if len(fails) < 20:
                    fails.append((f, 'function %s differs' % k))
        if not keep:
            os.remove(my)
            os.remove(ref)
    print('%d files: %d objects byte identical to cc -c, %d failed to assemble' % (n, same, len([x for x in fails if not x[1].startswith(('function', 'relocations'))])))
    print('functions (__text between symbols): %d identical, %d differ' % (fn_same, fn_diff))
    print('relocation tables (symbols by name): %d identical, %d differ' % (rel_same, rel_diff))
    for k in sorted(set(sec_same) | set(sec_diff)):
        print('  %-28s %d identical, %d differ' % (k, sec_same.get(k, 0), sec_diff.get(k, 0)))
    print('object bytes: %d (macho) vs %d (cc -c); assembly time %.2f s vs %.2f s' % (size_my, size_ref, t_my, t_ref))
    for f, why in fails[:20]:
        print('==', f, why)


main()
