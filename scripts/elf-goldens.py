#!/usr/bin/env python3
# The arm64 goldens as linux/arm64 executables linked by selfhost/macho
# (decision 0182 stage 3), run in a linux/arm64 container with no C
# toolchain: scripts/elf-goldens.py <compiler> builds every
# internal/llvm/testdata/golden program (want.txt) and every
# internal/arm64/testdata/run program (the Go backend's output, run on the
# host) with `<compiler> build --target=linux/arm64 --ld=macho`, runs them
# all in one `docker run` of $ELF_IMAGE (default debian:bullseye, glibc
# 2.31, the runtime pack's floor) with XO_LEAKCHECK=1, the golden's
# args.txt, env.txt, and stdin.txt, its directory as the working
# directory, and compares as scripts/selfhost-goldens.py does.
# ELF_BINS=DIR skips the builds and runs DIR/<golden name> instead;
# XO_GOLD_DIR: where the binaries and results go (default
# .tools/elfgold); XO_GO_XO: the Go `xo` for the references (default
# .tools/xo); XO1_FLAGS: more build flags. Docker must already be running;
# this script never starts it.
import os, re, shutil, subprocess, sys, glob
from concurrent.futures import ThreadPoolExecutor

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
comp = os.path.abspath(sys.argv[1]) if len(sys.argv) > 1 else ''
out = os.path.abspath(os.environ.get('XO_GOLD_DIR', '.tools/elfgold'))
bins = os.environ.get('ELF_BINS', '')
image = os.environ.get('ELF_IMAGE', 'debian:bullseye')
os.makedirs(out, exist_ok=True)
time_re = re.compile(r'\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z')
leak_re = re.compile(r'xo leakcheck: (\d+) live of (\d+) allocations')


def stable(s):
    s = time_re.sub('<time>', s)
    ls = [l for l in s.split('\n') if l and not l.startswith(' ') and not l.startswith('\t')
          and not l.startswith('crash bundle:') and not l.startswith('xo leakcheck:')]
    return '\n'.join(ls) + '\n' if ls else ''


def read(d, n):
    p = os.path.join(d, n)
    return open(p, 'rb').read() if os.path.exists(p) else None


def name_of(d):
    return d.rstrip('/').replace('/', '_')


def build(d):
    if bins:
        b = os.path.join(bins, name_of(d))
        return d, '' if os.path.exists(b) else 'no binary ' + b
    b = os.path.join(out, name_of(d))
    r = subprocess.run([comp, 'build', '--backend=arm64', '--target=linux/arm64', '--ld=macho', '--std=' + os.path.join(root, 'std')] +
                       os.environ.get('XO1_FLAGS', '').split() + ['-o', b, d], capture_output=True, text=True)
    if r.returncode != 0:
        return d, 'build: ' + (r.stderr.strip().split('\n') or [''])[-1][:200]
    return d, ''


def want_go(d):
    b = os.path.join(out, name_of(d) + '.go')
    subprocess.run([os.environ.get('XO_GO_XO', os.path.join(root, '.tools/xo')), 'build', '-o', b, d], check=True, capture_output=True)
    args = (read(d, 'args.txt') or b'').decode().split()
    env = dict(os.environ)
    for kv in (read(d, 'env.txt') or b'').decode().split():
        k, _, v = kv.partition('=')
        env[k] = v
    p = subprocess.run([b] + args, cwd=d, capture_output=True, env=env, input=read(d, 'stdin.txt') or b'')
    return 'exit %d\n--- stdout\n%s--- stderr\n%s' % (p.returncode, p.stdout.decode('utf-8', 'replace'), stable(p.stderr.decode('utf-8', 'replace')))


def main():
    jobs = []
    for d in sorted(glob.glob('internal/llvm/testdata/golden/*')):
        if os.path.isdir(d):
            jobs.append((d, 'golden'))
    for d in sorted(glob.glob('internal/arm64/testdata/run/*')):
        if os.path.isdir(d):
            jobs.append((d, 'go'))
    fails = {}
    with ThreadPoolExecutor(4) as ex:
        for d, why in ex.map(lambda j: build(j[0]), jobs):
            if why:
                fails[d] = why
    bindir = bins or out
    res = os.path.join(out, 'res')
    shutil.rmtree(res, ignore_errors=True)
    os.makedirs(res)
    # one container runs every program: cd to its directory, its args,
    # env, and stdin, XO_LEAKCHECK=1, a 60 s limit
    sh = ['#!/bin/bash', 'for t in cc gcc clang ld as; do command -v $t >/dev/null && echo "toolchain: $t" >> /res/toolchain; done',
          'ldd --version | head -1 > /res/glibc']
    for d, _ in jobs:
        if d in fails:
            continue
        n = name_of(d)
        args = (read(d, 'args.txt') or b'').decode().split()
        env = (read(d, 'env.txt') or b'').decode().split()
        stdin = '/repo/%s/stdin.txt' % d if read(d, 'stdin.txt') is not None else '/dev/null'
        q = lambda s: "'" + s.replace("'", "'\\''") + "'"
        sh.append('cd /repo/%s && env XO_LEAKCHECK=1 %s timeout 60 /bins/%s %s < %s > /res/%s.out 2> /res/%s.err; echo $? > /res/%s.code' % (
            d, ' '.join(q(e) for e in env), n, ' '.join(q(a) for a in args), stdin, n, n, n))
    open(os.path.join(res, 'run.sh'), 'w').write('\n'.join(sh) + '\n')
    r = subprocess.run(['docker', 'run', '--rm', '--platform', 'linux/arm64', '-v', root + ':/repo', '-v', os.path.abspath(bindir) + ':/bins:ro',
                        '-v', res + ':/res', image, 'bash', '/res/run.sh'], capture_output=True, text=True)
    if r.returncode != 0:
        print('docker:', r.stderr.strip()[:500])
    glibc = open(os.path.join(res, 'glibc')).read().strip() if os.path.exists(os.path.join(res, 'glibc')) else '?'
    tools = open(os.path.join(res, 'toolchain')).read().split() if os.path.exists(os.path.join(res, 'toolchain')) else []
    ok = 0
    for d, kind in jobs:
        if d in fails:
            continue
        n = name_of(d)
        try:
            code = int(open(os.path.join(res, n + '.code')).read().strip())
            so = open(os.path.join(res, n + '.out'), 'rb').read().decode('utf-8', 'replace')
            se = open(os.path.join(res, n + '.err'), 'rb').read().decode('utf-8', 'replace')
        except (OSError, ValueError) as e:
            fails[d] = 'no result: %s' % e
            continue
        got = 'exit %d\n--- stdout\n%s--- stderr\n%s' % (code, so, stable(se))
        want = open(os.path.join(d, 'want.txt'), encoding='utf-8', newline='').read() if kind == 'golden' else want_go(d)
        if got != want:
            fails[d] = 'differs:\n--- want\n%s--- got\n%s' % (want[:600], got[:600])
            continue
        if code == 0:
            m = leak_re.search(se)
            if not m:
                fails[d] = 'no leak check line'
                continue
            if m.group(1) != '0':
                fails[d] = '%s allocations live' % m.group(1)
                continue
        ok += 1
    print('image %s (%s), C toolchain in the container: %s' % (image, glibc, ' '.join(tools) or 'none'))
    print('%d programs: %d pass, %d fail' % (len(jobs), ok, len(fails)))
    for d, why in sorted(fails.items()):
        print('FAIL', d, why.split('\n')[0][:150])
    for d, why in list(sorted(fails.items()))[:int(os.environ.get('SHOW', '8'))]:
        print('==', d, why[:1200])


main()
