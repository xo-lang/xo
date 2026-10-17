#!/usr/bin/env python3
# Generates std/native/llrt/sha3.xo keccak (python3 scripts/keccaklazy.py):
# Keccak-f[1600] with lazy rotations (XKCP's ARMv8 form). Each lane is
# held as a value s and a pending rotation q (the lane is s rotated
# right by q), so a rotation costs nothing where it is made (rho, D's
# rotation by 1) and is applied where the lane is read, as the rotated
# operand of the eor or bic that reads it: arm64 folds a rotation by a
# constant read once into that operand (compiler/arm64 isel.xo
# rot_operand). The 4 rounds of the loop's body run lazily; at its end
# the 25 pending rotations are applied, so the loop carries plain lanes.
out = []
n = [0]

# FIPS 202 3.2.2's rotation offsets (left), lane x + 5 y
ROT = [0, 1, 62, 28, 27, 36, 44, 6, 55, 20, 3, 10, 43, 25, 39, 41, 45, 15, 21, 8, 18, 2, 61, 56, 14]


def v(p):
    n[0] += 1
    return f"{p}{n[0]}"


def emit(s):
    out.append("    " + s)


def rot(x, k):
    """x rotated right by k as Xo source (a plain x for 0)."""
    k %= 64
    if k == 0:
        return x
    return f"rt.int_wrap(5, {x}, {k})"


def xor(a, b):
    """(s, q) values a ^ b: ror(sa, qa) ^ ror(sb, qb) is
    ror(sa ^ ror(sb, qb - qa), qa)."""
    (sa, qa), (sb, qb) = a, b
    t = v("t")
    emit(f"let {t} = {sa} ^ {rot(sb, qb - qa)}")
    return (t, qa)


def andn(a, b):
    """~a & b: ror(sb, qb) & ~ror(sa, qa) is ror(sb & ~ror(sa, qa - qb), qb)."""
    (sa, qa), (sb, qb) = a, b
    t = v("t")
    emit(f"let {t} = {sb} & ~{rot(sa, qa - qb)}")
    return (t, qb)


def rotl(a, r):
    """a rotated left by r: only its pending rotation changes."""
    s, q = a
    return (s, (q - r) % 64)


def chi_xor(a, t):
    """a ^ t, the result keeping t's pending rotation (t is the fresh
    value, read once)."""
    return xor(t, a)


def round_(A, j):
    # theta
    C = []
    for x in range(5):
        c = A[x]
        for y in range(1, 5):
            c = xor(c, A[x + 5 * y])
        C.append(c)
    D = [xor(C[(x - 1) % 5], rotl(C[(x + 1) % 5], 1)) for x in range(5)]
    # rho and pi: B[y, 2x + 3y] = rotl(A[x, y] ^ D[x], r), made per
    # plane of chi right before it (25 at once spilled more)
    src = [None] * 25
    for x in range(5):
        for y in range(5):
            src[y + 5 * ((2 * x + 3 * y) % 5)] = x + 5 * y
    B = [None] * 25
    # chi
    N = [None] * 25
    for y in range(5):
        for x in range(5):
            i = x + 5 * y
            k = src[i]
            B[i] = rotl(xor(A[k], D[k % 5]), ROT[k])
        for x in range(5):
            i = x + 5 * y
            t = andn(B[(x + 1) % 5 + 5 * y], B[(x + 2) % 5 + 5 * y])
            N[i] = chi_xor(B[i], t)
    # iota: the round constant rotated left by lane 0's pending rotation
    s, q = N[0]
    t = v("t")
    emit(f"let {t} = {s} ^ {rot(f'rcs[round + {j}]', 64 - q)}")
    N[0] = (t, q)
    return N


A = [(f"a{i}", 0) for i in range(25)]
for j in range(4):
    A = round_(A, j)
for i, (s, q) in enumerate(A):
    emit(f"a{i} = {rot(s, q)}")

print("""/// Keccak-f[1600] on the 25 lanes, with lazy rotations (XKCP's ARMv8
/// form, scripts/keccaklazy.py): a lane is a value and a pending
/// rotation, so rho's and theta's rotations are the rotated operand of
/// the eor or bic that reads the lane (arm64 folds a rotation by a
/// constant read once there: isel.xo rot_operand), not instructions of
/// their own. The loop's body is 4 rounds; the pending rotations are
/// applied at its end. Generated: do not edit.
fn keccak(var a: List[Int]) {
  // the lanes as raw memory at constant offsets (the arm64 backend
  // checked each index): a write first makes a this call's own
  if a.len() != 25 {
    fault("sha3: a state of ${a.len()} lanes")
  }
  a[0] = a[0]
  let la = rt.int_wrap(0, rt.obj_addr(a), rt.layout(2))""")
for i in range(25):
    print(f"  var a{i} = rt.mem_load(la, {8 * i}, 8)")
print("""  // the round constants once (a constant's every use reads it through
  // const_once)
  let rcs = RCS
  var round = 0
  while round < 24 {""")
print("\n".join(out))
print("    round += 4\n  }")
for i in range(25):
    print(f"  rt.mem_store(la, {8 * i}, a{i}, 8)")
print("}")
