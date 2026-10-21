# 0311: std/big, arbitrary precision integers (Go's math/big Int)

Status: Accepted (owner, 2026-10-21, within decision 0276)

## Context

Xo has no public big integer. The coverage table lists math/big as
planned, and the survey's gap 10 (public RSA and ECDSA keys from JWKs,
big integers) needs one. Other uses: ids and amounts past 2^63, exact
arithmetic for money, and number theory in tests. std/native/llrt/bigmod.xo
already holds constant-time modular arithmetic for RSA (decision 0202),
but it is internal, works only modulo an odd number, and has no
division, parsing, or formatting. The owner put math/big on lib1's
queue on 2026-10-21.

## Decision

1. **A module `std/big` with `big.Int`**, Go's big.Int as an immutable
   value: every operation returns a new Int, so there is no receiver
   aliasing and no `z.Add(x, y)` form. Xo has no operator overloading,
   so arithmetic is methods. Comparison is not: Int implements `Eq`,
   `Ord` (`<` works), `Hash`, `Display`, `Debug`, and `Json`.

   ```
   struct Int {}                                    // opaque
   struct BigErr { msg: Str }                       // Go's texts
   pub fn int(n: Int) -> big.Int                    // Go's NewInt
   pub fn parse(s: Str, base: Int = 10) -> big.Int ! BigErr   // SetString; base 0 reads 0x, 0o, 0b and _
   pub fn from_bytes(b: Bytes) -> big.Int           // SetBytes: big-endian, unsigned

   fn (x: big.Int) add(y: big.Int) -> big.Int       // also sub, mul
   fn (x: big.Int) quo(y: big.Int) -> big.Int       // truncated (Go Quo); also rem, quo_rem
   fn (x: big.Int) div(y: big.Int) -> big.Int       // Euclidean (Go Div); also mod, div_mod
   fn (x: big.Int) exp(y: big.Int, m: big.Int? = None) -> big.Int
   fn (x: big.Int) gcd(y: big.Int) -> big.Int       // also gcd_ext -> (g, a, b)
   fn (x: big.Int) mod_inverse(m: big.Int) -> big.Int?
   fn (x: big.Int) sqrt() -> big.Int                // floor; faults on a negative
   fn (x: big.Int) neg() -> big.Int                 // also abs, sign() -> Int, cmp_abs
   fn (x: big.Int) lsh(n: Int) -> big.Int           // also rsh (arithmetic, as Go)
   fn (x: big.Int) and(y: big.Int) -> big.Int      // also or, xor, and_not, not (two's complement, as Go)
   fn (x: big.Int) bit(i: Int) -> Int               // also bit_len, trailing_zeros
   fn (x: big.Int) probably_prime(n: Int) -> Bool
   fn (x: big.Int) text(base: Int = 10) -> Str      // 2..62, Go's digits
   fn (x: big.Int) bytes() -> Bytes                 // |x| big-endian; also fill_bytes(len)
   fn (x: big.Int) to_int() -> Int?                 // None when it does not fit (IsInt64)
   fn (x: big.Int) to_float() -> Float              // nearest, as Float64()
   ```

   - Division by zero, `sqrt` of a negative, and a negative shift fault,
     where Go panics.
   - Json is Go's: a JSON number of any length, decoded exactly.
2. **Rat and Float are later stages**, each with its own ADR when a user
   needs them (exact fractions; arbitrary precision floats).
3. **Implementation.**
   - Go backend: std/rt wraps `*big.Int`, copying before every mutating
     call, so values stay immutable.
   - Native: a port of Go's nat (nat.go, natdiv.go, natconv.go, arith.go
     with its assembly kernels as Xo loops: addVV, subVV, mulAddVWW,
     addMulVVW), including Karatsuba multiplication and recursive
     (Burnikel-Ziegler) division above Go's thresholds. Limbs are
     List[Int] as in bigmod.xo. Variable time: like Go's math/big it is
     not for secrets, and crypto keeps bigmod.
4. **Tests.**
   - Go's int_test.go and nat_test.go tables as std module tests.
   - A Go-generated corpus (scripts/stdcorpus big): random operands from
     1 to 4,000 bits, across the multiplication and division thresholds,
     every operation against Go's results.
   - A golden on every backend.

## Consequences

- RSA and ECDSA public keys from JWKs can be read with `big.from_bytes`,
  and JSON with integers past 2^63 decodes exactly into a `big.Int`.
- Immutable values allocate on every operation, which Go's in-place form
  avoids. Loops that do millions of operations are slower than Go's.
  An in-place builder (like StrBuilder) can come later, if a measurement
  asks for it.
- The native nat code is the largest port in std so far. It lands in
  stages behind the same API: schoolbook arithmetic and conversion
  first, then Karatsuba, then recursive division, each checked against
  the corpus.

## Alternatives

- **Go's mutable `z.Add(x, y)` API:** it is how Go avoids allocation,
  but it needs a `var` receiver on every call and invites aliasing
  mistakes. Xo's other value types are immutable.
- **Exposing bigmod.xo:** constant time and modular only, so not a
  general big integer.
- **A third-party package:** std follows Go's scope (decision 0276), and
  math/big is in it.
