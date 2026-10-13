#!/usr/bin/env python3
# The Go backend goldens with a compiler binary (decision 0178):
# scripts/selfhost-gogoldens.py <compiler> builds every
# internal/gen/testdata/golden program with `<compiler> build
# --backend=go` (a test_ program with `<compiler> test --backend=go`),
# runs it as internal/gen's TestGolden does (args.txt, env.txt,
# stdin.txt; no GOGC, GOMEMLIMIT, XO_GC_MIN_HEAP), and compares exit
# status, stdout, and the stable lines of stderr with want.txt, Go frame
# lists dropped (stableStderr, dropGoFrames). XO_GOLD_DIR: where the
# binaries go (default .tools/gogold); JOBS: parallel builds (default 4).
import os, re, subprocess, sys, glob
from concurrent.futures import ThreadPoolExecutor

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
comp = os.path.abspath(sys.argv[1])
out = os.path.abspath(os.environ.get('XO_GOLD_DIR', '.tools/gogold'))
os.makedirs(out, exist_ok=True)
time_re = re.compile(r'\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z')
go_frame_re = re.compile(r'(?m)^ +x_[^ \n]+\n +[^ \n]+\.xo:\d+\n')


def stable(s):
    s = time_re.sub('<time>', s)
    ls = []
    for l in s.split('\n'):
        if l.startswith('  at '):
            ls.append(l)
        elif l and not l.startswith(' ') and not l.startswith('\t') and not l.startswith('crash bundle:'):
            ls.append(l)
    return '\n'.join(ls) + '\n' if ls else ''


def harness(d):
    def read(n):
        p = os.path.join(d, n)
        return open(p, 'rb').read() if os.path.exists(p) else None
    args = (read('args.txt') or b'').decode().split()
    env = {k: v for k, v in os.environ.items() if k not in ('GOGC', 'GOMEMLIMIT', 'XO_GC_MIN_HEAP')}
    for kv in (read('env.txt') or b'').decode().split():
        k, _, v = kv.partition('=')
        env[k] = v
    return args, env, read('stdin.txt') or b''


def one(d):
    name = os.path.basename(d)
    bin = os.path.join(out, name)
    cmd = 'test' if name.startswith('test_') else 'build'
    if os.path.exists(bin):
        os.remove(bin)
    regress = os.path.join(d, 'regressions_test.xo')
    had = os.path.exists(regress)
    b = subprocess.run([comp, cmd, '--backend=go', '--std=' + os.path.join(root, 'std'), '-o', bin, d],
                       capture_output=True, text=True)
    # `xo test` writes shrunk contract fuzz failures into the module
    # (core 9.2); a golden directory must stay as committed
    if not had and os.path.exists(regress):
        os.remove(regress)
    # `test` runs the binary too, and exits with its status: the binary
    # is rerun below as TestGolden runs it
    if b.returncode != 0 and not (cmd == 'test' and os.path.exists(bin)):
        return d, 'build: ' + (b.stderr.strip().split('\n') or [''])[-1][:200]
    args, env, stdin = harness(d)
    p = subprocess.run([bin] + args, cwd=d, capture_output=True, env=env, input=stdin)
    got = 'exit %d\n--- stdout\n%s--- stderr\n%s' % (p.returncode, p.stdout.decode('utf-8', 'replace'),
                                                     stable(p.stderr.decode('utf-8', 'replace')))
    want = open(os.path.join(d, 'want.txt'), encoding='utf-8', newline='').read()
    if go_frame_re.sub('', got) != go_frame_re.sub('', want):
        return d, 'differs:\n--- want\n%s--- got\n%s' % (want[:600], got[:600])
    return d, ''


jobs = [d for d in sorted(glob.glob('internal/gen/testdata/golden/*')) if os.path.isdir(d)]
only = os.environ.get('ONLY', '')
if only:
    jobs = [d for d in jobs if any(o in d for o in only.split(','))]
ok = 0
fails = []
with ThreadPoolExecutor(int(os.environ.get('JOBS', '4'))) as ex:
    for d, why in ex.map(one, jobs):
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
