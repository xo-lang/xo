#!/usr/bin/env python3
# The arm64 goldens with a compiler binary (decision 0158 switch-over),
# scripts/selfhost-goldens.py <compiler> builds every internal/llvm/testdata/golden
# program (TestGoldenRun: want.txt, no allocation live at exit) and every
# internal/arm64/testdata/run program (TestRun: the Go backend's output)
# with `<compiler> build`, runs it with XO_LEAKCHECK=1, and compares
# (XO_GO_XO: the Go `xo` building the reference, default .tools/xo;
# XO_GOLD_DIR: where the binaries go, default .tools/gold).
import os, re, subprocess, sys, glob
from concurrent.futures import ThreadPoolExecutor

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
comp = os.path.abspath(sys.argv[1])
out = os.path.abspath(os.environ.get('XO_GOLD_DIR', '.tools/gold'))
os.makedirs(out, exist_ok=True)
time_re = re.compile(r'\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z')
leak_re = re.compile(r'xo leakcheck: (\d+) live of (\d+) allocations')


def stable(s):
    s = time_re.sub('<time>', s)
    ls = [l for l in s.split('\n') if l and not l.startswith(' ') and not l.startswith('\t')
          and not l.startswith('crash bundle:') and not l.startswith('xo leakcheck:')]
    return '\n'.join(ls) + '\n' if ls else ''


def run(bin, d):
    p = subprocess.run([bin], cwd=d, capture_output=True, env=dict(os.environ, XO_LEAKCHECK='1'))
    so = p.stdout.decode('utf-8', 'replace')
    se = p.stderr.decode('utf-8', 'replace')
    leak = ''
    if p.returncode == 0:
        m = leak_re.search(se)
        if not m:
            leak = 'no leak check line'
        elif m.group(1) != '0':
            leak = '%s allocations live' % m.group(1)
    return 'exit %d\n--- stdout\n%s--- stderr\n%s' % (p.returncode, so, stable(se)), leak


def one(d, want_fn):
    name = d.replace('/', '_')
    bin = os.path.join(out, name)
    b = subprocess.run([comp, 'build', '--std=' + os.path.join(root, 'std'), '-o', bin, d], capture_output=True, text=True)
    if b.returncode != 0:
        return d, 'build: ' + (b.stderr.strip().split('\n') or [''])[-1][:200]
    got, leak = run(bin, d)
    want = want_fn(d)
    if got != want:
        return d, 'differs:\n--- want\n%s--- got\n%s' % (want[:600], got[:600])
    if leak:
        return d, leak
    return d, ''


def want_golden(d):
    return open(os.path.join(d, 'want.txt')).read()


def want_go(d):
    name = d.replace('/', '_') + '.go'
    bin = os.path.join(out, name)
    subprocess.run([os.environ.get('XO_GO_XO', os.path.join(root, '.tools/xo')), 'build', '-o', bin, d], check=True, capture_output=True)
    p = subprocess.run([bin], cwd=d, capture_output=True)
    return 'exit %d\n--- stdout\n%s--- stderr\n%s' % (p.returncode, p.stdout.decode('utf-8', 'replace'), stable(p.stderr.decode('utf-8', 'replace')))


jobs = []
for d in sorted(glob.glob('internal/llvm/testdata/golden/*')):
    if os.path.isdir(d):
        jobs.append((d, want_golden))
for d in sorted(glob.glob('internal/arm64/testdata/run/*')):
    if os.path.isdir(d):
        jobs.append((d, want_go))
ok = 0
fails = []
with ThreadPoolExecutor(4) as ex:
    for d, why in ex.map(lambda j: one(*j), jobs):
        if why:
            fails.append((d, why))
        else:
            ok += 1
print('%d programs: %d pass, %d fail' % (len(jobs), ok, len(fails)))
reasons = {}
for d, why in fails:
    k = why.split('\n')[0][:90]
    reasons[k] = reasons.get(k, 0) + 1
for k, v in sorted(reasons.items(), key=lambda kv: -kv[1]):
    print('%4d %s' % (v, k))
for d, why in fails[:int(os.environ.get('SHOW', '8'))]:
    print('==', d, why[:1200])
