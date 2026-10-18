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
            # T is t0..t4 (the reduction's shifted words and its top bit),
            # below 2p for f below p (every P-256 field value is: nistec.xo
            # field_mul), so T + f g_i is below p 2^64 + p < 2^320: five
            # words, neither chain carries out of word 4
            r, _ = chain(T, lo + ["0"], keep=False)
            r2, _ = chain(r[1:5], hi, keep=False)
            t = [r[0]] + r2 + ["0"]
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
    Montgomery reduction rounds over the square's low half and its high
    half added once."""
    out.clear()
    n[0] = 0
    for i in range(4):
        emit(f"let f{i} = ld(f, {8 * i})")

    def p(half, i, j):
        return (half, f"f{i}", f"f{j}")
    # the cross products f_i f_j (i < j) at words 1..6, each made right
    # before the pair that adds it (all 12 up front kept callee saved
    # registers on arm64); their sum is below 2^448, so no word 7. Row
    # f0 (f1, f2, f3) at words 1..4:
    w = [None, force(p("lo", 0, 1))]
    r, _ = chain([p("lo", 0, 2), p("lo", 0, 3), "0"], [p("hi", 0, 1), p("hi", 0, 2), p("hi", 0, 3)], keep=False)
    w += r
    # f1 f2, f1 f3 and f2 f3 together at words 3..6, then added in
    u3 = force(p("lo", 1, 2))
    u, _ = chain([p("lo", 1, 3), p("hi", 1, 3), "0"], [p("hi", 1, 2), p("lo", 2, 3), p("hi", 2, 3)], keep=False)
    r, _ = chain([w[3], w[4], "0", "0"], [u3] + u, keep=False)
    w[3:5] = r[:2]
    w += r[2:]
    # doubled (word 7 the carry), plus the squares f_k^2 at words 2k, 2k + 1
    d, c = chain(w[1:7], w[1:7])
    t0 = force(p("lo", 0, 0))
    sq = [p("hi", 0, 0), p("lo", 1, 1), p("hi", 1, 1), p("lo", 2, 2), p("hi", 2, 2), p("lo", 3, 3), p("hi", 3, 3)]
    t, _ = chain(d + [c], sq, keep=False)
    t = [t0] + t
    # 4 reduction rounds over the low half a = t0..t3: q = a0, a = (a + q
    # p) / 2^64 (below 2^192 + p < 2^256: four words, the last pair hi3
    # and the carry); then the high half added: (f f + Q p) / 2^256,
    # below 2p for f below p, its top bit the carry
    a = t[:4]
    for i in range(4):
        q = a[0]
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
        a, _ = chain(a[1:] + ["0"], [ql, qh, lo3, hi3], keep=False)
    r, c = chain(a, t[4:8])
    return r[0], r[1], r[2], r[3], c


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
/// q 2^32 + q). The same result as mul4 with P-256's modulus for f and g
/// below p (the accumulator's width rests on it: field_mul); generated
/// by scripts/nistecmul4p.py.
fn mul4p(h: Int, f: Int, g: Int) {""")
print(mul_text)
print("}")
print("""
/// mul4p of f with itself: the square's 6 cross products once, doubled,
/// plus the 4 diagonal squares (10 products where mul4p makes 16), then
/// 4 Montgomery reduction rounds over the square's low half, each one
/// chain of cpu.adc with mul4p's addend of shifts, and its high half
/// added once. The same result as mul4p for f below p; generated by
/// scripts/nistecmul4p.py.
fn sqr4p(h: Int, f: Int) {""")
print(sqr_text)
print("}")
