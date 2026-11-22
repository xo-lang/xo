"""Python twins of bench/perf/conc/xo: `python conc.py <row>` prints the same
line as the Xo program. Tasks are asyncio tasks on one thread (spawn,
fanout, pingpong); mutex uses threading.Lock (the GIL runs one thread at a
time in a CPython 3.13 that is not the free threaded build); pmap spreads
the chunks over one process per CPU with multiprocessing. There is no
rwmutex row: the standard library has no read write lock."""
import asyncio
import multiprocessing
import os
import sys
import threading


def steps(x):
    n = x
    c = 0
    while n > 1:
        n = n // 2 if n % 2 == 0 else 3 * n + 1
        c += 1
    return c


async def spawn():
    count = 0

    async def one(i):
        nonlocal count
        count += i

    for _ in range(200):
        await asyncio.gather(*(one(i) for i in range(1000)))
    return f"spawn {count}"


async def fanout():
    total = 0

    async def one(i, r):
        return i * 2 + r

    for r in range(100):
        res = [asyncio.ensure_future(one(i, r)) for i in range(1000)]
        acc = 0
        for f in res:
            acc += await f
        total += acc
    return f"fanout {total}"


async def pingpong():
    ping = asyncio.Queue(1)
    pong = asyncio.Queue(1)

    async def echo():
        while True:
            v = await ping.get()
            if v is None:
                break
            await pong.put(v + 1)

    t = asyncio.ensure_future(echo())
    v = 0
    for _ in range(100000):
        await ping.put(v)
        v = await pong.get()
    await ping.put(None)
    await t
    return f"pingpong {v}"


def mutex():
    lock = threading.Lock()
    c = [0]

    def work():
        for _ in range(25000):
            with lock:
                c[0] += 1

    ts = [threading.Thread(target=work) for _ in range(8)]
    for t in ts:
        t.start()
    for t in ts:
        t.join()
    return f"mutex {c[0]}"


SIZE, CHUNKS = 200000, 64


def chunk(args):
    r, k = args
    acc = 0
    for i in range(k * SIZE // CHUNKS, (k + 1) * SIZE // CHUNKS):
        acc += steps(i + r)
    return acc


def pmap():
    total = 0
    with multiprocessing.Pool(os.cpu_count()) as pool:
        for r in range(20):
            total += sum(pool.map(chunk, [(r, k) for k in range(CHUNKS)]))
    return f"pmap {total}"


if __name__ == "__main__":
    row = sys.argv[1] if len(sys.argv) > 1 else ""
    # REPEAT, the second argument (default 1; task #687, as the Xo and Go twins since #686):
    # the row runs that many times and every run must give the first run's line.
    reps = int(sys.argv[2]) if len(sys.argv) > 2 else 1
    if row in ("spawn", "fanout", "pingpong"):
        f = {"spawn": spawn, "fanout": fanout, "pingpong": pingpong}[row]

        def work():
            return asyncio.run(f())

    elif row in ("mutex", "pmap"):
        work = {"mutex": mutex, "pmap": pmap}[row]
    else:
        print("usage: conc.py spawn|fanout|pingpong|mutex|pmap [REPEAT]", file=sys.stderr)
        sys.exit(2)
    first = work()
    for _ in range(1, reps):
        if work() != first:
            print("repeat: a run printed something else")
            sys.exit(1)
    print(first)
