#!/usr/bin/env python3
# Generates the field arithmetic of std/native/llrt/x25519.xo (python3
# scripts/x25519fe64.py): elements of GF(2^255 - 19) as four 64 bit
# limbs, kept below 2^256 and reduced modulo 2^256 - 38 (twice p), every
# sum a chain of cpu.adc and cpu.adc_carry (decision 0231's addendum),
# every difference a chain of cpu.borrow; constant time (no branch or
# index on a value).
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


def chain(xs, ys, cin="0", keep=True):
    """xs[j] + ys[j] with the carry chained; the sums and the last carry
    (None when keep is false: the last pair makes only its sum)."""
    c = cin
    r = []
    for idx, (x, y) in enumerate(zip(xs, ys)):
        # a literal 0 operand with no carry in adds nothing: no
        # instruction, and no carry out (the arm64 emitter would hold
        # the 0 in a register and break the adcs chain at it)
        if c == "0" and (x == "0" or y == "0"):
            r.append(y if x == "0" else x)
            continue
        if not keep and idx == len(xs) - 1:
            s = v("s")
            emit(f"let {s} = cpu.adc({x}, {y}, {c})")
            r.append(s)
            return r, None
        s, c = adc(x, y, c)
        r.append(s)
    return r, c


def sbb_chain(xs, ys):
    """xs - ys with the borrow chained (bits.Sub64's borrow formula)."""
    b = None
    r = []
    for x, y in zip(xs, ys):
        e = v("e")
        emit(f"let {e} = rt.int_wrap(1, {x}, {y})")
        if b is None:
            bo = v("b")
            emit(f"let {bo} = cpu.borrow({x}, {y})")
            r.append(e)
        else:
            d = v("d")
            emit(f"let {d} = rt.int_wrap(1, {e}, {b})")
            bo = v("b")
            emit(f"let {bo} = cpu.borrow({x}, {y}) | cpu.borrow({e}, {b})")
            r.append(d)
        b = bo
    return r, b


def mul38(x):
    return f"rt.int_wrap(2, {x}, 38)"


def fold_top(r, top):
    """r (4 words) + top 2^256 modulo 2^256 - 38, top small: + 38 top,
    and the carry out again (38 more on word 0, which then cannot carry)."""
    t = v("t")
    emit(f"let {t} = {mul38(top)}")
    r2, c = chain(r, [t, "0", "0", "0"])
    w = v("w")
    emit(f"let {w} = rt.int_wrap(0, {r2[0]}, {mul38(c)})")
    return [w] + r2[1:]


def reduce8(t):
    """t (8 words) modulo 2^256 - 38: the high half times 38 added to the
    low half (two chains), then the small top folded."""
    lo = []
    hi = []
    for i in range(4):
        a = v("lo")
        b = v("hi")
        emit(f"let {a} = rt.int_wrap(2, {t[4 + i]}, 38)")
        emit(f"let {b} = rt.u64_mul_hi({t[4 + i]}, 38)")
        lo.append(a)
        hi.append(b)
    r, c1 = chain(t[:4], lo)
    r2, c2 = chain(r[1:], hi[:3])
    top = wadd(wadd(hi[3], c1), c2)
    return fold_top([r[0]] + r2, top)


def store(rs):
    for i, x in enumerate(rs):
        emit(f"st(h, {8 * i}, {x})")


def load(nm):
    for i in range(4):
        emit(f"let {nm}{i} = ld({nm}, {8 * i})")


def gen_mul():
    load("f")
    load("g")
    # the product's words t[0..8] (t[8] stays 0: the product is below 2^512)
    t = ["0"] * 10
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
        # the low halves at words i..i+3, the carry on to word i+4
        r, c = chain(t[i:i + 5], lo + ["0"])
        t[i:i + 5] = r
        if c != "0":
            t[i + 5] = c if t[i + 5] == "0" else wadd(t[i + 5], c)
        # the high halves at words i+1..i+4, the carry on to word i+5
        r, c = chain(t[i + 1:i + 6], hi + ["0"])
        t[i + 1:i + 6] = r
        if c != "0":
            t[i + 6] = c if t[i + 6] == "0" else wadd(t[i + 6], c)
    store(reduce8(t[:8]))


def gen_sq():
    load("f")

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
    r, c = chain([l02, l03], [h01, h02])
    w = {1: l01, 2: r[0], 3: r[1], 4: wadd(h03, c)}
    x, c = chain([l13], [h12])
    x5 = wadd(h13, c)
    r, c = chain([w[3], w[4], "0"], [l12, x[0], x5])
    w[3], w[4], w[5] = r
    w6 = c
    r, c = chain([w[5], w6], [l23, h23])
    w[5], w[6] = r
    w[7] = c
    d, _ = chain([w[k] for k in range(1, 8)], [w[k] for k in range(1, 8)], keep=False)
    sq = [prod(k, k) for k in range(4)]
    t, _ = chain(["0"] + d, [sq[0][0], sq[0][1], sq[1][0], sq[1][1], sq[2][0], sq[2][1], sq[3][0], sq[3][1]], keep=False)
    store(reduce8(t))


def gen_add():
    load("f")
    load("g")
    r, c = chain([f"f{i}" for i in range(4)], [f"g{i}" for i in range(4)])
    store(fold_top(r, c))


def gen_sub():
    load("f")
    load("g")
    r, b = sbb_chain([f"f{i}" for i in range(4)], [f"g{i}" for i in range(4)])
    # borrowed: + 2^256 - 38 (that is, - 38), and again on a second borrow
    t = v("t")
    emit(f"let {t} = {mul38(b)}")
    r2, b2 = sbb_chain(r, [t, "0", "0", "0"])
    w = v("w")
    emit(f"let {w} = rt.int_wrap(1, {r2[0]}, {mul38(b2)})")
    store([w] + r2[1:])


def gen_mul121665():
    load("f")
    lo = []
    hi = []
    for j in range(4):
        a = v("lo")
        b = v("hi")
        emit(f"let {a} = rt.int_wrap(2, f{j}, 121665)")
        emit(f"let {b} = rt.u64_mul_hi(f{j}, 121665)")
        lo.append(a)
        hi.append(b)
    r, c = chain(lo[1:], hi[:3])
    store(fold_top([lo[0]] + r, wadd(hi[3], c)))


def gen_freeze():
    """f reduced below p: bit 255 folded as 19, then p subtracted when
    the value + 19 reaches 2^255 (one subtraction: the value is below
    p + 38)."""
    load("f")
    c = v("c")
    emit(f"let {c} = rt.int_wrap(4, f3, 63)")
    m = v("m")
    emit(f"let {m} = f3 & 9223372036854775807")
    t = v("t")
    emit(f"let {t} = rt.int_wrap(2, {c}, 19)")
    r, _ = chain(["f0", "f1", "f2", m], [t, "0", "0", "0"], keep=False)
    u, _ = chain(r, ["19", "0", "0", "0"], keep=False)
    mk = v("mk")
    emit(f"let {mk} = ct.mask_bit(rt.int_wrap(4, {u[3]}, 63))")
    res = []
    for i in range(4):
        y = u[i] if i < 3 else f"({u[3]} & 9223372036854775807)"
        x = v("x")
        emit(f"let {x} = ct.pick({mk}, {y}, {r[i]})")
        res.append(x)
    store(res)


docs = {
    "fe64_mul": "/// h = f g modulo 2^256 - 38 (f, g below 2^256): the 512 bit product's\n/// high half times 38 added to its low half.",
    "fe64_sq": "/// h = f f modulo 2^256 - 38: 6 cross products once, doubled, plus the\n/// 4 squares (10 products where fe64_mul makes 16).",
    "fe64_add": "/// h = f + g modulo 2^256 - 38.",
    "fe64_sub": "/// h = f - g modulo 2^256 - 38.",
    "fe64_mul121665": "/// h = 121665 f modulo 2^256 - 38 (RFC 7748's a24).",
    "fe64_freeze": "/// h = f reduced below p (the canonical value).",
}
gens = [("fe64_mul", "h: Int, f: Int, g: Int", gen_mul), ("fe64_sq", "h: Int, f: Int", gen_sq), ("fe64_add", "h: Int, f: Int, g: Int", gen_add), ("fe64_sub", "h: Int, f: Int, g: Int", gen_sub), ("fe64_mul121665", "h: Int, f: Int", gen_mul121665), ("fe64_freeze", "h: Int, f: Int", gen_freeze)]
print("// ---------------------------------------------------------------------------\n// The field on 64 bit limbs, generated by scripts/x25519fe64.py: do not edit\n")
for name, params, g in gens:
    out.clear()
    n[0] = 0
    g()
    print(docs[name])
    print(f"fn {name}({params}) {{")
    print("\n".join(mark_unused(out)))
    print("}\n")
