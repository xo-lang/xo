#!/usr/bin/env python3
# Generates std/native/llrt/nistec.xo mul4p (python3 scripts/nistecmul4p.py):
# the Montgomery product for P-256's field prime p = 2^256 - 2^224 +
# 2^192 + 2^96 - 1 (limbs m0 = 2^64-1, m1 = 2^32-1, m2 = 0, m3 =
# 2^64-2^32+1; -p^-1 mod 2^64 = 1), CIOS over local words with every
# sum a chain of cpu.adc and cpu.adc_carry (decision 0231's addendum):
# per word of g the low halves of f g_i, then the high halves, then
# q p / 2^64 for q = t0, whose limbs are shifts of q.
out = []
n = [0]


def v(p):
    n[0] += 1
    return f"{p}{n[0]}"


def emit(s):
    out.append("  " + s)


def adc(a, b, cin):
    s = v("s")
    k = v("k")
    emit(f"let {s} = cpu.adc({a}, {b}, {cin})")
    emit(f"let {k} = cpu.adc_carry({a}, {b}, {cin})")
    return s, k


def wadd(a, b):
    s = v("w")
    emit(f"let {s} = rt.int_wrap(0, {a}, {b})")
    return s


def chain(xs, ys):
    """xs[j] + ys[j] with the carry chained; the sums and the last carry."""
    c = "0"
    r = []
    for x, y in zip(xs, ys):
        s, c = adc(x, y, c)
        r.append(s)
    return r, c


for nm in ["f", "g"]:
    for i in range(4):
        emit(f"let {nm}{i} = ld({nm}, {8 * i})")
T = None
for i in range(4):
    lo = []
    hi = []
    for j in range(4):
        a = v("lo")
        b = v("hi")
        emit(f"let {a} = rt.int_wrap(2, f{j}, g{i})")
        emit(f"let {b} = rt.u64_mul_hi(f{j}, g{i})")
        lo.append(a)
        hi.append(b)
    if T is None:
        # t = lo, then + hi a word up (hi3 + 1 does not overflow)
        r, c = chain(lo[1:], hi[:3])
        t = [lo[0]] + r + [wadd(hi[3], c), "0"]
    else:
        # T is t0..t4 (the reduction's shifted words and its top)
        r, c = chain(T, lo + ["0"])
        t5 = c
        r2, c2 = chain(r[1:5], hi)
        t = [r[0]] + r2 + [wadd(t5, c2)]
    # reduce: q = t0; t + q p is divisible by 2^64; shifted down a word
    # the addend over t1..t4 is (ql, qh, q - ql, hi3) (q m3's halves)
    q = t[0]
    ql = v("ql")
    qh = v("qh")
    emit(f"let {ql} = rt.int_wrap(3, {q}, 32)")
    emit(f"let {qh} = rt.int_wrap(4, {q}, 32)")
    lo3 = v("lo")
    hi3 = v("hi")
    emit(f"let {lo3} = rt.int_wrap(1, {q}, {ql})")
    emit(f"let {hi3} = rt.int_wrap(1, rt.int_wrap(1, {q}, {qh}), cpu.borrow({q}, {ql}))")
    r, c = chain(t[1:5], [ql, qh, lo3, hi3])
    top = wadd(t[5], c) if t[5] != "0" else c
    T = r + [top]
print("""/// mul4 for P-256's field prime (p's limbs 2^64 - 1, 2^32 - 1, 0,
/// 2^64 - 2^32 + 1; -1/p mod 2^64 = 1): CIOS over local words, every sum
/// a chain of cpu.adc (adcs), each reduction step by shifts (t0 + q m0
/// is q 2^64 for q = t0, q m1 + q is q 2^32, m2 is 0, q m3 is q 2^64 -
/// q 2^32 + q). The same result as mul4 with P-256's modulus; generated
/// by scripts/nistecmul4p.py.
fn mul4p(h: Int, f: Int, g: Int) {""")
print("\n".join(out))
print(f"  sub_m4(h, {T[0]}, {T[1]}, {T[2]}, {T[3]}, {T[4]}, -1, 4294967295, 0, -4294967295)")
print("}")
