#!/usr/bin/env python3
# The arm64 goldens with a compiler binary (decision 0158 switch-over),
# scripts/selfhost-goldens.py <compiler> builds every internal/llvm/testdata/golden
# program (TestGoldenRun: want.txt, no allocation live at exit) and every
# internal/arm64/testdata/run program (TestRun: the Go backend's output,
# recorded in internal/arm64/testdata/runwant by the Go `xo` of the
# go-compiler-final tag, decision 0215) with `<compiler> build`, runs it
# with XO_LEAKCHECK=1, and compares
# (XO_GOLD_DIR: where the binaries go, default .tools/gold; XO1_BACKEND:
# the --backend= to pass the compiler, default arm64 (xo1 defaults to go
# as xo does), or llvm, decision 0178;
# XO1_FLAGS: more flags for `<compiler> build`, split at spaces, such as
# --ld=macho; XO1_PATH: the PATH of `<compiler> build` alone, such as
# /nonexistent to show a build needs no external tool, decision 0182;
# XO1_CMD=run: `<compiler> run` builds and runs each program in one
# command, in the program's directory, its output compared the same way,
# decision 0178 stage 1).
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


def run_cmd(d, flags):
    # `<compiler> run` in the program's directory: the build and the run
    # in one command, the program's output and status passed through
    args, env, stdin = harness(d)
    env['XO_LEAKCHECK'] = '1'
    env['PWD'] = os.path.abspath(d)
    # env.txt is the program's environment, and the compiler sees it too:
    # a HOME such as env_dirs' /home/test has no user cache to build in
    env['XO_BUILD_DIR'] = os.path.join(out, 'runbuild')
    p = subprocess.run([comp, 'run'] + flags + ['--std=' + os.path.join(root, 'std'), '.', '--'] + args,
                       cwd=d, capture_output=True, env=env, input=stdin)
    so = p.stdout.decode('utf-8', 'replace')
    se = p.stderr.decode('utf-8', 'replace')
    if p.returncode != 0 and se.startswith(('xo: ', 'xo1: ')):
        return None, 'build: ' + se.strip().split('\n')[-1][:200]
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
    be = os.environ.get('XO1_BACKEND', '') or 'arm64'
    flags = ['--backend=' + be] if be else []
    flags = flags + os.environ.get('XO1_FLAGS', '').split()
    if os.environ.get('XO1_CMD') == 'run':
        got, leak = run_cmd(d, flags)
        if got is None:
            return d, leak
        want = want_fn(d)
        if got != want:
            return d, 'differs:\n--- want\n%s--- got\n%s' % (want[:600], got[:600])
        if leak:
            return d, leak
        return d, ''
    benv = dict(os.environ)
    if 'XO1_PATH' in os.environ:
        benv['PATH'] = os.environ['XO1_PATH']
    b = subprocess.run([comp, 'build'] + flags + ['--std=' + os.path.join(root, 'std'), '-o', bin, d], capture_output=True, text=True, env=benv)
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
    return open(os.path.join(d, 'want.txt'), encoding='utf-8', newline='').read()


def want_go(d):
    # the Go backend's output recorded by the Go `xo` of the
    # go-compiler-final tag (decision 0215)
    p = os.path.join('internal/arm64/testdata/runwant', os.path.basename(d) + '.txt')
    return open(p, encoding='utf-8', newline='').read()


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
