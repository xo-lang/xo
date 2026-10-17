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


def mark_unused(lines):
    """Names a value nothing reads with a leading _ (Xo's rule for an
    unused binding; the compiler drops the code)."""
    import re as _re
    text = "\n".join(lines)
    outl = []
    for i, l in enumerate(lines):
        m = _re.match(r"  let (\w+) = ", l)
        if m:
            nm = m.group(1)
            rest = "\n".join(lines[i + 1:])
            if not _re.search(r"\b" + nm + r"\b", rest):
                l = l.replace(f"let {nm} = ", f"let _{nm} = ", 1)
        outl.append(l)
    return outl


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


def force(x):
    """An operand: a name, or a product (half, a, b) made here, right
    before the pair that reads it (the arm64 chain makes it between its
    adcs, which mul and umulh leave the flags of)."""
    if isinstance(x, tuple):
        half, a, b = x
        r = v(half)
        op = f"rt.int_wrap(2, {a}, {b})" if half == "lo" else f"rt.u64_mul_hi({a}, {b})"
        emit(f"let {r} = {op}")
        return r
    return x


def chain(xs, ys, cin="0", keep=True):
    """xs[j] + ys[j] with the carry chained; the sums and the last carry
    (None when keep is false: the last pair makes only its sum)."""
    c = cin
    r = []
    for idx, (x, y) in enumerate(zip(xs, ys)):
        x = force(x)
        y = force(y)
        if not keep and idx == len(xs) - 1:
            s = v("s")
            emit(f"let {s} = cpu.adc({x}, {y}, {c})")
            r.append(s)
            return r, None
        s, c = adc(x, y, c)
        r.append(s)
    return r, c


def gen_mul():
    # g's words load at their rows and each product is made right before
    # the pair of the chain that adds it: fewer values live at once (all
    # 8 products and g up front kept 5 callee saved pairs on arm64, LLVM
    # 1)
    for i in range(4):
        emit(f"let f{i} = ld(f, {8 * i})")
    T = None
    for i in range(4):
        emit(f"let g{i} = ld(g, {8 * i})")
        lo = [("lo", f"f{j}", f"g{i}") for j in range(4)]
        hi = [("hi", f"f{j}", f"g{i}") for j in range(4)]
        if T is None:
            # t = lo, then + hi a word up (hi3 + 1 does not overflow)
            # (the top word is hi3 + the carry: the chain's last pair, one
            # adcs with xzr, not a cset and an add)
            lo0 = force(lo[0])
            r, _ = chain(lo[1:] + [hi[3]], hi[:3] + ["0"], keep=False)
            t = [lo0] + r + ["0"]
        else:
            # T is t0..t4 (the reduction's shifted words and its top)
            r, c = chain(T, lo + ["0"])
            t5 = c
            r2, _ = chain(r[1:5] + [t5], hi + ["0"], keep=False)
            t = [r[0]] + r2
        # reduce: q = t0; t + q p is divisible by 2^64; shifted down a word
        # the addend over t1..t4 is (ql, qh, q - ql, hi3) (q m3's halves)
        q = t[0]
        ql = v("ql")
        qh = v("qh")
        emit(f"let {ql} = rt.int_wrap(3, {q}, 32)")
        emit(f"let {qh} = rt.int_wrap(4, {q}, 32)")
        # q - ql and q - qh - borrow as one chain of q + ~x: subs, sbcs on
        # arm64 (was sub, subs and cset for the borrow, sub, sub)
        lo3 = v("lo")
        bk = v("k")
        hi3 = v("hi")
        emit(f"let {lo3} = cpu.adc({q}, ~{ql}, 1)")
        emit(f"let {bk} = cpu.adc_carry({q}, ~{ql}, 1)")
        emit(f"let {hi3} = cpu.adc({q}, ~{qh}, {bk})")
        if t[5] != "0":
            T, _ = chain(t[1:6], [ql, qh, lo3, hi3, "0"], keep=False)
        else:
            r, c = chain(t[1:5], [ql, qh, lo3, hi3])
            T = r + [c]
    return T


def gen_sqr():
    """sqr4p: h = f f / R mod p, the square's 6 cross products once,
    doubled, plus the 4 diagonal squares (10 products, not 16), then 4
    Montgomery reduction rounds over the 8 word square, each one chain."""
    out.clear()
    n[0] = 0
    for i in range(4):
        emit(f"let f{i} = ld(f, {8 * i})")
    def prod(i, j):
        a = v("lo")
        b = v("hi")
        emit(f"let {a} = rt.int_wrap(2, f{i}, f{j})")
        emit(f"let {b} = rt.u64_mul_hi(f{i}, f{j})")
        return a, b
    l01, h01 = prod(0, 1)
    l02, h02 = prod(0, 2)
    l03, h03 = prod(0, 3)
    l12, h12 = prod(1, 2)
    l13, h13 = prod(1, 3)
    l23, h23 = prod(2, 3)
    # f0 (f1, f2, f3) at words 1..4
    r, c = chain([l02, l03], [h01, h02])
    w = {1: l01, 2: r[0], 3: r[1], 4: wadd(h03, c)}
    # f1 (f2, f3) at words 3..5
    x, c = chain([l13], [h12])
    x4 = x[0]
    x5 = wadd(h13, c)
    r, c = chain([w[3], w[4], "0"], [l12, x4, x5])
    w[3], w[4], w[5] = r
    w6 = c
    # f2 f3 at words 5..6
    r, c = chain([w[5], w6], [l23, h23])
    w[5], w[6] = r
    w[7] = c
    # doubled: words 1..7 (the top carry is 0: the cross sum is below 2^447)
    d, _ = chain([w[k] for k in range(1, 8)], [w[k] for k in range(1, 8)], keep=False)
    # the diagonal squares at words 0..7
    sq = [prod(k, k) for k in range(4)]
    t, _ = chain(["0"] + d, [sq[0][0], sq[0][1], sq[1][0], sq[1][1], sq[2][0], sq[2][1], sq[3][0], sq[3][1]], keep=False)
    t.append("0")
    # 4 reduction rounds: q = t[i]; t + q p 2^(64 i) has word i zero; the
    # addend (ql, qh, q - ql, hi3) at words i+1..i+4, the carry on to the top
    for i in range(4):
        q = t[i]
        ql = v("ql")
        qh = v("qh")
        emit(f"let {ql} = rt.int_wrap(3, {q}, 32)")
        emit(f"let {qh} = rt.int_wrap(4, {q}, 32)")
        # q - ql and q - qh - borrow as one chain of q + ~x: subs, sbcs on
        # arm64 (was sub, subs and cset for the borrow, sub, sub)
        lo3 = v("lo")
        bk = v("k")
        hi3 = v("hi")
        emit(f"let {lo3} = cpu.adc({q}, ~{ql}, 1)")
        emit(f"let {bk} = cpu.adc_carry({q}, ~{ql}, 1)")
        emit(f"let {hi3} = cpu.adc({q}, ~{qh}, {bk})")
        xs = t[i + 1:9]
        ys = [ql, qh, lo3, hi3] + ["0"] * (len(xs) - 4)
        r, _ = chain(xs, ys, keep=False)
        t[i + 1:9] = r
    return t[4], t[5], t[6], t[7], t[8]


mul_top = gen_mul()
out.append(f"  sub_p256(h, {mul_top[0]}, {mul_top[1]}, {mul_top[2]}, {mul_top[3]}, {mul_top[4]})")
mul_text = "\n".join(mark_unused(out))
sqr_top = gen_sqr()
out.append(f"  sub_p256(h, {sqr_top[0]}, {sqr_top[1]}, {sqr_top[2]}, {sqr_top[3]}, {sqr_top[4]})")
sqr_text = "\n".join(mark_unused(out))
print("""/// mul4 for P-256's field prime (p's limbs 2^64 - 1, 2^32 - 1, 0,
/// 2^64 - 2^32 + 1; -1/p mod 2^64 = 1): CIOS over local words, every sum
/// a chain of cpu.adc (adcs), each reduction step by shifts (t0 + q m0
/// is q 2^64 for q = t0, q m1 + q is q 2^32, m2 is 0, q m3 is q 2^64 -
/// q 2^32 + q). The same result as mul4 with P-256's modulus; generated
/// by scripts/nistecmul4p.py.
fn mul4p(h: Int, f: Int, g: Int) {""")
print(mul_text)
print("}")
print("""
/// mul4p of f with itself: the square's 6 cross products once, doubled,
/// plus the 4 diagonal squares (10 products where mul4p makes 16), then
/// 4 Montgomery reduction rounds over the 8 word square, each one chain
/// of cpu.adc with mul4p's addend of shifts. The same result as mul4p;
/// generated by scripts/nistecmul4p.py.
fn sqr4p(h: Int, f: Int) {""")
print(sqr_text)
print("}")
