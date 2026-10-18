#!/usr/bin/env python3
"""macho-symview.py code|data EXE: a layout independent view of a linked
Mach-O x86_64 executable, to compare two links of the same objects
(scripts/macho-link-compare.sh: Xo's linker against Apple's ld, decision
0254 stage 6a). macOS only (objdump, nm, xcrun dyld_info).

code: the __text disassembly with every address operand named (symbol+off,
a cstring's text, a GOT slot's or stub's import, section+off).
data: every fixup (location and target named), hashes of the sections
without pointers, the unwind info's entries and LSDAs, each FDE's function.
"""
import subprocess, sys, re, bisect, struct, hashlib
mode, exe = sys.argv[1], sys.argv[2]
data = open(exe, 'rb').read()
# sections
sects = []
ncmds = struct.unpack_from('<I', data, 16)[0]; o = 32
while ncmds:
    cmd, size = struct.unpack_from('<II', data, o)
    if cmd == 0x19:
        ns = struct.unpack_from('<I', data, o + 64)[0]
        for k in range(ns):
            s = o + 72 + 80 * k
            name = data[s:s+16].rstrip(b'\0').decode(); seg = data[s+16:s+32].rstrip(b'\0').decode()
            addr, sz = struct.unpack_from('<QQ', data, s + 32)
            off = struct.unpack_from('<I', data, s + 48)[0]
            sects.append((addr, sz, seg + ',' + name, off))
    o += size; ncmds -= 1
def sect_of(a):
    for addr, sz, n, off in sects:
        if addr <= a < addr + sz:
            return addr, sz, n, off
    for addr, sz, n, off in sects:
        if a == addr + sz and sz:
            return addr, sz, n, off
    return None
# symbols
syms = []
for line in subprocess.run(['nm', '-n', '-defined-only', exe], capture_output=True, text=True).stdout.splitlines():
    p = line.split()
    if len(p) >= 3:
        syms.append((int(p[0], 16), p[2]))
syms.sort()
addrs = [a for a, _ in syms]
# GOT binds
got = {}
for line in subprocess.run(['xcrun', 'dyld_info', '-fixups', exe], capture_output=True, text=True).stdout.splitlines():
    m = re.search(r'(0x[0-9A-Fa-f]+)\s+bind\s+\S+?/(\S+)', line)
    if m:
        got[int(m.group(1), 16)] = m.group(2)
    m = re.search(r'(0x[0-9A-Fa-f]+)\s+rebase\s+(0x[0-9A-Fa-f]+)', line)
    if m:
        got[int(m.group(1), 16)] = ('rebase', int(m.group(2), 16))
def u32(a):
    s = sect_of(a); return struct.unpack_from('<i', data, s[3] + a - s[0])[0]
def name(a, depth=0):
    s = sect_of(a)
    if s is None:
        return 'abs:%x' % a
    addr, sz, n, off = s
    if n == '__TEXT,__stubs':
        slot = a + 6 + u32(a + 2)
        return 'stub:' + str(got.get(slot))
    if n == '__DATA_CONST,__got':
        g = got.get(a)
        if isinstance(g, tuple):
            return 'got:' + (name(g[1], 1) if depth == 0 else 'x')
        return 'got:' + str(g)
    if n == '__TEXT,__cstring':
        b = data[off + a - addr:]
        return 'str:' + repr(b[:b.index(b'\0')][:40])
    i = bisect.bisect_right(addrs, a) - 1
    if i >= 0 and sect_of(addrs[i]) == s:
        return '%s+%x' % (syms[i][1], a - addrs[i])
    return '%s+%x' % (n, a - addr)
if mode == 'code':
    out = subprocess.run(['objdump', '-d', '--no-show-raw-insn', '--section=__text', exe], capture_output=True, text=True).stdout
    for line in out.splitlines():
        m = re.match(r'\s*([0-9a-f]+):\s+(.*)', line)
        if not m:
            if line.endswith(':') and not line.startswith(exe):
                print(re.sub(r'^[0-9a-f]+ ', '', line))
            continue
        ins = m.group(2)
        t = re.search(r'# 0x([0-9a-f]+)', ins)
        if t:
            ins = ins[:ins.index('#')].strip()
            ins = re.sub(r'-?0x[0-9a-f]+\(%rip\)', '[' + name(int(t.group(1), 16)) + ']', ins)
        b = re.match(r'(j\w+|call\w*)\s+0x([0-9a-f]+)', ins)
        if b:
            ins = b.group(1) + ' ' + name(int(b.group(2), 16))
        print(ins)
else:
    for line in subprocess.run(['xcrun', 'dyld_info', '-fixups', exe], capture_output=True, text=True).stdout.splitlines():
        m = re.search(r'(\S+)\s+(\S+)\s+(0x[0-9A-Fa-f]+)\s+(bind|rebase)\s+(\S+)', line)
        if m and m.group(2) != '__got':
            a = int(m.group(3), 16)
            t = name(int(m.group(5), 16)) if m.group(4) == 'rebase' else m.group(5).split('/')[-1]
            print('fixup', m.group(2), name(a), '->', t)
    for addr, sz, n, off in sects:
        if n in ('__TEXT,__cstring', '__TEXT,__gcc_except_tab', '__TEXT,__const', '__DATA,__thread_data', '__TEXT,__literal16', '__TEXT,__literal8', '__TEXT,__literal4'):
            print('bytes', n, sz, hashlib.sha1(data[off:off + sz]).hexdigest())
    u = subprocess.run(['objdump', '--unwind-info', exe], capture_output=True, text=True).stdout
    for line in u.splitlines():
        m = re.search(r'Function offset=(0x[0-9a-f]+), encoding=(0x[0-9a-f]+)', line)
        if m:
            enc = int(m.group(2), 16)
            mode = enc & 0x0f000000
            print('unwind', name(0x100000000 + int(m.group(1), 16)), hex(enc if mode != 0x04000000 else enc & 0xff000000))
        m = re.search(r'Function offset=(0x[0-9a-f]+), LSDA offset=(0x[0-9a-f]+)', line)
        if m:
            print('lsda', name(0x100000000 + int(m.group(1), 16)), name(0x100000000 + int(m.group(2), 16)))
    # FDEs
    for addr, sz, n, off in sects:
        if n == '__TEXT,__eh_frame':
            p = off; cie_l = {}
            while p < off + sz:
                ln = struct.unpack_from('<I', data, p)[0]
                if ln == 0: break
                cid = struct.unpack_from('<I', data, p + 4)[0]
                if cid != 0:
                    f = p + 8
                    pc = addr + (f - off) + struct.unpack_from('<q', data, f)[0]
                    rng = struct.unpack_from('<Q', data, f + 8)[0]
                    body = data[f + 16:p + 4 + ln]
                    print('fde', name(pc), hex(rng), len(body))
                p += 4 + ln
