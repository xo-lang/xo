#!/usr/bin/env python3
# Generates the field arithmetic of std/native/llrt/x25519.xo (python3
# scripts/x25519fe64.py): elements of GF(2^255 - 19) as four 64 bit
# limbs, kept below 2^256 and reduced modulo 2^256 - 38 (twice p), every
# sum a chain of cpu.adc and cpu.adc_carry (decision 0231's addendum),
# every difference a chain of cpu.borrow; constant time (no branch or
# index on a value).
# After regenerating, run scripts/fieldcheck.py (the generated code against
# big integers, and a planted mutation it must catch).
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
    adcs, which mul and umulh leave the flags of: calls.xo ADC_SPLIT)."""
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
    # through the barrier (decision 0222): x is a carry or borrow bit, or
    # a small top word; LLVM made 38 times a bit a conditional select
    # (TestConstantTimeLLVM, 2026-10-18), which x86's cmov conversion may
    # turn into a branch
    return f"rt.int_wrap(2, rt.ct_barrier({x}), 38)"


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
    lo = [("lo", t[4 + i], "38") for i in range(4)]
    hi = [("hi", t[4 + i], "38") for i in range(3)]
    r, c1 = chain(t[:4], lo)
    h3 = v("hi")
    emit(f"let {h3} = rt.u64_mul_hi({t[7]}, 38)")
    # the top word hi3 + c1 + the second chain's carry: that chain's last
    # pair (adcs, no cset and add)
    r2, _ = chain(r[1:] + [h3], hi + [c1], keep=False)
    top = r2[3]
    return fold_top([r[0]] + r2[:3], top)


def store(rs):
    for i, x in enumerate(rs):
        emit(f"st(h, {8 * i}, {x})")


def load(nm):
    for i in range(4):
        emit(f"let {nm}{i} = ld({nm}, {8 * i})")


def add_row(t, i, ps):
    """t[i..i+3] += ps, the carry on through t[i+4] to t[i+5] (which then
    cannot carry: the product's bound)."""
    if t[i + 5] == "0":
        r, c = chain(t[i:i + 5], ps + ["0"])
        t[i:i + 5] = r
        if c != "0":
            t[i + 5] = c
    else:
        r, _ = chain(t[i:i + 6], ps + ["0", "0"], keep=False)
        t[i:i + 6] = r


def gen_mul():
    load("f")
    # the product's words t[0..7]; g's words load two rows at a time and
    # each product is made right before the pair that adds it (all 8 products
    # of a row up front kept 7 callee saved pairs on arm64). After row i
    # the sum is below 2^(64 (i + 5)) (f g_0..g_i): five words t[i..i+4]
    # take a row, the low halves' carry is word i+4 and the high halves'
    # chain ends there with no carry out.
    t = ["0"] * 8
    for i in range(4):
        # g's words two at a time (one ldp on arm64)
        if i % 2 == 0:
            emit(f"let g{i} = ld(g, {8 * i})")
            emit(f"let g{i + 1} = ld(g, {8 * i + 8})")
        lo = [("lo", f"f{j}", f"g{i}") for j in range(4)]
        hi = [("hi", f"f{j}", f"g{i}") for j in range(4)]
        if i == 0:
            t[0] = force(lo[0])
            r, _ = chain(lo[1:] + ["0"], hi, keep=False)
            t[1:5] = r
            continue
        r, c = chain(t[i:i + 4], lo)
        t[i:i + 4] = r
        t[i + 4] = c
        r, _ = chain(t[i + 1:i + 5], hi, keep=False)
        t[i + 1:i + 5] = r
    store(reduce8(t))


def gen_sq():
    load("f")

    def p(half, i, j):
        return (half, f"f{i}", f"f{j}")
    # the cross products f_i f_j (i < j) at words 1..6, each made right
    # before the pair that adds it (all 12 up front kept 3 callee saved
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
    store(reduce8([t0] + t))


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
