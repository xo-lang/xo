#!/usr/bin/env python3
"""Measures bench/perf/rwread with one or two xo binaries (A, B): builds
the program with each (each xo with its own std), then runs every
configuration ROUNDS times, A and B alternating and the order flipped
each round, and prints for each the medians of reads a millisecond (the
program's own), wall ms, CPU ms (user plus system, from the child's
rusage) and peak RSS MB, with their ranges, and B/A ratios.

usage: bench/perf/rwread/measure.py OUT XO_A [XO_B]
       bench/perf/rwread/measure.py OUT --bins BIN_A [BIN_B]
(--bins: rwread binaries built already, e.g. on another host)
environment:
  THREADS  worker counts (XO_THREADS), spaces or commas, default "1 2 4 8 N" (N: the CPUs
           this process may run on)
  WRITES   writes per 1,000 accesses, default "1 10 100"
  TASKS    tasks, default 256; OPS accesses a task, default 4000
  THINK    work steps between accesses, default 50; HOLD a write's
           steps inside the lock, default 200
  ROUNDS   default 5
  PIN      a CPU list for taskset on Linux (e.g. 0-5); N is then its
           CPU count, and thread counts above it are dropped
"""
import os, statistics, subprocess, sys, tempfile, time

out = sys.argv[1]
xos = sys.argv[2:]
prebuilt = xos[:1] == ['--bins']
if prebuilt:
    xos = xos[1:]
if not xos or len(xos) > 2:
    sys.exit(__doc__)
here = os.path.dirname(os.path.abspath(__file__))
os.makedirs(out, exist_ok=True)
pin = os.environ.get('PIN', '')
pre = ['taskset', '-c', pin] if pin and sys.platform.startswith('linux') else []

def ncpu():
    if pre:
        n = 0
        for part in pin.split(','):
            a, _, b = part.partition('-')
            n += int(b or a) - int(a) + 1
        return n
    if hasattr(os, 'sched_getaffinity'):
        return len(os.sched_getaffinity(0))
    return os.cpu_count()

n = ncpu()
threads = [int(t.replace('N', str(n))) for t in os.environ.get('THREADS', '1 2 4 8 N').replace(',', ' ').split()]
threads = sorted(set(t for t in threads if t <= n))
writes = os.environ.get('WRITES', '1 10 100').replace(',', ' ').split()
args = [os.environ.get('TASKS', '256'), os.environ.get('OPS', '4000')]
think, hold = os.environ.get('THINK', '50'), os.environ.get('HOLD', '200')
rounds = int(os.environ.get('ROUNDS', '5'))

bins = []
for i, xo in enumerate(xos):
    if prebuilt:
        bins.append(os.path.abspath(xo))
        continue
    b = os.path.join(out, 'rwread-' + 'AB'[i])
    subprocess.run([xo, 'build', '-o', b, here], check=True)
    bins.append(b)

# one run, reaped with wait4, whose rusage gives its CPU. Peak RSS: on
# darwin wait4's; on Linux a process keeps the peak RSS of the one it was
# started from across exec (a spawn from here reported this script's 12 MB
# for every run, against 4.8 to 8.2 MB under GNU time), so the run goes
# through /usr/bin/time, a small process, and its %M is the peak
linux = sys.platform.startswith('linux')
if linux and not os.path.exists('/usr/bin/time'):
    sys.exit('measure.py: Linux needs GNU time (/usr/bin/time) for the peak RSS')

def run(b, th, w):
    env = dict(os.environ, XO_THREADS=str(th))
    with tempfile.TemporaryFile() as fo, tempfile.TemporaryFile() as fe:
        fa = [(os.POSIX_SPAWN_DUP2, fo.fileno(), 1), (os.POSIX_SPAWN_DUP2, fe.fileno(), 2)]
        argv = pre + [b] + args + [w, think, hold]
        if linux:
            argv = ['/usr/bin/time', '-f', 'RWREAD-RSS-KB %M'] + argv
        t0 = time.monotonic()
        pid = os.posix_spawnp(argv[0], argv, env, file_actions=fa)
        _, status, ru = os.wait4(pid, 0)
        wall = (time.monotonic() - t0) * 1000
        fo.seek(0); fe.seek(0)
        so, se = fo.read().decode(), fe.read().decode()
    rc = os.waitstatus_to_exitcode(status)
    if rc != 0:
        sys.exit(f'{b} failed ({rc}): {se[-400:]}')
    f = so.split()
    kv = {f[i]: f[i + 1] for i in range(1, len(f) - 1, 2)}
    if kv.get('sum') != '8386560' or f[-1] != '0':
        sys.exit(f'{b}: wrong result: {so}')
    cpu = (ru.ru_utime + ru.ru_stime) * 1000
    if linux:
        mb = int(se.split('RWREAD-RSS-KB ')[-1].split()[0]) / 1024
    else:
        mb = ru.ru_maxrss / 1048576  # bytes on darwin
    return int(kv['reads_per_ms']), wall, cpu, mb

res = {}
log = open(os.path.join(out, 'runs.txt'), 'w')
for r in range(rounds):
    for th in threads:
        for w in writes:
            order = list(range(len(bins)))
            if r % 2:
                order.reverse()
            for i in order:
                x = run(bins[i], th, w)
                res.setdefault((th, w, i), []).append(x)
                print(f'round {r + 1} threads {th} writes {w} {"AB"[i]}: reads/ms {x[0]} wall {x[1]:.0f} cpu {x[2]:.0f} rss {x[3]:.1f}', file=log, flush=True)

def med(v):
    return statistics.median(v)

names = ['reads/ms', 'wall ms', 'CPU ms', 'RSS MB']
print(f'rwread: tasks {args[0]} ops {args[1]} think {think} hold {hold}, {rounds} rounds, {n} CPUs{" pinned " + pin if pre else ""}')
for i, xo in enumerate(xos):
    v = '' if prebuilt else ' (' + subprocess.run([xo, 'version'], capture_output=True, text=True).stdout.strip() + ')'
    print(f'{"AB"[i]}: {xo}{v}')
print()
hdr = '| threads | writes/1000 | side | ' + ' | '.join(names) + ' |'
print(hdr)
print('|' + '---|' * (hdr.count('|') - 1))
for th in threads:
    for w in writes:
        meds = []
        for i in range(len(bins)):
            cols = list(zip(*res[(th, w, i)]))
            m = [med(c) for c in cols]
            meds.append(m)
            cells = [f'{m[k]:,.0f} [{min(cols[k]):,.0f} to {max(cols[k]):,.0f}]' if k < 3 else
                     f'{m[k]:.1f} [{min(cols[k]):.1f} to {max(cols[k]):.1f}]' for k in range(4)]
            print(f'| {th} | {w} | {"AB"[i]} | ' + ' | '.join(cells) + ' |')
        if len(bins) == 2:
            rat = [meds[1][k] / meds[0][k] if meds[0][k] else 0 for k in range(4)]
            print(f'| {th} | {w} | B/A | ' + ' | '.join(f'{x:.3f}' for x in rat) + ' |')
