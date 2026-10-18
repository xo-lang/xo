#!/usr/bin/env python3
"""fieldcheck.py [--quick]: checks the generated field code of
std/native/llrt (x25519.xo's fe64_mul and fe64_sq from
scripts/x25519fe64.py, nistec.xo's mul4p and sqr4p from
scripts/nistecmul4p.py) by interpreting the generated Xo against Python's
big integers, on random operands and the fields' edges; then a planted
mutation (one carry dropped in each function) must be caught, so the
oracle is known to bite. Run it from the checkout root after
regenerating either file. The interpreter knows only what the generators
emit: let bindings of cpu.adc, cpu.adc_carry, cpu.carry, rt.int_wrap
(ops 0 to 4), rt.u64_mul_hi, rt.ct_barrier, ld, st, ~x, and sub_p256's
call (its operands checked, not the call). Exit status 1 on any wrong
case."""
import random
import re
import sys

M = (1 << 64) - 1


def split(s):
    r, d, cur = [], 0, ""
    for ch in s:
        if ch == "," and d == 0:
            r.append(cur)
            cur = ""
            continue
        d += ch == "("
        d -= ch == ")"
        cur += ch
    r.append(cur)
    return r


def ev(e, env):
    e = e.strip()
    if e.startswith("~"):
        return (~ev(e[1:], env)) & M
    m = re.fullmatch(r"(\w+(?:\.\w+)*)\((.*)\)", e)
    if m:
        fn, a = m.group(1), [ev(x, env) for x in split(m.group(2))]
        if fn == "cpu.adc":
            return (a[0] + a[1] + a[2]) & M
        if fn == "cpu.adc_carry":
            return (a[0] + a[1] + a[2]) >> 64
        if fn == "cpu.carry":
            return (a[0] + a[1]) >> 64
        if fn == "rt.u64_mul_hi":
            return (a[0] * a[1]) >> 64
        if fn == "rt.ct_barrier":
            return a[0]
        if fn == "rt.int_wrap":
            op, x, y = a
            return {0: lambda: (x + y) & M, 1: lambda: (x - y) & M, 2: lambda: (x * y) & M,
                    3: lambda: (x << y) & M, 4: lambda: x >> y}[op]()
        if fn == "ld":
            return env["mem"][a[0]][a[1] // 8]
        raise ValueError(f"unknown function {fn}")
    if re.fullmatch(r"-?\d+", e):
        return int(e) & M
    return env[e]


def body(src, name):
    m = re.search(r"fn " + name + r"\(([^)]*)\) \{\n(.*?)\n\}", src, re.S)
    if not m:
        raise SystemExit(f"fieldcheck: no fn {name}")
    return [l.strip() for l in m.group(2).split("\n")]


def run(lines, f, g):
    """The words stored through h, or sub_p256's operands (value, top)."""
    env = {"mem": {1: f, 2: g, 0: [0] * 4}, "h": 0, "f": 1, "g": 2}
    for l in lines:
        m = re.fullmatch(r"let (\w+) = (.*)", l)
        if m:
            env[m.group(1)] = ev(m.group(2), env)
            continue
        m = re.fullmatch(r"st\(h, (\d+), (\w+)\)", l)
        if m:
            env["mem"][0][int(m.group(1)) // 8] = env[m.group(2)]
            continue
        m = re.fullmatch(r"sub_p256\(h, (.*)\)", l)
        if m:
            w = [ev(x, env) for x in split(m.group(1))]
            return sum(x << (64 * i) for i, x in enumerate(w[:4])) + (w[4] << 256), w[4]
        raise ValueError(f"unknown statement {l}")
    return sum(x << (64 * i) for i, x in enumerate(env["mem"][0])), 0


def limbs(x):
    return [(x >> (64 * i)) & M for i in range(4)]


def x25519_cases(rnd, n):
    p = (1 << 255) - 19
    edge = [0, 1, 2, 37, 38, p - 1, p, p + 37, (1 << 256) - 39, (1 << 256) - 1, 1 << 255, (1 << 192) - 1, M, M << 64]
    vals = edge + [rnd.getrandbits(256) for _ in range(n)] + [(1 << 256) - 1 - rnd.getrandbits(rnd.randint(1, 80)) for _ in range(n // 3)]
    return p, edge, vals


def nistec_cases(rnd, n):
    p = 2**256 - 2**224 + 2**192 + 2**96 - 1
    edge = [0, 1, 2, p - 1, p - 2, p - 3, p >> 1, 2**64, 2**64 - 1, 2**192, 2**192 - 1, 2**224, 2**255, 2**255 - 1, 2**256 - 2**224, p - 2**96]
    vals = edge + [rnd.randrange(p) for _ in range(n)] + [p - 1 - rnd.getrandbits(rnd.randint(1, 100)) for _ in range(n // 3)]
    return p, edge, vals


def check_x25519(src, rnd, n):
    p, edge, vals = x25519_cases(rnd, n)
    mul, sq = body(src, "fe64_mul"), body(src, "fe64_sq")
    bad = cases = 0
    for x in vals:
        for y in [rnd.choice(vals) for _ in range(3)] + edge[:4]:
            v, _ = run(mul, limbs(x), limbs(y))
            cases += 1
            bad += v % p != x * y % p or v >= 1 << 256
        v, _ = run(sq, limbs(x), limbs(x))
        cases += 1
        bad += v % p != x * x % p or v >= 1 << 256
    return cases, bad


def check_nistec(src, rnd, n):
    p, edge, vals = nistec_cases(rnd, n)
    ri = pow(2**256, -1, p)
    mul, sq = body(src, "mul4p"), body(src, "sqr4p")
    bad = cases = 0
    for x in vals:
        for y in [rnd.choice(vals) for _ in range(3)] + edge:
            v, top = run(mul, limbs(x), limbs(y))
            cases += 1
            bad += v % p != x * y * ri % p or v >= 2 * p or top > 1
        v, top = run(sq, limbs(x), limbs(x))
        cases += 1
        bad += v % p != x * x * ri % p or v >= 2 * p or top > 1
    return cases, bad


def mutate(src, name):
    """src with one carry in fn name's body dropped (the sixth chained one)."""
    a = src.index(f"fn {name}(")
    ms = list(re.finditer(r"cpu\.adc\((\w+), (\w+), (k\d+)\)", src[a:]))
    m = ms[min(5, len(ms) - 1)]
    return src[:a + m.start()] + f"cpu.adc({m.group(1)}, {m.group(2)}, 0)" + src[a + m.end():]


def main():
    n = 300 if "--quick" in sys.argv else 3000
    ok = True
    for path, check, names in [("std/native/llrt/x25519.xo", check_x25519, ["fe64_mul", "fe64_sq"]),
                               ("std/native/llrt/nistec.xo", check_nistec, ["mul4p", "sqr4p"])]:
        src = open(path).read()
        cases, bad = check(src, random.Random(7), n)
        print(f"{path}: {cases} cases, {bad} wrong")
        ok = ok and bad == 0
        for name in names:
            _, mbad = check(mutate(src, name), random.Random(7), n // 10)
            print(f"  {name} with a carry dropped: {mbad} wrong (must be more than 0)")
            ok = ok and mbad > 0
    sys.exit(0 if ok else 1)


main()
