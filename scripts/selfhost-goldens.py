#!/usr/bin/env python3
# The arm64 goldens with a compiler binary (decision 0158 switch-over),
# scripts/selfhost-goldens.py <compiler> builds every internal/llvm/testdata/golden
# program (TestGoldenRun: want.txt, no allocation live at exit) and every
# internal/arm64/testdata/run program (TestRun: the Go backend's output)
# with `<compiler> build`, runs it with XO_LEAKCHECK=1, and compares
# (XO_GO_XO: the Go `xo` building the reference, default .tools/xo;
# XO_GOLD_DIR: where the binaries go, default .tools/gold; XO1_BACKEND:
# a --backend= to pass the compiler, such as llvm, decision 0178;
# XO1_FLAGS: more flags for `<compiler> build`, split at spaces, such as
# --ld=macho; XO1_PATH: the PATH of `<compiler> build` alone, such as
# /nonexistent to show a build needs no external tool, decision 0182).
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


def harness(d):
    # args.txt (fields), env.txt (NAME=VALUE fields), stdin.txt, as
    # internal/llvm's TestGoldenRun passes them
    def read(n):
        p = os.path.join(d, n)
        return open(p, 'rb').read() if os.path.exists(p) else None
    args = (read('args.txt') or b'').decode().split()
    env = dict(os.environ)
    for kv in (read('env.txt') or b'').decode().split():
        k, _, v = kv.partition('=')
        env[k] = v
    return args, env, read('stdin.txt') or b''


def run(bin, d):
    args, env, stdin = harness(d)
    env['XO_LEAKCHECK'] = '1'
    p = subprocess.run([bin] + args, cwd=d, capture_output=True, env=env, input=stdin)
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
    be = os.environ.get('XO1_BACKEND', '')
    flags = ['--backend=' + be] if be else []
    benv = dict(os.environ)
    if 'XO1_PATH' in os.environ:
        benv['PATH'] = os.environ['XO1_PATH']
    b = subprocess.run([comp, 'build'] + flags + ['--std=' + os.path.join(root, 'std')] + os.environ.get('XO1_FLAGS', '').split() + ['-o', bin, d], capture_output=True, text=True, env=benv)
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
    args, env, stdin = harness(d)
    p = subprocess.run([bin] + args, cwd=d, capture_output=True, env=env, input=stdin)
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
for d, why in fails:
    print('FAIL', d, why.split('\n')[0][:150])
for d, why in fails[:int(os.environ.get('SHOW', '8'))]:
    print('==', d, why[:1200])
