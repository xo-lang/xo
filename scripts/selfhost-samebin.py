#!/usr/bin/env python3
# Do the two compiler drivers link the same binary? (decision 0174)
# scripts/selfhost-samebin.py <xo1> [DIR] builds every program of
# internal/llvm/testdata/golden and internal/arm64/testdata/run with
# `<xo1> build` and with the Go driver (`xo build --backend=arm64`;
# XO_GO_XO, default .tools/xo) into DIR/x1/<name>/<base> and
# DIR/go/<name>/<base> (the same file name: the ad hoc code signature
# names the file), and compares the binaries: byte identical, or else
# whether their code (__TEXT) and data (__DATA, __DATA_CONST) sections
# are (otool -s); default DIR .tools/samebin.
import os, subprocess, sys, glob
from concurrent.futures import ThreadPoolExecutor

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
x1 = os.path.abspath(sys.argv[1])
out = os.path.abspath(sys.argv[2] if len(sys.argv) > 2 else '.tools/samebin')
goxo = os.environ.get('XO_GO_XO', os.path.join(root, '.tools/xo'))


def sections(bin):
    # the section contents, without the load commands (LC_UUID) and the
    # code signature
    p = subprocess.run(['otool', '-l', bin], capture_output=True, text=True)
    names, seg, sect = [], None, None
    for l in p.stdout.split('\n'):
        l = l.strip()
        if l.startswith('sectname '):
            sect = l.split()[1]
        elif l.startswith('segname ') and sect:
            names.append((l.split()[1], sect))
            sect = None
    data = []
    for seg, sect in names:
        if seg.startswith('__TEXT') or seg.startswith('__DATA'):
            r = subprocess.run(['otool', '-s', seg, sect, bin], capture_output=True, text=True)
            data.append(r.stdout.split('\n', 1)[1] if '\n' in r.stdout else r.stdout)
    return data


def one(d):
    name = d.replace('/', '_')
    base = os.path.basename(d)
    a = os.path.join(out, 'x1', name, base)
    b = os.path.join(out, 'go', name, base)
    os.makedirs(os.path.dirname(a), exist_ok=True)
    os.makedirs(os.path.dirname(b), exist_ok=True)
    r1 = subprocess.run([x1, 'build', '--std=' + os.path.join(root, 'std'), '-o', a, d], capture_output=True, text=True)
    if r1.returncode != 0:
        return d, 'xo1 build failed: ' + r1.stderr.strip()[-200:]
    r2 = subprocess.run([goxo, 'build', '--backend=arm64', '-o', b, d], capture_output=True, text=True)
    if r2.returncode != 0:
        return d, 'go build failed: ' + r2.stderr.strip()[-200:]
    if open(a, 'rb').read() == open(b, 'rb').read():
        return d, 'identical'
    if sections(a) == sections(b):
        return d, 'same code and data'
    return d, 'different code or data'


jobs = [d for d in sorted(glob.glob('internal/llvm/testdata/golden/*')) + sorted(glob.glob('internal/arm64/testdata/run/*')) if os.path.isdir(d)]
counts = {}
with ThreadPoolExecutor(4) as ex:
    for d, why in ex.map(one, jobs):
        k = why.split(':')[0]
        counts[k] = counts.get(k, 0) + 1
        if k != 'identical':
            print('==', d, why)
print('%d programs: %s' % (len(jobs), ', '.join('%d %s' % (v, k) for k, v in sorted(counts.items(), key=lambda kv: -kv[1]))))
