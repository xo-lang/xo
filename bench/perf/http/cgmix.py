#!/usr/bin/env python3
"""cgmix.py DIR BIN SCEN [OUT.json]: per request instruction counts of DIR/run/cg.SCEN.{100,200}
(callgrind --dump-instr=yes) by cause, from BIN's disassembly (objdump).

DIR is a callgrind-docker.sh OUT run with KEEP=1 ANN=1 REPS=1
CGFLAGS=--dump-instr=yes (one backend), BIN its server (DIR/srv-amd64,
DIR/srv-llvm). Each executed instruction goes to a bucket: stack access
(spill/stack), load, store, register move, call/frame (call, ret, push,
pop, rsp adjust), branch/cmp, overflow check (jo), bounds check, atomic,
RC/drop helper functions, ALU, ALU with a memory operand. OUT.json keeps
the buckets and the per function counts. x86-64 only (2026-10-19, the
amd64 gap by cause: bench/results/amd64-native-vs-llvm-2026-10-19.md)."""
import re, sys, subprocess, collections
d, binp, scen = sys.argv[1], sys.argv[2], sys.argv[3]

def load(f):
    ir = collections.Counter()
    pos = 0; skip = False; npos = 1
    for l in open(f, errors='replace'):
        if l.startswith('positions:'):
            npos = len(l.split()) - 1; continue
        if l.startswith('calls='):
            skip = True; continue
        if skip:
            skip = False
            t = l.split()
            a = t[0]
            if a.startswith('0x'): pos = int(a, 16)
            elif a != '*' and a[0] in '+-': pos += int(a)
            continue
        if not l or not (l[0] in '0123456789+-*'): continue
        t = l.split()
        if len(t) <= npos: continue
        a = t[0]
        if a.startswith('0x'): pos = int(a, 16)
        elif a == '*': pass
        elif a[0] in '+-': pos += int(a)
        else: continue
        ir[pos] += int(t[npos])
    return ir

a = load(f"{d}/run/cg.{scen}.100"); b = load(f"{d}/run/cg.{scen}.200")
reqs = [int(open(f"{d}/run/req.{scen}.{n}").read()) for n in (100, 200)]
n = reqs[1] - reqs[0]
per = {k: (b[k] - a.get(k, 0)) / n for k in b}
dis = subprocess.run(["objdump", "-d", "--no-show-raw-insn", binp], capture_output=True, text=True).stdout
ins = {}; fn = {}; cur = None
for l in dis.split('\n'):
    m = re.match(r'^([0-9a-f]+) <(.+)>:', l)
    if m: cur = m.group(2); continue
    m = re.match(r'^\s*([0-9a-f]+):\s+(\S+)\s*(.*)$', l)
    if m:
        ad = int(m.group(1), 16); ins[ad] = (m.group(2), m.group(3)); fn[ad] = cur

def cause(op, ops, f=""):
    o = op
    if f and (f.startswith("xo_h") or ".drop." in f or f.startswith("x.drop") or "obj_2erc" in f or "rc_free" in f or f.startswith("xo.drop")): return 'rc/drop'
    if o in ('jo', 'jno'): return 'overflow chk'
    if o in ('ja', 'jae', 'jb', 'jbe') and ('fault' in ops or 'bounds' in ops): return 'bounds chk'
    if o in ('call', 'callq', 'ret', 'retq') or o.startswith('push') or o.startswith('pop') or (o in ('lea','leaq') and '%rbp' in ops and '%rsp' in ops) or (o in ('sub','subq','add','addq') and ops.endswith('%rsp') and ops.startswith('$')): return 'call/frame'
    if o.startswith('mov') or o in ('lea', 'leaq'):
        if '(%rsp)' in ops or '(%rbp)' in ops: return 'spill/stack'
        if '(' in ops:
            return 'load' if ops.split(',')[-1].strip().startswith('%') else 'store'
        return 'move'
    if o.startswith('j') or o.startswith('cmp') or o.startswith('test') or o.startswith('set') or o.startswith('cmov'): return 'branch/cmp'
    if o.startswith('lock') or o.startswith('xchg') or o.startswith('cmpxchg'): return 'atomic'
    if o.startswith('movdqu') or o.startswith('movups'): return 'copy'
    if '(' in ops: return 'alu+mem'
    return 'alu'

tot = collections.Counter(); byfn = collections.Counter(); unk = 0
for ad, c in per.items():
    if c <= 0: continue
    if ad not in ins: unk += c; continue
    op, ops = ins[ad]
    k = cause(op, ops, fn[ad] or "")
    tot[k] += c
    byfn[(fn[ad], k)] += c
print(f"{scen}: total {sum(tot.values())+unk:.0f} (unmapped {unk:.0f})")
for k, v in tot.most_common(): print(f"  {k:12s} {v:10.0f}")
if len(sys.argv) > 4:
    out = sys.argv[4]
    import json
    json.dump({"tot": tot, "byfn": {f"{a}|{b}": v for (a, b), v in byfn.items()}}, open(out, "w"))
