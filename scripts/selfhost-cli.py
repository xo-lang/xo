#!/usr/bin/env python3
# The core commands of the self hosted CLI against the Go `xo` (decision
# 0178 stage 2): scripts/selfhost-cli.py <xo1> runs each command below
# with both binaries on every directory of the repository that holds .xo
# files (testdata cases included, dot directories and runs/ skipped) and
# compares exit status, standard output, and standard error byte for
# byte. Commands: check --json, check (human), vet --json, doc --all
# --json, fmt --check --json, parse --json. Neither writes a file.
#
# XO_GO_XO: the Go `xo` (default .tools/xo); XO_STD: the std directory
# xo1 reads (default <repo>/std); JOBS: parallel runs (default 4);
# CLI_CMDS: a comma separated subset of the command names (check,
# check-human, vet, doc, fmt, parse); CLI_SHOW: differences to print per
# command (default 3); CLI_STRIDE=N: every Nth directory only. Both run with XO_NO_SERVE=1 and the same
# environment otherwise. Prints per command the directories compared and
# how many are identical; exit status 1 when any differs.
import os, subprocess, sys
from concurrent.futures import ThreadPoolExecutor

root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(root)
xo1 = os.path.abspath(sys.argv[1])
goxo = os.path.abspath(os.environ.get('XO_GO_XO', '.tools/xo'))
env = dict(os.environ)
env['XO_NO_SERVE'] = '1'
env.setdefault('XO_STD', os.path.join(root, 'std'))
env['PWD'] = root
jobs = int(os.environ.get('JOBS', '4'))
show = int(os.environ.get('CLI_SHOW', '3'))

COMMANDS = {
    'check': ['check', '--json'],
    'check-human': ['check'],
    'vet': ['vet', '--json'],
    'doc': ['doc', '--all', '--json'],
    'fmt': ['fmt', '--check', '--json'],
    'parse': ['parse', '--json'],
}
names = os.environ.get('CLI_CMDS', ','.join(COMMANDS)).split(',')


def dirs():
    out = []
    for d, subdirs, files in os.walk('.'):
        subdirs[:] = sorted(s for s in subdirs if not s.startswith('.') and not (d == '.' and s == 'runs'))
        if any(f.endswith('.xo') for f in files):
            out.append(os.path.relpath(d, '.'))
    return sorted(out)


def run(binary, args):
    try:
        p = subprocess.run([binary] + args, capture_output=True, env=env, timeout=300)
        return (p.returncode, p.stdout, p.stderr)
    except subprocess.TimeoutExpired:
        return (-9, b'', b'timeout')


def compare(job):
    name, d = job
    args = COMMANDS[name] + [d]
    return name, d, run(goxo, args), run(xo1, args)


ds = dirs()[::int(os.environ.get("CLI_STRIDE", "1"))]
work = [(n, d) for n in names for d in ds]
with ThreadPoolExecutor(jobs) as ex:
    results = list(ex.map(compare, work))
bad = 0
for n in names:
    rs = [r for r in results if r[0] == n]
    same = [r for r in rs if r[2] == r[3]]
    print(f'{n}: {len(same)} of {len(rs)} directories identical')
    shown = 0
    for _, d, want, got in rs:
        if want == got or shown >= show:
            continue
        shown += 1
        print(f'  {d}: exit {want[0]} vs {got[0]}')
        for label, w, g in (('stdout', want[1], got[1]), ('stderr', want[2], got[2])):
            if w == g:
                continue
            wl, gl = w.decode(errors='replace').split('\n'), g.decode(errors='replace').split('\n')
            for i in range(max(len(wl), len(gl))):
                a = wl[i] if i < len(wl) else '<end>'
                b = gl[i] if i < len(gl) else '<end>'
                if a != b:
                    print(f'    {label} line {i + 1}:\n      go  {a[:300]}\n      xo1 {b[:300]}')
                    break
    bad += len(rs) - len(same)
sys.exit(1 if bad else 0)
