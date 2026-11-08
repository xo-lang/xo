# Xo Standard Library and Prelude

Status: draft 0.44 (2026-11-06; the same draft number as `spec/core.md`, decision 0089)
Companion to `spec/core.md`.

Signatures are written in Xo syntax. `[var]` before a receiver means the method
mutates and needs a `var` binding (core 3.4). Effects are listed with `uses`.
Everything here joins the compatibility promise with the first frozen
edition, named at 1.0 (core 13); until then the only edition is `preview`
(decision 0155), which may still change.

## 0. Conventions

- **Capabilities and fakes are handles.** Copying a `Net`, `Fs`, `Clock`, ...
  or a test fake shares the same underlying resource. They can be captured in
  closures and stored in structs.
- **Every error type implements `Display`.** So any std error converts to
  `Error` at a `?` site.
- **Every error type returned by a blocking operation has a `Canceled`
  variant** (core 7.2).
- Functions that cannot fail have no `!`. Lookups that may miss return `T?`.
- Braces are literal in plain strings (interpolation is `${x}`), so route
  patterns are ordinary strings: `"GET /users/{id}"`.
- Fields of std structs are `pub`: they can be read and the structs can be
  constructed (useful for testing pure functions).
- **Outside text is valid UTF-8** (decision 0259). Every std API that makes a
  `Str` from bytes it read keeps `Str` valid UTF-8: each byte of an invalid
  sequence becomes U+FFFD (as a decoder counts it, one per byte). That covers
  `Fs.read_text`, `File.read_line`, file names (`read_dir`, `glob`,
  `read_link`, `cwd_path`), `Stdio.read_line`, `read_all`, `lines`,
  `Child.read_line`, `Env.get`, `vars`, `args`, `program`, `home`, CLI values,
  archive entry names, HTTP header names and values (requests and responses),
  and JSON strings. URL parts keep the byte instead, as a percent escape:
  `Request.path`, `url.path`, `url.host`, and query keys and values (a request
  for `/a/\x82` has the path `/a/%82`, and a query escape that decodes to an
  invalid byte stays escaped). `Bytes.to_str()` and `Response.text()` return
  None instead; the exact bytes stay available where an API returns `Bytes`
  (`Fs.read`, `Response.body`, `Request.body`).

---

## 1. Prelude interfaces

Available everywhere without `use`.

```
interface Display { fn display(self) -> Str }
interface Error   { fn display(self) -> Str }   // any Display type is an Error
interface Debug   { fn debug(self) -> Str }     // implemented for every type
```

`Eq`, `Hash`, and `Ord` are builtin interfaces with no user written methods.
`Eq` and `Hash` are automatic when all fields implement them (handles and
function values never do, so a value holding one is not comparable: core
2.9, decision 0112); `Ord` comes from
`derive Ord` (field order, then variant order; a newtype's value's order,
core 2.8, decision 0390). Builtin scalar types, `Str`,
`Duration`, and `Time` are `Ord`. `Float` is `Ord` with a total order in which
NaN sorts after every other value (all NaNs are equal in the order) and
`-0.0` and `0.0` are equal: `-Inf < ... < -0.0 = 0.0 < ... < +Inf < NaN`
(decision 0053). This order drives `<`, `<=`, `>`, `>=` on Float (core
2.1), `sort` and `sort_by` (stable, so `-0.0` and `0.0` keep their input
order and NaNs end up last), derived `Ord` on types with Float fields,
`T: Ord` generic code, and `min`, `max`, `clamp`. `==` on Float is IEEE
(NaN differs from itself), not this order.

`Int`, the sized integers, `Float`, `Bool`, `Rune`, `Str`, `Duration`, `Time`,
and every std error type implement `Display`, and so does every type with
`derive Display`: `e.display()` is callable on each of them (the same text
as `"${e}"`), as well as through a `T: Display` bound or an `Error` value.
`List[T]`, `Map[K, V]` and `Set[T]` do when every type argument does, with their Debug text (`[1, 2]`, `{"a": 1}`, `#{"x"}`; decision 0525). `Bytes` does not (use `to_str()`). `Json` is implemented by `derive Json` and by
builtin types (section 5). `SqlRow` is implemented only by `derive SqlRow`
(section 10.4).

Every `Ord` type has `a.min(b)`, `a.max(b)`, `a.clamp(lo, hi)`. `a.min(b)`
is `b` when `b < a` and otherwise `a`; `a.max(b)` is `b` when `b > a` and
otherwise `a`: on a tie the receiver is kept (`(-0.0).min(0.0)` is
`-0.0`, `(0.0).min(-0.0)` is `0.0`). Under the Float order `x.min(NaN)` is
`x` and `x.max(NaN)` is NaN. `a.clamp(lo, hi)` is `a.max(lo).min(hi)`.

When a value is passed where an interface type is expected (for example a
`Map[Str, Debug]` literal), it is converted to that interface implicitly
(core 2.10).

---

## 2. Prelude methods on builtin types

### 2.1 Numbers

```
Int.parse(s: Str) -> Int?
Int.parse_base(s: Str, base: Int) -> Int?   // base 2 to 36, or 0 for 0b, 0o, 0x, 0 prefixes and _ separators
Float.parse(s: Str) -> Float?
Bool.parse(s: Str) -> Bool?                 // 1 t T TRUE true True, 0 f F FALSE false False
Int(x: Float) -> Int                 // truncates toward zero; faults on NaN or out of range
Float(x: Int) -> Float
Int32(x: Int) -> Int32               // faults out of range; same for every sized type
Int32.try(x: Int) -> Int32?
Int8.wrapping_from(x: Int) -> Int8       // Go's conversion T(x): the low bits kept, sign extended for a signed T, no fault;
                                         // also Int16, Int32, UInt8, UInt16, UInt32 (Int8.wrapping_from(300) is 44, UInt8.wrapping_from(-1) is 255)
UInt64.from_bits(b: Int) -> UInt64       // the same 64 bits (Go's uint64(x)): -1 is 2^64 - 1; UInt64.wrapping_from(x) is the same
fn (u: UInt64) to_bits() -> Int          // the same 64 bits back (Go's int64(u)): 2^63 and up are negative
Int(r: Rune) -> Int                  // the code point; every integer type (core 2.1)
Rune(n: Int) -> Rune                 // faults unless n is a Unicode scalar value
Rune.try(n: Int) -> Rune?

fn (n: Int) abs() -> Int
fn (n: Int) pow(e: Int) -> Int
fn (n: Int) wrapping_add(m: Int) -> Int      // also wrapping_sub, wrapping_mul
fn (n: Int) wrapping_shl(k: Int) -> Int      // k mod 64, the bits shifted out dropped
fn (n: Int) shr_logical(k: Int) -> Int       // the 64 bits shifted right by k mod 64, zeros in (`>>` keeps the sign)
fn (n: Int) rotate_right(k: Int) -> Int      // also rotate_right32: the low 32 bits by k mod 32, zero extended
fn (n: Int) swap_bytes() -> Int              // the 8 bytes reversed
fn (n: Int) mul_hi_unsigned(m: Int) -> Int   // the high word of the 128 bit product as unsigned words
fn (n: Int) lt_unsigned(m: Int) -> Bool      // also le_unsigned: compared as unsigned 64 bit words
fn (n: Int) saturating_add(m: Int) -> Int    // also saturating_sub, saturating_mul
fn (n: Int) checked_add(m: Int) -> Int?      // also checked_sub, checked_mul, checked_div
fn (n: Int) to_str_base(base: Int) -> Str    // base 2 to 36, lower case digits
fn (n: Int) to_str_width(width: Int, base: Int = 10, zero: Bool = false, plus: Bool = false) -> Str  // Go's %8d, %08d, %-8d, %+d, %x (decision 0410)
fn (n: Int) leading_zeros() -> Int           // also trailing_zeros, ones_count: math/bits on the word

fn (f: Float) floor() -> Float               // also ceil, round, trunc, abs, sqrt
fn (f: Float) to_fixed(prec: Int, width: Int = 0, zero: Bool = false, plus: Bool = false) -> Str  // strconv.FormatFloat 'f'; also to_exp ('e'), to_general ('g'); prec below 0 is the shortest (decision 0410)
fn (f: Float) is_nan() -> Bool               // also is_inf
fn (f: Float) exp() -> Float                 // also exp2, expm1, log, log2, log10, log1p, sin, cos, tan,
                                             // asin, acos, atan, sinh, cosh, tanh, cbrt
fn (f: Float) pow(y: Float) -> Float
fn (f: Float) atan2(x: Float) -> Float       // Go's Atan2(f, x)
fn (f: Float) hypot(y: Float) -> Float
fn (f: Float) mod(y: Float) -> Float         // Go's Mod: the remainder with the sign of f
fn (f: Float) to_bits() -> Int               // the IEEE 754 bits; Float.from_bits(b) back
fn (f: Float32) to_bits() -> UInt32          // the binary32 bits (Go's Float32bits); Float32.from_bits(b: UInt32) back (Float32frombits).
                                             // A signaling NaN comes back quiet on the native backends (the Go backend keeps it)
```

Int's word methods (decision 0260) treat an `Int` as a 64 bit word for
hashes, checksums, and protocol and crypto code: none faults, and the
native backends compile each to its instruction (a shift, `ror`, `rev`,
`umulh`, an unsigned compare), not a call.

`Int.parse(s)` is Go's `strconv.ParseInt(s, 10, 64)` and `Float.parse(s)`
is `strconv.ParseFloat(s, 64)`, None where they return an error: decimal
and hexadecimal (`0x1.8p1`) text, underscores between digits (`1_000`),
`inf`, `infinity` (either sign), and `nan`, any case; None when the
value is out of range. The text of a Float (interpolation, Display,
Debug) is the shortest that reads back the same value
(`strconv.FormatFloat(f, 'g', -1, 64)`) with `.0` added to whole
numbers: `1.0`, `0.1`, `1e+21`, `-0.0`, `+Inf`, `NaN`.

`Int.parse_base`, `to_str_base`, and `Bool.parse` are Go's `ParseInt(s,
base, 64)`, `FormatInt`, and `ParseBool` (decision 0287), None where Go
returns an error; a base outside 0 and 2 to 36 faults (Go panics for
FormatInt). Float's math functions (decision 0287) are Go's math package
on every backend, bit for bit and on every architecture: Go's portable
algorithms (exp.go, log.go, sin.go, ...) with every operation rounded,
the same in a native build and on the Go backend (which runs a copy of
them with each product rounded, not Go's package: Go fuses
multiply-adds on arm64 and runs other assembly algorithms on amd64).
Every Float operation in Xo rounds, on every backend: no backend fuses a
multiply and an add (golden float_no_fma). A fused multiply-add is asked
for by name: `x.fma(y, z)` is `x * y + z` computed exactly and rounded
once (IEEE 754 fusedMultiplyAdd, Go's `math.FMA`), the same bits on every
backend (decision 0385, golden float_fma). Special values are Go's:
`(-1.0).log()` is NaN, `(0.0).log()` is -Inf, `x.pow(0.0)` is 1.0. Float
constants (`Float.PI`, `Float.INF`) are a later step; `Float.parse("inf")`
gives infinity.

A conversion is a call on the type name, `Float(n)`, not an associated
function: there is no `Float.from(n)`. XO0601 for an unknown associated
function of a numeric type called with one numeric argument (`Float.from(n)`,
`Int.from(x)`, `Int64.new(n)`) carries the note `convert with Float(x)` and,
as its first fix, the edit to `Float(n)`.

### 2.2 Str, Rune, Bytes

`Str` is UTF-8. `+` concatenates `Str` and `Bytes`. `Bytes` holds any bytes;
build one piece by piece with `BytesBuilder` (2.7), whose appends are
amortized O(1) where `out = out + more` copies `out` each time.

```
fn (s: Str) len() -> Int                     // bytes
fn (s: Str) rune_count() -> Int
fn (s: Str) is_empty() -> Bool
fn (s: Str) contains(sub: Str) -> Bool
fn (s: Str) starts_with(p: Str) -> Bool
fn (s: Str) ends_with(p: Str) -> Bool
fn (s: Str) equal_fold(other: Str) -> Bool   // Unicode simple case folding (strings.EqualFold), not only ASCII
fn (s: Str) find(sub: Str) -> Int?           // byte offset
fn (s: Str) rfind(sub: Str) -> Int?          // the last sub's byte offset (strings.LastIndex); Some(s.len()) for ""
fn (s: Str) count(sub: Str) -> Int           // non-overlapping copies (strings.Count); rune_count() + 1 for ""
fn (s: Str) find_any(chars: Str) -> Int?     // the first rune of chars (strings.IndexAny); None for "" and no match
fn (s: Str) rfind_any(chars: Str) -> Int?    // the last rune of chars (strings.LastIndexAny)
fn (s: Str) contains_any(chars: Str) -> Bool // strings.ContainsAny
fn (s: Str) contains_rune(r: Rune) -> Bool   // strings.ContainsRune; an invalid byte of s reads as U+FFFD
fn (s: Str) find_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx   // the first rune f accepts (strings.IndexFunc)
fn (s: Str) rfind_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx  // the last one (strings.LastIndexFunc)
fn (s: Str) contains_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bool ! E uses fx     // whether one exists (strings.ContainsFunc)
// a predicate is a closure (`s.trim_where(fn(r) { r.is_space() })`), a named function or a bound
// method value; an unbound reference such as `Rune.is_space` is no value (XO0601)
fn (s: Str) cut(sep: Str) -> (Str, Str)?     // before and after the first sep (strings.Cut); None when absent
fn (s: Str) cut_prefix(prefix: Str) -> Str?  // the rest after a prefix (strings.CutPrefix); None when absent
fn (s: Str) cut_suffix(suffix: Str) -> Str?   // s up to a suffix (strings.CutSuffix); None when absent
fn (s: Str) split(sep: Str) -> List[Str]
fn (s: Str) split_n(sep: Str, n: Int) -> List[Str]  // at most n pieces, the unsplit rest in the last (strings.SplitN); n below 0 is split, 0 is no piece
fn (s: Str) split_after(sep: Str) -> List[Str]      // like split, each piece keeping its separator (strings.SplitAfter)
fn (s: Str) split_after_n(sep: Str, n: Int) -> List[Str] // strings.SplitAfterN
fn (s: Str) fields() -> List[Str]            // runs of non-white-space text (strings.Fields, unicode.IsSpace)
fn (s: Str) split_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> List[Str] ! E uses fx // the runs f rejects (strings.FieldsFunc); views of s like split
fn (s: Str) lines() -> List[Str]             // splits on \n, strips \r
fn (s: Str) trim() -> Str                    // also trim_start, trim_end
fn (s: Str) trim_prefix(p: Str) -> Str       // also trim_suffix
fn (s: Str) trim_chars(chars: Str) -> Str    // also trim_start_chars, trim_end_chars (strings.Trim, TrimLeft, TrimRight)
fn (s: Str) trim_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Str ! E uses fx       // strings.TrimFunc; also trim_start_where, trim_end_where
// split, split_n, split_after, split_after_n, fields, split_where, lines, trim, trim_start, trim_end, trim_chars,
// trim_start_chars, trim_end_chars, trim_where, trim_start_where, trim_end_where, trim_prefix and trim_suffix
// return views of s on native targets (O(1) per piece, no copy; a piece keeps s alive), as slices do (core 3.6)
fn (s: Str) to_lower() -> Str                // also to_upper (Unicode aware)
fn (s: Str) map_runes[E, fx: Effects](f: fn(Rune) -> Rune? ! E uses fx) -> Str ! E uses fx        // strings.Map; None drops the rune, and s itself when nothing changes
fn (s: Str) replace(old: Str, new: Str) -> Str
fn (s: Str) replace_n(old: Str, new: Str, n: Int) -> Str  // the first n copies (strings.Replace); n below 0 for every one, 0 for none
fn (s: Str) repeat(n: Int) -> Str            // strings.Repeat; empty for 0, a fault below 0
fn (s: Str) pad_left(width: Int, fill: Str = " ") -> Str   // to width runes (a minimum, as Go's %*s); also pad_right, center (decision 0410)
fn (s: Str) clone() -> Str                   // an explicit copy in a buffer of its own (a slice or split piece shares its source on native targets)
fn (s: Str) runes() -> List[Rune]
fn (s: Str) bytes() -> Bytes                 // no copy: s's own bytes (a short inline Str is boxed once), so s.bytes()[i] is byte i in O(1)

fn (b: Bytes) len() -> Int                   // b[i] is the byte at i, a UInt8
fn (b: Bytes) is_empty() -> Bool             // `for x in b` yields each byte (core 3.5)
fn (b: Bytes) to_str() -> Str?               // None if not valid UTF-8
fn (b: Bytes) to_str_lossy() -> Str          // each byte of an invalid sequence U+FFFD (the outside text rule; Go's string([]rune(s)))
fn (b: Bytes) to_valid_utf8(replacement: Str) -> Str  // bytes.ToValidUTF8: each run of invalid bytes becomes one replacement
fn (b: Bytes) to_hex() -> Str                // lowercase
fn (b: Bytes) to_base64() -> Str             // standard encoding, padded
Bytes.from_hex(s: Str) -> Bytes?
Bytes.from_base64(s: Str) -> Bytes?
fn (b: Bytes) rune_count() -> Int            // an invalid byte counts as one rune
fn (b: Bytes) decode_rune(i: Int) -> (Rune, Int)  // the rune at byte i and its size
fn (b: Bytes) quote() -> Str                 // also on Str: a quoted literal
fn (b: Bytes) find(sub: Bytes) -> Int?       // bytes.Index; Some(0) for an empty sub
fn (b: Bytes) rfind(sub: Bytes) -> Int?      // bytes.LastIndex; Some(b.len()) for an empty sub
fn (b: Bytes) contains(sub: Bytes) -> Bool   // bytes.Contains; true for an empty sub
fn (b: Bytes) starts_with(p: Bytes) -> Bool  // bytes.HasPrefix; true for an empty p
fn (b: Bytes) ends_with(p: Bytes) -> Bool    // bytes.HasSuffix; true for an empty p
fn (b: Bytes) count(sub: Bytes) -> Int       // bytes.Count, non-overlapping; the rune count plus one for an empty sub
fn (b: Bytes) find_any(chars: Str) -> Int?   // the first rune of chars (bytes.IndexAny); None for "" and no match
fn (b: Bytes) rfind_any(chars: Str) -> Int?  // the last rune of chars (bytes.LastIndexAny)
fn (b: Bytes) contains_any(chars: Str) -> Bool // bytes.ContainsAny
fn (b: Bytes) contains_rune(r: Rune) -> Bool // bytes.ContainsRune; an invalid byte of b reads as U+FFFD
fn (b: Bytes) find_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx   // the first rune f accepts (bytes.IndexFunc)
fn (b: Bytes) rfind_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx  // the last one (bytes.LastIndexFunc)
fn (b: Bytes) contains_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bool ! E uses fx     // whether one exists (bytes.ContainsFunc)
// a predicate is a closure (`b.split_where(fn(r) { r.is_space() })`), a named function or a bound
// one, as Str's find_where takes it (strings.IndexFunc there)
fn (b: Bytes) split(sep: Bytes) -> List[Bytes]     // bytes.Split; an empty sep splits after each UTF-8 sequence (Go's explode), and an empty b gives no piece
fn (b: Bytes) split_n(sep: Bytes, n: Int) -> List[Bytes]  // at most n pieces, the unsplit rest in the last (bytes.SplitN); n below 0 is split, 0 is no piece
fn (b: Bytes) split_after(sep: Bytes) -> List[Bytes]      // like split, each piece keeping its separator (bytes.SplitAfter)
fn (b: Bytes) split_after_n(sep: Bytes, n: Int) -> List[Bytes] // bytes.SplitAfterN
fn (b: Bytes) cut(sep: Bytes) -> (Bytes, Bytes)?  // before and after the first sep (bytes.Cut); None when absent
fn (b: Bytes) cut_prefix(prefix: Bytes) -> Bytes? // the rest after a prefix (bytes.CutPrefix); None when absent
fn (b: Bytes) cut_suffix(suffix: Bytes) -> Bytes? // b up to a suffix (bytes.CutSuffix); None when absent
fn (b: Bytes) fields() -> List[Bytes]        // runs of non-white-space bytes (bytes.Fields, unicode.IsSpace)
fn (b: Bytes) split_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> List[Bytes] ! E uses fx // the runs f rejects (bytes.FieldsFunc)
fn (b: Bytes) trim() -> Bytes                // white space at both ends (bytes.TrimSpace)
fn (b: Bytes) trim_prefix(p: Bytes) -> Bytes // also trim_suffix (bytes.TrimPrefix, TrimSuffix)
fn (b: Bytes) trim_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bytes ! E uses fx       // bytes.TrimFunc; also trim_start_where, trim_end_where (TrimLeftFunc, TrimRightFunc)
// split, split_n, split_after, split_after_n, cut, cut_prefix, cut_suffix, fields, split_where, trim,
// trim_prefix, trim_suffix, trim_where, trim_start_where and trim_end_where return views of b on native
// targets (O(1) per piece, no copy; a piece keeps b alive), as slices do (core 3.6)
fn (b: Bytes) replace(old: Bytes, new: Bytes) -> Bytes  // bytes.ReplaceAll; b itself when nothing changes
fn (b: Bytes) replace_n(old: Bytes, new: Bytes, n: Int) -> Bytes  // the first n copies (bytes.Replace); n below 0 for every one, 0 for none
fn (b: Bytes) to_lower() -> Bytes            // bytes.ToLower; an invalid byte becomes U+FFFD, as Go maps it
fn (b: Bytes) to_upper() -> Bytes            // bytes.ToUpper
fn (b: Bytes) map_runes[E, fx: Effects](f: fn(Rune) -> Rune? ! E uses fx) -> Bytes ! E uses fx      // bytes.Map; None drops the rune, and b itself when nothing changes
fn (b: Bytes) equal_fold(other: Bytes) -> Bool  // bytes.EqualFold, Unicode simple case folding
fn (b: Bytes) repeat(n: Int) -> Bytes        // bytes.Repeat; empty for 0, a fault below 0
fn (b: Bytes) clone() -> Bytes               // an explicit copy in a buffer of its own
fn (b: Bytes) indexed() -> List[(Int, UInt8)]  // each byte with its offset, as List.indexed
fn (static: Bytes) zeroed(n: Int) -> Bytes   // n zero bytes (make([]byte, n)); a fault for n < 0
fn (s: Str) unquote() -> Str?                // Go's Unquote (decision 0287); None when s is not a literal or the text is not UTF-8

fn (r: Rune) is_digit() -> Bool              // also is_letter, is_space
fn (r: Rune) is_upper() -> Bool              // also is_lower, is_title
fn (r: Rune) is_punct() -> Bool              // also is_control, is_graphic, is_mark, is_symbol, is_number
fn (r: Rune) to_lower() -> Rune              // also to_upper, to_title
fn (r: Rune) utf8_len() -> Int               // 1, 2, 3 or 4 bytes (utf8.RuneLen; a Rune is a scalar value, so Go's -1 cannot happen)
fn (r: Rune) is_print() -> Bool              // strconv.IsPrint
fn (r: Rune) quote() -> Str                  // 'a', '\n', '\u2028'
```

Unicode and quoting follow Go's `unicode`, `unicode/utf8`, and `strconv`
packages exactly (decision 0079), so text a Go program prints can be
reproduced:

- `is_letter` is a letter (category L), `is_digit` a decimal digit (Nd),
  `is_space` white space (`\t \n \v \f \r`, space, U+0085, U+00A0, and
  category Zs, plus U+2028 and U+2029), `is_print` `strconv.IsPrint` (a
  letter, mark, number, punctuation, symbol, or the ASCII space).
- The other classes are Go's `IsUpper`, `IsLower` and `IsTitle` (the
  cased letters), and `IsPunct`, `IsControl`, `IsGraphic`, `IsMark`,
  `IsSymbol` and `IsNumber` (the categories P, C, L+M+N+P+S+Zs, M, S and
  N). `to_title` is `unicode.ToTitle`: the upper case of an ASCII letter,
  and the title case of a digraph (`ǅ` for `Ǆ` and `ǆ`).
- `utf8_len` is `utf8.RuneLen` of the rune: 1, 2, 3 or 4 bytes. A `Rune`
  is a Unicode scalar value (core 2.4), so Go's -1 for an invalid rune
  cannot happen; `Rune.try(n)` is `utf8.ValidRune` for an `Int`.
- `decode_rune(i)` reads the UTF-8 sequence at byte `i`: `(U+FFFD, 1)`
  for an invalid or short sequence, `(U+FFFD, 0)` at `i == len()`, and a
  fault when `i` is negative or past the end.
- `r.quote()` is `strconv.QuoteRune`: single quotes, `\'` and `\\`
  escaped, printable runes as they are, `\a \b \f \n \r \t \v`, then
  `\xHH` below U+0080, `\uHHHH`, or `\UHHHHHHHH`.
- `s.quote()` and `b.quote()` are `strconv.Quote`: double quotes, the same
  escapes with `\"` in place of `\'`, and every byte of an invalid UTF-8
  sequence as `\xHH`.

`trim_chars(chars)`, `trim_start_chars(chars)` and `trim_end_chars(chars)`
remove the leading, the trailing, or both kinds of runes of `s` that are
in `chars`, as Go's `strings.Trim`, `TrimLeft` and `TrimRight` do (an
empty `chars` returns `s`); `trim`, `trim_start` and `trim_end` remove
Unicode white space instead. All are views of `s`.

Slicing (`s[a..b]`) is in core 3.6.

### 2.3 List[T]

```
List[T].with_capacity(n: Int) -> List[T]     // empty, room for n elements
fn (xs: List[T]) len() -> Int
fn (xs: List[T]) is_empty() -> Bool
fn (xs: List[T]) get(i: Int) -> T?
fn (xs: List[T]) first() -> T?               // also last
fn (xs: List[T]) contains(x: T) -> Bool      // T: Eq
fn (xs: List[T]) index_of(x: T) -> Int?      // T: Eq
fn (xs: List[T]) indexed() -> List[(Int, T)]
fn (xs: List[T]) map[U](f: fn(T) -> U) -> List[U]
fn (xs: List[T]) filter(f: fn(T) -> Bool) -> List[T]
fn (xs: List[T]) filter_map[U](f: fn(T) -> U?) -> List[U]   // f's Some values, in order
fn (xs: List[T]) fold[A](init: A, f: fn(A, T) -> A) -> A
fn (xs: List[T]) find(f: fn(T) -> Bool) -> T?
fn (xs: List[T]) any(f: fn(T) -> Bool) -> Bool   // also all
fn (xs: List[T]) sort() -> List[T]           // T: Ord, stable
fn (xs: List[T]) sort_by[K: Ord](key: fn(T) -> K) -> List[T]
fn (xs: List[T]) group_by[K: Hash](key: fn(T) -> K) -> List[(K, List[T])]   // keys in first appearance order, each group in input order
fn (xs: List[T]) reverse() -> List[T]
fn (xs: List[T]) to_set() -> Set[T]
(0..n).to_list()                             // Range[Int] to List[Int]
fn (xs: List[Str]) join(sep: Str) -> Str
xs + ys                                      // concatenation

fn ([var] xs: List[T]) push(x: T)
fn ([var] xs: List[T]) pop() -> T?           // removes the last element
fn ([var] xs: List[T]) insert(i: Int, x: T)
fn ([var] xs: List[T]) remove_at(i: Int) -> T
fn (var xs: List[T]) heap_push(x: T)           // T: Ord; a binary min heap in the list (decision 0410)
fn (var xs: List[T]) heap_pop() -> T?           // the least element, None for an empty list
fn (var xs: List[T]) heap_remove(i: Int) -> T   // the element at i; a fault outside the list
fn (var xs: List[T]) heap_fix(i: Int)           // after the element at i changed (set, then fix)
fn (var xs: List[T]) heapify()                  // make any list a heap
fn ([var] xs: List[T]) set(i: Int, x: T)     // also xs[i] = x on a var list
fn (var xs: List[T]) sort_in_place()         // T: Ord, stable, sorts the receiver
fn (var xs: List[T]) sort_in_place_by[K: Ord](key: fn(T) -> K)   // stable
```

`sort` of a `List[Int]` runs Go's pattern-defeating quicksort (the
algorithm of `slices.Sort`), which does not keep equal elements in input
order; equal Ints cannot be told apart, so the result is the stable sort's.
Every other element type, `Float` included (`-0.0` and `0.0` compare
equal), keeps the stable merge.

`sort_in_place` and `sort_in_place_by` order the receiver itself and return
nothing (decision 0374); they are stable exactly like `sort` and `sort_by`. A
list another binding shares is copied first, the way `push` and `set` copy
one, so the other holder keeps its own order; a uniquely owned list is sorted
in its own array with a scratch under one list. A `List[Int]` receiver is
sorted by the same pattern-defeating quicksort as `sort`, in its own array
with no scratch.

A builder whose receiver nothing else holds reuses the receiver's array
(decision 0508): `sort` of a list passed at its last use (a temporary such
as `xs.freeze().sort()`, or a binding not read again) sorts that list's own
array, and `map` of a list of 256 or more elements to its own element type
writes each result over its element. A list another binding holds is copied
first, so results never change, only allocation.

Higher order methods (`map`, `filter`, ...) pass through the callback's effects
and errors through effect variables (core 6.2); `map` with a failing callback
returns `List[U] ! E`.

`List[T].with_capacity(n)` (also `List.with_capacity(n)` where the type is
known) is an empty list, equal to `[]`, whose first `n` pushes append
without growing it; `n` below 0 counts as 0. Capacity is not observable
(decision 0170). A new list has room for at least 4 elements, or for 1
when its elements are over 256 bytes (decision 0452's addendum: a list of
one connection's state held four).

### 2.4 Map[K, V] and Set[T]

Insertion ordered. `put` on an existing key keeps its position.

```
fn (m: Map[K, V]) len() -> Int
fn (m: Map[K, V]) is_empty() -> Bool
fn (m: Map[K, V]) get(k: K) -> V?
fn (m: Map[K, V]) get_or_fault(k: K, reason: Str) -> V   // faults with the key and reason when missing
fn (m: Map[K, V]) contains(k: K) -> Bool
fn (m: Map[K, V]) keys() -> List[K]
fn (m: Map[K, V]) values() -> List[V]
fn (m: Map[K, V]) entries() -> List[(K, V)]
fn (m: Map[K, V]) filter(f: fn(K, V) -> Bool) -> Map[K, V]
fn (m: Map[K, V]) map[U](f: fn(K, V) -> U) -> Map[K, U]   // every key, in insertion order
fn (m: Map[K, V]) any(f: fn(K, V) -> Bool) -> Bool        // insertion order; stops at the first true
fn (m: Map[K, V]) all(f: fn(K, V) -> Bool) -> Bool        // insertion order; stops at the first false

fn ([var] m: Map[K, V]) put(k: K, v: V)
fn ([var] m: Map[K, V]) remove(k: K) -> V?
fn ([var] m: Map[K, V]) update[R](k: K, init: V, f: fn(var V) -> R) -> R
    // reads k (or init if missing), runs f, stores the result, returns f's value
    // in place of copy-modify-store (`var ids = by.get(n) ?? []; ids.push(i); by[n] = ids`):
    //   by.update(n, [], fn(var ids) { ids.push(i) })   (xo vet XO0532, decision 0266)

fn (s: Set[T]) len() -> Int
fn (s: Set[T]) contains(x: T) -> Bool
fn (s: Set[T]) union(o: Set[T]) -> Set[T]    // also intersect, difference
fn (s: Set[T]) is_subset(o: Set[T]) -> Bool   // every element of s is in o
fn (s: Set[T]) intersects(o: Set[T]) -> Bool  // a shared element; stops at the first, allocates nothing
fn (s: Set[T]) to_list() -> List[T]
fn (s: Set[T]) is_empty() -> Bool
fn (s: Set[T]) any(f: fn(T) -> Bool) -> Bool   // also all
fn (s: Set[T]) filter(f: fn(T) -> Bool) -> Set[T]
fn (s: Set[T]) map[U](f: fn(T) -> U) -> Set[U]
fn ([var] s: Set[T]) add(x: T)
fn ([var] s: Set[T]) remove(x: T) -> Bool
```

A `Set` literal is `#{1, 2, 3}`, and `#{}` is an empty one whose element
type the context gives (`let seen: Set[Str] = #{}`; core 1.6); a list
converts with `to_set()`. `[]` alone is a `List`, not a `Set`.

Iterating a `var` collection while mutating it iterates the value as it was
when the loop started.

### 2.5 Option[T] and Result[T, E]

```
fn (o: T?) is_some() -> Bool                 // also is_none
fn (o: T?) or_err[E](e: E) -> Result[T, E]
fn (o: T?) map[U](f: fn(T) -> U) -> U?
fn (o: T?) and_then[U](f: fn(T) -> U?) -> U?
fn (o: T?) or_fault(msg: Str) -> T           // faults on None
o ?? default                                 // T

fn (r: Result[T, E]) is_ok() -> Bool         // also is_err
fn (r: Result[T, E]) map[U](f: fn(T) -> U) -> Result[U, E]
fn (r: Result[T, E]) map_err[F](f: fn(E) -> F) -> Result[T, F]
fn (r: Result[T, E]) ok() -> T?
fn (r: Result[T, E]) err() -> E?
fn (r: Result[T, E]) or_fault(msg: Str) -> T // faults on Err, message includes the error
fn (r: Result[T, E]) ignore_err(reason: Str) -> T?
    // the only way to discard an error (core 2.6); logs the error and reason at debug level
```

### 2.6 Duration and Time

Operators are in core 2.1. `Duration` displays as `1.5s`, `250ms`; `Time`
displays as RFC 3339 in UTC.

```
Duration.from_secs(s: Float) -> Duration      // rounds to the nearest nanosecond
Duration.from_secs_int(s: Int) -> Duration    // the same as `1s * s`; faults on overflow
Duration.from_millis(ms: Int) -> Duration
Duration.from_nanos(ns: Int) -> Duration
Duration.parse(s: Str) -> Duration ! DurationErr
fn (d: Duration) seconds() -> Float          // Go's Duration.Seconds to the bit: whole seconds plus the rest over 1e9 (decision 0503)
fn (d: Duration) millis() -> Int             // truncated
fn (d: Duration) nanos() -> Int
fn (d: Duration) scale(f: Float) -> Duration // rounds to the nearest nanosecond
fn (d: Duration) ceil_seconds() -> Int
fn (d: Duration) abs() -> Duration           // durations may be negative
fn (d: Duration) round(m: Duration) -> Duration     // Duration.Round: nearest multiple, halves away from zero, saturating; d when m <= 0
fn (d: Duration) truncate(m: Duration) -> Duration  // Duration.Truncate: toward zero; d when m <= 0

struct DurationErr { input: Str }  // displays as `invalid duration "1x": expected numbers with units ...`

Time.UNIX_EPOCH: Time
Time.from_unix(secs: Int) -> Time
Time.from_unix_millis(ms: Int) -> Time
Time.rfc3339_offset(s: Str) -> Duration?   // the offset the text carries (Go's Zone() of time.Parse; Z is zero), None when parse_rfc3339 is None
Time.parse_rfc3339(s: Str) -> Time?   // None for text that is not RFC 3339 or a time outside Time's range (1677-09-21 to 2262-04-11, int64 nanoseconds since 1970; decision 0364)
fn (t: Time) unix() -> Int
fn (t: Time) unix_millis() -> Int
fn (t: Time) format_rfc3339() -> Str
```

`Duration.parse` accepts Go's `time.ParseDuration` syntax after trimming
white space: an optional sign, then one or more decimal numbers (with an
optional fraction) each followed by a unit `ns`, `us` (or `µs`), `ms`,
`s`, `m`, `h`: `"1.5s"`, `"-2h45m"`, `"300ms"`. `"0"` alone is zero. A
number without a unit, an unknown unit, or a value past about 292 years
is `Err(DurationErr{input: s})`. It is the form JSON decoding of a
`Duration` accepts, and it reads back what `Duration` displays (decision
0088). Unlike `Int.parse`, it returns a `Result`: the error says what a
duration looks like.

### 2.7 Concurrency and state types

```
Chan[T].new(cap: Int = 0) -> Chan[T]
fn (c: Chan[T]) send(x: T)                   // blocks; cancellation point
fn (c: Chan[T]) recv() -> T?                 // None when closed and drained
fn (c: Chan[T]) try_send(x: T) -> Bool         // false if full or closed; never blocks
fn (c: Chan[T]) try_recv() -> T?             // None if empty; never blocks
fn (c: Chan[T]) close()
fn (c: Chan[T]) len() -> Int

Mutex[T].new(v: T) -> Mutex[T]
fn (m: Mutex[T]) with[R, E, fx: Effects](f: fn(var T) -> R ! E uses fx) -> R ! E uses fx
fn (m: Mutex[T]) with_read[R, E, fx: Effects](f: fn(T) -> R ! E uses fx) -> R ! E uses fx
fn (m: Mutex[T]) reset(v: T)                 // stores v, clears the poison (decision 0482)

Atomic[T].new(v: T) -> Atomic[T]             // T is Int or Bool
fn (a: Atomic[T]) get() -> T
fn (a: Atomic[T]) set(v: T)
fn (a: Atomic[Int]) add(n: Int) -> Int       // returns the new value
fn (a: Atomic[Int]) max(n: Int) -> Int       // stores the larger value, returns it
fn (a: Atomic[T]) swap(v: T) -> T
fn (a: Atomic[T]) compare_and_swap(old: T, new: T) -> Bool

fn (t: Task[T, E]) await() -> T ! E          // written `t.await`

Buf[T].new() -> Buf[T]                       // also with_capacity(n)
fn ([var] b: Buf[T]) push(x: T)
fn (b: Buf[T]) len() -> Int
fn (b: Buf[T]) freeze() -> List[T]           // moves b
StrBuilder.new() -> StrBuilder
fn ([var] b: StrBuilder) write(s: Str)
fn (b: StrBuilder) freeze() -> Str           // moves b
BytesBuilder.new() -> BytesBuilder           // also with_capacity(n)
fn ([var] b: BytesBuilder) write(data: Bytes)
fn ([var] b: BytesBuilder) write_byte(x: UInt8)
fn ([var] b: BytesBuilder) write_str(s: Str)  // its UTF-8 bytes
fn ([var] b: BytesBuilder) write_rune(r: Rune) // its UTF-8 encoding
fn ([var] b: BytesBuilder) put_le(at: Int, v: Int, n: Int) // overwrite n (1, 2, 4, 8) bytes in place; put_be too
fn (b: BytesBuilder) len() -> Int            // also on StrBuilder
fn (b: BytesBuilder) freeze() -> Bytes       // moves b
ByteBuffer.new() -> ByteBuffer               // also with_capacity(n); move only
fn ([var] b: ByteBuffer) write(data: Bytes)  // appended at the back
fn (b: ByteBuffer) len() -> Int              // the unread bytes
fn (b: ByteBuffer) peek(n: Int) -> Bytes     // up to n unread bytes, not consumed
fn ([var] b: ByteBuffer) next(n: Int) -> Bytes // up to n bytes, consumed
fn ([var] b: ByteBuffer) read_until(delim: UInt8) -> Bytes? // through delim, consumed; None: nothing consumed
fn ([var] b: ByteBuffer) discard(n: Int)     // up to n bytes
fn (b: ByteBuffer) find(sub: Bytes) -> Int?  // offset in the unread bytes

Secret[T].new(v: T) -> Secret[T]
fn (s: Secret[T]) reveal() -> T
```

`ByteBuffer` (decision 0480) is Go's bytes.Buffer for cutting records from
bytes that arrive in pieces: the loop `b.write(piece)` then `while let
Some(rec) = b.read_until(0x0A)` costs time linear in the bytes, since
`read_until` resumes its search where the last call with the same
delimiter stopped, and a write keeps its bytes without copying them (a
record inside one written piece is a view of it; one that spans pieces is
joined once). `peek`, `next` and `read_until` return views that stay
valid after later writes (Go's `Next` is valid only until the next
write); `peek` and `find` copy when the bytes they cover span pieces. A count `n` below 0 is 0 and above `len()` is `len()`.

`BytesBuilder.put_le` and `put_be` (decision 0399) overwrite the `n` bytes
`at..at + n` of what has been written, in place (Go's
`binary.LittleEndian.PutUint32` on a slice): the low `n` bytes of `v`, lowest
first (`put_le`) or highest first (`put_be`), the rest of `v` dropped. `n` is
1, 2, 4 or 8 and the range must lie within `len()`; any other `n`, or a range
outside, is a fault. The length does not change and nothing is copied.
`write_le` and `write_be` (decision 0561) append the low `n` bytes of `v`
instead (Go's `binary.LittleEndian.AppendUint64` cut to `n`), lowest or
highest first; `n` is 0 to 8, any other is a fault.

A fault that unwinds out of a `with` closure poisons the mutex (core 5.4,
decision 0482): from then on `with` and `with_read` fault with `XO-F010`.
`m.reset(v)` takes the mutex whether or not it is poisoned (waiting is a
cancellation point), stores `v`, clears the poison, and drops the value it
held. An `Err` from the closure, or a cancellation while it waits, leaves the
value unchanged and does not poison. Like Rust's `Mutex` poisoning, it keeps
a half changed value from being used silently; unlike Go's `sync.Mutex` with
a deferred `Unlock`, which leaves it in place.

`Mutex.with_read` (decision 0063) is shared read only access: many readers
or one writer. `f` gets the value read only (mutating it is an error, and
its parameter cannot be declared `var`); nothing is written back. Readers
run at the same time as each other; `with` waits until no reader is
inside, and readers wait while a writer is inside.

- Writers are preferred: while a `with` waits, a new `with_read` waits
  behind it, so a stream of readers cannot starve a writer. So a reader
  must not wait (on a channel, a task, or another lock) for a reader that
  has not entered yet: with a writer queued in between that is a deadlock
  (XO-F001, reported with the readers that hold the mutex).
- In a real run the order among waiters is not fixed (as for `with`,
  decision 0047). Under `xo test` and `--seed` it is first come, first
  served, with the readers at the head of the queue entering together;
  readers inside together interleave at scheduling points like any tasks,
  and `--explore` enumerates those interleavings.
- Waiting is a cancellation point; a canceled waiter leaves the queue
  (readers queued behind a canceled writer enter at once).
- Calling `with` or `with_read` on a mutex the task already holds in
  either mode faults (`XO-F004`).
- A value read in `with_read` and used in a later `with` on the same mutex
  gets the split critical section warning `XO0804` (core 5.4).

```
struct Stats {
  hits: Int
  last: Str
}

fn record(m: Mutex[Stats], path: Str) {
  m.with(fn(var s) {
    s.hits += 1
    s.last = path
  })
}

fn report(m: Mutex[Stats]) -> Str {
  m.with_read(fn(s) { "${s.hits} hits, last ${s.last}" })
}
```

---

## 3. Capabilities

Predeclared interfaces. `main(os: Os)` receives the real ones; tests receive
fakes (section 8).

A program's own type that implements one (a wrapper that prefixes every
line, a clock that runs ahead, a read-only view) is used wherever the
capability is expected (core 2.10, rule 2), and a program may
build an `Os` with a literal, `Os{net: ..., fs: ..., stdio: ...}`, to
hand code written against `Os` attenuated or wrapped capabilities. Both
work the same on every backend (decision 0307).

### 3.1 Fs (`uses fs.read` / `fs.write`)

Paths use `/` on every OS. `os.fs` is rooted at the filesystem root, accepts
absolute paths (`/home/me`, `C:/data`), and resolves relative paths against
the working directory. In a sub capability, paths are relative to its root,
`"."` and `"/"` both mean the root, and a path that escapes it (`..`) fails
with `Denied`.

On Windows (decision 0197, stage 4) `os.fs`, `sub`, and the other
operations also take the forms the OS writes, so a path read from a
variable or an argument works as it is: `\` separates as `/` does; a
drive path (`C:/data`, `C:\data`) and a UNC path (`//server/share/x`,
`\\server\share\x`) are absolute; a path rooted at `/` without a drive
is on the working directory's drive. Every path std gives back uses `/`
only: `FileInfo.path`, `create_temp`, `read_link`, `glob`, and
`Env.program`, `home`, `config_dir`, and `cache_dir` (3.5). On other OSes
`\` is an ordinary character of a name, as before.

`cwd_path` (decision 0218) is the working directory as an absolute path
with `/` separators, the directory `cwd()` is rooted at, asked of the OS
(getcwd; `GetCurrentDirectoryW` on Windows) and never taken from `$PWD`
alone: `$PWD` is used only when it names the same directory (it keeps
the links the shell's path has, as Go's `os.Getwd` does), so a stale
`$PWD` from a parent that started the program without a shell (`PWD=/`
from the macOS Dock) is ignored. It is the same on every capability
derived from `os.fs`. It fails with `Io` when the OS cannot give it
(a removed directory). WASI has no working directory: there it fails
with `Io` (planned: the wasm backend reports the call as unsupported
today, as it does `cwd()`).
A test's `MemFs` gives `/`, its root, where its `cwd()` is rooted.

```
enum FsErr {
  NotFound(path: Str)
  Exists(path: Str)
  NotEmpty(path: Str)
  NotDir(path: Str)
  IsDir(path: Str)
  Denied(path: Str)
  Io(path: Str, msg: Str)
  Canceled
}

`display` prints plain messages (decision 0358): `PATH: no such file or
directory`, `PATH: file exists`, `PATH: directory not empty`, `PATH: not a
directory`, `PATH: is a directory`, `PATH: permission denied`, `PATH: MSG`
for `Io` (the text alone when the path is empty), and `canceled`. `Debug`
keeps the variant form.

struct FileInfo {
  name: Str              // last path segment
  path: Str              // relative to the capability root
  is_dir: Bool
  size: Int
  modified: Time
  is_symlink: Bool = false  // a link itself: read_dir and walk report links, stat follows them
}

struct DirEntry {          // read_dir_entries, walk_entries, walk_dir_entries (decision 0483): no stat
  name: Str
  path: Str                // relative to the capability root
  is_dir: Bool             // from the directory entry; a link is not followed
  is_symlink: Bool = false
}                          // fn (e: DirEntry) info(fs: Fs) -> FileInfo ! FsErr uses fs.read: the lstat now

struct DiskUsage {         // the file system holding a path (decision 0426)
  total: Int               // bytes
  free: Int                // bytes free, a superuser's reserve included
  available: Int           // bytes free to this user
  files: Int               // inodes (0 on Windows)
  files_free: Int          // free inodes (0 on Windows)
}

fn (f: Fs) sub(dir: Str) -> Fs                               // lazy: dir need not exist yet
fn (f: Fs) cwd() -> Fs                                       // sub capability at the working directory
fn (f: Fs) cwd_path() -> Str ! FsErr                         // the working directory's absolute path, from the OS
fn (f: Fs) read_only() -> Fs
fn (f: Fs) write_only() -> Fs                // reads fail with Denied
fn (f: Fs) case_sensitive() -> Bool                          // of the underlying filesystem
fn (f: Fs) stat(path: Str) -> FileInfo? ! FsErr  uses fs.read // None when missing
fn (f: Fs) exists(path: Str) -> Bool ! FsErr     uses fs.read
fn (f: Fs) disk_usage(path: Str) -> DiskUsage ! FsErr uses fs.read // statfs of path's file system
fn (f: Fs) read(path: Str) -> Bytes ! FsErr      uses fs.read
fn (f: Fs) read_text(path: Str) -> Str ! FsErr   uses fs.read
fn read_text_or(fs: Fs, path: Str, default: Str) -> Str ! FsErr uses fs.read   // default when missing
fn read_or(fs: Fs, path: Str, default: Bytes) -> Bytes ! FsErr uses fs.read     // the same for bytes
fn read_mapped(fs: Fs, path: Str) -> Bytes ! FsErr uses fs.read                  // read, maybe sharing the file's pages
fn write_atomic(fs: Fs, path: Str, data: Bytes) -> () ! FsErr uses fs            // old or new, never a part
fn (f: Fs) read_dir(dir: Str) -> List[FileInfo] ! FsErr uses fs.read   // sorted by name
fn (f: Fs) walk(dir: Str) -> List[FileInfo] ! FsErr     uses fs.read
    // recursive, parents before children, siblings by name; excludes `dir` itself;
    // fails with NotFound if `dir` does not exist;
    // paths are relative to the capability root with no "./" prefix
enum WalkStep { Continue, SkipDir, Stop }
fn walk_dir[fx: Effects](fs: Fs, dir: Str, visit: fn(FileInfo) -> WalkStep uses fx)
    -> List[FileInfo] ! FsErr uses (fs.read, fx)
    // walk's order, visit decides: SkipDir (on a file: the rest of its directory), Stop
fn (f: Fs) write(path: Str, data: Bytes) -> () ! FsErr  uses fs.write  // create or truncate
fn (f: Fs) create_temp(dir: Str, prefix: Str) -> Str ! FsErr uses fs.write
    // returns the new file's path, relative to the capability root
fn (f: Fs) rename(from: Str, to: Str) -> () ! FsErr     uses fs.write  // atomic, replaces a file
fn (f: Fs) remove(path: Str) -> () ! FsErr              uses fs.write  // file or empty dir
fn (f: Fs) remove_all(path: Str) -> () ! FsErr          uses fs.write
fn (f: Fs) create_dir_all(dir: Str) -> () ! FsErr       uses fs.write
fn create_dir_all_mode(fs: Fs, dir: Str, mode: Int) -> () ! FsErr uses fs  // made ones get mode
fn (f: Fs) set_modified(path: Str, t: Time) -> () ! FsErr uses fs.write
```

`disk_usage(path)` (decision 0426; Go's `syscall.Statfs`, and
`GetDiskFreeSpaceEx` on Windows) is the space of the file system that
holds `path`, a file or a directory resolved inside the Fs like `stat`:
`NotFound` when it is missing. Byte counts are the block counts times
the fragment size (`f_frsize`); Windows reports no inodes (`files` and
`files_free` are 0); WASI and embedded files (`embed.dir`) answer `Io`.
A test's MemFs answers what `set_disk_usage` set, all zero before.
Record and replay record it (`fs.disk_usage`).

`read_text_or` and `read_or` (approved by the owner, 2026-10-21) give
the file, or `default` when it does not exist (`FsErr.NotFound`, Go's
`errors.Is(err, fs.ErrNotExist)`). Any other error is still the result
(`IsDir`, `Denied` under `sub` or `only`), so a typo in a path to an
unreadable file is not silently a default. They are free functions, not
methods of `Fs`: an `Fs` is any type with its methods (core 2.4), and a
new interface method would stop every such type from satisfying it. They
call `read_text` and `read`, so MemFs, FakeFs and attenuated capabilities
answer as they do for those.

`read_mapped` (decision 0402) returns the file's bytes like `read`, with
the same errors. On the native darwin and linux backends it may share
the file's pages instead of copying them (mmap, read only and private):
only the pages the program touches are read. The program must not
truncate the file or change it in place while the `Bytes` (or a slice
of it) is alive: a truncation makes a later read of the cut pages fault,
and a change in place shows through. Replacing the file by rename is
safe (the mapping keeps the old file). Where mapping is not possible
(wasm, the Go backend, Windows, an empty file or one of 4 GiB or more, a
pipe or device, mmap failing, a user value converted to `Fs`, any `Fs`
in a test) it is `read`. Use `read` for a file other processes may
change in place. A free function, as `read_or`.

`create_dir_all_mode` (std survey gap 16, Go's `os.MkdirAll(dir, perm)`)
is `create_dir_all`, then `set_mode(d, mode)` on each directory it
created, deepest first, so a mode without write or search permission
(0o500) still reaches the inner ones. Unlike Go the umask does not
apply: the mode is exact. A directory already there keeps its mode, as
in Go, and a file in the way is create_dir_all's error. A free function
over `exists`, `create_dir_all` and `set_mode`, so any `Fs` answers.

`write_atomic(fs, path, data)` replaces the file at path so that a
reader, or the file after a crash, holds the old contents or the new,
never a part: it writes a temporary file `.NAME.NNNNNN` in path's
directory, syncs it, gives it path's permission bits (0o644 for a new
file), renames it over path, and syncs the directory where the OS allows
it (not Windows). A failure removes the temporary file. Atomic only
within one file system, which the temporary file beside path ensures.
`File.sync` commits an open file's writes to stable storage (Go's
File.Sync); a MemFs file has nothing to sync.

`walk_dir` (std survey gap 16, Go's `filepath.WalkDir`) calls `visit`
on each entry under `dir` in `walk`'s order, without following symbolic
links. `visit` returns `Continue`, `SkipDir` (a directory's entries are
not visited; on a file, the rest of the file's directory is skipped, as
Go's `SkipDir`) or `Stop` (Go's `SkipAll`). The result is every entry
`visit` was called on, in order, since a closure cannot add to a
captured list (core 3.7): on a huge tree, return `SkipDir` or `Stop`, or
do the work in `visit` and drop the result. Two differences from Go:
`dir` itself is not visited (as `walk`), and the first error (a
directory that cannot be read) ends the walk with that error, where Go
passes it to the function. A free function, so any `Fs` answers.

`DirEntry` (decision 0483, Go's `fs.DirEntry`) is an entry with its
`name`, `path`, `is_dir` and `is_symlink` from the directory entry itself
and no stat. `read_dir_entries(fs, dir)` is `read_dir`, `walk_entries(fs,
dir)` is `walk`, and `walk_dir_entries(fs, dir, visit)` is `walk_dir`,
each returning `DirEntry` in the same order; `entry.info(fs)` is the stat
now (an lstat, Go's `DirEntry.Info`) and fails with NotFound when the entry
was removed since the listing. Where the file system reports no type
(`DT_UNKNOWN`) the entry is stat'ed when listed, as in Go; a user `Fs` and
a MemFs answer through `read_dir`. Free functions, so any `Fs` answers.

Handles, links, globs, and locks (decision 0163; the Go backend and
MemFs, and native builds on the real filesystem, decision 0172; a native
test binary's MemFs has all of them, as on the Go backend, since
2026-10-15):

```
fn (f: Fs) open(path: Str) -> File ! FsErr        uses fs.read   // read as a stream
fn (f: Fs) create(path: Str) -> File ! FsErr      uses fs.write  // write; created or truncated
fn (f: Fs) open_append(path: Str) -> File ! FsErr uses fs.write  // write at the end; created if missing
fn (f: Fs) open_rw(path: Str) -> File ! FsErr     uses fs.write  // read and write; created if missing, not truncated (Go's O_RDWR|O_CREATE)
fn (f: Fs) create_new(path: Str) -> File ! FsErr  uses fs.write  // write a new file: Exists(path) if anything is there (O_EXCL; a symbolic link counts)
fn (f: Fs) append(path: Str, data: Bytes) -> () ! FsErr uses fs.write
fn (f: Fs) copy(from: Str, to: Str) -> () ! FsErr uses fs        // contents and permission bits
fn (f: Fs) mode(path: Str) -> Int ! FsErr         uses fs.read   // permission bits: 0o644
fn (f: Fs) set_mode(path: Str, mode: Int) -> () ! FsErr uses fs.write
fn (f: Fs) symlink(target: Str, link: Str) -> () ! FsErr uses fs.write
fn (f: Fs) read_link(path: Str) -> Str? ! FsErr   uses fs.read   // None when not a link
fn (f: Fs) glob(pattern: Str) -> List[Str] ! FsErr uses fs.read
fn (f: Fs) lock(path: Str) -> FileLock ! FsErr    uses fs.write  // waits; a cancellation point
fn (f: Fs) try_lock(path: Str) -> FileLock? ! FsErr uses fs.write

struct File {}                                    // handle
fn (f: File) read(max: Int) -> Bytes? ! FsErr     uses fs.read   // None at the end
fn (f: File) read_line() -> Str? ! FsErr          uses fs.read   // without \n or \r\n
fn (f: File) read_at(offset: Int, max: Int) -> Bytes? ! FsErr uses fs.read   // decision 0479: Go's ReadAt once; position untouched; None at the end
fn (f: File) write(data: Bytes) -> () ! FsErr     uses fs.write
fn (f: File) write_at(offset: Int, data: Bytes) -> () ! FsErr uses fs.write   // Go's WriteAt: every byte, position untouched, a gap reads as zeros; an open_append file is an error
fn (f: File) truncate(size: Int) -> () ! FsErr    uses fs.write   // Go's Truncate: cut or zero padded, position unchanged
fn (f: File) stat() -> FileInfo ! FsErr           uses fs.read    // Go's File.Stat: from the handle
fn (f: File) sync() -> () ! FsErr                 uses fs.write   // fsync (F_FULLFSYNC on macOS)
fn (f: File) close() -> () ! FsErr                               // closing again does nothing
struct FileLock {}                                // handle
fn (l: FileLock) unlock()                                        // unlocking again does nothing
```

```
fn count_errors(fs: Fs, path: Str) -> Int ! FsErr uses fs.read {
  let f = fs.open(path)?
  defer { f.close().ignore_err("read only") }
  var n = 0
  while let Some(line) = f.read_line()? {
    if line.contains("ERROR") {
      n += 1
    }
  }
  n
}
```

- A `File` is opened for reading (`open`) or writing (`create`,
  `open_append`); the other kind of operation is `Io(path, "file is open
  for reading")` (or writing). `read` returns at most `max` bytes (at
  most 1 MiB), `None` at the end. `open_rw` opens for both: its reads are
  not buffered, so `read`, `read_line` and `write` share one position as
  Go's `os.File` (a wasm build buffers reads, so there a read after a
  write on one handle should be a `read_at`); on MemFs its reads see the
  file's current contents. A custom `Fs` implements `open_rw` too.
  Operations after `close` are `Io(path,
  "file already closed")`. Writes are not buffered by Xo: each `write`
  reaches the OS. Handles are not recorded by record and replay yet.
- `open` of a directory is `IsDir`. `create` and `open_append` do not
  create missing parent directories on the real filesystem (MemFs does,
  as its `write` does).
- `read_dir` and `walk` fail with the error of a failed directory read
  (never a list cut short) and of an entry whose stat fails (`Denied`
  for an entry of a directory without search permission), naming that
  entry, as Go's os.File.Readdir and filepath.WalkDir; an entry removed
  between the listing and its stat is left out. `walk` lists each
  directory in name order and stops at the first error.
- `copy` reads `from` and writes `to` (created or truncated) with
  `from`'s permission bits; a directory is `IsDir(from)`.
- `mode` and `set_mode` are the permission bits (`0o777` mask). Windows
  reports and sets what it can (read only or not).
- On WebAssembly (WASI preview1), which has no permission bits or
  advisory locks, `mode`, `set_mode`, and `lock` are `Io(path,
  "operation not supported (WASI has no file modes or locks)")` (after
  `NotFound` and the other errors of the path itself), and
  `set_modified` before 1970 is `Io(path, "WASI has no times before
  1970")`. `case_sensitive` is probed once in the working directory: an
  entry whose name has a letter, looked up with its letters' case
  flipped (found as the same file: not case sensitive; not found: case
  sensitive), else a probe file created and removed; `true` when neither
  is possible.
- `symlink(target, link)` stores `target` as written; a relative target
  is relative to the link's directory. In a sub capability a target that
  is absolute or leaves the root (`..` past it) is `Denied(link)`: a sub
  capability is a lexical boundary, and links created through it must
  not escape it. Links that already exist are followed by the OS as
  before (the boundary is lexical, decision 0024).
- `glob(pattern)` lists the paths matching `pattern`, sorted, relative to
  the capability root as the pattern is written: `*` (any run of
  characters but `/`), `?`, `[abc]`, `[a-z]`, `[^a]` in a segment, and a
  segment `**` for any number of directories (zero included). Names that
  start with `.` match only a segment that starts with `.`. A missing
  directory matches nothing; a malformed pattern is `Io(pattern, "bad
  pattern: ...")`. It works through `read_dir`, so attenuation applies.
- `lock(path)` takes an exclusive advisory lock on `path` (created if
  missing), waiting until no other holder has it; waiting is a
  cancellation point (`Canceled`). `try_lock` returns `None` at once when
  the lock is held. The lock is the OS's (`flock` on Unix, `LockFileEx`
  on Windows), so it excludes other processes that lock the same file,
  and other `lock` calls in the same program. It is advisory: reads and
  writes are not blocked. A program that exits releases its locks.
- A `FileLock` dropped while held is released: natively at the drop, on
  the Go backend when a `lock` or `try_lock` of a lock that is held runs
  the collector, which finds the dropped handle (within a poll, 10 ms).
  So locking a path again after dropping its lock works on every backend
  (golden lock_drop_relock). A `File` dropped open is closed at the drop
  natively and at a collection on the Go backend. `unlock()` and
  `close()` (or `defer`) release at once everywhere and say what is
  meant (gotchas/go-backend-dropped-lock-held.md).

### 3.2 Net (`uses net.dial` / `net.listen`)

```
enum NetErr { InvalidAddr(msg: Str), Refused, Timeout, Io(msg: Str), Canceled, Denied(addr: Str) }

`display` prints `invalid address: MSG`, `connection refused`, `i/o timeout`,
MSG for `Io`, `canceled`, and `denied by Net.only: ADDR` (decision 0358).

fn (n: Net) listen(addr: Str) -> Listener ! NetErr uses net.listen
fn (n: Net) dial(addr: Str) -> Conn ! NetErr       uses net.dial
fn (n: Net) dial_timeout(addr: Str, d: Duration) -> Conn ! NetErr uses net.dial   // decision 0406
fn (n: Net) lookup_host(host: Str) -> List[Str] ! NetErr uses net.dial            // decision 0406
fn (n: Net) interfaces() -> List[NetInterface] ! NetErr uses net.listen           // decision 0426
fn (n: Net) only(hosts: List[Str]) -> Net          // attenuation (below)
fn (l: Listener) addr() -> Str                     // "host:port" actually bound
fn (l: Listener) accept() -> ServerConn ! NetErr   uses net.listen
fn (l: Listener) close()
fn (c: Conn) read(max: Int) -> Bytes? ! NetErr     uses net.dial    // None at the end of the stream
fn (c: Conn) write(data: Bytes) -> () ! NetErr     uses net.dial    // writes every byte
fn (c: Conn) close()
fn (c: ServerConn) read(max: Int) -> Bytes? ! NetErr uses net.listen   // the same, on an accepted connection
fn (c: ServerConn) write(data: Bytes) -> () ! NetErr uses net.listen
fn (c: ServerConn) close()
fn (c: Conn) set_deadline(clock: Clock, t: Time?) uses clock         // decision 0430: Go's SetDeadline
fn (c: Conn) set_read_deadline(clock: Clock, t: Time?) uses clock    // Go's SetReadDeadline
fn (c: Conn) set_write_deadline(clock: Clock, t: Time?) uses clock   // Go's SetWriteDeadline
// ServerConn has the same three

interface Stream {                                 // either kind of connection
  fn read(self, max: Int) -> Bytes? ! NetErr uses net
  fn write(self, data: Bytes) -> () ! NetErr uses net
  fn close(self)
}
interface NetConn {                                // decision 0481: Go's net.Conn
  fn read(self, max: Int) -> Bytes? ! NetErr uses net
  fn write(self, data: Bytes) -> () ! NetErr uses net
  fn close(self)
  fn set_deadline(self, clock: Clock, t: Time?) uses clock
  fn set_read_deadline(self, clock: Clock, t: Time?) uses clock
  fn set_write_deadline(self, clock: Clock, t: Time?) uses clock
}
```

`NetConn` (decision 0481) is a connection with deadlines: `Conn`,
`ServerConn` and `tls.TlsConn` satisfy it, and every NetConn is also a
Stream. Code that talks over plain TCP or TLS chosen at run time writes
the type on the join, `let c: NetConn = if tls { tls_conn } else { conn
}` (core 2.10; without it the arms are `XO0401`), then sets deadlines
through `c`. `Stream` keeps its three methods, so user types that
implement it are unaffected.

```
struct NetInterface {
  index: Int                  // the system's index, 1 and up
  name: Str                   // "lo", "eth0", "en0"; Windows: the friendly name
  mtu: Int                    // -1 when Windows reports none
  hardware_addr: Str          // "aa:bb:cc:dd:ee:ff"; "" when none
  flags: InterfaceFlags
  addrs: List[Str]            // CIDR text: "192.168.1.5/24", "fe80::1/64"
}
struct InterfaceFlags { up: Bool, broadcast: Bool, loopback: Bool, point_to_point: Bool, multicast: Bool, running: Bool }
```

`interfaces()` (decision 0426) is Go's `net.Interfaces` with each
interface's `Addrs`, sorted by index: Linux asks the kernel over a
`NETLINK_ROUTE` socket, macOS reads `getifaddrs`, Windows
`GetAdaptersAddresses` (flags as Go's: an adapter that is up is also
running; Ethernet and Wi-Fi broadcast; PPP and tunnels are point to
point). An address is CIDR text without a zone (Go's `IPNet.String`);
`std/netip` parses it. A hardware address of all zeros is "". An
attenuated `Net` (`only`) answers it too: the list names no remote host.
A test's FakeNet answers one loopback interface (`lo`, index 1,
`127.0.0.1/8` and `::1/128`); WASI is `Io`. Record and replay record it
(`net.interfaces`).

A connection's reads and writes perform the effect of how it was made
(decision 0280): a `Conn` from `dial` is `net.dial`, a `ServerConn` from
`accept` is `net.listen`. So a client library declares `uses net.dial`
alone though it reads and writes, and a function given only a
`ServerConn` cannot dial. Both are handles with the same behavior;
`Stream` takes either, under `net`.

`dial` of a name with IPv6 and IPv4 addresses (decision 0462) races them
as Go's dual stack dial does: the addresses of the first one's family in
order, and, 300 ms after the first started or once those failed, the
other family's beside them; the first connection wins and the other
attempt is dropped. An error is the first family's when it got one. A
name of one family dials its addresses one at a time, as before.

TCP connections (decision 0071): `accept` waits for the next connection;
`read` returns at most `max` bytes (at most 1 MiB; fewer when fewer are
there) and `None` at the end of the stream; `write` returns when every
byte is written. All three are cancellation points that report
cancellation as `Canceled` (core 7.2). `close` is idempotent and makes a
waiting `accept`, `read`, or `write` on the same listener or connection
fail with `Io`. A connection or listener dropped open (its last use
passed, core 5.5) is closed at the drop natively, so its peer reads the
end of the stream; on the Go backend only when it is collected (a
FakeNet one never). Keep the value used, or `close` it, while the peer
should see it open. A refused `dial` is `Refused`. `dial` gives up after 30
seconds with `Io` (`dial tcp ADDR: i/o timeout`); `dial_timeout(addr, d)`
gives up after `d`, over the lookup and the connect, with `Timeout`
(decision 0406).

Deadlines (decision 0430) are Go's net.Conn deadlines.
- `set_deadline(clock, t)` sets both directions; `set_read_deadline` and
  `set_write_deadline` set one.
- A read or write that would wait past `t` fails with `Timeout`
  (`i/o timeout`), and so does every later one until the deadline moves.
  One whose deadline has already passed fails at once, even with data
  waiting.
- The connection stays open: a later `t`, or `None` (no deadline), lets
  operations run again. A deadline set while a read or write waits
  applies to that wait.
- `t` is on `clock`, and the wait is measured from now.
- A write that times out may have written part of its data.
- Deadlines never cancel: cancellation stays `Canceled` (core 7.2), and
  a `task.with_timeout` around the call still cancels it.
- On FakeNet's in-memory connections, a deadline already passed fails at
  once, and a waiting read or write is woken by its deadline, on every
  backend: the test's virtual clock moves to it (native reads since
  2026-11-05; Go backend reads and writes and native writes since
  2026-11-07, which before deadlocked with "no timer is pending" or never
  woke).
- They are on the concrete types, not on `Stream` (decision 0430).

Names (decision 0406): `lookup_host(host)` returns host's addresses as
text, in the order `dial` tries them, from the resolver `dial` uses (the
system's getaddrinfo on macOS and Windows, Xo's own reading /etc/hosts
and resolv.conf on Linux); an IP literal returns itself and a name no
source knows is `Io` (`lookup NAME: no such host`, Go's text). An
attenuated `Net` refuses a host it does not admit (`Denied`). FakeNet
answers `localhost` and IP literals. `netip.lookup(net, host)` (section
28) is the typed form.

`addr` reports the port the system chose for `listen(":0")`. An unspecified
host (`:0`, `0.0.0.0`, `[::]`) is reported as the loopback address
(`127.0.0.1`, `::1`), so the result can be dialed locally:
`"http://${l.addr()}/health"`. The fake network of tests gives
`127.0.0.1:49152`, `127.0.0.1:49153`, ... for `:0`. Under record and
replay (core 11.3) the recorded address is returned (decision 0055).

### 3.2a std/udp (decision 0406)

```
struct UdpConn {}
pub fn listen(n: Net, addr: Str) -> UdpConn ! NetErr uses net.listen   // Go's net.ListenPacket("udp", addr)
pub fn dial(n: Net, addr: Str) -> UdpConn ! NetErr uses net.dial       // Go's net.DialUDP
fn (c: UdpConn) read_from(max: Int = 65535) -> (Bytes, Str) ! NetErr uses net   // a datagram and its sender
fn (c: UdpConn) write_to(data: Bytes, addr: Str) -> () ! NetErr uses net
fn (c: UdpConn) read(max: Int = 65535) -> Bytes ! NetErr uses net      // a dialed socket's peer
fn (c: UdpConn) write(data: Bytes) -> () ! NetErr uses net
fn (c: UdpConn) local_addr() -> Str
fn (c: UdpConn) close()
```

One call sends or receives one datagram. A datagram longer than
`read_from`'s `max` is cut to `max` and the rest discarded, as Go's
ReadFrom. `read` and `write` need a socket from `dial` (`Io("read udp:
not connected")`, `Io("write udp: destination address required")`
otherwise). An attenuated `Net` checks `dial`'s address and each
`write_to` destination (`Denied`). `local_addr` reports the port the
system chose for `:0` (an unspecified host as the loopback address, as
`Listener.addr`). The reads and writes are cancellation points
(`Canceled`); `close` makes a waiting read fail with `Io`. The fake
network of tests has no UDP yet (`Io`; run with `xo test --real`). A
UdpConn is a std module's handle rather than the prelude's because a
prelude handle is a compiler list (decision 0406).

**Unix domain sockets** (decision 0190): an address `unix:PATH` listens
on or dials the socket file PATH (`os.net.listen("unix:/tmp/app.sock")`).
`addr` returns the address as given; closing the listener removes the
socket file (once), as Go's does; a listen on an existing file fails
with `Io` (`listen unix PATH: bind: address already in use`); a dial of
a missing file is `Io` (`dial unix PATH: connect: no such file or
directory`) and of a file nobody listens on is `Refused`. In an
attenuated `Net` an entry `unix:PATH` admits exactly that address (the
path keeps its case); host entries never admit a socket and `unix:`
entries never admit a host. FakeNet listens on them in memory. Used by `xo serve`.

Most code uses `std/http` instead of raw connections.

**Attenuation** (decision 0156): `n.only(hosts)` is a `Net` whose `dial`
and `listen` succeed only for an address whose host matches an entry:
an exact host or IP (`api.example.com`, `127.0.0.1`, compared without
case), `*.example.com` (any subdomain, not `example.com` itself), or
`host:port` (that host and port only). A listen address without a host
(`:8080`) has the host `0.0.0.0`. Any other address fails with
`Denied(addr)` before a name is looked up. An HTTP client made from it
(`http.client(n)`, 4.5) checks each request's host and port (80 or 443
by scheme when the URL has none), and each redirect, failing with
`ClientErr.Connect("denied by Net.only: <host:port>")`; `sql.open` with
an attenuated `Net` fails with `SqlErr.Conn` (the driver dials by
itself, so the list cannot be enforced). `only` on an attenuated `Net`
keeps both lists: an address must pass each. Names are checked as
written, not as resolved: an allowed name that resolves to another
address is allowed. Native builds (`--no-gc`) have it too (decision
0175): a `Net` there is the lists of its attenuations; hosts in dial and
listen addresses and in redirects are lowered ASCII only.

```
let api = os.net.only(["api.example.com"])
let weather = weather.client(api)   // the library can reach api.example.com only
```

### 3.3 Clock (`uses clock`)

```
fn (c: Clock) now() -> Time
enum ClockErr { Canceled }

`display` prints `canceled` (decision 0358).

fn (c: Clock) sleep(d: Duration)             // cancellation point; unwinds when cancelled
fn (c: Clock) sleep_or_cancel(d: Duration) -> () ! ClockErr
    // like sleep, but cancellation is Err(Canceled) instead of an unwind
fn (c: Clock) since(t: Time) -> Duration
```

`sleep_or_cancel` lets a function return cancellation as its own error
(core 7.2, decision 0088):

```
fn backoff(clock: Clock, d: Duration) -> () ! FetchErr uses clock {
  match clock.sleep_or_cancel(d) {
    Ok(_) => Ok(())
    Err(Canceled) => Err(FetchErr.Cancelled)
  }
}
```

### 3.4 Rand (`uses rand`)

Cryptographically secure for real capabilities, seeded and deterministic in
tests.

```
fn (r: Rand) int(lo: Int, hi: Int) -> Int    // in [lo, hi)
fn (r: Rand) float() -> Float                // in [0, 1)
fn (r: Rand) bool() -> Bool
fn (r: Rand) bytes(n: Int) -> Bytes
fn (r: Rand) shuffle[T](xs: List[T]) -> List[T]
```

Native builds (`--no-gc`, decision 0083) draw from the operating system's
generator (`getentropy`, `getrandom`); `shuffle` draws one `int` per
step as on the Go backend, so a test's seeded shuffle is the same
(decision 0107). `shuffle` is derived from `int` on every backend, for a
program's own Rand too: Fisher-Yates from the end with that value's
`int`; a `shuffle` method the type declares is not called (decision
0307).

### 3.5 Env (`uses env`)

```
enum EnvErr { Missing(name: Str) }

`display` prints `NAME is not set` (decision 0358).

fn (e: Env) get(name: Str) -> Str?
fn (e: Env) vars() -> Map[Str, Str]
fn (e: Env) args() -> List[Str]              // without the program name
fn (e: Env) program() -> Str
fn (e: Env) home() -> Str ! EnvErr
fn (e: Env) only(names: List[Str]) -> Env   // attenuation (below)
fn (e: Env) config_dir() -> Str ! EnvErr     // where configuration goes (below)
fn (e: Env) cache_dir() -> Str ! EnvErr
fn (e: Env) hostname() -> Str              // Go's os.Hostname; "" where the system cannot say (WASI)
```

`program()` is the name the program was started with (argv[0]), and
`home()` the home directory (`HOME`; on Windows `USERPROFILE`, read when
the `Env` admits `HOME`); on Windows both use `/` separators (3.1).

`config_dir()` and `cache_dir()` (decision 0163) are read through the
`Env`, so attenuation and fakes apply: `XDG_CONFIG_HOME` (or
`XDG_CACHE_HOME`) when it holds an absolute path (`/x`, or `C:/x`), on
every OS; else
`<home>/.config` (`<home>/.cache`) on Linux and other Unix systems,
`<home>/Library/Application Support` (`<home>/Library/Caches`) on macOS,
`%AppData%` (`%LocalAppData%`) on Windows, with `/` separators. A
missing home is `Err(Missing("HOME"))` (as `home()`), a missing Windows
variable `Err(Missing("AppData"))`. The program adds its own directory:
`os.fs.sub(os.env.config_dir()? + "/mytool")`. Every backend (the
native ones read `HOME` for the home directory).

`e.only(names)` (decision 0156) is an `Env` that reads only the listed
variables: `get` of another name is `None`, `vars` holds only listed
names, and `home` is `Err(Missing("HOME"))` unless `HOME` is listed;
`args` and `program` are unchanged. Attenuating again keeps both lists.
On Windows names match without ASCII case, as its environment compares
them (`only(["PATH"])` admits `Path`). Native builds have it too
(decision 0172).

### 3.6 Stdio (`uses stdio`)

```
enum IoErr { Closed, Io(msg: Str), Canceled }

`display` prints `closed`, MSG for `Io`, and `canceled` (decision 0358).

fn (s: Stdio) print(text: Str)               // write errors are ignored
fn (s: Stdio) println(text: Str)
fn (s: Stdio) eprint(text: Str)
fn (s: Stdio) eprintln(text: Str)
fn (s: Stdio) write(data: Bytes) -> () ! IoErr
fn (s: Stdio) read_line() -> Str? ! IoErr    // None at end of input
fn (s: Stdio) read_all() -> Str ! IoErr
fn (s: Stdio) lines() -> List[Str] ! IoErr   // reads all input

// terminals and binary input (decision 0162)
fn (s: Stdio) stdin_is_tty() -> Bool         // also stdout_is_tty, stderr_is_tty
fn (s: Stdio) width() -> Int?                // columns of the terminal; None without one
fn (s: Stdio) read_secret(prompt: Str) -> Str? ! IoErr   // a line without echo (passwords)
fn (s: Stdio) read_bytes(max: Int) -> Bytes? ! IoErr     // any bytes; None at end of input
fn (s: Stdio) read_all_bytes() -> Bytes ! IoErr
```

The reads of standard input are cancellation points while they wait: a
cancelled task, and every reader once a stop is requested (core 7.5), gets
`Err(IoErr.Canceled)` at once. Input read after a reader gave up is kept
for the next read (decision 0088). The native backends read standard
input the same way (decision 0102); a test binary's standard input is
empty there.

Terminals (decision 0162), on every backend:

- `stdin_is_tty`, `stdout_is_tty`, `stderr_is_tty` tell whether the
  stream is a terminal (a terminal's settings can be read: termios on
  Unix, a console mode on Windows); a pipe, a file, or `/dev/null` is not.
- `width()` is the columns of the terminal on standard output, else
  standard error, else standard input; `None` when none is a terminal.
  `COLUMNS` is not read (it is a variable: read it through `Env`).
- `read_secret(prompt)` writes `prompt` to standard error, turns the
  terminal's echo off while it reads a line of standard input (as
  `read_line`), restores it (also when the read is cancelled), and then
  writes the newline the user typed, which was not echoed. When standard
  input is not a terminal it reads a line without touching anything.
- `read_bytes(max)` returns at most `max` bytes (at most 1 MiB) of
  standard input as they arrive, any bytes, `None` at the end;
  `read_all_bytes()` the rest of the input. They share the input buffer
  of `read_line` and `read_all`, so text and binary reads can be mixed.
- Targets without terminal support (wasip1, js, plan9, Linux on mips and
  ppc) report no terminal and read secrets with echo on.

### 3.7 Proc (`uses proc`)

```
struct ProcOutput { code: Int, stdout: Bytes, stderr: Bytes, usage: ProcUsage = ProcUsage{user: 0s, system: 0s, max_rss: 0} }
enum ProcErr { NotFound(cmd: Str), Io(msg: Str), Canceled, Denied(cmd: Str) }

`display` prints `CMD: executable file not found`, MSG for `Io`, `canceled`,
and `denied by Proc.only: CMD` (decision 0358).

fn (p: Proc) exit(code: Int) -> Never        // exits immediately; defer blocks do not run
fn (p: Proc) set_grace(d: Duration)          // time between stop requested and cancel (default 30s)
fn (p: Proc) run(cmd: Str, args: List[Str]) -> ProcOutput ! ProcErr
fn (p: Proc) only(cmds: List[Str]) -> Proc   // attenuation (below)
fn (p: Proc) cmd(name: Str, args: List[Str] = []) -> Cmd       // a command, not started (3.8)
fn (p: Proc) pipeline(cmds: List[Cmd]) -> ProcOutput ! ProcErr // (3.8)
fn (p: Proc) kill(pid: Int, signal: Str = "TERM") -> () ! ProcErr  // signals another process
fn (p: Proc) notify(signals: List[Signal]) -> Chan[Signal]    // decision 0407
fn (p: Proc) stop_notify(c: Chan[Signal])
fn (p: Proc) request_stop()
fn (p: Proc) pid() -> Int                    // this process's ID (0 under WASI); decision 0410
fn (p: Proc) ppid() -> Int                   // the parent's ID (0 under WASI)
fn (p: Proc) usage() -> ProcUsage            // this process's CPU time and peak memory; decision 0426
fn (p: Proc) children_usage() -> ProcUsage   // the waited children's

struct ProcUsage { user: Duration, system: Duration, max_rss: Int }
enum Signal { Hangup, Interrupt, Quit, Terminate, User1, User2, WindowChange }
```

**Signals** (decision 0407, Go's `os/signal`): `notify(signals)` returns a
channel that receives each listed signal as it arrives; it holds one
pending signal of each listed kind and drops a repeat while one is
pending, as Go drops a send to a full channel. Several subscriptions to
one signal each receive it. While any subscription lists `Interrupt`
(or `Terminate`), that signal no longer starts the shutdown of core 7.5:
the program decides, and other signals keep their default (a `Hangup`
subscription leaves Ctrl-C alone). A subscribed `Quit` replaces the task
dump. `stop_notify(c)` ends c's subscription, closes c, and gives each
signal with no subscription left its default back (Hangup, User1 and
User2 end the program then, as they do unsubscribed). `request_stop()`
starts core 7.5's phase 1 as the first signal does, so a program that
took over Interrupt shuts down the same way (on wasm, which has no
signals, it is the only way to start it; decision 0447 R2j):

```xo
let sigs = os.proc.notify([Signal.Interrupt, Signal.Terminate, Signal.Hangup])
loop: while true {
  match sigs.recv() {
    Some(Signal.Hangup) => reload(os)
    Some(_) => {
      log.info("shutting down")
      os.proc.request_stop()       // servers stop accepting and drain
      break loop
    }
    None => break loop
  }
}
```

On Windows Ctrl-C and Ctrl-Break are `Interrupt`, and the console's close,
a logoff and a system shutdown are `Terminate` (Go's mapping); the other
kinds are accepted and never arrive. A test's FakeProc gives a channel
that receives nothing.

`usage()` (decision 0426; Go's `syscall.Getrusage(RUSAGE_SELF)`) is
this process's CPU time in user mode and in the kernel and its peak
resident set in bytes on every system (Linux reports KiB, converted);
`children_usage()` is the same for the children the program has waited
for (`run`, `pipeline`, `Child.wait`; `RUSAGE_CHILDREN`), the largest
child's peak as `max_rss`. On Windows `usage` reads `GetProcessTimes` and
the peak working set, and the runtime adds each waited child's times
and peak to the children's totals, which Windows does not keep (the Go
backend's totals have the times only: `max_rss` 0). WASI answers zeros.
An attenuated `Proc` answers as the real one: it is the process's own
state, as `pid`. A test's FakeProc answers with the real process's.

A child's own usage (decision 0426's addendum; Go's
`ProcessState.UserTime`, `SystemTime` and `SysUsage`) is
`ProcOutput.usage` of `run`, `pipeline` and `Cmd.output`, and
`Child.wait_usage()`'s: its CPU time in user mode and in the kernel and
its peak resident set in bytes, read as it is waited for (`wait4`'s
rusage; on Windows `GetProcessTimes` and the peak working set of its
handle). A pipeline's adds its commands' times and keeps the largest
peak. Unlike `children_usage`, whose `max_rss` is the largest of every
child waited for, each child's is its own. A ProcOutput built as a
literal, a FakeProc's output, and WASI have zeros; the Go backend's
`max_rss` is 0 on Windows (os/exec closes the handle first).

`kill` (decision 0190) sends the named signal to the process `pid`: one
of `HUP`, `INT`, `QUIT`, `KILL`, `TERM`, `USR1`, `USR2`, `CONT`, `STOP`
(the names of `kill -l` without `SIG`; another name is
`Io("unknown signal \"NAME\"")`). A `pid` of 0 or less (a process group
for kill(2)) is `Io("invalid process id N")` and nothing is sent. A
failure is `Io` with the system's text (`no such process`, `operation
not permitted`). An attenuated `Proc` sends signals only when every list
names `"kill"` (`os.proc.only(["kill"])`), else `Denied("kill")`. Record
and replay record it (`proc.kill`). Where the system has no signals
(Windows), only `KILL` is sent: it ends the process with the status
137 (128 plus SIGKILL's number, as on Unix), and another name is
`Io("signal NAME is not supported on this system")`. Used by `xo dump` (core 11.4) to ask a
running program for its task tree.

`p.only(cmds)` (decision 0156) is a `Proc` whose `run` fails with
`Denied(cmd)` for a command not listed (compared as written: `git` and
`/usr/bin/git` are different entries); `exit` and `set_grace` are
unchanged. A `Cmd` made by an attenuated `Proc` keeps its lists (3.8).
Native builds have it too (decision 0172).

The native backends run commands with `posix_spawn` (on Windows
`CreateProcessW`, decision 0197), reading the output on the event loop;
cancelling the task kills the command (decision 0107). On Windows a
command name without `\`, `/`, or `:` is looked up in `PATH` (`;`
separated) with the extensions of `PATHEXT`, as Go's exec.LookPath; the
arguments are quoted for the command line as Go's syscall.EscapeArg
does, and also when they hold a line end or another space character
(the MSYS2 and Cygwin runtimes split there).

Returning `Err` from `main` prints the error with its frames to standard error
and exits with status 1. Prefer that over `exit` for failures.

### 3.8 Commands, children, and pipelines

Status: implemented on the Go backend and its fakes (decision 0161,
2026-10-11) and natively (decision 0172, 2026-10-11: `posix_spawn` in
the C runtime, pipes on the event loop; FakeProc in native test
binaries).

```
struct Cmd {}                                 // a command that has not run; a value
fn (c: Cmd) dir(path: Str) -> Cmd             // working directory (relative: to the program's)
fn (c: Cmd) env(e: Env) -> Cmd uses env       // the child's environment is exactly e.vars()
fn (c: Cmd) set_env(name: Str, value: Str) -> Cmd
fn (c: Cmd) stdin(data: Bytes) -> Cmd         // its standard input
fn (c: Cmd) inherit() -> Cmd                  // standard streams are the program's (the terminal)
fn (c: Cmd) merge_stderr() -> Cmd             // standard error into the standard output stream
fn (c: Cmd) output() -> ProcOutput ! ProcErr uses proc   // run to the end, collect the streams
fn (c: Cmd) run() -> Int ! ProcErr uses proc            // run on the terminal; the exit status
fn (c: Cmd) spawn() -> Child ! ProcErr uses proc        // start; returns at once

struct Child {}                               // a started command; a handle
fn (c: Child) pid() -> Int
fn (c: Child) write(data: Bytes) -> () ! ProcErr uses proc        // to its standard input
fn (c: Child) close_stdin()
fn (c: Child) read(max: Int) -> Bytes? ! ProcErr uses proc        // its output; None at the end
fn (c: Child) read_err(max: Int) -> Bytes? ! ProcErr uses proc    // its error stream
fn (c: Child) read_line() -> Str? ! ProcErr uses proc             // a line of its output
fn (c: Child) wait() -> Int ! ProcErr uses proc                   // closes its input; the status
fn (c: Child) wait_usage() -> (Int, ProcUsage) ! ProcErr uses proc // wait, and the child's own usage
fn (c: Child) kill()
```

```
fn changed_files(p: Proc, repo: Str) -> List[Str] ! ProcErr uses proc {
  let out = p.cmd("git", ["status", "--porcelain"]).dir(repo).output()?
  (out.stdout.to_str() ?? "").lines().filter(fn(l) { !l.is_empty() })
}

fn edit(p: Proc, env: Env, path: Str) -> Int ! ProcErr uses (proc, env) {
  let editor = env.get("EDITOR") ?? "vi"
  p.cmd(editor, [path]).run()?   // the editor gets the terminal
}

fn follow(p: Proc, out: Stdio) -> () ! ProcErr uses (proc, stdio) {
  let child = p.cmd("tail", ["-f", "app.log"]).spawn()?
  while let Some(line) = child.read_line()? {
    out.println("log: ${line}")
  }
}
```

- `cmd` builds; nothing runs until `output`, `run`, `spawn`, or
  `pipeline`. The builder methods return a changed copy, so a `Cmd`
  can be prepared once and run several times.
- Environment: without `env`, the child inherits the program's
  environment (as `run`). `env(e)` replaces it with exactly the
  variables `e.vars()` shows when `env` is called, so an attenuated
  `Env` narrows it: `p.cmd("make").env(os.env.only(["PATH", "HOME"]))`.
  `set_env` adds or replaces one variable on top of either.
- Standard input: the `stdin` data, else the program's own with
  `inherit`, else empty for `output` and `run`'s terminal, and a pipe
  written with `Child.write` for `spawn` (closed by `close_stdin` or
  `wait`). Standard output and error: collected by `output`, the
  program's own with `run` or `inherit`, pipes read with `Child.read`
  and `read_err` for `spawn`. `merge_stderr` sends the error stream
  into the output stream (`output`'s `stderr` is then empty).
- Exit status: the child's exit code; a child ended by a signal is 128
  plus the signal number. On Windows `Child.kill` (and cancellation)
  ends the child with the status 137, as SIGKILL does on Unix.
- `read` returns at most `max` bytes (at most 1 MiB) as they arrive and
  `None` at the end of the stream; `read_line` strips `\n` and `\r\n`.
  Reads and `wait` are cancellation points; cancelling the task that
  spawned a child kills it. Reading a stream that is not a pipe
  (inherited or merged) is `Io`. A child that fills one pipe while the
  program waits on the other stops: read both from separate tasks, or
  merge them.
- `pipeline(cmds)` connects each command's standard output to the next
  one's input, runs them together, and returns the last one's output,
  the error streams of all of them in command order, and the status of
  the rightmost command that failed (bash's `pipefail`), else 0. The
  first command's `stdin` data feeds the pipeline; `merge_stderr` on a
  command sends its errors down the pipe. An empty list is an empty
  output.
- Attenuation (decision 0156): every command is checked against the
  lists of the `Proc` that made it before it starts (`Denied(name)`);
  for a pipeline, every command before any starts.
- Tests: `FakeProc.cmd` builds commands answered by `on_run(name, out)`,
  as `run` is (the standard input is ignored): `output` returns it,
  `run` writes its streams to the test's `FakeStdio` and returns its
  code, `spawn` gives a `Child` reading it (`pid` is 0, `write` and
  `kill` do nothing), and `pipeline` gives the last command's output
  with the error streams and failing statuses of all. A command with no
  `on_run` is `NotFound`.
- Record and replay (core 11.3) record `output`, `run`, and `pipeline`;
  a `Child`'s reads are not recorded yet.
- Native builds (decision 0172) differ in what cancellation does to a
  running command: the command is killed when the cancelled task waits
  on it (`output`, `run`, `pipeline`, `Child.wait`, `Child.read`), and
  that call is `Canceled` (the Go backend kills it at once and its
  `wait` gives the status 137). An attenuated `Proc` or `Env` debugs as
  `<proc>` or `<env>`, without its lists.

---

## 4. std/http

### 4.1 Routing

```
pub fn router() -> Router
fn ([var] r: Router) handle[E: Error, fx: Effects](pattern: Str, h: fn(Request) -> Response ! E uses fx) uses fx
fn ([var] r: Router) handle_stream[E: Error, fx: Effects](pattern: Str, h: fn(Request, BodyReader) -> Response ! E uses fx) uses fx
    // the body read as h asks (decision 0310); req.body is empty
fn ([var] r: Router) wrap[E: Error, fx: Effects](m: fn(Request, Next) -> Response ! E uses fx) uses fx
fn ([var] r: Router) mount(prefix: Str, sub: Router)   // sub's routes under prefix, behind sub's middleware
fn ([var] r: Router) not_found[E: Error, fx: Effects](h: fn(Request) -> Response ! E uses fx) uses fx            // the 404's response
fn ([var] r: Router) method_not_allowed[E: Error, fx: Effects](h: fn(Request) -> Response ! E uses fx) uses fx   // the 405's; Allow added when missing
fn (r: Router) call(req: Request) -> Response      // in process dispatch, for tests
fn serve_component(r: Router)                      // answers a wasi:http component's request; on the wasm backend the build's output is that component (decision 0447); faults elsewhere
    // a wasi:http component's Client (decision 0447): send and open go through the host; the whole request and response are held, so a request
    // blocks the instance, a piped body is read first, trailers are refused, and with_roots and with_cert fail because the host does TLS
    // in a component a handle_stream route reads the request body from the host as it arrives, and BodyReader.trailers has the
    // request's declared trailers; http.stream's body goes to the host as it is written (the head with the first write or flush,
    // its Content-Type detected from those bytes when missing), and its set_trailer trailers follow it
type Next = fn(Request) -> Response
```

- Registering a handler or middleware counts as performing its effects, so the
  function that builds the router declares them. This is where capabilities
  are captured.
- Pattern: `"[METHOD ]/path"`. Segments: literal, `{name}` (any segment),
  `{name:Int}` (must parse as `Int`, else 400 before the handler runs),
  `{name...}` (rest of the path). Without a method, any method matches.
  plain strings: `"GET /users/{id:Int}"`.
- Unmatched path: 404 (`{"error":"not found"}`), or the `not_found`
  hook's response. Path matched with another method: 405 (`{"error":"method
  not allowed"}`), or the `method_not_allowed` hook's response; either
  carries `Allow` listing the methods of the routes that match the path,
  in registration order without repeats (Go 1.22's ServeMux), unless the
  hook's response has one. A hook runs as a handler does (inside the
  middleware, its error a response, a fault a 500); a mounted router's
  hooks are not copied (its misses are `r`'s).
- Streamed request bodies (decision 0310): for a `handle_stream` route the
  server calls the handler before reading the body (Go's `r.Body`):
  `req.body` is empty and `BodyReader.read(max)` gives the next bytes (at
  most `max`, default 32 KiB; `None` at the end) of a Content-Length or
  chunked body as they arrive, `read_all()` the rest. `Expect:
  100-continue` is answered on the first read. A body cut short is
  `IoErr.Io("unexpected EOF")`; a malformed chunked body's error is
  net/http's (`invalid byte in chunk length`, `malformed chunked
  encoding`) and sticks. What the handler leaves unread is discarded
  when less than 256 KiB (the connection stays open), else the response
  says `Connection: close` (net/http's `maxPostHandlerReadBytes`); an
  `Expect: 100-continue` body not read to its end closes the connection.
  `XO_HTTP_MAX_BODY` does not apply. `Router.call` streams `req.body`.
  Middleware see the request without its body. Over HTTP/2 the body
  comes as DATA frames bring it, and the stream's flow control window
  is given back as the handler reads (the connection's at once); a body
  the client is still sending when the handler returns ends with
  RST_STREAM NO_ERROR after the response, as net/http's.
  `BodyReader.multipart(req)` reads a multipart/form-data body part by
  part as it arrives (4.4). Not yet: a recording server (decision 0219)
  reads the body first and streams it from memory. A body is discarded when the
  handler returns, so a streamed response's function (`http.stream`)
  cannot read it on the native backends.
- Middleware run in registration order, outermost first, and see the final
  response after error mapping.
- Middleware may fail: `Err(e)` from a middleware is mapped to a response
  exactly like a handler's (section 4.2), and the middleware registered
  before it see that response. A middleware that cannot fail has
  `E = Never`. `Next` itself cannot fail: it returns the mapped response
  of everything inside.
- Scoped middleware: `r.mount(prefix, sub)` adds every route of `sub` to
  `r` with `prefix` (a path, segments as in patterns) in front of its
  pattern. A mounted route runs `r`'s middleware, then `sub`'s, then the
  handler. A request no route of `sub` matches never reaches `sub`'s
  middleware (it is a 404 or 405 of `r`). The routes are copied (`Router`
  is a value): registering on `sub` after `mount` does not change `r`. To
  guard one route, mount a router holding only that route. `mount` has no
  effects of its own; they were counted where `sub` registered them.
- `Router`, `Next`, `call`, and `serve` carry no effects in their types:
  effects are accounted where handlers and middleware are registered, which
  is where their capabilities are captured.

```
// public routes on r, authenticated routes on api, no path comparison
fn routes(db: sql.Db) -> http.Router uses net.dial {
  var api = http.router()
  api.wrap(fn(req, next) -> http.Response ! AuthErr {
    let user = authenticate(db, req)?
    next(req.with_value(user_key(), user))
  })
  api.handle("GET /notes", fn(req) { list(db, req.value(user_key()).or_fault("set by auth")) })
  var r = http.router()
  r.handle("GET /health", fn(_req) { http.text(200, "ok") })
  r.mount("/api", api)
  r
}
```

### 4.2 Error to status mapping

```
interface StatusError {
  fn status(self) -> Int
  fn display(self) -> Str
}
```

When a handler returns `Err(e)`:

- if `e` implements `StatusError`, the response is `e.status()` with JSON body
  `{"error": e.display()}`;
- otherwise the response is 500 with `{"error": "internal error"}` and the
  error is logged with its frames.

A fault in a handler or a middleware is isolated: 500, logged, crash
bundle written (the middleware registered before a faulting one sees no
response: the 500 replaces the whole chain's). A fault in the server's
own code for a connection ends that connection alone (HTTP/2: a fault
in a stream's handling resets the stream), logged as `http: panic
serving` with the fault's message, as net/http recovers per connection;
the server serves on. On the
native backends (`--no-gc`, decisions 0074, 0083, and 0085) it is isolated
the same way (the handler unwinds, the server keeps serving) but no crash
bundle is written, and the error of a 500 is logged without frames.
Streamed bodies (`http.stream`) are sent natively as net/http sends them.

#### 4.2.1 Status codes (decision 0269, stage A)

```
type Status struct(Int)        // a distinct integer type
const OK: Status = Status(200)
const CREATED: Status = Status(201)
const NOT_FOUND: Status = Status(404)
...                            // every code Go's net/http names
```

`http.Status` is a newtype over `Int` (core 2.8) with a constant for each
code Go's net/http names (`http.OK` to `http.NETWORK_AUTHENTICATION_REQUIRED`,
62 of them, Go's names in upper case). A constant is a compile time value,
so it is a pattern: `match s { http.OK | http.CREATED => ... http.Status(500..=599) => ... _ => ... }`
(a `_` or binding arm is needed: the set is open). Stage A is additive: the
status parameters and fields stay `Int` (`json`, `text`, `Response.status`,
`StatusError.status`); the move to `Status`, its methods and `xo fix`'s
rewrite of integer literals are the later step of decision 0269.

### 4.3 Requests and responses

```
struct Request {
  method: Str
  path: Str
  url: Url
  headers: Map[Str, Str]     // canonical header keys, e.g. "Retry-After"
  body: Bytes
  remote_ip: Str
}
fn (r: Request) param(name: Str) -> Str             // faults if the pattern has no such segment
fn (r: Request) param_opt(name: Str) -> Str?        // None if the pattern has no such segment
fn (r: Request) param_int(name: Str) -> Int         // segment must be declared {name:Int}
fn (r: Request) query(name: Str) -> Str?
fn (r: Request) query_all(name: Str) -> List[Str]   // every value of a repeated key, in order
fn (r: Request) form() -> url.Values ! url.EscapeErr   // an urlencoded POST, PUT, or PATCH body (20)
fn (r: Request) multipart_form() -> multipart.Form ! multipart.MultipartErr   // decision 0282
fn (r: Request) header(name: Str) -> Str?           // case insensitive for a token name; any other name matches only exactly
fn (r: Request) json[T: Json]() -> T ! json.DecodeErr
fn (r: Request) with_header(name: Str, value: Str) -> Request
fn (r: Request) header_values(name: Str) -> List[Str]
fn (r: Request) host() -> Str                 // Go's Request.Host: the Host header as sent (HTTP/2's :authority)
fn (r: Request) tls() -> Bool                 // Go's r.TLS != nil; url.scheme is then "https"
fn (r: Request) tls_state() -> TlsState?      // Go's r.TLS: what the handshake negotiated; None for a plain connection
struct TlsState {                             // Go's tls.ConnectionState
  version: Str                                // "TLS 1.2" or "TLS 1.3"
  cipher_suite: Str                           // Go's CipherSuiteName, e.g. "TLS_AES_128_GCM_SHA256"
  server_name: Str                            // the SNI the client sent, "" for none (an IP address)
  resumed: Bool                               // the handshake resumed a session
  peer_certificates: List[x509.Certificate]   // the client's certificates, leaf first: empty unless with_client_auth asked (4.6)
  verified_chains: List[List[x509.Certificate]] // the chains that verified them (decision 0433)
}
fn (r: Request) raw_query() -> Str            // Go's URL.RawQuery
fn (r: Request) escaped_path() -> Str         // Go's URL.EscapedPath
pub fn redirect(req: Request, url: Str, status: Int) -> Response   // Go's Redirect: Location resolved against the path, an HTML body on GET
pub fn status_text(code: Int) -> Str   // Go's http.StatusText: `Not Found` for 404; empty for a code Go does not name
pub fn error_page(status: Int, msg: Str) -> Response                // Go's Error: text/plain, nosniff, msg and a newline
pub fn error_html(status: Int, title: Str, detail: Str) -> Response   // text/html (nosniff): the title in <title> and <h1>, the detail in <p>, both escaped
pub fn not_found() -> Response                                      // Go's NotFound   // every field of name, in order (decision 0288); found the way header is
fn (r: Request) add_header(name: Str, value: Str) -> Request   // one more field of name
fn (r: Request) cookie(name: Str) -> Cookie?        // the first Cookie pair of that name (Go's Request.Cookie)
fn (r: Request) cookies() -> List[Cookie]           // every Cookie pair that parses
fn (r: Request) with_cookie(c: Cookie) -> Request   // name=value appended to Cookie (Go's AddCookie)

struct Cookie {
  name: Str
  value: Str
  quoted: Bool = false      // the value is (or was) in double quotes
  path: Str = ""
  domain: Str = ""
  expires: Time? = None
  max_age: Int = 0          // seconds; 0 unset, < 0 delete now (Max-Age=0)
  secure: Bool = false
  http_only: Bool = false
  same_site: SameSite = SameSite.Default
  partitioned: Bool = false
}
enum SameSite { Default, Lax, Strict, None }
struct CookieErr { msg: Str }
fn (c: Cookie) display() -> Str                     // the Set-Cookie value (Go's Cookie.String)
pub fn parse_cookie(line: Str) -> List[Cookie] ! CookieErr      // Go's ParseCookie
pub fn parse_set_cookie(line: Str) -> Cookie ! CookieErr        // Go's ParseSetCookie

struct Key[T] {}                                   // handle
pub fn key[T](name: Str) -> Key[T]
fn (k: Key[T]) name() -> Str
fn (r: Request) with_value[T](k: Key[T], v: T) -> Request
fn (r: Request) value[T](k: Key[T]) -> T?

pub fn new_request(method: Str, url: Str, headers: Map[Str, Str] = {}, body: Bytes = b"") -> Request
    // remote_ip is "127.0.0.1"; for tests and clients

struct Response {
  status: Int
  headers: Map[Str, Str]
  body: Bytes
}
pub fn json[T: Json](status: Int, v: T) -> Response   // sets Content-Type
pub fn text(status: Int, s: Str) -> Response
pub fn empty(status: Int) -> Response
pub fn stream[fx: Effects](status: Int, f: fn(BodyWriter) -> () ! IoErr uses fx) -> Response uses fx
    // the body is produced by `f` while the response is sent (for large exports)
pub fn serve_content(req: Request, name: Str, modtime: Time?, content: Bytes, headers: Map[Str, Str] = {}) -> Response
    // Go's ServeContent (decision 0306): Range (206, multipart/byteranges), the
    // conditional requests against modtime and the ETag in headers (304, 412),
    // 416, Content-Type from headers, name's extension, or the content
fn (w: BodyWriter) write(data: Bytes) -> () ! IoErr
fn (w: BodyWriter) flush() -> () ! IoErr
fn (w: BodyWriter) set_trailer(name: Str, value: Str)   // a trailer after the body (decision 0433)
fn (r: Response) with_trailer_names(names: List[Str]) -> Response   // the `Trailer` header (HTTP/1.1)
fn (r: Response) with_full_duplex() -> Response   // the stream may read the request body while it writes (decision 0433)
pub fn pipe() -> (PipeWriter, BodyReader)          // a body one task writes while the client sends it (Go's io.Pipe)
struct PipeWriter {}
fn (w: PipeWriter) write(data: Bytes) -> () ! IoErr   // held before the reader's first read (up to 64 KiB), then waits for the reader; Io("io: read/write on closed pipe") after close
fn (w: PipeWriter) close()                            // the reader's next read is None
fn (w: PipeWriter) close_with_error(e: IoErr)         // the reader's next read is Err(e)
    // a response with status 101 made by std/websocket's upgrade hands its
    // connection over: the server writes nothing itself (decision 0313)
fn (b: BodyReader) read(max: Int = 32768) -> Bytes? ! IoErr   // a handle_stream body's next bytes, None at its end
fn (b: BodyReader) read_all() -> Bytes ! IoErr                // the rest of it
fn (b: BodyReader) multipart(req: Request) -> PartReader ! multipart.MultipartErr   // the body's multipart/form-data parts as they arrive (decision 0310)
fn (b: BodyReader) trailers() -> Map[Str, Str]   // the request's trailers, empty until read returned None (decision 0433)
fn (p: PartReader) next() -> StreamPart? ! multipart.MultipartErr   // the next part (the open one's unread rest dropped), None after the last
fn (p: PartReader) read(max: Int = 32768) -> Bytes? ! multipart.MultipartErr   // the open part's next bytes, None at its end
fn (p: StreamPart) header_values(name: Str) -> List[Str]   // every value of a part's header, in order
fn (r: Response) with_header(name: Str, value: Str) -> Response
fn (r: Response) header(name: Str) -> Str?          // case insensitive for a token name; any other name matches only exactly
fn (r: Response) header_values(name: Str) -> List[Str]   // every field of name, in order (decision 0288); found the way header is
fn (r: Response) add_header(name: Str, value: Str) -> Response   // one more field of name
fn (r: Response) with_cookie(c: Cookie) -> Response     // one more Set-Cookie field (Go's SetCookie); none for an invalid name
fn (r: Response) cookies() -> List[Cookie]              // every Set-Cookie field that parses (Go's Response.Cookies)
fn (r: Response) text() -> Str?                     // the body as text; None if not valid UTF-8
fn (r: Response) decode_json[T: Json]() -> T ! json.DecodeErr
```

`text` and `decode_json` read the whole body, collecting a streamed one
first; `text` is `r.body.to_str()` for a body that is not streamed. In
tests: `expect(resp.text()) == Some("ok")`, or `resp.text() ?? ""`.

Typed request values (decision 0055) let a middleware hand a value to the
handlers behind it without encoding it in a header. `with_value` returns a
copy of the request carrying `v` under `k`; `value` returns the latest value
set under `k`, or `None`. A key is identified by its name and its type
`T`: `http.key[User]("user")` called twice gives the same key, and
`http.key[Int]("user")` is a different one, so a value is always read back
at the type it was stored with. Values are local to the process: they are
not headers, are never sent by a client or read from the network, take no
part in `==`, `hash`, or `debug` of a `Request`, and a fresh request
(`new_request`, or one received by `serve`) has none. So a client cannot
forge one. Declare a key once as a function or pass it around:

```
fn user_key() -> http.Key[User] { http.key("user") }

fn me(req: http.Request) -> http.Response ! ApiErr {
  let user = req.value(user_key()).or_err(ApiErr.Unauthorized)?
  http.json(200, user)
}
```

Cookies (decision 0288) are net/http's cookie.go: `display()` is
`""` for a name that is not a token, quotes a value with a space or a
comma (or `quoted`), drops the bytes a value or path may not hold, drops
a domain that is not a domain name or IPv4 address (a leading dot is not
sent), writes `Expires` in GMT (`Wed, 10 Nov 2010 23:00:00 GMT`) for a
year from 1601, `Max-Age=0` for a negative `max_age`, and `SameSite`
only when not `Default`. Go logs each dropped byte or domain; Xo drops
them silently. `parse_set_cookie` reads every attribute Go does
(`Expires` in RFC 1123 or `Mon, 02-Jan-2006 15:04:05 MST` form, the
weekday and zone name not checked, as UTC), ignoring unknown ones; its
errors are Go's (`http: blank cookie`, `http: '=' not found in cookie`,
`http: invalid cookie name`, `http: invalid cookie value`); Go's `Raw`,
`RawExpires` and `Unparsed` are not kept. `Request.cookies()` reads the
`Cookie` field as Go's readCookies (invalid pairs skipped, at most 3000).
**Fields with repeated names** (decision 0288): a Request or Response
keeps its header fields in order behind `headers`, which is their view:
a name's values joined with ", ", except `Set-Cookie`, whose view is its
first field (cookies are never joined). `header_values(name)` returns
each field of the name (case insensitive), as received or as added;
`add_header` appends one; `with_header` replaces every field of the name
with one. A struct literal or a direct change of `headers` sets that
name to the one value written (a Map entry that is not the view of the
name's fields wins), so code that only uses the Map works as before. A
response or a client request sends one field per value (HTTP/1.1 and
HTTP/2), so two `with_cookie` calls send two `Set-Cookie` fields. The
server and the client keep each received field (an HTTP/2 request's
`Cookie` fields arrive as one, joined with `; `, as net/http's).

`with_header` returns a copy with the header set (canonical key), like
`Response.with_header`.

`query(name)` is the first value of a key and `query_all(name)` every
value, in order (Go's `URL.Query()`; a pair with a `;` or a bad escape
is skipped). `form()` (decision 0279) is Go's `PostForm`: the body of a
POST, PUT, or PATCH whose `Content-Type` media type
(`mime.parse_media_type`, 23; no header is `application/octet-stream`)
is `application/x-www-form-urlencoded`, parsed with `url.parse_query`;
any other request gives empty `Values`. A Content-Type that does not
parse is `EscapeErr` with mime's text (`mime: invalid media parameter`
for `text/plain; boundary=`, whatever the type, as Go's `ParseForm`),
and a bad escape or `;` is `EscapeErr` with no values (Go's `ParseForm`
keeps the pairs parsed around the error; Xo returns the error alone).
Its size is the body's, which the server already limits
(`XO_HTTP_MAX_BODY`, 4.6). A request built as a struct literal has no raw query:
`query_all` gives its `url.query` value.

`multipart_form()` (decision 0282) is Go's `ParseMultipartForm`: the
`Content-Type` must parse (`mime.parse_media_type`) as
`multipart/form-data` (else `request Content-Type isn't
multipart/form-data`, any method; `multipart/mixed`, which Go accepts only
in `MultipartReader`, is refused too) with a `boundary` (else `no multipart
boundary param in Content-Type`); the body is read with
`multipart.parse_form` (26), whose files stay in memory: the body is
already limited by the server.

`BodyReader.multipart(req)` (decision 0310 phase 3) is Go's
`Request.MultipartReader` for a `handle_stream` route: the same
Content-Type checks and errors as `multipart_form()`, then the parts as
the body arrives, none of it held in memory beyond the part being read.
`next()` is `NextPart`: a `StreamPart{name, filename: Str?, headers}`
read as `multipart.Part` reads (form name, file name's last element,
each header's first value; `header_values(name)` gives every value, as
textproto's `MIMEHeader`), the previous part's unread data dropped, a
quoted-printable part decoded and its Content-Transfer-Encoding header
removed; `None` after the closing boundary. `read(max)` is `Part.Read`
(at most `max` bytes, at least 1; `None` at the part's end and before
the first `next()`). Errors are Go's texts (`multipart: NextPart: EOF`,
`unexpected EOF` for a body cut inside a part, quoted-printable errors)
and stick.

#### The client's address behind proxies

```
fn (r: Request) client_ip(trusted: List[netip.Prefix] = [], header: ProxyHeader = ProxyHeader.XForwardedFor) -> netip.Addr?
enum ProxyHeader { XForwardedFor, Forwarded }
```

- No trusted prefixes: the peer (`remote_ip`). A peer outside every
  trusted prefix: the peer, whatever the headers say (a client can write
  them).
- Otherwise the chain of the one header named (`header`: the header the
  server's own proxies write; the other is never read, since a proxy that
  writes one passes a client's copy of the other through): every field of
  it in order, split on commas (`X-Forwarded-For`), or RFC 7239 elements
  (`Forwarded`: commas and semicolons outside quoted strings, the `for=`
  parameter, quoted values unquoted). Walked from the right, the first
  hop that is not a trusted address ends the walk: the client when it
  parses, else None: a hidden (`for=_hidden`), unknown, or missing hop
  means the client is unknown and every hop on its left came from that
  client, and garbage would let a client shift the walk. When every hop
  is trusted the leftmost is the client (a trusted proxy appended it);
  with no hops, the peer.
- Addresses parse bare, as `[v6]`, `[v6%zone]:port`, or `v4:port`, and
  are unmapped before matching: `::ffff:10.0.0.1` is inside
  `10.0.0.0/8` and is returned as `10.0.0.1`.
- Golden `http_client_ip` (every backend).

### 4.4 Server

```
pub fn server(l: Listener, r: Router) -> Server
fn (s: Server) serve() -> () ! HttpErr uses net.listen
fn (s: Server) addr() -> Str                       // the listener's addr()
fn (s: Server) with_tls(certs: List[TlsCert]) -> Server   // HTTPS (4.6)
fn (s: Server) with_read_timeout(d: Duration) -> Server          // Go's ReadTimeout: a request's head and body, from its first byte
fn (s: Server) with_read_header_timeout(d: Duration) -> Server   // Go's ReadHeaderTimeout
fn (s: Server) with_idle_timeout(d: Duration) -> Server          // Go's IdleTimeout: the wait for a kept alive connection's next request
fn (s: Server) with_max_header_bytes(n: Int) -> Server           // Go's MaxHeaderBytes (plus 4096 of slack; 1 MB when 0): a longer head is 431; HTTP/2's header list limit is n plus 320 (Go's maxHeaderListSize)
fn (s: Server) with_max_body(n: Int) -> Server                   // the body read before a handler (64 MB when 0): a longer one is 413
fn (s: Server) with_protocols(p: Set[Protocol]) -> Server        // Go's Server.Protocols (4.6, decision 0434)
fn (s: Server) with_h2_config(c: H2Config) -> Server             // Go's Server.HTTP2 (4.6)
pub fn max_body(n: Int) -> fn(Request, Next) -> Response         // Go's MaxBytesReader as a middleware: 413 `http: request body too large`
enum HttpErr { Io(msg: Str), Canceled }
```

- Timeouts (Go's): a connection whose request's head or body is not
  complete when the read or head timeout passes closes without a reply
  (net/http's conn.serve treats a timeout as a common read error); the
  idle timeout (the read timeout when 0) closes a kept alive connection
  waiting for its next request. The header cap has net/http's 4096 bytes
  of slack over `with_max_header_bytes`. A TLS connection's record reads
  take the same deadlines, and its handshake the smaller of the read and
  header timeouts (net/http's handshake deadline), so a client that
  connects and sends nothing, or a slow one, cannot hold an HTTPS
  connection past them; a handshake past its deadline closes the
  connection without a reply.
- A body over `with_max_body` is `413` with MaxBytesReader's text and the
  connection closes, before the handler runs; a body over the default cap
  (no `with_max_body`) reaches the handler cut to the cap with the
  connection closed after the response, as before. The Go backend
  answers 413 from the Content-Length (a chunked body over the cap is
  cut by MaxBytesReader).

`serve` runs until its scope is cancelled, then stops accepting, waits for
in flight requests, and returns `Ok(())`. A connection handed over by a
101 (std/websocket's `upgrade`, a proxied upgrade) is in flight until its
function returns (decision 0429): on a stop `serve` waits for it within
the grace period (core 7.5), so a long lived handler watches
`task.stopping()` and closes, a WebSocket with 1001. Each request runs as an isolated
task with its own span and request id. Its memory may come from a request
scoped arena (core 5.5, decision 0066), which no program can observe. A
response with a body (not 1xx, 204, or 304) that is not a stream is sent
with a `Content-Length` header unless the handler set one; only streamed
bodies use chunked transfer encoding. To run a server alongside background
work:

```
scope s {
  s.spawn_background(fn() { run_evictor(limiter, os.clock) })
  srv.serve()?
}
```

### 4.5 Client

```
pub fn client(net: Net, timeout: Duration = 30s) -> Client
fn (c: Client) get(url: Str) -> Response ! ClientErr uses net.dial
fn (c: Client) send(req: Request) -> Response ! ClientErr uses net.dial
fn (c: Client) post_json[T: Json](url: Str, v: T) -> Response ! ClientErr uses net.dial
fn (c: Client) with_roots(pem: Bytes) -> Client ! TlsErr   // 4.6
fn (c: Client) with_redirects(max: Int) -> Client          // decision 0404
fn (c: Client) with_protocols(p: Set[Protocol]) -> Client  // Go's Transport.Protocols (4.6, decision 0434)
fn (c: Client) with_h2_config(cfg: H2Config) -> Client     // Go's Transport.HTTP2 (4.6)
fn (r: Request) with_timeout(d: Duration) -> Request       // decision 0404
fn (r: Request) with_idle_timeout(d: Duration) -> Request  // decision 0404
fn (r: Request) with_expect_continue(timeout: Duration = 1s) -> Request  // Go's Transport.ExpectContinueTimeout: sets `Expect: 100-continue`; an HTTP/1.1 client sends the head, waits up to timeout for `100 Continue`, then sends the body; a final response first is the result and the body is never sent (the connection is not reused); other 1xx are skipped. A with_body_reader body is not read before the 100. No effect without a body, over HTTP/2 or for Client.open
fn (r: Request) with_body_reader(b: BodyReader) -> Request // decision 0404
fn (r: Request) with_trailers(names: List[Str], f: fn() -> Map[Str, Str]) -> Request   // names declared up front, f's values sent after the body (decision 0433)

pub fn reverse_proxy(client: Client, target: Url) -> ReverseProxy   // decision 0404
struct ReverseProxy {}
fn (p: ReverseProxy) with_rewrite(f: fn(Request, Request) -> Request) -> ReverseProxy
fn (p: ReverseProxy) serve(req: Request, body: BodyReader) -> Response uses net.dial

fn (c: Client) open(req: Request) -> ClientStream ! ClientErr uses net.dial
struct ResponseHead { status: Int, headers: Map[Str, Str] }
struct ClientStream {}
fn (s: ClientStream) head() -> ResponseHead
fn (s: ClientStream) read(max: Int = 32768) -> Bytes? ! ClientErr uses net.dial
fn (s: ClientStream) read_all(max_size: Int = 1073741824) -> Bytes ! ClientErr uses net.dial
fn (s: ClientStream) close()
fn (s: ClientStream) trailers() -> Map[Str, Str]   // the response's trailers, empty until read returned None (decision 0433)

enum ClientErr {
  InvalidUrl(msg: Str)
  Connect(msg: Str)
  Timeout
  Protocol(msg: Str)
  Canceled
}

pub fn parse_url(s: Str) -> Url ! UrlErr
struct Url { scheme: Str, host: Str, port: Int?, path: Str, query: Map[Str, Str] }
struct UrlErr { msg: Str }
```

Full duplex (decision 0433): inside a `handle_stream` route's
`http.stream` function the request's BodyReader stays readable, so a
handler reads request pieces while it writes response pieces. Over
HTTP/2 a stream always allows it; over HTTP/1.1 only a response made
`with_full_duplex()` does (Go's `ResponseController.EnableFullDuplex`,
since many HTTP/1 clients read nothing until they finish writing): its
head goes out at once, and a request that asked for `100-continue` gets
it before the first read. `http.pipe()` gives a BodyReader that a
PipeWriter fills as a task writes; with `Request.with_body_reader` and
`Client.open`, which returns at the response's head, one task writes a
request while another reads the response (a gRPC style bidirectional
stream). A read takes at most `max` bytes of a written piece. Until the
reader's first read the pipe holds what is written, up to 64 KiB, and
each write returns at once, so a task may write a request's first
pieces and then open it (decision 0444); a write past 64 KiB held, and
from the first read on each write, waits until the reader has taken
it, as Go's io.Pipe does.
A client that is done with the request (its response ended or failed,
or the stream was closed) closes the pipe's reading end, as net/http
closes a request body: a write waiting then, and any after, fails with
`Io("io: read/write on closed pipe")`. The native backend's HTTP/1.1
server and client do it (the client sends the body from a task and the
connection is not reused afterwards); the native HTTP/2 client's
duplex pipe follows (until then its pump waits for a piped piece
before it reads the response).

Trailers (decision 0433) are fields sent after a body, as net/http sends
them: a streamed response's `set_trailer` (Go's `TrailerPrefix`; declared
in advance or not), a request's `with_trailers`, read by
`ClientStream.trailers` and `BodyReader.trailers` once the body has
ended, each name canonical and the names in sorted order. A request's
trailer names are declared in its head (`with_trailers(names, f)`; an
HTTP/2 server keeps only declared names, as Go's does), and f's values
for other names are dropped. Over HTTP/1.1 they follow the last chunk
(a body with a Content-Length carries none, and a streamed response
that writes no body is sent with `Content-Length: 0`, so it carries
only the trailers it declared with `with_trailer_names`, as net/http
does, while one that writes a body and sets a trailer is sent chunked
on every backend, so its trailers follow declared or not);
over HTTP/2 they are a HEADERS frame ending the stream, after the
response's HEADERS and DATA (with no body, two HEADERS frames; never
merged into the first). Names a trailer may not carry are dropped when
set and when received: Go's list, Authorization, Cache-Control,
Connection, Content-Encoding, Content-Length, Content-Range,
Content-Type, Expect, Host, Keep-Alive, Max-Forwards, Pragma,
Proxy-Authenticate, Proxy-Authorization, Proxy-Connection, Range, Realm,
Te, Trailer, Transfer-Encoding and Www-Authenticate. Every backend sends
and receives them (the native one since e0444461a); golden http_trailers
checks the native and Go backends agree.

`display` of these errors prints plain text (decision 0358): `HttpErr` the
message of an `Io` or `canceled`; `ClientErr` `invalid URL: MSG`, MSG for a
`Connect`, `request timed out`, `protocol error: MSG`, `canceled`; `UrlErr`,
`CookieErr` and `TlsErr` their message.

Any HTTP status is a successful `Response`; only transport failures are
`ClientErr`. A request's query is sent as it was given (Go's `URL.RawQuery`:
repeated keys and their order kept) unless its `url.query` was changed,
then as `url.query` builds it. The client does not retry. It follows up to 10 redirects for
`GET` and `HEAD` (Go's Client counts requests, so it follows 9), or as
many as `with_redirects(max)` says: `with_redirects(0)` returns a 3xx
response as it came, its `Location` as the server sent it (Go's
`ErrUseLastResponse`), and past `max` the request fails with `Protocol`
(`stopped after MAX redirects`). A redirect to another host does not
carry `Authorization`, `Cookie`, or their proxy forms (Go drops them
only for a host that is not the same or a subdomain). A request's
`with_timeout(d)` limits its whole exchange, the body included, in place
of the client's `timeout` (Go's `Client.Timeout`); it is a client side
builder, and a handler's own Request ignores it. `ClientErr.Connect`
means the request did not reach the server (no connection or no TLS
handshake), so any method may be retried; `Timeout` and `Protocol` may
have reached it.

`c.open(req)` sends the request as `send` does (the same redirects,
header and Net checks) and returns once the head of the response has
arrived; the body is read from the stream as it arrives (decision 0404).
`read(max)` returns the next bytes, at most `max`, and `None` at the end;
a body cut short (a connection closed before its `Content-Length`, a
chunked body without its last chunk) is `Protocol` (`unexpected EOF`),
never an end. Errors after `open` are never `Connect`. `read_all(max_size)`
returns the rest, `Protocol` past `max_size` bytes. A gzip body is decoded
as it is read, on `send`'s terms. Timeouts: the head must arrive within
the request's or the client's timeout, and then each `read` within
`with_idle_timeout(d)` (the same timeout when unset); a stream has no
limit over its whole body, and a `Timeout` from `read` means the head was
received. `close` releases the connection and again does nothing; a
stream dropped without `close` is released when its last copy goes. A
body read to its end gives its connection back to the client's pool; any
other HTTP/1.1 connection is closed. Over HTTP/2 each `read` takes the
next DATA frame's bytes as it arrives (the window updated as they are
received), `close` before the end resets the stream (RST_STREAM CANCEL)
and the connection stays pooled, and a stream dropped without `close`
before its end closes its connection.

`req.with_body_reader(b)` sends a `handle_stream` route's body (or any
BodyReader) as it is read, in place of `body`: with the request's own
`Content-Length` field when it has one, else chunked (HTTP/1.1) or as
DATA frames up to the end of the source (HTTP/2). Such a body is read
once: a 307 or 308 is returned as it came instead of followed, a 301,
302 or 303 is followed as a GET without a body, and a request that fails
on a dead pooled connection is not sent again. An error reading the
source fails the request with `Protocol` and the source's text. It is a
client side builder.

`reverse_proxy(client, target)` forwards a `handle_stream` route's
requests (`r.handle_stream("/api/{rest...}", fn(req, body) { p.serve(req,
body) })`) as Go's `httputil.ReverseProxy` with `Rewrite`, `SetURL` and
`SetXForwarded` does: the target's scheme and host, its path joined
before the request's (one slash between), the target's query merged with
the request's; the hop-by-hop fields (`Connection` and the names it lists,
`Proxy-Connection`, `Keep-Alive`, `Proxy-Authenticate`,
`Proxy-Authorization`, `Te`, `Trailer`, `Transfer-Encoding`, `Upgrade`)
removed from the request and the response; `X-Forwarded-For`
set to the peer's `remote_ip` after the client's own `X-Forwarded-*`
fields are dropped (Go's Rewrite: a client cannot forge the chain; behind
a trusted proxy the `with_rewrite(f)` hook, which gets the received
request and the outgoing one, copies the chain from the first),
`X-Forwarded-Host` the request's `Host`,
`X-Forwarded-Proto` its scheme (`http` when unknown); then the
`with_rewrite` hook. The request body is sent as it is read
(`with_body_reader`), redirects are returned as they came, and the
response body is streamed back, flushed after each piece when it has no
`Content-Length` or is `text/event-stream`. A failure to reach the
target is an empty 502, a timeout 504, each logged as `http: proxy
error` with the error. A protocol switch (WebSocket, decision 0404 stage
E) goes through as httputil's: the `Upgrade` the client asks for (with
`Connection: upgrade`) is kept on the outgoing request, the target's 101
is written back with its header as it came, and the bytes are copied
both ways over the client's connection (handed over as std/websocket's
upgrade hands it, decision 0313) until either side ends. A target that
switches to another protocol than the one asked for is a 502 (`backend
tried to switch protocol "X" when "Y" was requested`); a requested
protocol that is not printable ASCII is a 400. Over HTTP/1.1 only.

Behind a trusted proxy, the hook keeps the chain it received:

```xo
let p = http.reverse_proxy(client, target).with_rewrite(fn(inb: http.Request, out: http.Request) -> http.Request {
  match inb.header("X-Forwarded-For") {
    Some(prior) => out.with_header("X-Forwarded-For", "${prior}, ${inb.remote_ip}")
    None => out
  }
})
```

`parse_url` accepts absolute RFC 3986 URLs with an
`http` or `https` scheme and a host; anything else is a `UrlErr`. An
`https` URL is requested over TLS (4.6).

Compression is net/http's Transport's: a request without its own
`Accept-Encoding` or `Range` field (and not `HEAD`) is sent with
`Accept-Encoding: gzip`, and a non-empty response body whose
`Content-Encoding` is `gzip` (any case) is decoded, with
`Content-Encoding` and `Content-Length` removed from its headers; a
body that does not decode is `Protocol` with gzip's text (`gzip: invalid
header`, `unexpected EOF`). A request that sets `Accept-Encoding` or
`Range` itself gets the body and headers as the server sent them.

### 4.6 TLS (HTTPS)

```
type TlsCert = tls.TlsCert                         // handle: a certificate chain with its private key (27)
pub fn tls_cert(cert_pem: Bytes, key_pem: Bytes) -> TlsCert ! TlsErr
struct TlsErr { msg: Str }
fn (s: Server) with_tls(certs: List[TlsCert]) -> Server
fn (s: Server) with_client_auth(mode: tls.ClientAuth, roots: tls.Roots? = None) -> Server
fn (c: Client) with_roots(pem: Bytes) -> Client ! TlsErr
fn (c: Client) with_cert(cert: TlsCert) -> Client
fn (c: Client) with_server_name(name: Str) -> Client
fn (c: Client) with_verify_conn(f: fn(List[x509.Certificate]) -> () ! Str) -> Client
fn (c: Client) with_verify(v: tls.Verify) -> Client
struct TlsOptions { server_name: Str, verify_conn: Bool, custom_roots: Bool, client_cert: Bool, verify: tls.Verify }
fn (c: Client) tls_options() -> TlsOptions
fn (r: Request) with_host(host: Str) -> Request
```

`with_server_name(name)` is Go's `tls.Config.ServerName`: the SNI and the name the certificate is verified against, in
place of the URL's host ("" keeps the URL's). `with_verify_conn(f)` is `tls.Config.VerifyConnection`: `f` gets the server's
certificates (leaf first) after the handshake and the verification mode's own check, before any request byte; an `Err`
text fails the connection with that text; it runs once per new connection. A client with a server name or hook is its own
pool key. `Request.with_host(host)` is Go's `Request.Host`: the Host of HTTP/1.1 and the `:authority` of HTTP/2 for the
first host's requests, while the connection and the TLS name stay the URL's (decision 0541).

`with_verify(v)` selects how https checks the server's certificate (Go's `InsecureSkipVerify` for `Verify.None`;
`Verify.ChainOnly` checks the chain against the roots but not the name; the default is `Verify.Full`). It is UNSAFE
alone: `Verify.None` accepts any certificate, so a request on a `Verify.None` client without a `with_verify_conn` fails
("tls: Verify.None needs a verify_conn (an unverified connection must be pinned)"); pair it with a pin
(`tls.pin_sha256`, `tls.pin_sha1`) for a self signed server whose certificate is known only by its digest. A client with a
mode has its own pool key; `with_roots` keeps the server name, hook and mode, so the call order of the four does not
matter. A wasi:http component's host does TLS, so there `with_verify` has no effect.

```
fn serve_https(os: Os, r: http.Router) -> () ! Error uses (fs.read, net.listen) {
  let cert = http.tls_cert(os.fs.read("cert.pem")?, os.fs.read("key.pem")?)?
  http.server(os.net.listen(":8443")?, r).with_tls([cert]).serve()?
}
```

- `tls_cert` takes a PEM certificate chain (leaf first) and the PEM private
  key of its leaf (PKCS #1, PKCS #8, or SEC 1; RSA or ECDSA), and checks
  that they match. Its errors are Go's `tls.X509KeyPair` texts, for example
  `tls: private key does not match public key`. Reading the files is the
  caller's (with `Fs`), so no capability is hidden in the call.
- `with_tls` returns the server speaking HTTPS on its listener: TLS 1.2
  and 1.3 with Go's default cipher suites and ECDHE, `certs[0]` unless the
  client's server name (SNI) is one of another certificate's DNS names
  (wildcards in the leftmost label). An empty list makes `serve` fail with
  `Io("http: with_tls needs a certificate")`. Everything else in 4.4 holds
  (shutdown, the arena, `Content-Length`); a client that sends plain HTTP
  gets `400` with the body `Client sent an HTTP request to an HTTPS
  server.` and the connection closes. Failed handshakes are not logged.
- Protocols and HTTP/2 settings (decision 0434, Go 1.24's shape):

  ```
  enum Protocol { Http1, Http2, UnencryptedHttp2 }
  struct H2Config {                     // Go's HTTP2Config; 0: the default, out of range values clamped as Go
    max_concurrent_streams: Int = 0     // 250
    max_read_frame_size: Int = 0        // 1 MiB (16384 to 16 MiB)
    max_receive_buffer_per_connection: Int = 0   // 1 MiB
    max_receive_buffer_per_stream: Int = 0       // 1 MiB
    max_decoder_header_table_size: Int = 0       // 4096
    max_encoder_header_table_size: Int = 0       // 4096
    send_ping_timeout: Duration = 0s    // read silence before a PING; none
    ping_timeout: Duration = 0s         // the wait for its ack; 15 s
    write_byte_timeout: Duration = 0s   // no write progress closes the connection; none
  }
  ```

  Without `with_protocols` a server and a client speak `#{Http1,
  Http2}`. A server with `UnencryptedHttp2` serves HTTP/2 with prior
  knowledge (h2c: the client opens with the HTTP/2 preface) on a plain
  listener, and HTTP/1.1 on the same port when `Http1` is in the set;
  without `Http1` an HTTP/1.1 connection closes. A client with
  `UnencryptedHttp2` and not `Http1` opens an `http://` URL with HTTP/2
  prior knowledge; with both it uses HTTP/1.1 (it cannot know the server
  speaks h2c). Over TLS, a set without `Http2` leaves `h2` out of ALPN
  and one without `Http1` leaves out `http/1.1`. `serve` fails with
  `Io("http: with_protocols needs a protocol the listener can speak")`
  for an empty set, or a TLS server with neither `Http1` nor `Http2`.
  There is no `Upgrade: h2c` (RFC 9113 deprecates it; net/http has none).
  The native backends store the set and the config; the h2c server and
  client and the per connection settings land with decision 0434's
  native step.
- Mutual TLS (decision 0433): `with_client_auth(mode, roots)` makes the
  server ask for a client certificate in `mode` (std/tls's `ClientAuth`,
  Go's `tls.Config.ClientAuth`): `None` asks for none (the default);
  `Request` asks and checks nothing; `RequireAny` requires one and checks
  nothing; `VerifyIfGiven` checks one if given; `RequireAndVerify`
  requires one and checks it. A check verifies the client's chain against
  `roots` (Go's `ClientCAs`; the system's when `None`) with the
  `clientAuth` extended key usage and no host name. A client that fails
  the mode gets Go's alert (`remote error: tls: certificate required`,
  `remote error: tls: bad certificate`, ...) and the server logs nothing.
  A request's `tls_state()` has the client's certificates in
  `peer_certificates` and the chains that verified them in
  `verified_chains` (each leaf first, ending in a root; empty when the
  mode checked none). `with_cert(cert)` makes a client present `cert`
  when a server asks for one; without it, it presents none. The native
  backends do this over TLS 1.3 (a client presents a certificate only
  when one of the request's certificate authorities issued it, as Go's
  does) and over TLS 1.2.
- HTTP/2 (decision 0121): over TLS a server offers `h2` and `http/1.1`
  by ALPN and prefers `h2`, and the client offers both; plain `http` is
  HTTP/1.1 unless `with_protocols` says otherwise (below). Nothing else in the API changes, and every backend
  behaves as net/http's HTTP/2 does: a request from Go's client (or Xo's)
  carries `User-Agent: Go-http-client/2.0` and no `Connection` field,
  several `Cookie` fields arrive joined with `; `, and a response's
  `Connection` field is not sent (`close` ends the connection after its
  streams). Each request runs as a task of its own, many at once on a
  connection. Limits per connection: 250 concurrent streams, a header
  list of 1048896 bytes counting 32 per field (a larger one is answered
  `431`), 1 MiB flow control windows and frames; a client that resets
  streams faster than their handlers end is disconnected. A fault in a
  streamed body resets its stream, so the client gets
  `Protocol("stream error: stream ID 3; INTERNAL_ERROR; received from
  peer")` (a failure while a response body is read has no method and
  URL in its text, as on Go). When `serve` stops, streams in flight
  finish. The client keeps an HTTP/2 connection for later requests to
  the same host, as Go's Transport does (stream ids go up on it).
- The client verifies an `https` server's certificate chain and its host
  name (an IP address against the certificate's IP addresses) against the
  system's roots: macOS's trust settings, the CA bundle and directories of
  Linux (`SSL_CERT_FILE` and `SSL_CERT_DIR` override them there).
  `with_roots` returns a client that trusts the certificates of `pem`
  instead (for a private CA or a test certificate); `TlsErr("tls: no
  certificates found in PEM input")` when it has none. The TLS handshake
  must finish within 10 seconds and within the client's `timeout`, else
  `Timeout`.
- TLS failures are `ClientErr`s with Go's texts: a certificate that does
  not verify is `Protocol("Get \"https://...\": tls: failed to verify
  certificate: x509: ...")` (`certificate signed by unknown authority`,
  `certificate is valid for a, b, not c`, `certificate has expired or is
  not yet valid: ...`, whose current time is in the local zone, as Go's,
  and in UTC on wasm), a server answering plain HTTP is `Protocol("...:
  http: server gave HTTP response to HTTPS client")`, and an alert of the
  server (no shared cipher suite, a required client certificate) is
  `Connect("remote error: tls: handshake failure")`.
- Tests: `FakeNet` (8.1) and in memory listeners serve `https` URLs and
  `with_tls` servers in process, without TLS.
- The native backends (`--no-gc`) implement this in Xo (decision 0202):
  TLS 1.3 and 1.2, client and server, Go 1.26's defaults and texts.
  The client offers Go 1.26's groups in its order (the post quantum
  hybrids `X25519MLKEM768`, `SecP256r1MLKEM768` and
  `SecP384r1MLKEM1024`, then X25519, P-256, P-384 and P-521) and takes
  ECDSA certificates on P-256, P-384 and P-521 (P-224 too in chains and
  on TLS 1.2, as Go); the server takes `X25519MLKEM768`, X25519, P-256
  and P-384, and P-256 and P-384 ECDSA certificate keys. On macOS the
  system's roots are checked by the Security framework through the same
  calls Go makes. (Mbed TLS, decision 0105, was removed on 2026-10-15;
  `XO_LLRT_C=tlsio` and `tlssrv` are gone with it.)
- Environment switches, for compatibility and debugging (read by the
  runtime, every backend unless noted):
  - `XO_TLS_MLKEM=0`: the client offers none of the ML-KEM hybrids, so
    X25519 is its first and only key share (groups X25519, P-256, P-384,
    P-521), as Go's `GODEBUG=tlsmlkem=0`; for a server that mishandles a large
    ClientHello, and for handshake measurements without ML-KEM.
  - `XO_HTTP_MAX_BODY=<bytes>` (native backends): the largest request
    body the server reads (default 64 MiB; a longer one is cut short as
    a body whose client went away). Go has no such limit. A
    `handle_stream` route's body has none either (decision 0310).

### 4.7 Response compression

```
fn gzip_responses(min_size: Int = 1024, level: compress.Level = compress.Level.Default) -> fn(Request, Next) -> Response
// r.wrap(http.gzip_responses())
```

A middleware that compresses responses with gzip (decision 0305, an Xo
addition: Go's standard library has none). It follows the rules of
klauspost/compress's gzhttp:

- **Passed through unchanged:**
  - a status below 200, 204, 304, or 206;
  - a response with its own Content-Encoding or a Content-Range;
  - a Content-Type that is compressed already: `image/*`, `video/*`,
    `application/zip`, `application/gzip`, `font/woff2`.
- **Vary.** Every other response gets `Vary: Accept-Encoding`, because
  its body could depend on that header.
- **When it compresses.** The request's Accept-Encoding must allow it: a
  `gzip` or `*` element whose q is not 0. A body must be `min_size`
  bytes or more; a streamed body needs a Content-Type of its own (its
  bytes are not sniffed). It then:
  - compresses the body with `compress.gzip` at `level`;
  - sets `Content-Encoding: gzip`;
  - drops Content-Length;
  - makes a strong ETag weak (`W/"..."`);
  - sets the Content-Type sniffed from the uncompressed body
    (`mime.detect_content_type`) when the response had none, so the
    server does not sniff the gzip bytes.
- **Streamed responses** run through a `compress.Compressor`. Each
  `BodyWriter.flush` flushes it (Go's `Writer.Flush`), so the client can
  decode what has been sent.
- Tests: golden http_gzip_responses (go, arm64, LLVM).

---

## 5. std/json

```
pub fn encode[T: Json](v: T) -> Bytes
pub fn encode_str[T: Json](v: T) -> Str
pub fn decode[T: Json](data: Bytes) -> T ! DecodeErr
pub fn decode_str[T: Json](s: Str) -> T ! DecodeErr
pub fn encode_indent[T: Json](v: T, prefix: Str = "", indent: Str = "  ") -> Bytes      // Go's MarshalIndent
pub fn encode_indent_str[T: Json](v: T, prefix: Str = "", indent: Str = "  ") -> Str
pub fn indent(text: Str, prefix: Str = "", indent: Str = "  ") -> Str ! DecodeErr      // Go's Indent over a text (the whitespace after the value is kept)
pub fn compact(text: Str) -> Str ! DecodeErr                                            // Go's Compact

struct DecodeErr { path: Str, msg: Str }    // displays as "at $.user.email: expected string"

enum Patch[T] { Absent, Null, Value(v: T) } derive Json
fn (p: Patch[T]) apply_to(current: T) -> T    // Value(v) gives v; Absent and Null keep current
fn (p: Patch[T]) apply_opt(current: T?) -> T? // Absent keeps, Null clears, Value(v) sets
```

`derive Json` rules:

| Xo | JSON |
|---|---|
| struct | object, keys are field names, or a field's wire name (`kind as "type": Str`), or the `derive Json(case: camel \| pascal \| kebab \| snake)` rule (core 2.2, decision 0277) |
| `derive Json(match: fold)` struct or enum | keys matched as Go's encoding/json does: an exact key, else the first declared field whose key folds equal; the last selecting member wins; errors in Go's document order (core 2.2, decision 0550) |
| newtype (`type Email struct(Str)`) | its underlying value's JSON (core 2.8) |
| `T?` field | missing key or `null` decodes to `None`; `None` encodes as `null` |
| field with a default | missing key decodes to the default |
| other missing field | `DecodeErr` |
| `Patch[T]` field | missing key is `Absent`, `null` is `Null`, value is `Value(v)` |
| unknown keys | ignored |
| `Int`, `Float`, `Bool`, `Str` | number, number, boolean, string |
| `Int8`, `Int16`, `Int32`, `UInt8`, `UInt16`, `UInt32`, `UInt64`, `Float32` | number; decoding refuses a number out of the type's range (`expected integer` for a signed type, `expected non negative integer` for an unsigned one, `cannot unmarshal number 1e39 into Float32` for a Float32 past its largest value, Go's words with Xo's type), on every backend |
| `List[T]`, `Set[T]` | array |
| `Map[Str, V]` | object, keys sorted by their bytes as Go's encoding/json writes them (decision 0543; a `json.Value` object keeps its order) |
| valued enum (`enum E: Int` or `enum E: Str`, decision 0269) | the variant's value: a number or a string; decoding refuses a value no variant has (`unknown value 7 of enum Weekday`), or, with a catch-all variant (`Unknown(value: Str)`, decision 0493), keeps it there and writes it back unchanged |
| enum unit variant | string `"Variant"` (its wire name: `as "name"` or `case:`, decision 0478, core 2.3) |
| enum variant with fields | object `{"Variant": {fields}}`, the name and the fields' keys as above |
| tagged enum, `derive Json(tag: "type")` | object `{"type": "Variant", fields...}`, a wrapped struct's fields beside the tag; with `content: "data"`, `{"type": "Variant", "data": payload}` (decision 0478, core 2.3) |
| a type with its own form, `derive Json(via: P, to: f, from: g)` | P's encoding of `f(v)`; decoding decodes a P at the same path and calls `g`, whose error's Display text is the `DecodeErr` message (decision 0478, core 2.3) |
| tuple `(A, B, ...)` | array `[a, b, ...]`; decodes only from an array of exactly that length (decision 0088) |
| `()` | `null` |
| `Duration` | string `"1.5s"` (any `Duration.parse` form decodes) |
| `Time` | RFC 3339 string (decoding refuses a time outside the range of `Time`, decision 0364) |
| `Bytes` | base64 string |
| `json.Value` | any JSON value as it is (Go's `any` and `RawMessage`; decision 0408 S3): encodes to its own text, decodes to the node (a `null` is `Value.Null`) |

Builtin types listed above implement `Json`, so `http.json(200, {"msg": "hi"})`
works without a struct.

- `encode_indent` is `encode` then `indent` (Go's MarshalIndent): every
  element and member on its own line, prefixed by `prefix` and one
  `indent` per level, `: ` after a key, an empty `[]` or `{}` as it is;
  `indent` over a text keeps every number's and string's spelling (the
  escapes of a string too) and drops the text's own whitespace; `compact`
  drops it and nothing else. A text that is not JSON is `parse_value`'s
  `DecodeErr`.

---

### 5.1 Values of any shape (decision 0299)

```
enum Value { Null, Bool(b: Bool), Int(n: Int), Num(n: Float), Str(s: Str), Trusted(kind: Str, s: Str), List(items: List[Value]), Object(fields: List[(Str, Value)]) }
pub fn to_value[T: Json](v: T) -> Value      // v's encoding as a Value
pub fn parse_value(text: Str) -> Value ! DecodeErr
fn (v: Value) encode() -> Str
fn (v: Value) get(key: Str) -> Value?        // an object's field
```

- A number out of Float's range (`1e999`, `-1e309`) is an error at its
  path, `cannot unmarshal number 1e999 into Float` (Go's words with Xo's
  type), as `decode` of a Float gives; an underflow (`1e-999`) is `0.0`.
- Go's decoding into `any`, with two differences: a number without a
  fraction or exponent that fits an Int is `Int` (Go gives float64, which
  loses ids above 2^53), any other `Num`; an object keeps its keys in the
  order of the text, each key once at its first position with its last
  value (as `decode` reads objects; Go's map has no order).
- Errors are `decode`'s (`at $: invalid JSON: ...`, Go's texts).
- Arrays and objects nest at most 10,000 deep (Go's limit, also a bound on
  hostile input): the 10,001st opening bracket is `invalid character '['
  exceeded max depth` (`'{'` for an object), on every backend. Golden
  `json_depth`.
- `encode` writes the parts as `encode` does (strings escaped, numbers,
  objects in their order); `parse_value(v.encode())` gives `v` back.
- Uses: JSON whose shape varies (webhook bodies), and std/template's data
  (decision 0299).
- Golden `json_value` (every backend).

### 5.2 Streams of values (decision 0309)

```
struct Decoder {}                                   // move only
pub fn decoder(max_value: Int = 67108864) -> Decoder
fn (var d: Decoder) write(data: Bytes) -> List[Value] ! DecodeErr   // the values completed so far
fn (var d: Decoder) finish() -> List[Value] ! DecodeErr            // the rest
pub fn from_value[T: Json](v: Value) -> T ! DecodeErr
```

- Go's json.Decoder reading `any`, push style (as std/compress's
  Decompressor, decision 0305): bytes go in, finished values come out.
  The input is JSON values one after another with optional whitespace
  between them: NDJSON, concatenated objects, numbers separated by
  spaces.
- Values end where Go's scanner ends them: `01` is the values 0 and 1,
  `truex` is `true` and then an error. A value that the bytes so far do
  not finish (a number at the very end may still go on) waits for more,
  or for `finish`, where it is `invalid JSON: unexpected EOF`.
- Each value is read as `parse_value` reads it, with its errors; invalid
  UTF-8 in a string is U+FFFD, as in Go. As Go's Decoder returns the
  values before an error first, a `write` that finds an error after some
  values returns them, and the next call returns the error.
- A value longer than `max_value` bytes is `invalid JSON: value exceeds
  max_value (N bytes)`, so a stream that never closes a value is bounded
  (Go has no bound).
- Tested against Go's json.Decoder on 1,200 generated streams, some cut
  short or with a byte changed, written in random pieces
  (internal/stdtest/testdata/corpus/json, scripts/stdcorpus).
- `from_value` decodes a Value into T as `decode` decodes its text.
- `derive Json` fields marked `as "key,omitempty"` (core 2.2) are left
  out of `encode`'s output when empty.
- Goldens `json_stream` and `json_omitempty` (every backend).
- The wire option `string` (decision 0555, core 2.2), `as "n,string"`,
  writes a scalar field (`Bool`, an integer, `Float`, `Float32`, `Str`,
  or an `Option` of one) as a JSON string holding its literal, as Go's
  `json:",string"` does: `5` as `"5"`, `true` as `"true"`, a `Str` quoted
  twice (`"\"abc\""`), `None` as `null`. `omitempty` is decided before
  quoting. Decoding wants a JSON string or `null`: its text, with no
  space around it, must be exactly a JSON literal of the field's type;
  otherwise `DecodeErr` with `invalid use of ,string struct tag, trying to
  unmarshal "TEXT" into T` (an unquoted value: `... unquoted value into
  T`), a number out of range keeping its own message. `null`, or the
  string `"null"`, goes to the field's own decoder as `null` does (an
  `Option` becomes `None`). `to_value` and `from_value` follow.
  Golden `json_string` (every backend).

### 5.3 Tokens (decision 0495)

```
enum Token { ObjectStart, ObjectEnd, ArrayStart, ArrayEnd, Key(s: Str), Str(s: Str),
             Number(text: Str), Bool(b: Bool), Null }
struct TokenReader {}                               // move only
pub fn token_reader(max_token: Int = 67108864) -> TokenReader
pub fn tokens(data: Bytes) -> TokenReader           // a whole text, read by next
fn (var r: TokenReader) write(data: Bytes) -> List[Token] ! DecodeErr
fn (var r: TokenReader) finish() -> List[Token] ! DecodeErr
fn (var r: TokenReader) next() -> Token? ! DecodeErr
fn (var r: TokenReader) more() -> Bool
fn (r: TokenReader) offset() -> Int
fn (var r: TokenReader) skip() -> () ! DecodeErr
fn (var r: TokenReader) value() -> Value ! DecodeErr

struct JsonWriter {}                                // move only
pub fn json_writer(escape_html: Bool = true) -> JsonWriter
fn (var w: JsonWriter) object_start() / object_end() / array_start() / array_end()
fn (var w: JsonWriter) key(name: Str) / str(s: Str) / int(n: Int) / bool(b: Bool) / null()
fn (var w: JsonWriter) float(f: Float) / number(text: Str) / raw(text: Str) / value(v: Value) -> () ! DecodeErr
fn (var w: JsonWriter) finish() -> Bytes
```

- The reader is the state machine of Go's `Decoder.Token` (stream.go:
  tokenState, tokenStack, tokenError), so tokens, errors (`invalid
  character ',' after array element`), `more` (Decoder.More) and `offset`
  (Decoder.InputOffset) are Go's. Differences: an object member's name
  is `Key`, a number is its text as written (Go's UseNumber), and the
  input that ends inside an array or object is `invalid JSON: unexpected
  EOF` (Go returns io.EOF there, as at a clean end). The empty text and
  white space give None. Values follow one another, as in 5.2.
- `tokens(data)` reads a text in memory with `next`; `write` and `finish`
  take bytes in pieces, as `Decoder` does, with the same rule for an error
  after some tokens (the next call returns it) and `max_token` bounding a
  string or number that is not complete.
- Strings are read as `decode` reads them (invalid UTF-8 and lone
  surrogate escapes are U+FFFD). `skip` steps over the next value (an
  array or object whole, found by its brackets and then checked) and
  `value` reads it as `parse_value` does.
- The writer inserts commas and colons and writes strings as encoding/json
  does (`"`, `\`, control characters, U+2028, U+2029 and invalid
  UTF-8 escaped; `<`, `>`, `&` as `\u003c`, `\u003e`, `\u0026` when `escape_html`) and
  floats as Go's floatEncoder (shortest form, exponent below 1e-6 or from
  1e21). `float` refuses NaN and infinities, `number` text that is not a
  JSON number, `raw` text that is not JSON (it is written compacted). A call
  out of order (a value where a key is due, `array_end` with no open
  array, `finish` with one open) faults. Top level values are separated by
  a newline.
- Tested against Go's Decoder.Token, More and InputOffset on 1,200
  generated streams and 55 hand picked ones, read whole and in random
  pieces, and against Marshal's strings and floats
  (internal/stdtest/testdata/corpus/jsontoken, scripts/stdcorpus);
  twin bench/perf/jsontoken.

## 6. std/log

The log functions perform the `log` effect (core 6.1, decision 0156): no
capability (there is nothing to pass from `main`), but a `pub` function
that logs declares `uses log`. `configure` performs `stdio`, not `log`.

```
pub fn debug(msg: Str, fields: Map[Str, Debug] = {}) uses log   // also info, warn, error
pub fn configure(out: Stdio, level: Level = Level.Info, format: Format = Format.Text) uses stdio
enum Level { Debug, Info, Warn, Error }
enum Format { Text, Json }
struct Record { level: Level, msg: Str, fields: Map[Str, Str], module: Str }   // as captured in tests (8.6)
```

```
log.info("listening", {"addr": addr, "port": port})
```

`enabled(level: Level) -> Bool` (decision 0558) says whether a record of
`level` would be written under the configured level. A call to `debug`,
`info`, `warn` or `error` (and the same methods of a `Logger`) is compiled
as `if enabled(level) { call }`: below the configured level its arguments
(the message, the fields and everything inside them) are not evaluated,
so an argument's side effect does not run, unlike Go's `log/slog`. A call
through a function value is not guarded (the callee's own check drops the
record). Golden `log_lazy` (every backend).

A `Logger` carries fields onto every record it writes (decision 0413,
log/slog's `Logger.With`):

```
struct Logger {}
pub fn with(fields: Map[Str, Debug]) -> Logger
fn (l: Logger) with(fields: Map[Str, Debug]) -> Logger         // the logger's fields plus these
fn (l: Logger) info(msg: Str, fields: Map[Str, Debug] = {}) uses log   // also debug, warn, error
```

```
let rl = log.with({"req": req_id})
rl.info("start")                       // ... INFO start req=r1
rl.warn("slow", {"ms": 120})           // ... WARN slow req=r1 ms=120
```

A record has the logger's fields first, in the order they were given
(a derived logger's after its parent's), then its own; a record's field
replaces the logger's value of the same name in the logger's place. A `Logger` is
a value: copies are independent and it is safe to share between tasks.
Records go to the same sink, level and format as the module functions'
(`configure` applies to both). A logger made in a dependency's code
names that dependency (below), wherever its methods are called.

Every record includes the time. A record written by the code of a
dependency (a required module version, not the main module or a
workspace module) names it, `<module path>@<version>` (decision 0173):
text records put it in brackets after the level
(`2026-10-11T09:00:00Z WARN [github.com/acme/billing@v1.2.0] retrying`),
JSON records as the key `"module"`, which replaces a field of the same
name (a dependency cannot attribute its record to another module), and
`Record.module` holds it in tests (`""` for the main module's records).
The call's source file decides: a closure of the main module run by a
dependency logs as the main module. Records of the main module carry no
attribution, on every backend (cmd/xo TestLogAttribution checks the
text and JSON records and `Record.module` on Go, arm64, and LLVM).
`xo audit` lists each dependency's log
call sites with the attribution its records carry.

**[later]** The current span and the
request id (core 11.6) are not attached yet. Without `configure`, logs go
to standard error as text at `Info`.

Under `xo test` (decision 0057) each test has its own log sink, starting at
`Info` with no output configured. Records that would go to standard error
are buffered instead and printed under the test's result only when the
test fails (or with `xo test -v`), like `t.Log` in Go; `--json` adds them
as a `log` array to the test's line. `log.configure(os.stdio, ...)` in a
test (or in a `main` run by `testing.run_main`) sends records to the fake
standard error instead, as in a program. Either way every record that
passes the level is also kept for `testing.logs()` (section 8.6). Nothing
logged in a test reaches the real standard error.

---

## 7. std/task

**Resource budgets** (decision 0173). `task.with_budget` bounds the work
of a call, typically a dependency's, against compute and memory denial of
service: `cpu` is running time (time its tasks spend outside blocking
waits, measured on the real monotonic clock, also under a `FakeClock`),
`alloc` is bytes allocated, and `steps` counts loop iterations and calls,
a deterministic unit. The budget covers every task started inside the
call, wherever it is started, and nested budgets all apply. Code polls
the budget at the start of every loop iteration and at the entry of
every function that calls user code, so under a budget a pure loop or a
recursion is a cancellation point. The polls are compiled only into a
program that calls `with_budget` somewhere (one without a budget is
unchanged); there they cost a nil check outside a budget. Ceilings are checked every 1024 steps of a thread (the last
quantum before a `steps` ceiling is shortened to hit it exactly).
Deferred code runs shielded and is not unwound; budgeted shielded code
that goes past twice a ceiling faults (`XO-F008`). Limits: on the Go
backend `alloc` is the process wide allocation since the budget started
(Go keeps no count per goroutine), an upper bound that also counts other
tasks running at the same time; one std operation (a large `repeat`, a
sort) is not interrupted; functions compiled to assembly by `--hybrid`
do not poll. The native backends (`--no-gc`, decision 0175) poll at
every loop iteration and at the entry of every function of the
program's own modules (leaf functions too, so a step count runs a little
higher than on the Go backend) and count `alloc` exactly per task (the
bytes each task allocated); `XO-F008` there names the `with_budget` call
as its location.

```
enum Timed[E] { TimedOut, Failed(from E) } derive Display
enum Isolated[E] { Faulted(info: Fault), Failed(from E) } derive Display
struct Fault { msg: Str, location: Str, frames: List[Str] } derive Display

pub fn with_timeout[T, E, fx: Effects](clock: Clock, d: Duration, f: fn() -> T ! E uses fx) -> T ! Timed[E]
    uses (clock, fx)
    // runs f in a child scope; on expiry cancels it, waits for it to unwind, returns Err(TimedOut)

pub fn isolate[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> T ! Isolated[E] uses fx

enum Budgeted[E] { Exceeded(resource: Str), Failed(from E) } derive Display
struct Budget { cpu: Duration, alloc: Int, steps: Int }    // 0: no ceiling
pub fn budget(cpu: Duration = 0s, alloc: Int = 0, steps: Int = 0) -> Budget
pub fn with_budget[T, E, fx: Effects](clock: Clock, b: Budget, f: fn() -> T ! E uses fx) -> T ! Budgeted[E]
    uses (clock, fx)
    // runs f in a child scope under the budget; when a ceiling is passed cancels
    // the scope, waits for it to unwind, returns Err(Exceeded("cpu" | "alloc" | "steps"))

pub fn is_canceled() -> Bool
pub fn stop_requested() -> Bool          // the first shutdown signal arrived (core 7.5)
pub fn stopping() -> Chan[()]            // closed when a stop is requested
pub fn deadline(clock: Clock) -> Time? uses clock   // decision 0430: Go's ctx.Deadline()
    // the nearest with_timeout deadline the current task works under, on clock;
    // None outside every with_timeout. A spawned task keeps the deadline in force
    // where it was spawned, also after that with_timeout's body returned

pub fn map_concurrent[T, R, E, fx: Effects](items: List[T], limit: Int, f: fn(T) -> R ! E uses fx)
    -> List[Result[R, E]] uses fx
    // runs f on every item with at most `limit` at once; results are in input order;
    // one failure does not stop the others (use with_timeout for a deadline)

struct Group {}                              // handle; see core 7.1
fn (g: Group) spawn[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> Task[T, E] uses fx
fn (g: Group) spawn_background[E, fx: Effects](f: fn() -> () ! E uses fx) uses fx

pub fn map_concurrent_until[T, R, E, fx: Effects](clock: Clock, d: Duration, items: List[T], limit: Int,
    f: fn(T) -> R ! E uses fx) -> List[Result[R, Timed[E]]] uses (clock, fx)
    // like map_concurrent, but items not finished after d are cancelled and give
    // Err(TimedOut); finished results are kept, in input order

pub fn once[T, E]() -> Once[T, E]           // Once is a handle: copies share it
fn (o: Once[T, E]) get[fx: Effects](f: fn() -> T ! E uses fx) -> T ! E uses fx
    // the first caller runs f; concurrent and later callers wait for and share its
    // result. If the running caller is cancelled before f finishes, the next
    // waiter runs f instead. A failed result is shared too; use a new Once to retry.
    // A fault in f is shared with every waiter as well.

pub fn semaphore(permits: Int) -> Semaphore  // Semaphore is a handle: copies share permits
fn (s: Semaphore) with[R, E, fx: Effects](f: fn() -> R ! E uses fx) -> R ! E uses fx
    // waits for a permit (cancellation point), runs f, releases the permit
```

When `f` cannot fail, `E` is `Never` and the `Failed` variant can be left out of
a `match` (core 2.3).

---

## 8. std/testing

`os`, `expect`, and `testing` are predeclared in test blocks; `expect` and
`testing` are also available anywhere in `_test.xo` files (core 9.2).

Native test binaries (`xo test --no-gc`, decision 0083) give the same fakes:
the same virtual clock (from 2026-01-01T00:00:00Z), the same seeded `Rand`
values, in memory listeners on ports from 49152, FakeNet handlers and
failures, captured streams and log records, MemFs (decision 0108),
`set_input` and reading standard input, FakeProc's `run` and `on_run`
(decision 0107), and `os.tasks` (canceled when the test ends, decision
0109). `os.proc.exit` ends the test (it passes), and inside `run_main`
unwinds main back to it. `testing.gen` and property tests make the Go
backend's values for a seed, and a failing property test reports the
same case and shrunk input (decision 0120). With `xo test --real` a test
gets real capabilities for the effects it declares (with `clock`, `net`,
or `proc` its clock is the real one). Many tests share a process, the
fakes starting over for each, and a test the process does not survive
runs again alone (decision 0235). `xo test --timeout=D` limits each
test's real time on every backend (default 60s, 0 for none, core 9.2);
natively the harness kills the process of a test past it.

```
struct TestOs {
  net: FakeNet
  fs: MemFs
  clock: FakeClock
  rand: Rand          // seeded from the test seed
  env: FakeEnv
  stdio: FakeStdio
  proc: FakeProc
  tasks: task.Group   // lives for the test; cancelled when the test ends
}
```

Each fake satisfies its capability interface and adds control methods.
`only` on `FakeNet`, `FakeEnv`, and `FakeProc` attenuates the fake as
on the real capability (3.2, 3.5, 3.7): a test can check that a
library given `os.net.only([...])` fails with `Denied` elsewhere.

### 8.1 FakeNet

```
fn (n: FakeNet) handle(pattern: Str, h: fn(http.Request) -> http.Response)
    // pattern: "[METHOD ]<absolute url pattern>", e.g. "GET https://api.test/users/{id}"
    // a second handle with the same method, URL prefix and path pattern replaces the first (last wins; Go's ServeMux panics)
fn (n: FakeNet) handle_tls(pattern: Str, cert: tls.TlsCert, h: fn(http.Request) -> http.Response)
    // handle for https with the server's certificate: the client's checks run first (decision 0544)
fn (n: FakeNet) fail(pattern: Str, err: http.ClientErr)       // every matching request, until clear_failures
fn (n: FakeNet) fail_next(pattern: Str, err: http.ClientErr)  // the next matching request only
fn (n: FakeNet) clear_failures()
fn (n: FakeNet) requests() -> List[http.Request]     // every request seen, including failed ones
```

- `handle_tls` runs, for an `https` request it matches, the checks of a real handshake for the client against `cert`
  before the handler: the chain against the client's roots (`with_roots`, else the system's) and the name against the
  leaf (the URL's host, or `with_server_name`), as `with_verify` allows (`Verify.None` skips both and needs a
  `with_verify_conn`, `ChainOnly` skips the name), then `with_verify_conn`. A refusal fails the request with
  `ClientErr.Connect` and the handshake's text before the handler runs. The handler's `req.tls_state()` is `Some` with
  version "TLS 1.3", the cipher suite and `server_name` (the SNI the client sent, "" for an IP address); a plain `handle`
  on an `https` URL is not checked. A FakeNet handler's `req.host()` is the request's own (`with_host`), else the URL's
  host and port as written (Go's `Request.Host`; decision 0544).
- `http.client(os.net)` calls registered handlers in process. Unhandled
  requests fail with `ClientErr.Connect("no fake handler for GET ...")`.
- `os.net.listen(":0")` gives an in memory listener, so a real `http.server`
  can be tested end to end with `http.client(os.net)`. Its `addr()` is
  `127.0.0.1:49152`, then `:49153`, ... A request to a listener whose
  `serve` has not started yet waits until it starts, like a connection in
  a real socket's backlog (a test that never serves it reports a deadlock
  naming the wait); after `serve` returns, requests fail with
  `ClientErr.Connect`.
- `Client.send` reaches a handler or a served router as one call, and
  the whole response comes back, an `http.stream` body made in full
  (2026-11-07: a `handle` handler's stream came back empty natively).
  `Client.open` runs the handler in its own task (2026-11-07 for every
  request; from 2026-11-04 for one whose body is a source, `http.pipe` or
  `with_body_reader`, which is then full duplex): the request body is the
  source or the whole body, the head comes back when the handler returns
  its response, and a streamed response's writes reach `read` as they are
  flushed, so a test can read one server-sent event before the handler
  writes the next, or write, read the answer and write again before it
  closes the pipe. Only tasks and channels carry it, so the test
  scheduler's seed and replay hold.
- Raw connections (decision 0292): `os.net.dial(addr)` connects to the
  open in memory listener at `addr` and returns at once, the connection
  waiting in the listener's backlog until `accept()` returns its other
  end; without one it fails with `Refused`. `listen` takes a host name
  too (`listen("db.test:5432")` listens at exactly that address, so a
  program dialing its configured host reaches the fake), and fails with
  `Io("address already in use")` while another listener has the
  address. What one end writes the other reads, in order; a read returns
  what is buffered (at most the size asked) and waits while nothing is;
  each direction buffers 64 KiB and a write waits for room. Closing one
  end ends the other's reads (`None` after the bytes it holds) and fails
  its writes (`Io("write tcp: connection reset by peer")`); closing the
  listener resets the connections still in its backlog. Reads, writes,
  and accept are task waits (cancellation, the virtual clock, the
  deadlock report), and `tls.client` passes the bytes through as on
  every FakeNet connection (27).

### 8.2 FakeClock

Virtual time starting at `2026-01-01T00:00:00Z`. When every task in the test is
blocked, time jumps to the next timer. Timers due at the same instant fire in
the order they were scheduled. Time does not move while cancelled tasks
unwind. `advance(d)` fires every timer due within `d`, in order, and lets the
woken tasks run before returning.

```
fn (c: FakeClock) advance(d: Duration)
fn (c: FakeClock) set(t: Time)
fn (c: FakeClock) sleeps() -> List[Duration]          // every sleep requested, in order
// FakeClock also has sleep_or_cancel (3.3), on virtual time
```

### 8.3 MemFs

In memory, case sensitive by default. `write` creates missing parent
directories. `write` sets the modified time to the
virtual clock's now; `rename` keeps the source's modified time. Paths in
`fail_next` and `files` are relative to the test's `os.fs` root; `fail_next`
matches an operation if any of its path arguments equals the given path
(for `rename`, either the source or the target).

```
enum FsOp { Read, Write, Rename, Remove, CreateDir, ReadDir }
fn (f: MemFs) fail_next(op: FsOp, path: Str, err: FsErr)   // next matching op fails once; `*` matches any run of characters within a segment
fn (f: MemFs) set_case_sensitive(on: Bool)
fn (f: MemFs) set_disk_usage(u: DiskUsage)       // what disk_usage answers (decision 0426); all zero before
fn (f: MemFs) files() -> Map[Str, Bytes]                  // every file, by path
```

MemFs implements the handles, links, globs, and locks of 3.1: a read
handle reads the contents as they were at `open`; a write handle writes
through at once; permission bits default to `0o644` for a file and
`0o755` for a directory; `read`, `read_text`, `open`, `copy`, `stat`, and
`exists` follow symbolic links (a chain of more than 40 is `Io`); a lock
held by one task makes another task's `lock` wait (deadlocks are
reported as for a `Mutex`). `fail_next` applies with `Read` (open, copy)
and `Write` (create, open_append, append, set_mode, symlink, lock).

### 8.4 FakeEnv, FakeStdio, FakeProc

```
fn (e: FakeEnv) set(name: Str, value: Str)
fn (e: FakeEnv) set_args(args: List[Str])
fn (s: FakeStdio) set_input(text: Str)
fn (s: FakeStdio) set_input_bytes(data: Bytes)        // set_input for any bytes
fn (s: FakeStdio) set_tty(width: Int)                 // the streams are terminals of width columns (0: not)
fn (s: FakeStdio) output() -> Str                     // everything printed to stdout
fn (s: FakeStdio) errors() -> Str                     // everything printed to stderr
fn (p: FakeProc) exit_code() -> Int?                  // set when exit was called
fn (p: FakeProc) signal_after(d: Duration)            // next run_main gets SIGTERM after d (virtual time)
fn (p: FakeProc) on_run(cmd: Str, out: ProcOutput)    // also answers Cmd runs (3.8)
fn (p: FakeProc) killed() -> List[Str]                // the signals kill recorded, "NAME PID" (3.7)
```

`FakeProc.kill(pid, signal)` checks its arguments as `Proc.kill` does and
records the signal instead of sending it (decision 0190).

In a test, `os.proc.exit(code)` records the code and unwinds the code under
test back to the test block, which continues.

```
pub fn run_main[fx: Effects](main: fn(Os) -> () ! Error uses fx, os: TestOs, args: List[Str] = []) -> Int
    uses fx
    // runs a program's main against fakes and returns its exit status
    // (0, 1 for an error returned from main, or the code passed to exit)
```

Std enums are named with their module (`testing.FsOp.Rename`); the prefix may
be omitted when the expected type is known (`fail_next(Rename, ...)`).

### 8.5 Generators

Property tests (`test "x" for v: T`) generate values for any type built from
builtin types, structs, and enums. `testing.gen[T](seed: Int) -> T` produces
one value directly.

### 8.6 Captured logs

```
pub fn logs() -> List[log.Record]     // records logged so far in this test, oldest first
```

A record's `fields` hold each value's text form, in the order given. Only
records at or above the sink's level are kept (`Info` unless the test
calls `log.configure`). Each test, and each case of a property test,
starts with none; tasks of the test and a `main` run by `run_main` log to
the same list. Outside a test, `logs()` is empty.

```
test "a failed purge is logged" {
  purge_once(broken_db(), os.clock)
  let recs = testing.logs()
  expect(recs.map(fn(r) { r.msg })) == ["purge failed"]
  expect(recs[0].level) == log.Level.Warn
}
```

---

## 9. std/path

Pure helpers for `/` separated paths, the form every `Fs` capability uses on
every OS (section 3.1).

```
pub fn join(a: Str, b: Str) -> Str   // joins and cleans
pub fn split(p: Str) -> (Str, Str)   // dir, to and including the last "/", and file
pub fn dir(p: Str) -> Str            // all but the last element, "." if none
pub fn base(p: Str) -> Str           // the last element
pub fn ext(p: Str) -> Str            // extension with the dot, or ""
pub fn clean(p: Str) -> Str          // removes `.`, `..`, repeated `/`
pub fn is_abs(p: Str) -> Bool        // starts with "/"

struct PathErr { pattern: Str }      // a pattern `match` cannot parse
pub fn is_match(pattern: Str, name: Str) -> Bool ! PathErr
                                     // `*` any run of non-`/`, `?` one non-`/`,
                                     // `[a-z]` and `[^a-z]` a byte class, `\c` c
```

`is_match` matches the whole of `name`, and a malformed pattern is `PathErr`
(Go's `path.ErrBadPattern`), reported even when `name` cannot match.

Go's path package's results on every backend. Every backend runs one
implementation, std/native/path.xo: the Go backend compiles it too
(std/go_compiled.txt, decision 0312), and `clean` is Go's algorithm.

---

## 10. std/sql

Status: draft (2026-10-08), implemented on every backend: the in memory
driver (`sql.memory`, ported to Xo for native builds, decision 0083, with
the same results and error texts; `max_open` does not bound it, as it
runs one statement at a time) and drivers written in Xo (decision 0278,
10.7). `sql.open(net, "postgres", dsn)` opens Postgres natively (10.8);
on the Go backend, and for any other driver name on every backend,
`sql.open` reports an unknown driver. A `Row`'s Debug text is its
columns and values on every backend, `Row(a: 1, b: "x", c: null)`;
`value` gives each column's value.

### 10.1 Opening

```
struct Db {}                                 // handle: copies share the database

pub fn open(net: Net, driver: Str, dsn: Str,
            max_open: Int = 10, max_idle: Int = 2, max_lifetime: Duration = 30m)
    -> Db ! SqlErr uses net.dial
pub fn memory(schema: Str = "") -> Db ! SqlErr         // no capability, no effect
// std/testing: fake_db(schema: Str) -> Db ! SqlErr    // same as memory(schema)
```

A network driver is opened from the `Net` capability, so a function that is not
given a `Net` (or an already opened `Db`) cannot reach a database. The in
memory driver performs no I/O and needs no capability; it is what tests use.
`schema` is a `;` separated list of statements run on the fresh database.

Pooling: `max_open` bounds the operations and open transactions in flight on a
`Db`; a task over the limit waits (a cancellation point, it gets `Canceled`).
`max_idle` and `max_lifetime` are passed to the driver (the in memory driver
ignores them). `max_open` below 1 is `Param`.

Every operation on a `Db` or `Tx` declares `uses net.dial`, because the `Db`
may be network backed. Declaring more than the in memory driver performs is
allowed (core 6.2), so the same code runs against both.

### 10.2 Parameters and values

```
enum SqlValue {
  Null, Integer(n: Int), Text(s: Str), Real(f: Float), Flag(b: Bool), At(t: Time),
  Blob(b: Bytes),                    // bytea, BLOB (decision 0278)
  JsonText(s: Str),                  // json, jsonb: the JSON text
  Array(items: List[SqlValue]),      // a one dimensional array
}
pub fn null() -> SqlValue
pub fn int(n: Int) -> SqlValue       // also str, float, bool, time
pub fn bytes(b: Bytes) -> SqlValue
pub fn json[T: Json](v: T) -> SqlValue             // JsonText of json.encode_str(v)
pub fn array(items: List[SqlValue]) -> SqlValue
```

Parameters are written `$1`, `$2`, ... and passed as a `List[SqlValue]`
(`[sql.str(name), sql.int(age)]`). Decision: a closed enum rather than
`List[Display]`, because the driver needs the type of each value (an `Int` and
its text look the same through `Display`), and the five variants cover the
column types every driver has. The count must equal the highest placeholder
number, else `Param`. Values are never interpolated into the statement by the
library; build the statement text from constants.

### 10.3 Queries

```
fn (d: Db) query(q: Str, args: List[SqlValue] = []) -> List[Row] ! SqlErr uses net.dial
fn (d: Db) query_one(q: Str, args: List[SqlValue] = []) -> Row ! SqlErr uses net.dial   // NoRows if empty; extra rows ignored
fn (d: Db) exec(q: Str, args: List[SqlValue] = []) -> ExecResult ! SqlErr uses net.dial
fn (d: Db) ping() -> () ! SqlErr uses net.dial
fn (d: Db) close() -> () ! SqlErr               // later operations fail with Closed; idempotent

struct ExecResult { rows_affected: Int, last_insert_id: Int }   // id is 0 when the driver has none
```

`query` runs a statement that returns rows, `exec` one that does not. A
write with a `RETURNING` clause returns rows, so it goes through `query`
(or `query_one`); through `exec` its rows are dropped.

A result is read as a cursor with `rows` (decision 0479, Go's
`*sql.Rows`):

```
fn (d: Db) rows(q: Str, args: List[SqlValue] = []) -> Rows ! SqlErr uses net.dial
fn (t: Tx) rows(q: Str, args: List[SqlValue] = []) -> Rows ! SqlErr uses net.dial
fn (r: Rows) columns() -> List[Str]
fn (r: Rows) next() -> Row? ! SqlErr uses net.dial          // None after the last row
fn (r: Rows) next_as[T: SqlRow]() -> T? ! SqlErr uses net.dial
fn (r: Rows) close() uses net.dial                          // idempotent
fn (d: Db) each(q: Str, args: List[SqlValue], f: fn(Row) -> () ! SqlErr) -> () ! SqlErr uses net.dial
```

The Postgres driver reads the rows off the socket as `next` asks, so a
result larger than memory can be walked (the server waits while the
client does not read); the in memory database and the Go backend read the
result whole and hand it out row by row. `rows` holds a pool connection
from the first row to the end: `next` returning `None` or an error
releases it, and `close` before the end reads and discards the rest and
releases it. A `Rows` that is neither read to the end nor closed keeps the
connection, so use `defer rows.close()` or `each`, which closes in every
path (an error from `f` ends the walk and is returned). On a `Tx` the
transaction's connection is busy until the cursor ends or is closed.
`query` and `query_as` are `rows` read to the end.

`copy_from` (decision 0479, pgx's `CopyFrom`) loads rows with COPY FROM
STDIN in the text format:

```
fn (d: Db) copy_from(table: Str, columns: List[Str], rows: fn() -> List[SqlValue]? ! SqlErr) -> Int ! SqlErr uses net.dial
fn (t: Tx) copy_from(...) -> Int ! SqlErr uses net.dial
fn quote_ident(name: Str) -> Str                   // "name", a double quote doubled
```

`rows` is called until it returns `None`; each row is a list of values in
the order of `columns`, sent in CopyData messages of about 64 KiB; the
result is the number of rows copied. An error from `rows` sends CopyFail
(the server rolls the COPY back) and is returned. `table` and `columns`
are identifiers, quoted by the driver; values are never interpolated. The
in memory database and the Go backend insert the rows one by one (rows
copied before an error from `rows` stay; use a transaction). A driver
without COPY returns `Driver` from `DriverConn.copy_from`, which every
driver implements since decision 0479.

### 10.4 Rows

```
struct Row {}
fn (r: Row) columns() -> List[Str]
fn (r: Row) value(name: Str) -> SqlValue ! SqlErr  // the column's value as read (pgx's Rows.Values)
fn (r: Row) is_null(name: Str) -> Bool ! SqlErr
fn (r: Row) int(name: Str) -> Int ! SqlErr       // also str, float, bool, time
fn (r: Row) int_opt(name: Str) -> Int? ! SqlErr  // also str_opt, float_opt, bool_opt, time_opt
fn (r: Row) bytes(name: Str) -> Bytes ! SqlErr   // also bytes_opt
fn (r: Row) json[T: Json](name: Str) -> T ! SqlErr   // a json column (or text) decoded; a decode error is Mismatch
fn (r: Row) ints(name: Str) -> List[Int] ! SqlErr    // an array of integers; also strs; a NULL element is Mismatch
fn (r: Row) items(name: Str) -> List[SqlValue] ! SqlErr  // an array's elements
```

Accessors read by column name (the first column of that name). A missing
column is `NoColumn`; a value of another type, or NULL read by a non optional
accessor, is `Mismatch(column, msg)`. `float` also accepts an integer. The
`_opt` forms map NULL to `None`.

Rows into structs (decision 0061):

```
interface SqlRow {}                              // prelude marker; only `derive SqlRow` implements it
fn (r: Row) decode[T: SqlRow]() -> T ! SqlErr
fn (d: Db) query_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> List[T] ! SqlErr uses net.dial
fn (d: Db) query_one_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> T ! SqlErr uses net.dial
fn (t: Tx) query_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> List[T] ! SqlErr uses net.dial
fn (t: Tx) query_one_as[T: SqlRow](q: Str, args: List[SqlValue] = []) -> T ! SqlErr uses net.dial
```

`derive SqlRow` is allowed on a struct without type parameters whose fields
are `Int`, `Float`, `Str`, `Bool`, `Time`, `Bytes`, or an `Option` of one
of them, or `List[Int]` or `List[Str]` (array columns, read with `ints`
and `strs`; decision 0278), else XO0404 at the derive. `decode` reads
each field from the column of the same name, or of its wire name
(`created as "created_at": Time`), or under `derive SqlRow(case: ...)`
(core 2.2, decision 0277), with the rules of the accessors above: a
missing column is
`NoColumn(name)`, a value of another type is `Mismatch(column, msg)`, NULL is
`None` for an `Option` field and `Mismatch` for any other. Fields are read in
declaration order and the first failure is returned. Columns without a field
are ignored, so `SELECT *` works with a struct naming some of the columns.
Field defaults do not apply: every field needs its column. `query_as` is
`query` followed by `decode` of every row; `query_one_as` is `query_one`
followed by `decode` (`NoRows` when there is no row).

```
struct Note {
  id: Int
  owner: Int
  body: Str
  expires: Time
  tag: Str?
} derive SqlRow, Json

fn notes(db: sql.Db, owner: Int) -> List[Note] ! sql.SqlErr uses net.dial {
  db.query_as[Note]("SELECT * FROM notes WHERE owner = $1 ORDER BY id", [sql.int(owner)])
}

fn add(db: sql.Db, owner: Int, body: Str, at: Time) -> Note ! sql.SqlErr uses net.dial {
  db.query_one_as[Note]("INSERT INTO notes (owner, body, expires) VALUES ($1, $2, $3) RETURNING *",
    [sql.int(owner), sql.str(body), sql.time(at)])
}
```

### 10.5 Transactions

```
enum Isolation { Default, ReadUncommitted, ReadCommitted, RepeatableRead, Serializable }
fn (d: Db) begin(isolation: Isolation = Isolation.Default, read_only: Bool = false) -> Tx ! SqlErr uses net.dial
fn (t: Tx) query(q: Str, args: List[SqlValue] = []) -> List[Row] ! SqlErr uses net.dial
fn (t: Tx) query_one(q: Str, args: List[SqlValue] = []) -> Row ! SqlErr uses net.dial
fn (t: Tx) exec(q: Str, args: List[SqlValue] = []) -> ExecResult ! SqlErr uses net.dial
fn (t: Tx) commit() -> () ! SqlErr uses net.dial
fn (t: Tx) rollback() -> () ! SqlErr uses net.dial
```

A transaction holds one pool permit from `begin` until `commit` or
`rollback`; a driver that is a single writer (the in memory one) also allows
one open transaction at a time, later `begin` calls wait. Commit a second time,
or use a finished `Tx`, and you get `TxDone`. `rollback` on a finished `Tx`
does nothing and succeeds, so the cleanup pattern is:

```
let tx = db.begin()?
defer { tx.rollback().ignore_err("nothing to undo if the commit ran") }
tx.exec(...)?
tx.commit()?
```

`defer` blocks are shielded (core 3.5), so the rollback still runs when the
task is cancelled or the function returns an error. A statement that fails
inside a transaction leaves the transaction usable and its earlier work intact.
A `Tx` is a handle: copies share the transaction.

`isolation` and `read_only` are Go's `sql.TxOptions`. A driver's
connection runs `SET TRANSACTION ISOLATION LEVEL ..., READ ONLY` (the
modes asked for, in that order) right after `begin`, nothing for the
defaults; an error there rolls back and is `begin`'s error. The in memory
driver is serializable whatever the level, and in a read only transaction
refuses every statement but a SELECT with
`Server(code: "25006", msg: "cannot execute INSERT in a read-only transaction")`,
as Postgres does.

### 10.6 Errors and the in memory driver

```
enum SqlErr {
  Syntax(msg: Str), Constraint(msg: Str, code: Str, name: Str), Param(msg: Str), NoTable(name: Str),
  NoRows, NoColumn(name: Str), Mismatch(column: Str, msg: Str),
  Conn(msg: Str), Driver(msg: Str), Server(code: Str, msg: Str), TxDone, Closed, Canceled
}
```

`display` prints plain messages (decision 0358): `syntax error: MSG`, MSG for
`Constraint`, `Param` and `Driver`, `no such table: NAME`, `no rows in result
set`, `no such column: NAME`, `column COL: MSG`, `connection: MSG`, `MSG
(SQLSTATE CODE)` for `Server`, `transaction already committed or rolled back`,
`database is closed`, and `canceled`. `Debug` keeps the variant form.

`Conn` is a lost or refused connection, `Driver` any other driver failure
(including an unknown driver name), `Constraint` a key, uniqueness, NOT NULL,
check, or type violation, and `Server` any other error a server reports
with an SQLSTATE code. `Constraint`'s `code` is the SQLSTATE (decision
0278): `23505` unique, `23503` foreign key, `23502` not null, `23514`
check, `42804` a value of another type; `name` is the constraint's name,
`""` when the driver gives none (the in memory driver never does).

The in memory driver (`memory`, `testing.fake_db`) supports: `CREATE TABLE [IF
NOT EXISTS]` (types `INT`/`INTEGER`/`BIGINT`, `TEXT`/`VARCHAR`, `REAL`/`FLOAT`,
`BOOL`, `TIMESTAMP`, `SERIAL`, `BYTEA`/`BLOB` (`Blob` values), `JSON`/`JSONB`
(`JsonText` values), and an array of any of them written `TYPE[]` (`Array`
values whose elements are of TYPE or NULL); `PRIMARY KEY` on a single integer column
auto-assigns keys; `NOT NULL`, `UNIQUE`, `DEFAULT literal`), `DROP TABLE`,
`CREATE INDEX [IF NOT EXISTS] [name] ON table (column)` (one column; unnamed
it is `table_column_idx`; a `WHERE column = value` on an `INT` or `TEXT`
column then looks the rows up instead of scanning, in the scan's order;
`CREATE UNIQUE INDEX` is not supported, declare the column `UNIQUE`),
`DROP INDEX [IF EXISTS] name`,
`INSERT ... VALUES` (several tuples), `SELECT` of `*`, expressions with `AS`,
or `COUNT(*)` from one table with `WHERE`, `ORDER BY` (several keys, `ASC`/`DESC`),
`LIMIT`/`OFFSET`, `UPDATE ... SET ... WHERE`, and `DELETE ... WHERE`.
`INSERT`, `UPDATE`, and `DELETE` take `RETURNING` with the items of
`SELECT` (`*`, expressions, `AS`; not `COUNT(*)`), yielding the inserted
rows, the updated rows with their new values, or the deleted rows, in table
order; generated keys and defaults are filled in.
Expressions: columns, literals, `$n`, `+ - * /`, `= != <> < <= > >=`, `AND`,
`OR`, `NOT`, `IS [NOT] NULL`, `[NOT] IN (...)`, `[NOT] LIKE`. No joins,
grouping, or subqueries. Reads inside a transaction see its own
writes; reads outside see the last commit. A write through `query` that
fails changes nothing, like one through `exec`.
Results of a `Db` are not recorded by record and replay (core 11.3) yet.

### 10.7 Drivers written in Xo

Status: implemented natively and on the Go backend (decision 0278 stage
B, 2026-10-20); the Postgres driver (10.8) uses it.

```
interface Driver[fx: Effects] {
  fn connect(self, net: Net, dsn: Str) -> DriverConn[fx] ! SqlErr uses (net.dial, fx)
}
interface DriverConn[fx: Effects] {
  fn query(self, q: Str, args: List[SqlValue]) -> DriverRows[fx] ! SqlErr uses (net.dial, fx)
  fn exec(self, q: Str, args: List[SqlValue]) -> ExecResult ! SqlErr uses (net.dial, fx)
  fn begin(self) -> () ! SqlErr uses (net.dial, fx)      // also commit, rollback, ping
  fn copy_from(self, table: Str, columns: List[Str], rows: fn() -> List[SqlValue]? ! SqlErr) -> Int ! SqlErr uses (net.dial, fx)
  fn close(self) uses (net.dial, fx)
  fn is_broken(self) -> Bool                       // dropped from the pool when true
}
struct DriverRows[fx: Effects] { columns: List[Str], rows: List[List[SqlValue]], cursor: DriverCursor[fx]? = None }
interface DriverCursor[fx: Effects] {            // the rest of a result on the connection (decision 0479)
  fn next(self) -> List[SqlValue]? ! SqlErr uses (net.dial, fx)   // None after the last row
  fn rest(self) -> List[List[SqlValue]] ! SqlErr uses (net.dial, fx)
  fn close(self) uses (net.dial, fx)             // reads and discards the remaining rows
}

pub fn open_driver[fx: Effects](net: Net, d: Driver[fx], dsn: Str, max_open: Int = 10, max_idle: Int = 2,
                   max_lifetime: Duration = 30m) -> Db[fx] ! SqlErr uses (net.dial, fx)
```

- The effect parameter (decision 0534): a driver's operations may perform
  effects beyond `net.dial`, named by its `fx`, and `Db`, `Tx` and `Rows`
  carry them: std/sqlite's driver is a `Driver[fs]`, so its Db's
  operations perform fs. The Postgres driver and the in memory one add
  none (`Db[()]`). A `sql.Db` written without the argument is an implicit
  effect variable (core 6.2), so most code is unchanged; a function that
  captures a Db in a closure whose type fixes its effects (a handler or
  middleware returned as `fn(...) uses net.dial`) names the argument,
  `sql.Db[()]` for Postgres or `memory`, and so do the functions that
  pass that Db along (stability/notes). An interface's effect argument is
  not inferred from the struct that implements it, so a driver's
  constructor returns `sql.Driver[fx]` with its effects written.

- A driver is a value passed to `open_driver`; there is no registry of
  names (a package global). std/sql gives it the pool: `max_open`
  permits (each operation and open transaction holds one; waiting is a
  cancellation point), at most `max_idle` idle connections, each closed
  once older than `max_lifetime` (on the monotonic clock). An operation
  takes an idle connection or connects, runs, and gives it back; a
  connection whose `is_broken` is true is closed instead. A transaction
  holds one connection from `begin` to `commit` or `rollback`.
- `open_driver` connects once (a `ping`) before it returns, so a wrong
  password or host fails there. `close` closes the idle connections;
  later operations are `Closed`.
- A `DriverConn` is used by one task at a time; the driver's waits are
  cancellation points that return `Canceled`.

### 10.8 Postgres

Status: implemented natively (decision 0278 stage C, 2026-10-20), TLS
through std/tls's `tls.client` (decision 0281); checked against pgx on
PostgreSQL 17 and 14, and 17 with TLS (scripts/pg-diff.sh). The Go
backend reports an unknown driver.

```
let db = sql.open(os.net, "postgres", "postgres://app:${pw}@db.internal:5432/app?sslmode=verify-full")?
```

`sql.open(net, "postgres", dsn, ...)` opens a PostgreSQL database
(version 13 or later) through `net`; the driver is written in Xo and
works on every native target. It dials through `net`, so an attenuated
`Net` (3.2) limits which database hosts it can reach (an address
outside the lists fails with `Conn("dial ...: denied by Net.only")`).

`open` takes one more argument for this driver:

```
pub fn open(net: Net, driver: Str, dsn: Str, max_open: Int = 10, max_idle: Int = 2,
            max_lifetime: Duration = 30m, roots: Bytes = b"") -> Db ! SqlErr uses net.dial
```

`roots` is PEM: the certificates trusted instead of the system's for
`verify-ca` and `verify-full` (a managed database's CA bundle, read by
the caller with `Fs`). `open` connects once and checks the server
(`ping`) before it returns, so a wrong password or host fails at `open`.

**Connection strings** (pgx's `ParseConfig`): a URL
`postgres://user:password@host:port/database?key=value&...` (also
`postgresql://`; percent escapes decoded; several `host:port` pairs
separated by `,` are tried in order) or keyword form `host=db
port=5432 user=app password='x y' dbname=app sslmode=require`. Keys:
`host` (a name, an IP, or an absolute directory for a Unix socket:
`/var/run/postgresql` dials `unix:/var/run/postgresql/.s.PGSQL.5432`),
`port` (5432), `user` (required: no environment is read), `password`,
`dbname` (the user name), `sslmode`, `connect_timeout` (seconds, 0 for
none), `application_name`, `search_path`, `timezone`, and
`options`, and any other key are sent as startup parameters (run time
settings, as pgx does: `statement_timeout=5s`). `sslrootcert`, `sslcert`,
`sslkey`, `sslpassword`, `sslcrl`, `passfile`, `service`, and
`servicefile` name files or secrets kept elsewhere and are `Param`. The
driver also sends `client_encoding=UTF8`, `TimeZone=UTC`, and
`DateStyle=ISO`.
Nothing comes from `PG*` variables or `~/.pgpass`: configuration is the
string and the arguments (read variables with `Env` and build the
string). The password never appears in errors, logs, or `Debug` of a
`Db`.

**TLS** (std/tls's `tls.client`, 27: SSLRequest in plain text, then the
handshake on the same connection), by `sslmode`:

| sslmode | TLS | Verification |
|---|---|---|
| `disable` | never | none |
| `allow` | only when a plain connection is refused | none |
| `prefer` | when the server accepts SSLRequest | none |
| `require` | always (else `Conn("server refused TLS connection")`) | none: encrypted, server not authenticated; the chain when `roots` is given (libpq's rule, as pgx) |
| `verify-ca` | always | the chain, against `roots` or the system roots |
| `verify-full` (default) | always | the chain and the host name |

A Unix socket never uses TLS, whatever `sslmode` says (pgx). Authentication: trust, cleartext password, MD5, and
SCRAM-SHA-256; a server that asks for another method (GSSAPI, SSPI,
SCRAM-SHA-256-PLUS only) is `Conn("unsupported authentication method
...")`.

**Statements.** `$1`, `$2`, ... are Postgres's own placeholders. A
statement without arguments runs with the simple query protocol, so
`exec` takes a `;` separated script (a schema); with arguments it is
one statement on the extended protocol (Parse, Bind, Execute, Sync, an
unnamed statement each time). Results are read whole into `List[Row]`.
`exec`'s `rows_affected` is the count of the command tag (`INSERT 0
3` is 3); `last_insert_id` is 0: use `RETURNING id` with `query_one`.
Transactions run `BEGIN`, `COMMIT`, and `ROLLBACK` on one connection
held from `begin` to its end; a statement that fails inside a
Postgres transaction aborts it (later statements are
`Server("25P02", ...)` until `rollback`), unlike the in memory driver.

**Pool.** `max_open` connections at most, `max_idle` kept open when
unused, each closed after `max_lifetime`. A connection that fails at
the socket or protocol level is dropped, and the operation fails with
`Conn`; operations are not retried (a write may have run).

**Cancellation.** When the task waiting on a statement is cancelled, the
statement returns `Canceled` and the connection is dropped (closed, so
the server stops the statement when it notices); sending CancelRequest
first, as pgx does, is a later step. Deadlines are `task.with_timeout`.
A COMMIT of a transaction that had failed is
`Server("25P02", "commit unexpectedly resulted in rollback")` (pgx's
ErrTxCommitRollback).

### 10.9 Values by Postgres type

Results are read in text format and decoded by the column's type:

| Postgres type | SqlValue | Read with |
|---|---|---|
| `int2`, `int4`, `int8`, `oid` | `Integer` | `int` |
| `float4`, `float8` | `Real` | `float` |
| `bool` | `Flag` | `bool` |
| `text`, `varchar`, `bpchar`, `name`, `citext` | `Text` | `str` |
| `timestamptz` | `At` | `time` |
| `timestamp` | `At`, the wall clock read as UTC | `time` |
| `bytea` | `Blob` | `bytes` |
| `json`, `jsonb` | `JsonText` | `json[T]`, `str` |
| arrays of the types above (one dimension) | `Array` | `ints`, `strs`, `items` |
| every other type (`numeric`, `uuid`, `date`, `time`, `interval`, `inet`, enums, ...) | `Text`, the server's text | `str` |

NULL is `Null` everywhere. Accessors convert where no information is
lost: `float` reads `Integer` and a `numeric` Text that parses as a
Float; `time` reads a `date` Text as its midnight in UTC; `str` reads
`JsonText`. Anything else is `Mismatch` as before.

Parameters are sent in text format with the type left to the server
(OID 0), except `Blob`, which is sent as binary `bytea`: `Integer` and
`Real` as their decimal text (`NaN`, `Infinity`), `Flag` as `true` or
`false`, `At` as RFC 3339 with nanoseconds, `JsonText` as is, `Array`
as an array literal (`{1,2,NULL}`, elements quoted as Postgres
requires). A UUID parameter is `sql.str(id)`: the server converts it.

The values and accessors are 10.2 and 10.4's.

`derive SqlRow` accepts `Bytes`, `List[Int]`, and `List[Str]` fields
too. Column names that are not field names use the field's wire name
(decision 0277, planned): `kind as "type": Str`.

---

## 11. std/crypto

Status: draft (2026-10-08, decisions 0056 and 0059). Go standard library
underneath, plus Argon2id and BLAKE2b vendored from `golang.org/x/crypto`
into the runtime (no module dependency). Native builds (`--no-gc`,
decision 0083) compute the same results in Xo (std/native/llrt: SHA-256,
HMAC, PBKDF2, BLAKE2b, Argon2, SHA-512 and Ed25519), with random bytes
from the operating system (`getentropy` on macOS, `getrandom` on Linux).

```
pub fn sha256(data: Bytes) -> Bytes                     // 32 bytes
pub fn hmac_sha256(key: Bytes, msg: Bytes) -> Bytes     // 32 bytes
pub fn equal(a: Bytes, b: Bytes) -> Bool                // constant time in the contents
pub fn random_bytes(r: Rand, n: Int) -> Bytes uses rand // faults when n < 0
pub fn token(r: Rand, n: Int = 32) -> Str uses rand     // n random bytes, unpadded URL safe base64
pub fn hash_password_argon2id(r: Rand, password: Str, memory_kib: Int = 19456,
                              iterations: Int = 2, parallelism: Int = 1) -> Str uses rand
pub fn hash_password(r: Rand, password: Str, iterations: Int = 600000) -> Str uses rand
pub fn verify_password(password: Str, hash: Str) -> Bool
pub fn ed25519_public_key(seed: Bytes) -> Bytes                   // 32 bytes; faults unless seed is 32 bytes
pub fn ed25519_sign(seed: Bytes, msg: Bytes) -> Bytes             // 64 bytes; faults unless seed is 32 or 64 bytes
pub fn ed25519_verify(public_key: Bytes, msg: Bytes, sig: Bytes) -> Bool
```

- Hashing, signing, and comparison are pure: no capability, no effect.
- Native builds compute SHA-256 (`sha256`, `hmac_sha256`, `hash_password`'s
  PBKDF2, and the TLS stack's SHA-256) with the CPU's SHA-256
  instructions where it has them (arm64's SHA2 extension: always on
  macOS, by `AT_HWCAP` on Linux) and in portable Xo elsewhere; the
  results are the same bytes either way (decision 0231, owner
  2026-10-16). Not observable from programs: it is an internal
  intrinsic (std/native/llrt/cpu.xo `sha256_block`), not API. SHA-1
  (`digest(Sha1, ...)`, `hmac(Sha1, ...)`, the TLS 1.2 CBC suites' MAC)
  likewise takes the CPU's SHA-1 instructions (arm64's SHA1 extension,
  x86-64's SHA-NI; `sha1_block`, decision 0231's SHA-1 addendum,
  2026-11-03), and SHA-512 and SHA-384 (`digest`, `hmac`, `Hasher`,
  Ed25519) arm64's SHA512 instructions (always on macOS, by `AT_HWCAP`
  on Linux; `sha512_block`, decision 0231's SHA-512 addendum,
  2026-11-03; portable Xo on x86-64).
- Randomness comes only from the `Rand` capability (section 3.4), as
  everywhere else: cryptographically secure in programs, seeded from the
  test seed in tests, so a test that creates tokens or salts is
  reproducible. A function that makes tokens takes a `Rand` and declares
  `uses rand`.
- `equal` takes time that depends on the lengths but not on the contents;
  compare digests of equal length (for example two `sha256` results) when
  the length is secret too.
- `hash_password_argon2id` is the recommended password hash: Argon2id
  (RFC 9106, version 0x13) with a 16 byte salt from `r` and a 32 byte key,
  encoded in the PHC string format
  `$argon2id$v=19$m=<memory_kib>,t=<iterations>,p=<parallelism>$<salt>$<key>`
  (salt and key in unpadded standard base64), which other Argon2 libraries
  read. The defaults, 19456 KiB (19 MiB), 2 passes, 1 lane, are the OWASP
  minimum for Argon2id (Password Storage Cheat Sheet); raise `memory_kib`
  first when the host allows. One hash at the defaults takes about 20 ms
  on an Apple M series core. It faults when `parallelism` is outside
  `1..=255`, `memory_kib` outside `8 * parallelism..=4194304` (4 GiB), or
  `iterations` outside `1..=1000`. Tests may pass small values
  (`memory_kib: 64, iterations: 1`).
- `hash_password` is PBKDF2-HMAC-SHA256 with a 16 byte salt from `r` and a
  32 byte key, encoded as `pbkdf2-sha256$<iterations>$<salt>$<key>`
  (salt and key in unpadded standard base64). It faults when `iterations`
  is outside `1..=100000000`. The default, 600000, follows the OWASP 2023
  guidance for PBKDF2-HMAC-SHA256; tests may pass a small count.
  PBKDF2 stays supported for existing hashes and for hosts that need a
  FIPS approved function; it is not memory hard, so prefer Argon2id for new
  code.
- `verify_password` tells the formats apart by prefix (`$argon2id$` or
  `pbkdf2-sha256$`), recomputes the key with the stored salt and
  parameters, and compares in constant time. A malformed hash, another
  Argon2 variant or version, or parameters outside the bounds above is
  `false`, never an error. To move users to Argon2id, rehash on the next
  successful login when the stored hash starts with `pbkdf2-sha256$`.
- Ed25519 (RFC 8032, decision 0189; added 2026-10-12): the private key is
  a 32 byte seed; make one with `random_bytes(r, 32)`, keep it secret, and
  publish `ed25519_public_key(seed)`. `ed25519_sign` also takes Go's 64
  byte private key, the seed followed by its public key (added
  2026-10-29); it signs with that public key as stored, as Go's
  `ed25519.Sign` does, and skips computing it, so a signer that keeps
  `seed + public_key` signs faster. `ed25519_sign` is deterministic (the
  same seed and message give the same 64 byte signature, so signing needs
  no `Rand`). `ed25519_verify` is `false`, never a fault, for a key that is
  not 32 bytes, a signature that is not 64, an encoding that is no point
  of the curve, or a signature scalar S that is not below the group order;
  it accepts exactly what Go's `crypto/ed25519.Verify` accepts (the
  cofactorless equation compared byte for byte, the non canonical point
  encodings Go accepts). Native builds compute it in C (SHA-512 and the
  curve arithmetic in xo_crypto.c), checked against Go on random inputs
  and the RFC 8032 vectors. The checksum log of the module commands
  (decision 0124) signs and verifies tree heads with it.

**Hashes and HMAC** (decision 0279, 2026-10-20; every backend):

```
enum Hash { Md5, Sha1, Sha224, Sha256, Sha384, Sha512, Sha3_256, Sha3_512 }
fn (h: Hash) size() -> Int                     // digest bytes: 16, 20, 28, 32, 48, 64, 32, 64
pub fn digest(h: Hash, data: Bytes) -> Bytes
pub fn hmac(h: Hash, key: Bytes, msg: Bytes) -> Bytes

struct Hasher {}                               // a value: copies are independent
pub fn hasher(h: Hash) -> Hasher
pub fn hmac_hasher(h: Hash, key: Bytes) -> Hasher
fn ([var] x: Hasher) write(data: Bytes)
fn (x: Hasher) sum() -> Bytes                  // the digest so far; writing may continue
fn (x: Hasher) written() -> Int
```

- The results are Go's (`md5.Sum`, `sha1.Sum`, `sha256.Sum224`, ...,
  `sha3.Sum512`, `crypto/hmac`); `sha256(d)` and `hmac_sha256(k, m)`
  are `digest(Sha256, d)` and `hmac(Sha256, k, m)`. HMAC hashes a key
  longer than the hash's block first (the block is the rate for SHA-3,
  as Go's).
- `Md5` and `Sha1` are broken for collision resistance; they are for
  protocols that require them (HMAC-SHA1 webhook signatures, TOTP,
  Postgres MD5 authentication, Content-MD5). An `xo vet` note for them
  outside an HMAC is planned.
- A `Hasher` takes a large body a piece at a time (`var x =
  crypto.hasher(Sha256)`, `x.write(chunk)` per chunk read from a `File`
  or a `Conn`, then `x.sum()`). Natively the hashes are the TLS stack's
  (std/native/llrt sha1.xo, sha256.xo, sha3.xo; SHA-256 with the CPU's
  instructions) and MD5 is ported from Go's crypto/md5; Go's vectors for
  each are in std/native/crypto_test.xo and llrt/md5_test.xo.

**Authenticated encryption and key derivation** (milestone 0.35; Go's
`cipher.AEAD` and `crypto/hkdf`; every backend; Go's `aesGCMTests` and
`hkdfTests` and Wycheproof's AES-GCM, ChaCha20-Poly1305 and HKDF vectors
in the corpus `internal/stdtest/testdata/corpus/aead`):

```
// seal a cookie, a token, or a row: a random nonce, kept with the message
let k = crypto.aead_key(crypto.Aead.AesGcm, key)?        // key from a KDF or a secret store
let sealed = k.seal_random(rand, b"user=42", b"session")
let plain = k.open_random(sealed, b"session")?           // Err if anything changed

enum Aead { AesGcm, ChaCha20Poly1305 }
fn (a: Aead) nonce_size() -> Int                         // 12
fn (a: Aead) overhead() -> Int                           // 16: the tag
struct AeadKey {}                                        // Debug text `<aeadkey>`: never key material
struct AeadErr { msg: Str }
fn aead_key(alg: Aead, key: Bytes) -> AeadKey ! AeadErr  // AES: 16, 24, 32 bytes; ChaCha20-Poly1305: 32
fn (k: AeadKey) alg() -> Aead
fn (k: AeadKey) seal_random(r: Rand, plaintext: Bytes, aad: Bytes = b"") -> Bytes uses rand
fn (k: AeadKey) open_random(sealed: Bytes, aad: Bytes = b"") -> Bytes ! AeadErr
fn (k: AeadKey) seal(nonce: Bytes, plaintext: Bytes, aad: Bytes = b"") -> Bytes
fn (k: AeadKey) open(nonce: Bytes, sealed: Bytes, aad: Bytes = b"") -> Bytes ! AeadErr
pub fn hkdf_extract(h: Hash, secret: Bytes, salt: Bytes = b"") -> Bytes          // h.size() bytes
pub fn hkdf_expand(h: Hash, prk: Bytes, info: Bytes, n: Int) -> Bytes
pub fn hkdf(h: Hash, secret: Bytes, salt: Bytes, info: Bytes, n: Int) -> Bytes  // Go's hkdf.Key
```

- **Nonces.** A nonce must never be used twice with one key: a repeated
  AES-GCM or ChaCha20-Poly1305 nonce breaks both the confidentiality and
  the authenticity of every message under that key. `seal_random` and
  `open_random` take a fresh 12 byte random nonce from `r` and keep it as
  the sealed message's first 12 bytes (Go's
  `cipher.NewGCMWithRandomNonce`): use them unless a protocol counts its
  nonces (TLS records, a numbered stream), which is what `seal` and
  `open` with an explicit nonce are for. With random nonces, rotate a key
  well before 2^32 messages.
- `seal` returns the ciphertext followed by the 16 byte tag, as Go's
  `Seal(nil, ...)`; `open` checks the tag before it returns anything. Every
  failure to open (a changed byte, another nonce or `aad`, a sealed text
  shorter than the tag) is the same `AeadErr`, `cipher: message
  authentication failed`, on every backend. A nonce that is not 12 bytes
  is a fault (a programmer error, as Go panics); a key of another length
  is an `Err` from `aead_key`.
- An `AeadKey` is made once per key (AES's key schedule and GHASH's table)
  and is pure to use: seal and open need no capability. It has no
  `Display` and derives nothing: it cannot be printed, logged, or encoded
  as JSON, as `Secret[T]`.
- Natively AES-GCM and ChaCha20-Poly1305 are the TLS stack's
  (std/native/llrt aesgcm.xo with the CPU's AES and carry-less multiply
  instructions, chacha.xo with NEON or SSE); the Go backend uses Go's
  crypto/cipher for AES-GCM.
- `hkdf_extract`, `hkdf_expand` and `hkdf` are HKDF (RFC 5869) over HMAC
  with any `Hash`. An empty salt is `h.size()` zero bytes, as Go's.
  `hkdf_expand` faults when `n` is negative or more than 255 times
  `h.size()` (Go returns an error; the length is a constant in practice).

**Public key signatures, keys, and PEM** (decision 0285, Accepted by
the owner 2026-10-20; every backend; Go's results on generated keys,
signatures, and mutations in std/native/crypto_test.xo, golden
`crypto_keys`):

```
enum KeyAlg { Rsa, EcdsaP224, EcdsaP256, EcdsaP384, EcdsaP521, Ed25519 }
struct PublicKey {}          // a value: ==; Debug text `<publickey>`
struct PrivateKey {}         // Debug text `<privatekey>`: never key material
struct KeyErr { msg: Str }
struct PemBlock { kind: Str, headers: Map[Str, Str], bytes: Bytes }

pub fn pem_decode(text: Str) -> List[PemBlock]
pub fn pem_encode(b: PemBlock) -> Str ! KeyErr
pub fn parse_public_key(der: Bytes) -> PublicKey ! KeyErr
pub fn parse_private_key(der: Bytes) -> PrivateKey ! KeyErr
pub fn jwk_public_key(jwk: Str) -> PublicKey ! KeyErr
pub fn marshal_public_key(k: PublicKey) -> Bytes
fn (k: PublicKey) alg() -> KeyAlg
fn (k: PrivateKey) alg() -> KeyAlg
fn (k: PrivateKey) public_key() -> PublicKey
pub fn verify(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool
pub fn verify_pss(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool
pub fn verify_raw_ecdsa(k: PublicKey, h: Hash, msg: Bytes, sig: Bytes) -> Bool
pub fn sign(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand
pub fn sign_pss(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand
pub fn sign_raw_ecdsa(r: Rand, k: PrivateKey, h: Hash, msg: Bytes) -> Bytes ! KeyErr uses rand
pub fn generate_key(r: Rand, alg: KeyAlg, bits: Int = 2048) -> PrivateKey ! KeyErr uses rand
```

- PEM is Go's `encoding/pem`: `pem_decode` is `pem.Decode` repeated on
  the rest (text outside blocks and malformed blocks skipped; header text
  and the type are Str, U+FFFD for bytes that are not UTF-8), and
  `pem_encode` is `pem.EncodeToMemory` (64 column lines; `Proc-Type`
  first, then the other headers sorted), `KeyErr` for a header key with
  a colon.
- `parse_public_key` reads a PKIX SubjectPublicKeyInfo (Go's
  `ParsePKIXPublicKey`) or a PKCS #1 RSAPublicKey; `parse_private_key`
  reads PKCS #8, PKCS #1, and SEC 1 EC keys (Go's `ParsePKCS8PrivateKey`,
  `ParsePKCS1PrivateKey`, `ParseECPrivateKey`, tried in that order as
  `tls.X509KeyPair` does). Errors carry Go's x509 texts (`x509: unsupported
  elliptic curve`, ...); DER that does not parse at all is `x509:
  malformed public key` or `x509: malformed private key` on every backend
  (Go's encoding/asn1 texts name its internal structures). DSA and other
  algorithms are `x509: unknown public key algorithm`.
- `verify` hashes `msg` with `h` and checks RSA PKCS #1 v1.5 or ECDSA
  ASN.1 DER (Go's `VerifyPKCS1v15`, `VerifyASN1`); `verify_raw_ecdsa`
  takes the fixed size r || s form of JWS (RFC 7518 3.4). Ed25519 signs
  the message itself and ignores `h`. A key and hash that do not fit, a
  wrong length, or a malformed signature is `false`, never a fault.
  `verify_pss` accepts any salt length (Go's `PSSSaltLengthAuto`);
  `sign_pss` uses a salt as long as the hash (RFC 7518 PS256, TLS 1.3).
- RSA keys verify and sign from 1024 bits (Go 1.24's minimum: `crypto/rsa:
  512-bit keys are insecure (see ...)`), and a key's public values are
  checked as Go checks them (`crypto/rsa: public exponent is even`, ...);
  a private key is checked when parsed (`crypto/rsa: p * q != n`, its CRT
  values computed when absent); keys of more than two primes are
  `crypto/rsa: multi-prime keys are not supported`. A message hash too
  long for the key is `crypto/rsa: message too long for RSA key size`;
  `sign_pss` with another kind of key is `KeyErr`. `generate_key(r, Rsa,
  bits)` is Go's `rsa.GenerateKey` (FIPS 186-5 A.1.1: two random primes of
  half the size, e = 65537, d = e^-1 mod lcm(p - 1, q - 1), the key checked
  as a parsed key is): `bits` from 1024 (`crypto/rsa: 512-bit keys are
  insecure (see ...)` below, Go 1.26's floor) to 8192 (`crypto/rsa: 8193-bit
  keys are too large`, the largest key std signs with); `bits` is ignored
  for other algorithms. The prime candidates are `r`'s bytes (Go's
  `randomPrime`: the top two bits and the low bit set), so the same `r`
  gives the same key on every backend; the Miller-Rabin bases come from the
  system's random bytes, as Go's.
- `jwk_public_key` reads RFC 7517 JSON (members matched exactly,
  unpadded base64url values; `use`, `kid`, `alg`, and private members
  ignored): `jwk: malformed JSON`, `jwk: unsupported key type "oct"`,
  `jwk: unsupported curve "P-192"`, `jwk: missing member "n"`, `jwk:
  member "n" is not unpadded base64url`, `jwk: coordinates of the wrong
  size for P-256`, `jwk: point not on the curve`, `jwk: invalid RSA
  key`.
- **Nonces.** ECDSA signing takes its nonce from an HMAC_DRBG seeded with
  the private key, bytes from `r`, and the message hash (Go 1.24's hedged
  construction): a `Rand` that repeats or returns zeros degrades to
  deterministic signing (as RFC 6979), so it cannot leak the key, and a
  test's seeded `Rand` gives reproducible signatures natively. On the Go
  backend ECDSA nonces and PSS salts come from Go's own generator (Go
  1.26 ignores a custom reader, and with `cryptocustomrand=1` it reads
  one extra byte at random), so those signatures verify but differ run
  to run; the bytes the native signer takes are drawn from `r` and
  dropped, so `r`'s later values match. Keys from `generate_key` (RSA included), RSA
  PKCS #1 v1.5, and Ed25519 signatures are the same bytes on every
  backend for the same `r`.
- **Timing.** Every operation on a private key or a nonce (scalar
  multiplication, the nonce inversion, RSA's CRT exponentiation) runs in
  time independent of the secret; bench/ct checks it with a Welch t-test
  (on demand and nightly).

Storing API tokens: tokens from `token` have 256 bits of entropy, so a
plain `sha256` digest is enough (no salt or stretching needed) and can be
looked up by value; compare the digest found with `equal`:

```
fn issue(db: sql.Db, r: Rand, user: Int) -> Str ! sql.SqlErr uses (rand, net.dial) {
  let t = crypto.token(r)
  db.exec("INSERT INTO tokens (user_id, digest) VALUES ($1, $2)",
    [sql.int(user), sql.str(crypto.sha256(t.bytes()).to_hex())])?
  t
}
```

Passwords chosen by people have little entropy: store
`hash_password_argon2id(os.rand, pw)` and check with `verify_password`.

---

## 12. std/go

Values that cross the Go interop boundary (core 4.4, decision 0041).
Bindings written by `xo bind` import this module; user code needs
`use std/go` only to name the type, for example in `! go.Error`.

```
struct Error {}                      // handle; implements Display, so it converts to Error at `?`
fn (e: Error) display() -> Str       // the Go error's Error() text, captured at the boundary
fn (e: Error) message() -> Str       // the same text
fn (e: Error) go_type() -> Str       // the Go dynamic type, for example "*url.Error"
fn error(message: Str) -> Error      // Go's errors.New, for an Xo callback's Err (decision 0416)

struct Any {}                        // a Go `any` value (decision 0412); Go's == and hash
fn (a: Any) display() -> Str         // Go's %v
fn (a: Any) type_name() -> Str       // Go's %T ("<nil>" for nil)
fn (a: Any) is_nil() -> Bool         // nil, or a nil pointer, slice or map
fn (a: Any) as_int() -> Int?         // Go's integer types that fit; None on another type
fn (a: Any) as_float() -> Float?     // float64, float32
fn (a: Any) as_bool() -> Bool?
fn (a: Any) as_str() -> Str?         // Go kind string
fn (a: Any) as_bytes() -> Bytes?     // []byte, copied
fn (a: Any) as_list() -> List[Any]?  // a slice or array
fn (a: Any) as_map() -> Map[Str, Any]?  // a map with string keys, in key order
fn (a: Any) as_handle[T]() -> T?     // the `use go` handle T holding the value
fn any[T](x: T) -> Any               // Int as int64, Float as float64, Str as string, Bytes as []byte,
                                     // Duration, Time, a handle's Go value, List/Map/Option of those (else XO1106)
struct Out[T] {}                     // a Go *T a Go function fills through any
fn out[T]() -> Out[T]                // a new pointer to T's Go zero value
fn (o: Out[T]) any() -> Any          // the pointer, passed where Go takes any
fn (o: Out[T]) get() -> T            // the value Go wrote; a fault on another type
struct Recv[T] {}                    // a Go channel Xo receives from (decision 0421)
fn (r: Recv[T]) recv() -> T? uses go         // waits; None once closed and empty; cancelable
fn (r: Recv[T]) try_recv() -> T?? uses go    // None when no value is ready, Some(None) once closed
fn (r: Recv[T]) each[fx: Effects](f: fn(T) uses fx) uses go, fx  // f with each value until closed
```

`fmt.sprintf("%d %s", [n, s])`: the elements of a list literal written for
a `List[go.Any]` parameter of a Go binding's function (Go's `...any` or
`[]any`) convert implicitly as `go.any(x)` (core 2.10). In a replay, a
go.Any a Go call returned is rebuilt from its recorded display text: it
displays, and its other methods fault unless the function is `replay
rerun` (core 11.3).

Calling these performs no Go call (no `go` effect), except Recv's
methods, which receive from the Go channel. The native backends do
not build `use go` code at all (`XO1201`, impossible).

---

## 13. std/time

Status: **Accepted** (decision 0094, 2026-10-09; drafted by the spec
audit, `bench/results/spec-audit-2026-10-09.md`). Implemented on the Go
backend (2026-10-09): `std/time.xo`, `std/rt/timezone.go`,
`timecivil.go`, `timefmt.go`; the compile time layout check is
compiler/check's (`internal/check/timelayout.go` until decision 0215). The native backends build all of it
(2026-10-09: `std/native/time.xo` over `std/llrt/xo_time.c`, the same
zone data per use, output identical to the Go backend's; decision 0094
"Native implementation"; decision 0089 makes native a preview at 1.0;
since 2026-10-13 the zone rules, TZif parsing, the POSIX TZ footer,
and the lookup, are Xo in `std/native/llrt/tz.xo`, decision 0193 step
5, and xo_time.c keeps the zone data and the table of loaded zones).
Points
the API text below leaves open are settled in 13.5. Instants,
durations, Unix seconds and milliseconds, and RFC 3339 in UTC stay in
the prelude (2.6).

Principles:

- `Time` stays an instant with no zone. A zone is a separate value, and a
  civil (wall clock) reading is a separate type, so code never compares a
  zoned and an unzoned value by accident.
- Zone data comes with the program, embedded per use, so loading a zone
  by name is pure, needs no capability, and gives the same answer on every
  OS and in tests. `utc()` and `fixed` need no data; a zone name that is a
  constant embeds only that zone (about 1 to 3 KB); a name known only at
  run time embeds the whole IANA database (about 400 KB compressed) and
  `xo build` prints a note naming the call that caused it.
  `--tzdata=system` reads the host's database instead (smaller binaries,
  host dependent answers). Only the machine's own zone needs authority:
  `local` takes `Env` (`TZ`, then the system setting), so a function that
  depends on where it runs says so.
- Layouts are written with named, lowercase fields, not Go's reference
  date, not C's `%` codes, and not the letter patterns of Java, C#, and
  Swift, where `MM` (month) and `mm` (minute) differ only in case:
  `"{year}-{month}-{day} {hour}:{minute}:{second}"`. Common formats are
  constants.
- Parsing returns a `Result` whose error names the position and the field
  expected, like `json.DecodeErr`.
- Daylight saving gaps and overlaps are explicit: converting a civil time
  that does not exist or exists twice is an error unless the caller picks
  a rule.

### 13.1 Zones

```
struct Zone {}                                   // handle; Eq, Display (its name)
enum ZoneErr { Unknown(name: Str), NoLocal(msg: Str) }
fn (e: ZoneErr) display() -> Str                 // unknown time zone "Mars/Olympus"

pub fn utc() -> Zone
pub fn zone(name: Str) -> Zone ! ZoneErr         // IANA name: "Europe/Berlin"; pure (embedded data)
pub fn fixed(name: Str, offset: Duration) -> Zone   // "UTC+2" style; faults past 24h
pub fn local(env: Env) -> Zone ! ZoneErr uses env   // TZ, then the system zone

fn (z: Zone) name() -> Str
fn (z: Zone) offset_at(t: Time) -> Duration      // offset from UTC at that instant
fn (z: Zone) abbreviation_at(t: Time) -> Str     // "CET", "CEST"
```

In tests `os.env` is a fake, so `local(os.env)` is `utc()` unless the
test sets `TZ`.

### 13.2 Civil time

```
struct Date { year: Int, month: Int, day: Int } derive Ord, Json
struct ClockTime { hour: Int, minute: Int, second: Int, nanos: Int } derive Ord, Json
struct Civil { date: Date, time: ClockTime, offset: Duration, zone: Str } derive Json
enum Weekday { Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday } derive Display, Ord, Json
enum Ambiguity { Earlier, Later, Reject }
enum CivilErr { Invalid(msg: Str), Skipped(at: Str), Repeated(at: Str) }
fn (e: CivilErr) display() -> Str                // invalid civil time: month 13 out of range 1 to 12

pub fn civil(t: Time, z: Zone) -> Civil
pub fn instant(date: Date, time: ClockTime, z: Zone, ambiguity: Ambiguity = Ambiguity.Reject) -> Time ! CivilErr
pub fn date(year: Int, month: Int, day: Int) -> Date ! CivilErr
fn (d: Date) weekday() -> Weekday
fn (d: Date) add_days(n: Int) -> Date
fn (d: Date) add_months(n: Int) -> Date          // clamps the day: Jan 31 + 1 month is Feb 28 or 29
fn (d: Date) days_until(o: Date) -> Int
fn (d: Date) display() -> Str                    // "2026-10-09"
```

`instant` with `Reject` returns `Skipped` for a time inside a spring
forward gap and `Repeated` for one inside a fall back overlap; `Earlier`
and `Later` pick the first or second instant (for a gap, the instant the
offset after or before the change gives, so `Earlier` is never after
`Later`). Arithmetic on `Time` stays exact
(`t + 24h` is always 24 hours); calendar arithmetic is on `Date`.

### 13.3 Formatting and parsing

```
const RFC3339: Str = "{year}-{month}-{day}T{hour}:{minute}:{second}{frac}{offset}"
const RFC1123: Str = "{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} {zone}"   // HTTP dates
const DATE: Str = "{year}-{month}-{day}"
const TIME: Str = "{hour}:{minute}:{second}"

struct ParseErr { input: Str, layout: Str, at: Int, msg: Str }
fn (e: ParseErr) display() -> Str                // at byte 5 of "2026-1x-09": expected 2 digit month
struct LayoutErr { layout: Str, msg: Str }
fn (e: LayoutErr) display() -> Str               // bad layout "{yr}": unknown field {yr}

pub fn format(t: Time, z: Zone, layout: Str) -> Str             // faults on a bad layout (constant layouts are checked at compile time)
pub fn parse(s: Str, layout: Str, z: Zone) -> Time ! ParseErr   // z applies when the text has no offset
pub fn check_layout(layout: Str) -> () ! LayoutErr
// Hot code uses a `const Layout`: format with a string layout parses it on every call (5.4x Go's wall,
// 4.7x for parse); the same layout as a const runs at 2.03x and 1.90x (bench/results/std-twin-rows2-2026-11-03.md)
struct Layout                                              // a layout parsed once (decision 0332)
pub fn layout(text: Str) -> Layout                         // faults on a bad layout; `const ISO: Layout = layout("{year}-{month}-{day}")` parses it on first use
fn (l: Layout) format(t: Time, z: Zone) -> Str             // as format(t, z, text), without the layout walk
fn (l: Layout) parse(s: Str, z: Zone) -> Time ! ParseErr   // as parse(s, text, z)
// consts RFC3339_LAYOUT, RFC1123_LAYOUT, DATE_LAYOUT, TIME_LAYOUT are the named layouts as Layouts
```

Fields, one spelling each: `{year}`; `{month}` 01 to 12, `{month:1}`
without padding, `{month:short}` `Jan`, `{month:name}` `January`; `{day}`
01 to 31, `{day:1}`; `{weekday:short}` `Mon`, `{weekday:name}` `Monday`;
`{hour}` 00 to 23, `{hour12}` 01 to 12 with `{ampm}` AM or PM; `{minute}`;
`{second}`; `{frac}` a fraction with as many digits as needed (none when
zero), `{frac:3}` exactly three (1 to 9), both with their leading `.`
(write `{second}{frac:3}`); `{offset}` `Z` or `+02:00`,
`{offset:compact}` `+0200`; `{zone}` the zone abbreviation. A literal
brace is written `{lbrace}` or `{rbrace}`. A layout that is a constant is
checked at compile time: an unknown field is an error with a did you mean
fix. Letter patterns and `%` codes from other languages (`{yyyy}`,
`{MM}`, `%Y`) are the habit error `XO0177` with a machine fix to the named
form (decision 0087).
Names are English only; locales are out of scope. Parsing accepts exactly
the layout (no guessing between formats); a two digit year is not offered.

### 13.4 Decided points

- Zone data is embedded per use (above), with `--tzdata=system` as the
  opt in alternative.
- `format` with a bad layout built at run time faults, like a bad index;
  `check_layout` returns a `Result` for code that builds layouts from
  input.
- Native backends carry the same embedded data in C; the Go backend uses
  Go's `time` package with the per use data. Implementation order: Go
  backend, checker rules, native runtime, cross backend diff tests.
- WebAssembly (`xo build --backend=wasm`, 2026-10-17) embeds the same per
  use data in the module's data segment, only when the program looks a
  zone up (a program without zones, and the playground's compiler, carry
  none); each lookup parses the zone again (no zone table shared between
  threads yet), with the same zone identity for the same name. A wasm
  build that does not choose zone data (compiler/wasm's own runs, the
  playground) refuses a program that looks zones up rather than report
  every zone unknown.

### 13.5 Implementation notes (2026-10-09)

Settled while implementing; examples are in the goldens
`internal/gen/testdata/golden/time_*` and
`internal/check/testdata/golden/time_layout`.

- **Zone data.** The zone data is the IANA time zone database that ships
  with std (`std/tzdata/zoneinfo.zip`, its IANA version in
  `std/tzdata/VERSION`), on every backend; no Go toolchain is needed.
  `XO_ZONEINFO` names another file, and `--tzdata=system` reads the
  operating system's database at run time instead.
  `time.zone("UTC")` needs no data. A constant name is a string literal,
  a `const`, or a `+` of them; a constant name the database does not have
  gets a build note and returns `ZoneErr.Unknown` at run time. `time.zone`
  used as a function value counts as a name known only at run time.
  Names are case sensitive. Measured on darwin/arm64 (2026-10-09): a
  program using only `utc()` is 7,968,146 bytes, one constant zone
  (Europe/Berlin) 7,968,994 (+848), a run time name 8,382,498 (+414 KB,
  the database), the same with `--tzdata=system` 7,969,666.
- **`local(env)`.** `TZ` from the `Env` (a leading `:` is dropped; empty
  or `UTC` is `utc()`); a name that is not embedded is looked up in the
  host database, since `local` is host dependent by nature. Without
  `TZ`, a fake `Env` gives `utc()` and the real one the system zone,
  named by the `/etc/localtime` link when there is one (`Local`
  otherwise).
- **Civil time.** `Civil.zone` is the zone's name (`Europe/Berlin`), not
  the abbreviation (`abbreviation_at` gives that). `date` accepts years 1
  to 9999; `Time` covers about 1678 to 2262, and `instant` outside it is
  `CivilErr.Invalid`. Date methods on a `Date` built with a struct
  literal and out of range fields normalize it (month 13 is January of the
  next year). For a gap, `Earlier` is the earlier instant and `Later` the later one
  (Temporal's `earlier` and `later`): 02:30 on 2026-03-08 in New York is
  01:30 EST with `Earlier` (the clock moved back by the gap) and 03:30 EDT
  with `Later`.
  Error display text, one sentence each, no trailing period:
  `ZoneErr.Unknown` is `unknown time zone "Mars/Olympus"`, `NoLocal` is
  `cannot determine the local time zone: <msg>`; `CivilErr.Invalid` is
  `invalid civil time: <msg>` (`month 13 out of range 1 to 12`),
  `Skipped` is `2026-03-08T02:30:00 America/New_York does not exist
  (skipped by a daylight saving change)`, `Repeated` is
  `2026-11-01T01:30:00 America/New_York happens twice (repeated by a
  daylight saving change)`; `ParseErr` is `at byte 5 of "2026-1x-09":
  expected 2 digit month` and `LayoutErr` is `bad layout "{yaer}":
  unknown field {yaer}`. The payload fields stay readable for code that
  matches on the error.
- **Layouts.** `{frac}` and `{frac:N}` include the leading `.` (so
  `{frac}` prints nothing for a whole second). `{offset}` prints `Z` for
  a zero offset, `{offset:compact}` prints `+0000`. `{hour12}` needs
  `{ampm}` in the same layout (a layout error otherwise). `{weekday}`
  alone is not a field. `LayoutErr` displays as
  `bad layout "{yaer}": unknown field {yaer}`. A constant layout error is
  `XO0420` (did you mean fix for a misspelled field); a layout written
  with Java, C#, or Swift letters (`{yyyy}`, `yyyy-MM-dd`), strftime
  codes (`%Y-%m-%d`), or Go's reference date (`2006-01-02`) is `XO0177`
  with a fix to the named fields when every piece has one (`%y`, a two
  digit year, has none).
- **Parsing.** Numeric fields have exactly their width (`{day:1}` and
  `{month:1}` take one or two digits). Fields missing from the layout
  default to 1970-01-01 and 00:00:00. A parsed weekday is read but not
  checked against the date (Go's time.Parse ignores it). Without `{offset}`, `{zone}` picks the instant whose abbreviation
  in `z` matches (`UTC` and `GMT` also mean +00:00), and a civil time in a
  gap or overlap with neither is a `ParseErr` (no silent choice).
- **HTTP dates.** `RFC1123` formatted in `utc()` ends in `UTC`, while
  HTTP requires `GMT`: write the zone literally,
  `"{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} GMT"`
  with `utc()`. Parsing with `RFC1123` accepts `GMT`.

---

### 13.6 Rounding, weekday numbers, Ticker (decision 0414)

```
fn (w: Weekday) number() -> Int                    // ISO 8601: Monday 1 to Sunday 7
fn weekday_from(n: Int) -> Weekday?                // None outside 1 to 7
fn truncate(t: Time, d: Duration) -> Time          // Go's Time.Truncate
fn round(t: Time, d: Duration) -> Time             // Go's Time.Round, halves up
struct Ticker { clock: Clock, period: Duration, next_ns: Atomic[Int] }
fn ticker(clock: Clock, period: Duration) -> Ticker uses clock
fn (t: Ticker) wait() -> Time uses clock
fn (t: Ticker) reset(period: Duration) -> Ticker uses clock
```

- `truncate` and `round` count multiples of `d` from the zero time
  (January 1 of year 1, UTC), as Go does, so for a `d` that divides a
  day the result is the start of the UTC minute, hour, or day; a `d` of
  0 or less returns `t`. They are functions of std/time, not methods of
  the prelude's `Time`.
- A ticker needs no task: `wait` sleeps until the next tick and returns
  its scheduled time. Ticks are at the ticker's start plus multiples of
  its period. A caller that falls behind gets the pending tick at once
  and the ticks after it that it missed are dropped, the phase kept (Go's
  channel of one). Each tick goes to one waiter: two tasks waiting on one
  ticker (or on copies of it, which share the schedule) get different
  ticks. `reset` returns a ticker whose first tick is one new period from
  now. A period of 0 or less faults (`time: non-positive interval for
  ticker`, Go's panic text). Under a `FakeClock` the ticks are virtual.

## 14. std/cli

Status: implemented on every backend (decision 0160, 2026-10-11): the Go
backend in `std/rt/cli.go`, the native backends in `std/native/cli.xo`,
with the same help text, usage errors, and faults (the golden programs
`internal/llvm/testdata/golden/cli_*` compare them).

Command line parsing for tools. A command is a value built by functions
(no struct tags, no reflection, no builder state to get out of order);
`parse` reads the arguments into a `Parsed` with typed accessors.

### 14.1 Declaring a command

```
struct Command {}      // handles
struct Flag {}
struct Arg {}

pub fn command(name: Str, help: Str, flags: List[Flag] = [], args: List[Arg] = [],
               commands: List[Command] = [], version: Str = "",
               single_dash_long: Bool = false, stop_at_positional: Bool = false) -> Command

pub fn bool_flag(name: Str, help: Str, short: Str = "", env: Str = "", default: Bool = false) -> Flag
pub fn str_flag(name: Str, help: Str, short: Str = "", default: Str = "", env: Str = "",
                required: Bool = false, value: Str = "str") -> Flag
pub fn int_flag(name: Str, help: Str, short: Str = "", default: Int = 0, env: Str = "",
                required: Bool = false) -> Flag
pub fn duration_flag(name: Str, help: Str, short: Str = "", default: Duration = 0s, env: Str = "",
                     required: Bool = false) -> Flag
pub fn float_flag(name: Str, help: Str, short: Str = "", default: Float = 0.0, env: Str = "",
                  required: Bool = false) -> Flag   // Float.parse syntax: 1.5, 2e-3, inf, NaN
pub fn choice_flag(name: Str, help: Str, choices: List[Str], short: Str = "", default: Str = "",
                   env: Str = "", required: Bool = false) -> Flag
pub fn strs_flag(name: Str, help: Str, short: Str = "", env: Str = "", value: Str = "str") -> Flag

pub fn arg(name: Str, help: Str, optional: Bool = false) -> Arg
pub fn rest(name: Str, help: Str, min: Int = 0) -> Arg
```

```
fn app() -> cli.Command {
  cli.command("tool", "Manages widgets.",
    flags: [
      cli.bool_flag("verbose", "print more", short: "v"),
      cli.str_flag("config", "the configuration file", default: "tool.toml", env: "TOOL_CONFIG", value: "path"),
    ],
    commands: [
      cli.command("add", "Adds widgets.",
        flags: [cli.int_flag("count", "how many", short: "n", default: 1)],
        args: [cli.arg("name", "the widget name")]),
      cli.command("remove", "Removes widgets.", args: [cli.rest("names", "widget names", min: 1)]),
    ],
    version: "1.4.0")
}
```

- `name` of the root command is the program's name in help and usage
  text; a subcommand's `name` is the word that selects it. A command has
  positional arguments or subcommands, not both.
- Command names are letters, digits, `.`, `_`, and `-` (not first);
  flag names are lowercase letters, digits, and `-` (not first);
  `short` is one ASCII letter or digit. `--help` and `-h` are reserved,
  and `--version` when the root command has a `version`. Flag names are
  unique along each path of commands (a subcommand may not repeat a flag
  of a command above it; sibling subcommands may share names).
- Positional arguments: required ones first, then optional ones, then at
  most one `rest`, last.
- A `bool_flag` is true when given (`--name`, `-s`) and takes `=false`
  or `=0` to turn it off; when it is not given (nor its `env` variable)
  `Parsed.bool` is its `default`, and the help shows `[default: true]`
  for a flag that defaults to true (2026-11-07, for Go flags such as
  goversioninfo's `-64` whose default depends on the host).
- `env` names a variable read when the flag is not on the command line
  (an empty value counts as unset; a `strs_flag` variable is split on
  `,`). Variables are read through the `Env` given to `parse`, so an
  attenuated `Env` (`only`, 3.5) hides unlisted ones.
- A mistake in the declaration (a bad or duplicate name, a short name
  used twice, a choice default not among the choices, an argument out of
  order, arguments and subcommands on one command) is a fault at the
  `parse` or `help` call: it is a bug in the program, not in its input.

### 14.2 Parsing

```
pub fn parse(cmd: Command, args: List[Str], env: Env) -> Parsed ! CliErr uses env
pub fn parse_or_exit(cmd: Command, env: Env, stdio: Stdio, proc: Proc) -> Parsed
    uses (env, stdio, proc)
pub fn help(cmd: Command, path: Str = "") -> Str     // path: "remote add"; faults if unknown
pub fn completion(cmd: Command, shell: Str) -> Str ! CliErr   // 14.5

enum CliErr {
  Help(text: Str)               // -h or --help: the selected command's help
  Version(text: Str)            // --version: "name version"
  Usage(msg: Str, usage: Str)   // a mistake in the arguments
}
fn (e: CliErr) display() -> Str  // the text of Help and Version, the msg of Usage
fn (e: CliErr) exit_code() -> Int // 0 for Help and Version, 2 for Usage
```

`args` are the arguments without the program's name (`Env.args`).
`parse_or_exit(cmd, env, stdio, proc)` parses `env.args()`; on `Help` or
`Version` it prints the text to standard output and exits with status 0,
on `Usage` it prints `error: <msg>`, an empty line, and `usage` to
standard error and exits with status 2 (the convention of Unix tools
for a usage error):

```
fn main(os: Os) ! {
  let p = cli.parse_or_exit(app(), os.env, os.stdio, os.proc)
  match p.command() {
    "add" => add(p.arg("name"), p.int("count"), p.bool("verbose"))?
    "remove" => remove(p.rest("names"))?
    _ => {}
  }
}
```

Rules, in order:

- `--name value`, `--name=value`, `-n value`, `-nvalue`, `-n=value`. A
  bool flag takes no separate value: `--verbose`, `-v`, or
  `--verbose=true|false|1|0`. Short bool flags combine (`-vq`); a short
  flag that takes a value ends the cluster (`-vn5`). A value may start
  with `-` (`--name -x`). Given twice, the last value wins, except for a
  `strs_flag`, which collects every value in order.
- `--` ends the flags: every later argument is positional. `-` alone is
  positional.
- `single_dash_long: true` (decision 0547; for ports of Go tools, whose
  users write `-icon=x.ico -64`) also reads `-name`, `-name=value` and
  `-name value` as the `--` forms when `name` is a declared long flag of
  the path (or `help`, or `version` when the root has one); otherwise the
  argument is a short cluster as above, and a word of more than one
  letter that is neither is `unknown flag -name` (with a did you mean of
  `-` form). It applies to the command that sets it and every
  subcommand under it. Under it, a long flag whose name also reads as a
  cluster of the path's short flags (`--output` beside `-o` taking a
  value, `--vq` beside bool `-v` and `-q`) is a declaration fault, except
  a one letter long flag whose short is that letter. Flags still come
  before or after positional arguments (Go's flag package stops at the
  first one; see `stop_at_positional`). Help text keeps the `--` forms.
- `stop_at_positional: true` (decision 0556; Go's flag package): the
  first positional argument (a lone `-` included) ends the flags, so it
  and every later argument are positional as written, `-64` included. A
  `--` before it ends the flags and is dropped; one after it is an
  argument. It applies to the command that sets it and every subcommand
  under it (a subcommand's name is not a positional), and `--help` says
  so under the usage line. Independent of `single_dash_long`.
- Flags may come before or after positional arguments. A subcommand is
  selected by the first positional argument of a command with
  subcommands; after it, the flags of every command on the path are
  accepted, those of the subcommand only after its name.
- `-h` or `--help` anywhere before `--` returns `Help` with the help of
  the command selected so far; `--version` (when the root has a version)
  returns `Version`.
- After the arguments: a command with subcommands and none selected is
  `missing command (one of a, b)`; then each flag of the path, in
  declaration order (root first), not given takes its variable, and a
  `required` one still missing is `missing required flag --name`; then
  positional arguments are assigned in order (`missing argument <name>`,
  also for a `rest` with fewer than `min`; `unexpected argument "x"` for
  one too many).
- Usage messages: `unknown flag --nmae (did you mean --name?)`,
  `unknown flag -x`, `unknown command "ad" (did you mean add?)`, `flag
  --count needs a value`, `invalid value "two" for --count: expected an
  integer` (`true or false`, `a duration such as 30s or 1m30s`, `one of
  text, json`), with ` (from $VAR)` before the colon for a value read
  from a variable. A suggestion is the closest name within two edits
  (fewer than the length of the word), the first declared on a tie.
- `usage` is `Usage: <path> [flags] <args>` (`<command>` for a command
  with subcommands), an empty line, and `Run '<path> --help' for more
  information.`

### 14.3 Reading the result

```
struct Parsed {}
fn (p: Parsed) command() -> Str          // the subcommand path, "" for the root ("remote add")
fn (p: Parsed) bool(name: Str) -> Bool
fn (p: Parsed) str(name: Str) -> Str     // also choice flags; the value, else the default
fn (p: Parsed) str_opt(name: Str) -> Str?
fn (p: Parsed) int(name: Str) -> Int     // also int_opt
fn (p: Parsed) duration(name: Str) -> Duration   // also duration_opt
fn (p: Parsed) float(name: Str) -> Float // also float_opt
fn (p: Parsed) strs(name: Str) -> List[Str]
fn (p: Parsed) is_set(name: Str) -> Bool // given on the command line or in its variable
fn (p: Parsed) arg(name: Str) -> Str     // a required positional argument
fn (p: Parsed) arg_opt(name: Str) -> Str?
fn (p: Parsed) rest(name: Str) -> List[Str]
```

- Flags are named without dashes. `_opt` accessors are `None` when the
  flag was not given (on the command line or in its variable); the plain
  ones return the default.
- A flag or argument of a command that was not selected reads as not
  given (its default, `None`, `[]`, `false`), so a program may read every
  flag up front; `arg` of one faults (it has no value to return).
- Reading a name that no command declares, or with the accessor of
  another type, is a fault naming the right accessor: `cli: --limit is
  declared by int_flag, so read it with int or int_opt`.

### 14.4 Help text

`help(cmd, path)` and `Help` give the same text: the command's help line
and an empty line (when it has one), the usage line, then sections
`Commands:`, `Arguments:`, `Flags:` (the command's flags, then `-h,
--help` and, when the root has a version, `--version`), and `Global
flags:` (the flags of the commands above), each row indented two spaces
and every description starting in one column, two spaces after the
widest left side. A flag's left side is `-n, --name <value>` (indented four
spaces, `--name <value>`, without a short name; no `<value>` for a bool flag), where value
is the `value` argument, `int`, `duration`, or the choices joined with
`|`. Its description is followed by `[default: x]` (when not empty or
zero), `[env: VAR]`, `[required]`, and `[repeatable]` as they apply.
No trailing newline.

```text
Manages widgets.

Usage: tool [flags] <command>

Commands:
  add                  Adds widgets.
  remove               Removes widgets.

Flags:
  -v, --verbose        print more
      --config <path>  the configuration file [default: tool.toml] [env: TOOL_CONFIG]
  -h, --help           show this help
      --version        show the version
```


### 14.5 Shell completions and versions

`completion(cmd, shell)` (decision 0165) returns a completion script for
`"bash"`, `"zsh"`, or `"fish"`, made from the declaration alone: the
program does not run while the user completes. Another shell is
`Usage("unknown shell \"x\" (one of bash, zsh, fish)", usage)`. A tool
offers it as a subcommand:

```
fn app() -> cli.Command {
  cli.command("tool", "Manages widgets.",
    commands: [
      cli.command("completion", "Prints a shell completion script.", args: [cli.arg("shell", "bash, zsh, or fish")]),
    ],
    version: VERSION)
}

fn main(os: Os) ! {
  let p = cli.parse_or_exit(app(), os.env, os.stdio, os.proc)
  if p.command() == "completion" {
    os.stdio.print(cli.completion(app(), p.arg("shell"))?)
  }
}
```

Users install it with `source <(tool completion bash)`, `tool completion
zsh > "${fpath[1]}/_tool"`, or `tool completion fish >
~/.config/fish/completions/tool.fish`. The scripts complete subcommand
names at each level, the flags accepted there (those of the commands
above included) with `-h`, `--help`, and `--version`, the values of
`choice_flag`s after their flag, and files after other flags that take a
value and where a command takes positional arguments (zsh and fish
describe each word with its help line). The text is fixed by the
implementation and the same on every backend; command names are letters,
digits, `.`, `_`, and `-`, so no quoting can break it.

A command's `version` usually comes from a constant stamped by the build:
`const VERSION: Str = "dev"` in the main module and `xo build --stamp
VERSION=1.4.0` (core 10.1, decision 0165) make `tool --version` print
`tool 1.4.0`.
---

## 15. std/term

Status: implemented on every backend (decision 0162, 2026-10-11).

Colors for terminal output that follow the user's settings. Whether a
stream is a terminal, its width, and input without echo are `Stdio`
methods (3.6).

```
struct Painter { on: Bool }       // wraps text in ANSI styles when on

pub fn color_enabled(out: Stdio, env: Env) -> Bool uses (stdio, env)
pub fn painter(out: Stdio, env: Env) -> Painter uses (stdio, env)
fn (p: Painter) bold(s: Str) -> Str   // also dim, underline, red, green, yellow, blue, magenta, cyan, gray
pub fn strip(s: Str) -> Str           // s without ANSI escape sequences
```

- `color_enabled`: `NO_COLOR` set and not empty is off (no-color.org);
  else `FORCE_COLOR` or `CLICOLOR_FORCE` set, not empty, and not `0` is
  on; else `TERM=dumb` is off; else on when standard output is a
  terminal. The variables are read through `env`, so an attenuated `Env`
  that does not list them reads as unset.
- A painter's methods return `s` unchanged when `on` is false or `s` is
  empty, else `ESC [ code m`, `s`, `ESC [ 0 m` with code 1 bold, 2 dim, 4
  underline, 31 red, 32 green, 33 yellow, 34 blue, 35 magenta, 36 cyan,
  90 gray. `Painter{on: false}` and `Painter{on: true}` are for tests.
- `strip` removes CSI sequences (`ESC [`, parameters, one final byte in
  `@` to `~`) and any other `ESC x` pair; text whose stripping would cut a
  UTF-8 sequence is returned unchanged.

```
fn report(out: Stdio, env: Env, failed: Int) uses (stdio, env) {
  let p = term.painter(out, env)
  if failed > 0 {
    out.println(p.red("${failed} failed"))
  } else {
    out.println(p.green("ok"))
  }
}
```

---

## 16. std/csv

Status: implemented on every backend (decision 0164, 2026-10-11; Go
parity decision 0298, 2026-10-21): the Go backend runs Go's encoding/csv
(`std/rt/csv.go`), the native backends port it (`std/native/csv.xo`);
Go's records, errors, and positions, and Go's Writer's text, on Go's
reader_test.go and writer_test.go inputs, on the 2,661 texts and 400 row
sets of the module test the corpus replaced, and on seeded random ones
(`internal/stdtest/testdata/corpus/csv`, generated by `go run
./scripts/stdcorpus csv`).

```
struct CsvErr { start_line: Int, line: Int, column: Int, msg: Str }   // Go's ParseError

pub fn parse(text: Str, sep: Rune = ',', comment: Rune = '\0', fields: Int = -1,
             lazy_quotes: Bool = false, trim_leading_space: Bool = false) -> List[List[Str]] ! CsvErr
pub fn parse_maps(text: Str, sep: Rune = ',', comment: Rune = '\0', lazy_quotes: Bool = false,
                  trim_leading_space: Bool = false) -> List[Map[Str, Str]] ! CsvErr
pub fn format(rows: List[List[Str]], sep: Rune = ',', crlf: Bool = false) -> Str
pub fn valid_sep(r: Rune) -> Bool           // a usable `sep`: Go's validDelim
struct Reader {}                            // records read from a File as they are asked for
pub fn reader(f: File, sep: Rune = ',', comment: Rune = '\0', fields: Int = -1,
              lazy_quotes: Bool = false, trim_leading_space: Bool = false) -> Reader
pub fn (var r: Reader) read() -> List[Str]? ! CsvErr uses fs.read   // None at the end of the file
pub fn valid_comment(r: Rune) -> Bool       // a usable `comment`, `'\0'` (none) included
```

- `csv.reader(file)` is Go's csv.NewReader(file) and `read()` its Read:
  the file is read in 64 KiB chunks as records are asked for (so a file
  larger than memory can be walked), a quoted field may hold line breaks,
  and records, errors and their line and column are `parse`'s on the same
  text. A failure of the file is a CsvErr with `line` 0 and the file's
  error text as `msg`. The options are `parse`'s.
- Go's csv.Reader: `sep` is Comma, `comment` Comment (`'\0'` for none:
  lines starting with it are skipped), `fields` FieldsPerRecord (-1 any
  number, the default here where Go's is 0; 0 the first record's; N
  exactly N), `lazy_quotes` LazyQuotes, `trim_leading_space`
  TrimLeadingSpace. RFC 4180 quoting; `\r\n` reads as `\n`; empty lines
  are skipped; a final `\r` ends the text as `\n` would.
- Errors are Go's ParseError: `msg` is `bare " in non-quoted-field`,
  `extraneous or missing " in quoted-field`, or `wrong number of
  fields`; line and column (1-based, bytes) are Go's; `display` is Go's
  Error() text: `parse error on line 1, column 5: ...`, `record on line
  2; parse error on line 3, column 1: ...`, `record on line 4: wrong
  number of fields`.
- `parse_maps` reads the first record as the header and makes a map per
  later record; every record must have the header's number of fields
  (FieldsPerRecord 0).
- `format` is Go's csv.Writer (`crlf`: UseCRLF): a field is quoted when it
  holds `sep`, a quote, `\r` or `\n`, starts with a space (Unicode), or
  is `\.`; quotes double. One difference: the only, empty field of a
  record is written `""` (Go writes an empty line, which reads back as no
  record), so `parse(format(rows))` gives `rows` back.
- `sep` and `comment` must be valid delimiters (not `"`, `\r`, `\n` or
  U+FFFD; not `\0` for `sep`; and not each other): a fault
  `csv: invalid field or comment delimiter` otherwise, where Go returns
  `errInvalidDelim` (decision 0298's note, 2026-10-23). `valid_sep` and
  `valid_comment` are Go's validDelim, `valid_comment` counting `'\0'`
  (Go's zero value, no comment) as valid, so a program whose separator or
  comment comes from data checks them, and a non-zero comment against the
  separator, before `parse` or `format` faults.

---

## 17. std/regexp

Status: implemented on the Go backend (decision 0164, Go's `regexp`)
and natively (decision 0172: Go's regexp/syntax parser, compiler, and
Pike VM ported to Xo, the same matches and errors). Every Unicode class
Go's regexp knows (`\p{Greek}`, `\pN`, `\p{Lu}`, `\p{Assigned}`, the
category aliases such as `\p{Letter}`) is in native builds too, from
Go's tables (Unicode 15.0.0), looked up by name as Go does.

```
struct Regex {}                          // an immutable compiled value (not a handle, core 2.9)
struct RegexErr { pattern: Str, msg: Str }   // `invalid regexp "(a": missing closing ): ...`

pub fn compile(pattern: Str) -> Regex ! RegexErr
pub fn must(pattern: Str) -> Regex       // faults on a bad pattern
pub fn quote(text: Str) -> Str           // matches text literally
fn (r: Regex) pattern() -> Str
fn (r: Regex) is_match(s: Str) -> Bool
fn (r: Regex) find(s: Str) -> Str?
fn (r: Regex) find_index(s: Str) -> (Int, Int)?     // byte offsets
fn (r: Regex) find_all(s: Str) -> List[Str]
fn (r: Regex) captures(s: Str) -> List[Str?]?       // [whole match, group 1, ...]
fn (r: Regex) named(s: Str) -> Map[Str, Str]?       // (?P<name>...) groups that took part
fn (r: Regex) replace_all(s: Str, repl: Str) -> Str // $1, ${name}, $$ in repl
fn (r: Regex) replace_all_literal(s: Str, repl: Str) -> Str
fn (r: Regex) split(s: Str) -> List[Str]
```

The syntax and semantics are RE2's as Go implements them: leftmost
first matching, no backreferences or lookaround, and time linear in the
input, so a pattern or input from a user cannot make matching run away.
In `replace_all`, `$name` takes the longest name it can (`$1x` is group
`1x`); write `${1}x`. Raw strings keep backslashes: `r"\d+"`.

---

## 18. std/archive

Status: implemented on the Go backend (decision 0164, Go's
`compress/gzip`, `archive/tar`, `archive/zip`) and natively (decision
0172: those packages ported to Xo, the same bytes and errors, zip64
archives and PAX sparse tar files included since 2026-10-21).

```
struct Entry { path: Str, data: Bytes, mode: Int, is_dir: Bool, modified: Time }
struct ArchiveErr { msg: Str }           // displays as `archive: msg`

pub fn file(path: Str, data: Bytes) -> Entry    // mode 0o644, modified at the Unix epoch
pub fn dir(path: Str) -> Entry                  // mode 0o755
pub fn gzip(data: Bytes) -> Bytes
pub fn gunzip(data: Bytes) -> Bytes ! ArchiveErr
pub fn tar(entries: List[Entry]) -> Bytes ! ArchiveErr
pub fn untar(data: Bytes) -> List[Entry] ! ArchiveErr
pub fn zip(entries: List[Entry]) -> Bytes ! ArchiveErr
pub fn unzip(data: Bytes) -> List[Entry] ! ArchiveErr
pub fn zip_reader(data: Bytes) -> ZipReader ! ArchiveErr   // decision 0479 step 3
fn (r: ZipReader) files() -> List[ZipFile]
fn (r: ZipReader) read(f: ZipFile) -> Bytes ! ArchiveErr
pub fn zip_writer(f: File) -> ZipWriter
fn (var w: ZipWriter) add(e: Entry) -> () ! ArchiveErr uses fs.write   // also create(path, mode), write(data), close()
struct ZipFile { path: Str, size: Int, compressed: Int, mode: Int, is_dir: Bool, modified: Time, offset: Int, method: Int, flags: Int, crc: Int }
```

- `zip_reader` reads a zip archive's central directory once and checks
  every entry as `unzip` does (paths, kinds, Go's errors), but inflates
  none: `r.files()` lists them (`size` is the inflated length) and
  `r.read(f)` inflates one and checks its CRC, so a program holding an
  archive of many large entries holds one inflated entry at a time, not
  all of them as `unzip` does. `zip_open(f, size)` does the same
  for a File (`Fs.stat` gives the size): the tail and the central
  directory are read, and an entry's bytes only when `read` asks (through
  `File.read_at`), so an archive larger than memory can be walked; the
  Go backend reads the File whole. `read` therefore `uses fs.read`. A
  streaming `EntryReader` is a later step of decision 0479.
- `zip_writer(f)` writes an archive to a File entry by entry
  (move only; the File stays the caller's, closed after `close`):
  `w.add(entry)` writes one whose data is in hand, `w.create(path, mode)`
  starts a streamed file entry that `w.write(data)` feeds (deflated as it
  comes, a data descriptor after it), and the next `add`, `create` or
  `close` ends it; `w.close()` writes the central directory. Memory is the
  directory records, not the data. Past 65,534 entries, or an entry, the
  directory or an offset at 4 GB, `zip` and `zip_writer` write zip64 as
  Go's writer does, byte for byte (decision 0499). (Decision 0479 sketched `create` returning an
  `EntryWriter`; a separate value cannot tell the writer how much it
  wrote, so the entry is the writer's current one.)

- In memory and pure: read the archive with `Fs.read`, write the
  entries with `Fs.write` (into a sub capability, so the archive's paths
  stay under it).
- `gzip` writes no name and no time in the header, so the output depends
  on the data only; `gunzip` reads concatenated members as one stream.
- Paths use `/`. Writing or reading an entry whose path is empty,
  holds a zero byte (a C API would truncate it), is
  absolute (`/x`, `C:/x`), or leaves the root (`../x`, `a/../../x`) is an
  error (zip slip); so is a tar or zip entry that is a link or a device.
  Directories are written with a trailing `/` and read without it. A PAX
  global header (`git archive` writes one holding the commit) describes
  the archive, not an entry, and `untar` skips it (2026-10-12) after the
  path rule checks its name (or its own `path` record), as Go's reader
  returns it under that name. Reading follows Go's archive/tar and
  archive/zip errors exactly; internal/stdtest/archivefuzz_test.go checks
  hostile archives against them on every backend.
- `tar` writes PAX headers with the entries' modes (`0o7777`) and times;
  `zip` deflates files and stores their modes and times (with Unix
  extended timestamps, so times before 1980 survive).

---

## 19. std/toml

Status: implemented on every backend (decision 0164): a TOML 1.0 parser
in `std/rt/toml.go` and the same parser in `std/native/toml.xo`, with
the same JSON text and errors (golden `toml_basic`).

```
use std/json

struct TomlErr { line: Int, msg: Str }   // displays as `toml: line 3: expected a value after =`
                                         // (line 0, an encode error: `toml: msg`)
enum DecodeErr { Syntax(err: TomlErr), Value(err: json.DecodeErr) }   // displays as the inner error

pub fn decode[T: Json](text: Str) -> T ! DecodeErr
pub fn to_json(text: Str) -> Str ! TomlErr
pub fn encode[T: Json](v: T) -> Str ! TomlErr
```

```
struct Server { host: Str, port: Int = 8080, timeout: Duration = 30s } derive Json
struct Config { name: Str, servers: List[Server] } derive Json

fn load(fs: Fs) -> Config ! Error uses fs.read {
  toml.decode[Config](fs.read_text("app.toml")?)?
}
```

- A TOML document maps onto JSON: a table is an object (keys in
  document order), an array an array, a string a string, an integer
  (decimal, `0x`, `0o`, `0b`, underscores between digits) a JSON
  integer, a float a JSON number (its text without `+` and underscores),
  a boolean a boolean, and a date or time its RFC 3339 text (a space
  between date and time and lower case `t` and `z` become `T` and `Z`;
  a field out of RFC 3339's range is an error, as Go's `time.Parse`
  checks it: a month 1 to 12, a day of that month (February 29 only in a
  leap year), hour 0 to 23, minute and second 0 to 59, an offset's hour
  0 to 23 and minute 0 to 59).
  `decode[T]` then follows the `derive Json` rules of section 5: missing
  keys take field defaults, unknown keys are ignored, `Duration` fields
  take `"1m30s"` text, an offset date-time decodes into `Time`, and a
  local date or time into `Str`.
- `inf` and `nan` are errors (JSON has no form for them).
- All of TOML 1.0 is read: comments, bare, quoted, and dotted keys,
  basic and literal strings and their multi-line forms (a line ending
  backslash trims the whitespace after it), `[table]` and `[[array of
  tables]]` headers, inline tables, and arrays over several lines with
  comments and a trailing comma. Defining a key or a table twice, adding
  to an inline table, and mixing a table with an array of tables are
  errors with their line. Conformance: toml-test v1.5.0's TOML 1.0.0
  documents (internal/stdtest/testdata/toml-test) and an invariant fuzz
  run on every backend (internal/stdtest/tomlfuzz_test.go).
- `to_json` gives the JSON text, for code that wants to read a document
  without a struct (`json.decode[Map[Str, ...]]`) or print it.
- `encode(v)` writes a document in the layout of
  pelletier/go-toml/v2's `Marshal` (std survey gap 16; golden
  `toml_encode`, whose `gotoml.go.txt` gives the same text through
  go-toml): in each table its plain keys first, then its tables
  (`[a.b]`, an empty line before a header that does not follow a
  header) and arrays of tables (`[[a]]`, a list of records that is not
  empty), in field order; other lists and records inside lists inline
  (`[1, 2]`, `{k = 'v'}`); a key bare when it is letters, digits, `_` and
  `-`, else quoted; a string a literal string (`'...'`) unless it holds
  `'` or a control character, then a basic string with `\b \t \n \f \r
  \" \\` and `\u00XX`; a float in decimal, never an exponent, with `.0`
  when it has no fraction. v is read through `json.to_value`, so a None
  field is left out, a `Time` or `Duration` is its JSON string (Go writes
  a `time.Time` as a bare date-time), a `Float` without a fraction is an
  integer, and a map's keys come in its JSON order (sorted, as Go);
  `decode` reads each back. A root that is not a record or a map, and a
  None inside a list (Go writes a nil pointer's zero value), are
  `TomlErr` with line 0: `toml: cannot encode an Int as a document
  root`, `toml: cannot encode a None in an array`.


---

## 20. std/url

Status: implemented on every backend (decision 0279, 2026-10-20): Go's
net/url (url.go of go1.26) on the Go backend, ported to Xo natively
(`std/native/url.xo`, Go's escape, query, and Values tables in
`url_test.xo`); golden `url_basic` on arm64, LLVM, Go, and wasm. Pure:
no capability.

```
struct EscapeErr { msg: Str }        // displays as Go's text: `invalid URL escape "%zz"`

pub fn query_escape(s: Str) -> Str   // space as +, as url.QueryEscape
pub fn query_unescape(s: Str) -> Str ! EscapeErr
pub fn path_escape(s: Str) -> Str    // one path segment: space as %20, / escaped
pub fn path_unescape(s: Str) -> Str ! EscapeErr

struct Values {}    // a value (not a handle): keys in first appearance order, each with its values in order
pub fn values() -> Values
pub fn parse_query(s: Str) -> Values ! EscapeErr   // a=1&b=2&a=3
fn (v: Values) get(key: Str) -> Str?                // the first value
fn (v: Values) all(key: Str) -> List[Str]           // every value, [] when missing
fn (v: Values) has(key: Str) -> Bool
fn (v: Values) keys() -> List[Str]
fn (v: Values) encode() -> Str                      // sorted by key, as Values.Encode
fn ([var] v: Values) add(key: Str, value: Str)
fn ([var] v: Values) set(key: Str, value: Str)      // replaces every value of key
fn ([var] v: Values) remove(key: Str)
```

- `query_escape` keeps letters, digits, `-`, `.`, `_`, and `~`, writes
  a space as `+`, and every other byte as `%XX` (upper case hex);
  `path_escape` also keeps `$ & + : = @` and writes a space as `%20`.
- `query_unescape` decodes `+` as a space and `%XX`; `path_unescape`
  leaves `+` alone. A `%` not followed by two hex digits is
  `EscapeErr`, its text naming the escape (`invalid URL escape "%4"`).
  A decoded byte sequence that is not UTF-8 keeps each invalid byte as
  its `%XX` escape (section 0's rule for URL parts), where Go returns
  the raw bytes.
- `parse_query` splits on `&`, then the first `=`; an empty pair is
  skipped and a pair without `=` has the value `""`. A `;` in a pair
  (`invalid semicolon separator in query`), a bad escape, or more than
  10000 parameters (`number of URL query parameters exceeded limit`) is
  `EscapeErr`. Go keeps the pairs parsed around an error; Xo returns
  the error alone.
- `encode` writes the keys sorted (byte order), each value as
  `key=value` with `query_escape`, joined by `&`; `parse_query` of it
  gives the same keys and values (in sorted key order).

```
var body = url.values()
body.add("grant_type", "authorization_code")
body.add("code", code)
c.send(http.new_request("POST", token_url,
  headers: {"Content-Type": "application/x-www-form-urlencoded"}, body: body.encode().bytes()))?
```

---

## 21. std/base64 and std/base32

Status: implemented on every backend (decision 0279, 2026-10-20): Go's
encoding/base64 and encoding/base32 on the Go backend, ported natively
(`std/native/base64.xo`, `base32.xo`; 3440 cases generated from Go in
their `_test.xo` files); golden `base64_basic`. Pure.

```
// std/base64
enum Base64 { Std, Url, RawStd, RawUrl }       // RFC 4648 sections 4 and 5; Raw: no padding
struct DecodeErr { at: Int }                   // displays as `illegal base64 data at input byte 4`
pub fn encode(data: Bytes, enc: Base64 = Std) -> Str
pub fn decode(s: Str, enc: Base64 = Std) -> Bytes ! DecodeErr

// std/base32
enum Base32 { Std, Hex, RawStd, RawHex }       // RFC 4648 sections 6 and 7
struct DecodeErr { at: Int }                   // displays as `illegal base32 data at input byte 4`
pub fn encode(data: Bytes, enc: Base32 = Std) -> Str
pub fn decode(s: Str, enc: Base32 = Std) -> Bytes ! DecodeErr
```

- Decoding follows Go's rules and error offsets (CorruptInputError):
  base64 skips `\r` and `\n` anywhere; base32 removes them first and
  counts the offset in what remains. A symbol outside the alphabet,
  padding in a Raw encoding, missing or misplaced padding in a padded
  one, and data after the padding are errors; the unused low bits of
  the last symbol need not be zero (Go's default, not `Strict`). Go
  returns the bytes decoded before an error; Xo returns the error.
- `Bytes.to_base64` and `Bytes.from_base64` (2.2) are `Std`. JWTs and
  cursors use `RawUrl`; TOTP secrets use base32 `RawStd` (remove spaces
  and upper case them first).

---

## 22. std/binary

Status: implemented on every backend (decision 0279, 2026-10-20): Go's
encoding/binary on the Go backend, ported natively
(`std/native/binary.xo`); golden `binary_basic`. Pure.

```
pub fn u16_be(n: Int) -> Bytes                 // the low 16 bits, 2 bytes; also u32_be (4), u64_be (8), and _le
pub fn get_u16_be(b: Bytes, at: Int) -> Int    // also get_u32_be, get_u64_be, and _le
pub fn get_i16_be(b: Bytes, at: Int) -> Int    // sign extended; also get_i32_be, get_i16_le, get_i32_le
pub fn uvarint(n: Int) -> Bytes                // n as an unsigned 64 bit word (binary.AppendUvarint)
pub fn varint(n: Int) -> Bytes                 // zig-zag (binary.AppendVarint)
pub fn get_uvarint(b: Bytes, at: Int) -> (Int, Int)?   // value and length; None when cut short or past 64 bits
pub fn get_varint(b: Bytes, at: Int) -> (Int, Int)?
```

- An Int is a 64 bit word here: a write keeps its low bits, and
  `get_u64_*` of a value past 2^63 - 1 is negative (as the Int word
  methods, decision 0260).
- A `get_` whose bytes run past the end faults with `binary: N bytes
  at A out of range for Bytes of length L`; `get_uvarint` and
  `get_varint` return None instead, as Go's `n <= 0` (also for a
  negative `at`).
- Build messages with a `BytesBuilder`: `bb.write(binary.u32_be(len))`.
## 23. std/mime

Status: implemented on every backend (decision 0282, 2026-10-20): Go's
mime (mediatype.go, type.go of go1.26) and net/http's sniff.go, on the
Go backend through Go's mime (the sniffing copied into std/rt) and
ported natively (`std/native/mime.xo`; Go's results on 1,300 generated
cases in `mime_test.xo`); golden `mime_basic`. Pure.

```
struct MediaType { kind: Str, params: Map[Str, Str] }   // kind and parameter names in lower case
struct MimeErr { msg: Str }                              // Go's texts: `mime: invalid media parameter`

pub fn parse_media_type(v: Str) -> MediaType ! MimeErr
pub fn format_media_type(kind: Str, params: Map[Str, Str]) -> Str
pub fn type_by_extension(ext: Str) -> Str?               // ".png" -> "image/png"
pub fn extensions_by_type(kind: Str) -> List[Str] ! MimeErr
pub fn detect_content_type(data: Bytes) -> Str
```

- `parse_media_type` is `mime.ParseMediaType`: the type before any `;`
  (lower cased, `type/subtype` or a single token, else `mime: no media
  type`, `mime: expected slash after first token`, ...), then `name=value`
  parameters (a token or a quoted string; a trailing `;` is allowed),
  with RFC 2231 continuations (`title*0=`, `title*1=`) and charsets
  (`filename*=utf-8''na%C3%AFve.txt`; utf-8 and us-ascii, others
  dropped). A malformed parameter is `mime: invalid media parameter`, a
  name given twice with different values `mime: duplicate parameter
  name`. Go also returns the media type with the first error; Xo returns
  the error alone. A value that decodes to bytes that are not UTF-8 is
  dropped.
- `format_media_type` is `mime.FormatMediaType`: lower cased type and
  names, parameters sorted by name, a value written as a token, quoted
  (`\"` and `\\` escaped), or RFC 2231 encoded when it holds a control or
  non ASCII byte; `""` when the type or a name is not a token.
- `type_by_extension` and `extensions_by_type` use Go's builtin table
  only (64 extensions), the same on every OS: no `/etc/mime.types`, which
  would be a file read outside the capabilities. A lookup tries the
  extension as given, then lower cased.
- `detect_content_type` is net/http's `DetectContentType` (the WHATWG
  MIME Sniffing algorithm over the first 512 bytes):
  `text/html; charset=utf-8`, `image/png`, `application/pdf`,
  `text/plain; charset=utf-8`, ..., else `application/octet-stream`.

---

## 24. std/uuid

Status: implemented on every backend (decision 0282, 2026-10-20),
following github.com/google/uuid v1.6.0 (outside Go's std; RFC 9562);
google/uuid's Parse results on its test inputs in
`std/native/uuid_test.xo`; golden `uuid_basic`. A `derive Json` struct
stores a UUID as its text, `u.display()`, and reads it back with
`uuid.parse`.

Since 0.34.0 a `Uuid` is two Ints, its 128 bits (decision 0387; it was
its canonical text through 0.33.0, where `u.0`, the pattern
`uuid.Uuid(p)` and `uuid.Uuid(s)` were deprecated as `XO0912`): those
uses are type errors now, and `u.display()`, `"${u}"`, `uuid.parse` and
`u.bytes()` are the ways in and out.

```
struct Uuid { ... } derive Ord            // 16 bytes as two Ints; its text, lower case 8-4-4-4-12: `u.display()`
struct UuidErr { msg: Str }               // google/uuid's texts: `invalid UUID length: 35`

pub fn v4(r: Rand) -> Uuid uses rand
pub fn v7(r: Rand, c: Clock) -> Uuid uses (rand, clock)
pub fn parse(s: Str) -> Uuid ! UuidErr
pub fn from_bytes(b: Bytes) -> Uuid ! UuidErr
pub fn nil() -> Uuid
fn (u: Uuid) version() -> Int
fn (u: Uuid) bytes() -> Bytes
fn (u: Uuid) display() -> Str
fn (u: Uuid) debug() -> Str            // Uuid("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
```

- A `Uuid` is its 16 bytes as two Ints (fields not visible outside
  std): equality and hashing compare the two words, and `a < b`
  (sorting, `min`, `max`; `derive Ord`) orders two UUIDs as their bytes,
  which is the order of their texts. `display()` makes the text on each
  call, as google/uuid's String does. It
  reads and writes as a Postgres `uuid` column through
  `sql.str(u.display())` and `uuid.parse(row.str(...)?)`.
- `v4` is 122 random bits from `r` with the version 4 and variant bits
  set; `v7` is the Unix time in milliseconds from `c` (48 bits), then
  random bits, so keys made later sort later to the millisecond (no
  counter within a millisecond: google/uuid keeps one in a package
  variable, which std does not have). Tests are reproducible: `os.rand`
  is seeded and `os.clock` virtual.
- `parse` takes `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`, `urn:uuid:` and
  that (prefix in any case), the 36 characters wrapped in two others (as
  google/uuid, `{...}`, without checking the braces), or 32 hex digits;
  hex digits in either case; `display()` of the result is the canonical
  lower case text.

---

## 25. std/mail

Status: implemented on every backend (decision 0282, 2026-10-20; messages
decision 0290): Go's net/mail addresses (message.go of go1.26) with mime's RFC 2047 encoded
words, through net/mail on the Go backend and ported natively
(`std/native/mail.xo`; Go's results on about 1,200 generated cases in
`mail_test.xo`); golden `mail_basic`. Pure.

```
struct Address { name: Str, address: Str }   // the display name decoded; local@domain
struct MailErr { msg: Str }                   // Go's texts: `mail: no address`, `mail: missing @ in addr-spec`
struct Message { headers: List[(Str, Str)], body: Bytes }   // decision 0290

pub fn parse_address(s: Str) -> Address ! MailErr
pub fn parse_address_list(s: Str) -> List[Address] ! MailErr
fn (a: Address) display() -> Str             // `"Ann Lee" <ann@example.com>`
pub fn read_message(msg: Bytes) -> Message ! MailErr
fn (m: Message) header(name: Str) -> Str?    // the first value, any case
pub fn parse_date(s: Str) -> Time ! MailErr
pub fn compose(from: Address, to: List[Address], subject: Str, text: Str,
               html: Str? = None, date: Time, id: Str) -> Bytes
```

- RFC 5322 as Go reads it: `name <local@domain>`, `local@domain
  (comment)` (the comment is the name), quoted names and local parts,
  domain literals holding an IP address (`a@[127.0.0.1]`), and groups
  (`team: a@x, b@y;`, flattened by `parse_address_list`; one address
  only for `parse_address`). Encoded words in names decode for utf-8,
  iso-8859-1, and us-ascii; another charset is `charset not supported:
  "koi8-r"`.
- `display` is Go's `Address.String`: the local part quoted when it is
  not a dot-atom, the name quoted when it is printable ASCII, else
  RFC 2047 encoded (B encoding when it holds a special character, else
  Q), in words of at most 75 bytes.
- `read_message` is Go's ReadMessage (message.go readHeader over
  textproto's continued lines): the fields in order, each name in
  canonical form when it is a valid token (`x-lower` is `X-Lower`, `Bad
  Key` stays), continuation lines unfolded with one space, values without
  leading spaces, and the body after the first blank line; a message that
  ends before a blank line keeps the fields read. Errors are Go's:
  `malformed initial line: ...`, `malformed header line: ...`, `EOF`.
- `parse_date` is Go's ParseDate: RFC 5322 dates with or without a
  weekday and seconds, one or two digit days, two digit years (69 to 99
  are 19xx), runs of spaces, a numeric zone, a zone name, or `UT`, and
  trailing comments. A zone name keeps the wall clock as UTC's (Go's
  `time.ParseInLocation` in UTC; `time.Parse` would take the local zone's
  offset for its own names, which depends on the machine). A date outside
  the years 1678 to 2262 is `mail: date out of range` (a `Time` is
  nanoseconds since 1970).
- `compose` writes `From`, `To`, `Subject` (encoded with mime's
  QEncoding when it is not ASCII), `Date` (RFC 1123 with a numeric zone,
  in UTC), `Message-ID: <id>`, `MIME-Version: 1.0`, and a text/plain body,
  or multipart/alternative with the HTML part (boundary `xo` and 28 hex
  digits of the SHA-256 of `id`), each body in quoted-printable (Go's
  Writer: lines of at most 76 bytes, CRLF); the bytes are those of Go's
  writers given the same boundary. Send them with `smtp.send` (29).
- Tests: `mail_test.xo` (Go's ReadMessage and ParseDate results on Go's
  test inputs and 3,000 random dates), golden `mail_messages` on every
  backend.

---

## 26. std/multipart

Status: implemented on every backend (decision 0282, 2026-10-20): Go's
mime/multipart over a body in memory, with net/textproto's header reader
and mime/quotedprintable's decoder, ported natively
(`std/native/multipart.xo`: Go's results on 1,400 generated bodies in
`multipart_test.xo`) and through mime/multipart on the Go backend;
golden `multipart_basic`. Pure except `random_boundary`.

```
struct Part { name: Str, filename: Str?, headers: Map[Str, Str], data: Bytes }
struct Form { values: url.Values, files: Map[Str, List[Part]] }
struct MultipartErr { msg: Str }

pub fn parse(body: Bytes, boundary: Str) -> List[Part] ! MultipartErr
pub fn parse_form(body: Bytes, boundary: Str, max_parts: Int = 1000) -> Form ! MultipartErr
pub fn write(parts: List[Part], boundary: Str) -> Bytes
pub fn form_data_content_type(boundary: Str) -> Str
pub fn random_boundary(r: Rand) -> Str uses rand
```

- `parse` is `Reader.NextPart` with each part read whole: the preamble
  and epilogue skipped, `\r\n` or bare `\n` lines, a part whose
  `Content-Transfer-Encoding` is `quoted-printable` decoded (and that
  header removed), Go's errors as text (`multipart: NextPart: EOF` for a
  body that ends without its closing boundary, `unexpected EOF` for a
  part cut short, `malformed MIME header line: ...`). A part's `name` is
  Content-Disposition `form-data`'s `name` and `filename` the last
  element of its `filename` (Go's `FileName`; `/` separated on every
  OS); `headers` holds the first value of each header, keys canonical.
- `parse_form` is `Reader.ReadForm` with the 32 MB of http's
  `ParseMultipartForm`: parts without a name are skipped, parts without a
  file name become `values` (in order of appearance), the others `files`;
  more than `max_parts` parts, or Go's memory and header limits, give
  `multipart: message too large`. Files stay in memory (Go writes those
  over the limit to disk). On the Go backend the header limits are per
  part (Go's NextPart), not per form.
- Text from the body (values, header values, names) is valid UTF-8,
  U+FFFD for each invalid byte (decision 0259); `data` is the bytes.
- `write` is `Writer` with `SetBoundary(boundary)`: parts in order, each
  part's headers sorted, a `Content-Disposition` made from `name` and
  `filename` (Go's `CreateFormField` and `CreateFormFile`, with
  `Content-Type: application/octet-stream` for a file) when the part has
  none. The boundary is used as given.
- Streaming parts waits for a streaming request body.
## 27. std/tls

A TLS client on a connection the program dialed (decision 0281): the
STARTTLS shape of Postgres's SSLRequest, SMTP, and Redis, where the
client speaks plaintext first and then upgrades the same socket.

```
enum Verify { Full, ChainOnly, None }
struct Roots {}                                    // handle: trusted certificates
struct TlsErr { msg: Str }
fn roots(pem: Bytes) -> Roots ! TlsErr
fn roots_of(certs: List[x509.Certificate]) -> Roots ! TlsErr
struct TlsCert {}                                  // handle: a certificate chain with its private key
fn cert(cert_pem: Bytes, key_pem: Bytes) -> TlsCert ! TlsErr
enum ClientAuth { None, Request, RequireAny, VerifyIfGiven, RequireAndVerify }
struct TlsConn {}                                  // handle
fn client(c: Conn, host: Str, roots: Roots? = None, verify: Verify = Verify.Full, alpn: List[Str] = [], cert: TlsCert? = None, verify_conn: (fn(List[x509.Certificate]) -> () ! Str)? = None) -> TlsConn ! NetErr uses net.dial
fn pin_sha1(pin: Str) -> (fn(List[x509.Certificate]) -> () ! Str) ! Str
fn pin_sha256(pin: Str) -> (fn(List[x509.Certificate]) -> () ! Str) ! Str
fn (t: TlsConn) read(max: Int) -> Bytes? ! NetErr  uses net.dial
fn (t: TlsConn) write(data: Bytes) -> () ! NetErr  uses net.dial
fn (t: TlsConn) close()
fn (t: TlsConn) set_deadline(clock: Clock, at: Time?) uses clock         // decision 0430: Go's tls.Conn SetDeadline
fn (t: TlsConn) set_read_deadline(clock: Clock, at: Time?) uses clock
fn (t: TlsConn) set_write_deadline(clock: Clock, at: Time?) uses clock
fn (t: TlsConn) alpn() -> Str                      // the protocol the server chose, "" for none
fn (t: TlsConn) version() -> Str                   // "TLS 1.2" or "TLS 1.3"
fn (t: TlsConn) peer_certificates() -> List[x509.Certificate]
```

```
fn pg_tls(os: Os, addr: Str, host: Str) -> tls.TlsConn ! Error uses net.dial {
  let c = os.net.dial(addr)?
  c.write(ssl_request())?
  if c.read(1)? != Some(b"S") {
    return Err(NetErr.Io("server refused TLS"))
  }
  tls.client(c, host)?
}
```

- `client` runs a TLS 1.2 or 1.3 client handshake (Go's default cipher
  suites and curves) on `c` and returns the encrypted connection. `c`
  belongs to the TlsConn from then on: a `read` or `write` on `c`, or a
  second `client`, fails with `Io("use of a connection upgraded to
  TLS")`; `close` sends close_notify and closes `c`; a `read` or `write`
  after `close` is `Io("use of closed network connection")`. `host` is
  the server name sent by SNI (not for an IP address) and the name
  verified.
- `verify`: `Full` checks the chain against the roots and the host name
  (an IP address against the certificate's IP addresses), as the http
  client does (4.6); `ChainOnly` checks the chain but not the name
  (libpq's `sslmode=verify-ca`); `None` checks nothing and only encrypts
  (`sslmode=require`): it accepts any certificate, so it does not stop an
  active attacker, and `xo vet` reports each use (`XO0902`).
- `roots`: `None` is the system's roots, as the http client's (macOS's
  trust settings; Linux's CA bundle and directories, `SSL_CERT_FILE` and
  `SSL_CERT_DIR`); `tls.roots(pem)` trusts exactly the certificates of
  `pem`, with http's `with_roots` parser and its error
  (`TlsErr("tls: no certificates found in PEM input")`);
  `tls.roots_of(certs)` trusts exactly `certs`, parsed with std/x509
  (41), `TlsErr("tls: no certificates")` when it is empty.
- `peer_certificates()` is the server's certificate chain as it sent it,
  leaf first, parsed (Go's `ConnectionState().PeerCertificates`); empty
  without TLS (FakeNet).
- Errors are `NetErr`s with Go's texts: a certificate that does not
  verify is `Io("tls: failed to verify certificate: x509: ...")`; a
  server that does not speak TLS is `Io("tls: first record does not look
  like a TLS handshake")`. The handshake must finish within 10 seconds,
  else `Timeout`; it and every read and write are cancellation points
  (`Canceled`, core 7.2).
- `alpn` lists the protocols offered (none by default); `alpn()` is the
  one the server chose.
- `cert(cert_pem, key_pem)` is http's `tls_cert` (4.6) with std/tls's
  error; `http.TlsCert` is this type (decision 0433). `client`'s `cert`
  is presented when the server asks for a certificate (mutual TLS);
  `ClientAuth` is the server's side, for `http.Server.with_client_auth`
  (4.6), in Go's order and meaning.

- Deadlines (decision 0430) are the connection's, as in Go's tls.Conn,
  with std 3.2's rules. A read that times out mid-record keeps what
  arrived, so the connection reads on after a new deadline. A deadline
  set on the Conn before `client` also limits the handshake (Go's
  Handshake), within its own 10 seconds. A write that fails (a timeout
  included, or a deadline already passed) may have sent part of a
  record, so every later write fails the same way, as Go's tls.Conn
  ("the TLS state is corrupt").
- Tests: on FakeNet `client` does no TLS: the TlsConn reads and writes the
  connection's plain bytes, `alpn()` is "" and `version()` is "TLS
  1.3", so a driver's tests can run an in-process fake server. Real TLS
  is covered by golden `tls_client` (every backend) and Docker
  differential tests against Go.
- Effects: `client` is `net.dial`, since only a dialed connection is
  upgraded, and so are a TlsConn's `read` and `write`, as a dialed
  `Conn`'s (decision 0280).
- Backends: native (std/native/llrt/tlsio.xo, the http client's TLS, on
  the connection's descriptor) and Go (crypto/tls; `ChainOnly` as
  InsecureSkipVerify with a check of the chain alone). Wasm has no Net.

---

## 28. std/netip

Status: implemented on every backend (decision 0287, 2026-10-20): Go's
net/netip, ported natively (`std/native/netip.xo`; Go's results on 3,323
address and prefix cases in `netip_test.xo`) and through net/netip on
the Go backend. Pure but for `lookup` (decision 0406: host's addresses
through `Net.lookup_host`, section 3.2).

```
struct Addr { bits: Int, hi: Int, lo: Int, zone: Str } derive Ord   // 32, 128, or 0; the 16 bytes as two words (top bit flipped, as Uuid); an IPv6 zone
struct Prefix { addr: Addr, len: Int }
struct ParseErr { msg: Str }   // ParseAddr("1.2.3"): IPv4 address too short

pub fn parse_addr(s: Str) -> Addr ! ParseErr
pub fn parse_prefix(s: Str) -> Prefix ! ParseErr
pub fn addr_from_bytes(b: Bytes) -> Addr?      // 4 or 16 bytes
pub fn lookup(n: Net, host: Str) -> List[Addr] ! NetErr uses net.dial   // decision 0406
fn (a: Addr) is_4() -> Bool                   // also is_6, is_4in6, is_loopback, is_private,
                                              // is_unspecified, is_link_local_unicast, is_multicast
fn (a: Addr) unmap() -> Addr
fn (a: Addr) bytes() -> Bytes                 // 4 or 16 bytes (AsSlice)
fn (a: Addr) display() -> Str                 // 1.2.3.4, 2001:db8::1, ::ffff:1.2.3.4, fe80::1%eth0
fn (p: Prefix) contains(a: Addr) -> Bool
fn (p: Prefix) masked() -> Prefix
fn (p: Prefix) display() -> Str
```

- Parsing is Go's ParseAddr and ParsePrefix, errors included: IPv4
  without leading zeros, IPv6 with `::`, an embedded IPv4, and zones (not
  in a prefix). An IPv4 address is kept as `::ffff:a.b.c.d` in the words
  `hi` and `lo`, with `bits` 32; `==` and the order are Go's (`Compare`:
  the bit length, the bytes, then the zone).
- Migration (decision 0485, Breaking): `Addr` has two words instead of the
  field `ip: Bytes`, so an address is three words and a zone with no
  allocation. A program that read `a.ip` reads `a.bytes()` (4 bytes for
  IPv4, 16 for IPv6, Go's `AsSlice`; `ip` was always 16) and builds an
  address from bytes with `netip.addr_from_bytes(b)`.
- A typical use, the client address behind trusted proxies:
  `trusted.any(fn(p) { p.contains(peer) })`.

## 29. std/smtp

Status: implemented (decision 0290, 2026-10-20) on every backend: Go's
net/smtp SendMail as one call, on the program's own connection (so
FakeNet's raw connections serve its tests, decision 0292); build the
message with std/mail's `compose` (25).

```
struct SmtpErr { code: Int, msg: Str }                  // the server's reply, or code 0; displays `CODE MSG`, or MSG for code 0 (decision 0358)
enum Auth { None, Plain(user: Str, password: Secret[Str]), CramMd5(user: Str, secret: Secret[Str]) }
enum Security { StartTls, Tls, None } derive Display

fn send(net: Net, addr: Str, from: Str, to: List[Str], msg: Bytes,
        auth: Auth = Auth.None, security: Security = Security.StartTls,
        roots: Bytes = b"", hello: Str = "localhost") -> () ! SmtpErr uses net.dial
```

- The steps are SendMail's: the greeting, EHLO (HELO when refused),
  STARTTLS, AUTH, `MAIL FROM:<from>` (with ` BODY=8BITMIME` and
  ` SMTPUTF8` when offered), `RCPT TO:<to>` for each, DATA with the
  message dot stuffed and in CRLF lines, QUIT. A reply with another code
  than expected is `SmtpErr{code, msg}` with the server's text (a
  multi-line reply's lines joined by `\n`).
- `Security.StartTls` (the default) requires the server's STARTTLS
  (`smtp: server doesn't support STARTTLS` otherwise) and verifies the
  host name through std/tls (against `roots`, a PEM bundle, when given);
  `Tls` is TLS from the first byte (port 465); `None` is plain text and
  is refused unless the host is `localhost` or a loopback address
  (owner, decision 0290).
- Go's checks and texts: a CR or LF in `from`, an address, or `hello` is
  `smtp: A line must not contain CR or LF`; PLAIN over plain text to a
  host that is not loopback is `unencrypted connection`; AUTH to a server
  that does not offer it is `smtp: server doesn't support AUTH`.
- Tests: golden test_smtp (a fake server on FakeNet, every backend),
  with CRAM-MD5 checked against net/smtp's auth_test vector.
## 30. std/template

Status: implemented on every backend (decision 0299, 2026-10-21): Go's
text/template, run by the Go backend itself (`std/rt/template.go`) and
ported natively (`std/native/template.xo`: lexer, parser, executor, the
built in functions, and fmt's printing); Go's output and error texts on
205 templates (`template_test.xo`) and golden `template_basic`.

```
struct TemplateErr { msg: Str }                          // Go's texts; displays the message (decision 0358)
type TemplateFn = fn(List[json.Value]) -> json.Value ! Str      // a template function (FuncMap, decision 0408)
struct Template { name: Str, sources: List[(Str, Str)], left: Str = "{{", right: Str = "}}", missing_key_error: Bool = false, funcs: Map[Str, TemplateFn] = {}, parsed: Parsed }

pub fn parse(name: Str, src: Str, left: Str = "{{", right: Str = "}}", funcs: Map[Str, TemplateFn] = {}) -> Template ! TemplateErr
pub fn parse_html(name: Str, src: Str, left: Str = "{{", right: Str = "}}", funcs: Map[Str, TemplateFn] = {}) -> Template ! TemplateErr   // html/template: contextual autoescaping (decision 0408)
fn (t: Template) add(name: Str, src: Str) -> Template ! TemplateErr
fn (t: Template) missing_key(error: Bool) -> Template
fn (t: Template) render(data: json.Value) -> Str ! TemplateErr
fn (t: Template) render_named(name: Str, data: json.Value) -> Str ! TemplateErr

pub fn html(s: Str) -> json.Value      // trusted content (decision 0408): json.Value.Trusted("html", s); Go's template.HTML
pub fn css(s: Str) -> json.Value       // CSS
pub fn js(s: Str) -> json.Value        // JS
pub fn jsstr(s: Str) -> json.Value     // JSStr
pub fn url(s: Str) -> json.Value       // URL
pub fn attr(s: Str) -> json.Value      // HTMLAttr
pub fn srcset(s: Str) -> json.Value    // Srcset
```

- `Template.parsed` is an opaque `Parsed` (std internal state, decision 0319): `parse` and `add` fill it with the parsed trees, which only the native implementation reads, so a render does not parse the sources again. A program does not build or read it; the Go backend leaves it empty.

- The data is a `json.Value` (5.1; `json.to_value(x)` for a type that
  derives Json). The template sees it as Go sees `encoding/json` data:
  numbers are int64 (an Int) or float64, objects `map[string]interface
  {}` (`.name` is a field, `index` an element; `range` and printing in
  sorted key order), arrays `[]interface {}`, null a nil.
- Everything of text/template: text and actions, `{{- ` and ` -}}` trim
  markers, comments, variables (`$x := `, `$x = `), pipelines, `if`,
  `else if`, `range` (lists, objects, integers, `break`, `continue`,
  `else`), `with`, `define`, `template`, `block`, constants (booleans,
  strings, numbers including hex, runes, and complex), and the built in
  functions `and`, `or`, `not`, `len`, `index`, `slice`, `eq`, `ne`, `lt`,
  `le`, `gt`, `ge`, `print`, `println`, `printf` (fmt's verbs and flags),
  `html`, `js`, `urlquery`, and `call` (no function values in JSON data,
  so `call` always fails). A missing field prints `<no value>`;
  `missing_key(true)` makes it an error.
- Custom functions (Go's FuncMap, decision 0408) are `parse`'s `funcs`:
  a name the templates call as they call a built in one (`{{upper .s}}`,
  `{{.s | upper}}`); the set's `add`ed templates see them too. A function
  gets its arguments as json.Values (the data's values, a constant as its
  value, a missing field as Null) and returns the value the template
  prints or passes on; an Err is Go's `error calling NAME: text`. A name
  neither built in nor in `funcs` is the parse error `function "x" not
  defined`. Functions are pure (`TemplateFn` has no effects).
- The html mode (`parse_html`, decision 0408) is Go's html/template:
  the same language, with every action's output escaped for the context
  it lands in (HTML text, RCDATA, an attribute name or value, a URL and
  its query, JavaScript code, strings, template literals and regexps,
  CSS and its strings and `url()`; comments are dropped), Go's context
  machine ported (context.go, transition.go, the escapers of html.go,
  attr.go, url.go, js.go, css.go, and the walk over the parsed trees of
  escape.go, all in `std/native/template.xo`);
  the Go backend runs html/template. The escaping errors are render's,
  as Go's are Execute's: a template ending inside a tag or script
  (`html/template:t: ends in a non-text context: {stateJS ...}`),
  branches ending in different contexts (`html/template:t:1:5: {{if}}
  branches end in different contexts: ...`), an action in an ambiguous
  part of a URL, a predefined escaper (`html`, `urlquery`) in a pipeline
  (`predefined escaper "html" disallowed in template`), an unfinished
  JS escape or regexp charset, a `/` that could start a division or a
  regexp. Go's TestEscape (the cases over Go's data as JSON, generated
  by a Go program), TestErrors and TestEscapeSet, and the escapers' own tests, are in
  `template_test.xo`.
- Trusted content (decision 0408): `template.html(s)` and the six other
  constructors make a `json.Value.Trusted` the html mode passes through
  unescaped in its own context (an `html` value in HTML text, a `url` in
  an attribute's URL, a `js` in a script) and escapes as text in any
  other (an `html` value in an attribute is its text with the tags
  stripped, as Go does); a template parsed with `parse` prints its
  string. Decoded JSON never makes one: `{"$html": "<script>"}` is an
  object and renders escaped.
- Errors are Go's: parse errors `template: t:1: function "nope" not
  defined`, execution errors `template: t:1:7: executing "t" at
  <.name.x>: can't evaluate field x in type interface {}`.
- `render` parses the set again each time (a parsed set is not a value
  yet); parse once with `parse` to check a template early.

## 31. std/embed

Files embedded at build time (decision 0304; core 3.1): a constant
`const SITE: embed.Dir = embed("static")` holds every file the path or
glob names, keyed by its slash path relative to the directory of the
declaring file (`static/index.html`), as Go's embed.FS. The checker fills
the Dir in; a program does not build one. Its methods read the data in
the binary and need no capability:

```
struct Dir { files: Map[Str, Bytes] }
fn (d: Dir) read(path: Str) -> Bytes?
fn (d: Dir) read_text(path: Str) -> Str?     // None also when not valid UTF-8
fn (d: Dir) contains(path: Str) -> Bool
fn (d: Dir) paths() -> List[Str]             // sorted
fn (d: Dir) sub(dir: Str) -> Dir             // the files under dir, keyed relative to it
fn (d: Dir) len() -> Int
fn (d: Dir) fs() -> Fs                       // a read-only Fs over the files
```

A single file embeds as a `Str` or `Bytes` constant instead (core 3.1).

`d.fs()` serves code written against `Fs` (a static file handler, a
template loader; decision 0307 makes it an `Fs` on the native backends).
Paths are relative to the Dir's keys; `/`, `.`, and a leading `./` name
the root, and a path that climbs above it is `NotFound`. Directories are
the ones the files' paths imply. `read`, `read_text` (`Io` when not valid
UTF-8), `stat` (size, `modified` the Unix epoch), `exists`, `read_dir`,
`walk`, `glob` (as `Fs.glob`), `mode` (0o444, directories 0o555),
`read_link` (always `None`), `sub`, `cwd_path` (`/` and the sub
directory), `read_only` and `write_only` (which denies reads) work as on
the real Fs; `write`, `create_temp`, `rename`, `remove`, `remove_all`,
`create_dir_all`, `set_modified`, `append`, `copy`, `set_mode`,
`symlink`, `lock`, `try_lock`, and `open` (no stream over embedded bytes
yet: read the whole file) are `FsErr.Denied`. Nothing needs a capability
beyond the `uses fs.read` that calling an `Fs` method states.

## 32. std/compress

Status: implemented on every backend (decision 0305, 2026-10-21), over
whole buffers and streaming; `http.gzip_responses` of 0305 follows. Go's compress/gzip, compress/flate,
and compress/zlib: the Go backend calls them, and the native backends
port them (std/native/llrt/flate.xo, which std/archive uses too). Every
level writes the bytes Go's writers write, and the decoders give Go's
output or Go's error. Pure.

```
enum Level { NoCompression, BestSpeed, Default, BestCompression, HuffmanOnly, Of(n: Int) }
struct CompressErr { msg: Str }   // Go's texts; displays as msg

pub fn gzip(data: Bytes, level: Level = Level.Default) -> Bytes
pub fn gunzip(data: Bytes, max_size: Int = 1073741824) -> Bytes ! CompressErr
pub fn deflate(data: Bytes, level: Level = Level.Default) -> Bytes       // raw RFC 1951
pub fn inflate(data: Bytes, max_size: Int = 1073741824) -> Bytes ! CompressErr
pub fn zlib(data: Bytes, level: Level = Level.Default) -> Bytes          // RFC 1950
pub fn unzlib(data: Bytes, max_size: Int = 1073741824) -> Bytes ! CompressErr

enum Format { Gzip, Deflate, Zlib }
struct Compressor {}                     // move only (core 5.3)
pub fn compressor(format: Format, level: Level = Level.Default) -> Compressor
fn (var c: Compressor) write(data: Bytes) -> Bytes   // the output ready so far (may be empty)
fn (var c: Compressor) flush() -> Bytes              // everything so far: Go's Flush, a sync block
fn (var c: Compressor) finish() -> Bytes             // the rest and the trailer; moves c
struct Decompressor {}                   // move only
pub fn decompressor(format: Format, max_size: Int = 1073741824) -> Decompressor
fn (var d: Decompressor) write(data: Bytes) -> Bytes ! CompressErr
fn (var d: Decompressor) finish() -> Bytes ! CompressErr      // the rest; moves d
```

- Streaming is push style: bytes in and bytes out, with no I/O of its own,
  so it fits a `Stream`, an `http.BodyWriter`, or a file read in pieces.
- **Compressor.** The concatenation of a Compressor's outputs is the bytes
  Go's Writer writes for the same Write, Flush and Close calls. `flush`
  and `finish` return everything up to that point on every backend.
  Between them, how much a `write` returns may differ by backend: Go's
  bit writer buffers up to 240 bytes.
- **Decompressor.** The concatenation of a Decompressor's outputs is what
  `gunzip`, `inflate`, or `unzlib` gives for the whole input.
  - How much a `write` returns may differ by backend. Natively it is each
    whole DEFLATE block once its input has arrived. On the Go backend it is
    what Go's reader returns, which can hold a member's last block until
    the next member or the end.
  - `finish` returns the rest, or `unexpected EOF` if the stream did not
    end.
  - More than `max_size` bytes in all is an error at once.

- `Level.Of(n)` is Go's level n, -2 (HuffmanOnly) to 9 (BestCompression);
  outside that range it is a fault (`compress: invalid compression level
  12: want value in range [-2, 9]`). `Default` is 6.
- `gzip` writes no name and no time, so the output depends only on the
  data and the level. `gunzip` reads every concatenated member, as Go's
  multistream reader does.
- The decoders stop at `max_size` (default 1 GiB) with
  `gzip: output exceeds max_size (N bytes)` (`flate:`, `zlib:`). Go has
  no limit; this is Go's reader through `io.LimitReader(r, max_size + 1)`,
  so an error found within the first max_size + 1 bytes of output (a bad
  checksum, a truncated stream) is reported in its place. Pass an
  explicit limit for untrusted input: a few KB of gzip can expand to
  gigabytes.
- Errors are Go's: `gzip: invalid header`, `gzip: invalid checksum`,
  `flate: corrupt input before offset N`, `zlib: invalid header`,
  `zlib: invalid checksum`, `zlib: invalid dictionary` (a preset
  dictionary other than the empty one), `unexpected EOF`, and `EOF` for
  an empty gzip input.
- Tests: internal/stdtest/testdata/corpus/compress (`go run
  ./scripts/stdcorpus compress`): 720 writes over every level and format
  against Go's bytes, 160 Compressors with Go's chunked writes and
  flushes, and 431 Go-written, corrupted, bounded, and bomb streams
  against Go's results, whole and fed to a Decompressor in pieces.

## 33. std/big

Status: implemented on every backend (decision 0311, 2026-10-21), stage
1: schoolbook multiplication and division natively. Go's math/big Int as
an immutable value: the Go backend calls math/big, and the native
backends port its nat arithmetic (std/native/big.xo). Every operation
gives Go's result. Variable time: not for secrets. Pure.

```
struct BigInt { key: List[Int] } derive Eq, Ord, Hash   // build with the functions, not a literal
struct BigErr { msg: Str }
pub fn from_int(n: Int) -> BigInt
pub fn parse(s: Str, base: Int = 10) -> BigInt ! BigErr    // Go's SetString; base 0 reads prefixes and _
pub fn from_bytes(b: Bytes) -> BigInt                      // unsigned big-endian
fn (x: BigInt) add(y: BigInt) -> BigInt        // also sub, mul
fn (x: BigInt) quo(y: BigInt) -> BigInt        // also rem, div, mod
fn (x: BigInt) quo_rem(y: BigInt) -> (BigInt, BigInt)   // (quo, rem); div_mod gives (div, mod)
fn (x: BigInt) exp(y: BigInt, m: BigInt? = None) -> BigInt
fn (x: BigInt) gcd(y: BigInt) -> BigInt
fn (x: BigInt) mod_inverse(m: BigInt) -> BigInt?
fn (x: BigInt) sqrt() -> BigInt                // also neg, abs, not
fn (x: BigInt) sign() -> Int                   // -1, 0 or 1
fn (x: BigInt) cmp(y: BigInt) -> Int           // -1, 0 or 1 as x <, ==, > y; also cmp_abs
fn (x: BigInt) lsh(n: Int) -> BigInt           // also rsh
fn (x: BigInt) and(y: BigInt) -> BigInt        // also or, xor, and_not
fn (x: BigInt) bit(i: Int) -> Int              // 0 or 1
fn (x: BigInt) bit_len() -> Int                // also trailing_zeros
fn (x: BigInt) probably_prime(n: Int) -> Bool
fn (x: BigInt) text(base: Int = 10) -> Str
fn (x: BigInt) bytes() -> Bytes                // |x| big-endian
fn (x: BigInt) fill_bytes(len: Int) -> Bytes
fn (x: BigInt) to_int() -> Int?
fn (x: BigInt) to_float() -> Float
```

- **The type is `BigInt`.** `Int` is predeclared (core 1.4), so Go's
  `big.Int` is `big.BigInt` here.
- **Order.** `<`, `==`, sorting, and Map keys use the derived Eq, Ord,
  and Hash. These are the numbers' order and equality, because `key`
  encodes the sign and limbs in order: zero is [0]; a number of n limbs
  is n (-n when negative), then its limbs from the most significant, each
  xor 2^63 (xor 2^63 - 1 when negative).
- **Division.** `quo` and `rem` truncate (Go's Quo and Rem). `div` and
  `mod` are Euclidean (Go's Div and Mod). A zero divisor is a fault, as
  is a zero modulus in `mod_inverse`.
- **Bitwise operations** act on two's complement, as Go's do. `rsh`
  rounds toward negative infinity.
- **`exp`** follows Go's Exp, including the sign rule for a negative base.
  With m, a negative exponent uses the inverse, and a missing inverse is
  a fault where Go returns nil.
- **`probably_prime`** runs Go's checks, Miller-Rabin with base 2, and
  the extra strong Lucas test (Baillie-PSW). Go's n extra rounds use
  pseudorandom bases; natively they use the first primes. No input is
  known where the two answers differ.
- **Json.** BigInt does not derive Json, which would write its key. Use
  `text` and `parse`.
- **Tests.**
  - Golden `big_basic` on every backend.
  - A Go-generated corpus of 2,600 operations on operands up to 1,500
    bits, plus Go's SetString on valid and invalid texts
    (internal/stdtest/testdata/corpus/big, scripts/stdcorpus big).

## 34. std/xml

Status: implemented on every backend (decision 0314, 2026-10-21): stage
1, tokens and a tree, and stage 2, `derive Xml` (decision 0315, below). Go's encoding/xml: the Go backend calls it, and the
native backends port its Decoder and Encoder (std/native/xml.xo). The
tokens, trees, encodings, and errors are Go's. Pure.

```
struct Name { space: Str, local: Str } derive Eq, Ord, Hash
struct Attr { name: Name, value: Str }
enum Token { Start(name, attrs), End(name), Text(s), Comment(s), ProcInst(target, inst), Directive(s) }
struct XmlErr { line: Int, msg: Str }   // line > 0: displays as Go's "XML syntax error on line N: MSG"
pub fn tokens(text: Str, strict: Bool = true) -> List[Token] ! XmlErr
pub fn raw_tokens(text: Str) -> List[Token] ! XmlErr
struct Reader {}                                              // tokens one at a time (decision 0463)
pub fn reader(text: Str, strict: Bool = true) -> Reader
pub fn raw_reader(text: Str) -> Reader
fn (var r: Reader) next() -> Token? ! XmlErr                  // None at the end of the input
struct Element { name: Name, attrs: List[Attr], children: List[Node] }
enum Node { Elem(e), Text(s), Comment(s), ProcInst(target, inst), Directive(s), Raw(s) }   // Raw: written as is by encode (derive Xml's text)
pub fn parse(text: Str, strict: Bool = true) -> Element ! XmlErr
fn (e: Element) attr(local) -> Str?;  child(local) -> Element?;  children_named(local);  text() -> Str
fn (e: Element) encode(indent: Str = "") -> Str ! XmlErr
pub fn escape(s: Str) -> Str
pub fn escape_to(var out: StrBuilder, s: Str)
pub fn encode_value[T: Xml](v: T, indent: Str = "") -> Str ! XmlErr   // Go's Marshal, MarshalIndent
pub fn decode[T: Xml](text: Str) -> T ! XmlErr                         // Go's Unmarshal
```

- `tokens` is Go's Decoder.Token until EOF:
  - names are resolved against `xmlns` and `xmlns:p` (`space` holds the
    URL), and start and end tags are matched;
  - entities and CDATA are decoded, and `\r\n` becomes `\n`.
- `raw_tokens` is RawToken: prefixes as written, nothing matched.
- `reader` and `raw_reader` give the same tokens one `next()` at a time
  (Go's Decoder.Token and RawToken loop) without building the list, so a
  program that counts or filters tokens holds the input and one token. At
  the end `next` returns None; after an error every later call returns the
  same error. The input stays a `Str` (the reader holds it).
- `strict: false` is Go's `Strict = false` with `HTMLAutoClose` and
  `HTMLEntity`: unquoted attribute values, `&nbsp;`, `<br>` without an
  end tag.
- `parse` builds the root element from the tokens. Outside it only
  whitespace, comments, processing instructions, and directives may
  appear. Its own errors have line 0: `xml: no root element`,
  `xml: more than one root element`, and `xml: text outside the root
  element`.
- `encode` writes the tree's tokens with Go's Encoder (EncodeToken,
  Indent("", indent)), so a namespace is written as Go writes it: an
  `xmlns` on each element of that space, and generated prefixes for
  attributes. Go's EncodeToken errors (a comment holding `-->`, an `xml`
  processing instruction after the start) are errors.
- `escape` is Go's EscapeText; `escape_to` appends the same text to a
  builder.
- No DTD processing and no external entities, as in Go.
- `derive Xml` (decision 0315, core 2.9) is Go's Marshal and Unmarshal for
  a struct:
  - `encode_value` writes the root element named for the struct, the
    attributes, then the fields in order (character data where its field
    is, `a>b>c` parents shared with the previous field, omitempty, `-`
    skipped), with Go's text for numbers (`strconv` shortest `g`),
    booleans, and EscapeText for text; `indent` is MarshalIndent's.
  - `decode` reads attributes first, then the elements in document
    order, as Unmarshal does: a repeated element overwrites a scalar,
    adds to a List, and merges into a struct or Option struct; text of
    numbers and booleans is trimmed and parsed with `strconv`'s errors
    (`strconv.ParseInt` and `strconv.ParseFloat`: `parsing "x": invalid
    syntax`, or `value out of range` for an integer or a float that
    overflows); unknown elements
    and attributes are ignored; a text with no element is `EOF`.
  - Not as Go: the document is read whole before fields are set, so a
    syntax error anywhere wins over a value error before it, and text
    after the root element is a syntax error where Go stops reading;
    `innerxml`, `comment`, `any`, namespaces in tags, and XMLName are not
    supported yet.
  - The generated code calls std/xmlcodec (`Writer`, which writes an
    encoded struct's text with no element tree, `events`, `int_of`,
    `float_of`, `bool_of`, `float_text`, `parse_root`), written once in std/native/xmlcodec.xo and compiled by every backend
    (decision 0312). Programs do not call it.
  - Tests: a Go-generated corpus of 1,500 documents decoded into a
    struct and re-encoded (compact and indented) in the xml corpus, on
    go, arm64, and LLVM; errorcheck `derive_xml`.
  - Tests: a Go-generated corpus of the helpers themselves (1,600
    documents and piece lists, 51 texts, 22 floats) against
    `encoding/xml` and `strconv` on the same input
    (internal/stdtest/testdata/corpus/xmlcodec, scripts/stdcorpus
    xmlcodec).
- Tests:
  - Golden `xml_basic` on every backend.
  - A Go-generated corpus of 2,000 documents (namespaces, entities,
    CDATA, comments, processing instructions, bad names, cuts), with
    tokens in all three modes, parse, and encode, plus EscapeText
    (internal/stdtest/testdata/corpus/xml, scripts/stdcorpus xml).

<!-- BEGIN prelude index: generated from std/prelude.xo by `go run ./scripts/stdsync`; do not edit -->

## Appendix A. Prelude index

Every function and method of std/prelude.xo, by type, as declared there
(the sections above explain them); interface methods take `self`. A
`static` receiver is a function on the type (`Int.parse(s)`); one with an
UPPER_SNAKE name and no parameters is a constant, read without a call
(`Time.UNIX_EPOCH`).
520 declarations.

### Range

```
fn (r: Range[Int]) to_list() -> List[Int]
```

### Display

```
fn display(self) -> Str
```

### Error

```
fn display(self) -> Str
```

### Debug

```
fn debug(self) -> Str
```

### Int

```
fn (static: Int) parse(s: Str) -> Int?
fn (static: Int) parse_base(s: Str, base: Int) -> Int?
fn (n: Int) to_str_base(base: Int) -> Str
fn (n: Int) to_str_width(width: Int, base: Int = 10, zero: Bool = false, plus: Bool = false) -> Str
fn (n: Int) abs() -> Int
fn (n: Int) pow(e: Int) -> Int
fn (n: Int) wrapping_add(m: Int) -> Int
fn (n: Int) wrapping_sub(m: Int) -> Int
fn (n: Int) wrapping_mul(m: Int) -> Int
fn (n: Int) wrapping_shl(k: Int) -> Int
fn (n: Int) shr_logical(k: Int) -> Int
fn (n: Int) rotate_right(k: Int) -> Int
fn (n: Int) rotate_right32(k: Int) -> Int
fn (n: Int) swap_bytes() -> Int
fn (n: Int) mul_hi_unsigned(m: Int) -> Int
fn (n: Int) lt_unsigned(m: Int) -> Bool
fn (n: Int) le_unsigned(m: Int) -> Bool
fn (n: Int) leading_zeros() -> Int
fn (n: Int) trailing_zeros() -> Int
fn (n: Int) ones_count() -> Int
fn (n: Int) saturating_add(m: Int) -> Int
fn (n: Int) saturating_sub(m: Int) -> Int
fn (n: Int) saturating_mul(m: Int) -> Int
fn (n: Int) checked_add(m: Int) -> Int?
fn (n: Int) checked_sub(m: Int) -> Int?
fn (n: Int) checked_mul(m: Int) -> Int?
fn (n: Int) checked_div(m: Int) -> Int?
```

### Bool

```
fn (static: Bool) parse(s: Str) -> Bool?
```

### Float

```
fn (static: Float) parse(s: Str) -> Float?
fn (f: Float) floor() -> Float
fn (f: Float) to_fixed(prec: Int, width: Int = 0, zero: Bool = false, plus: Bool = false) -> Str
fn (f: Float) to_exp(prec: Int, width: Int = 0, zero: Bool = false, plus: Bool = false) -> Str
fn (f: Float) to_general(prec: Int, width: Int = 0, zero: Bool = false, plus: Bool = false) -> Str
fn (f: Float) ceil() -> Float
fn (f: Float) round() -> Float
fn (f: Float) trunc() -> Float
fn (f: Float) abs() -> Float
fn (f: Float) sqrt() -> Float
fn (f: Float) is_nan() -> Bool
fn (f: Float) pow(y: Float) -> Float
fn (f: Float) exp() -> Float
fn (f: Float) exp2() -> Float
fn (f: Float) expm1() -> Float
fn (f: Float) log() -> Float
fn (f: Float) log2() -> Float
fn (f: Float) log10() -> Float
fn (f: Float) log1p() -> Float
fn (f: Float) sin() -> Float
fn (f: Float) cos() -> Float
fn (f: Float) tan() -> Float
fn (f: Float) asin() -> Float
fn (f: Float) acos() -> Float
fn (f: Float) atan() -> Float
fn (f: Float) sinh() -> Float
fn (f: Float) cosh() -> Float
fn (f: Float) tanh() -> Float
fn (f: Float) cbrt() -> Float
fn (f: Float) atan2(x: Float) -> Float
fn (f: Float) hypot(y: Float) -> Float
fn (f: Float) fma(y: Float, z: Float) -> Float
fn (f: Float) mod(y: Float) -> Float
fn (f: Float) is_inf() -> Bool
fn (f: Float) to_bits() -> Int
fn (static: Float) from_bits(b: Int) -> Float
```

### Int8

```
fn (static: Int8) try(x: Int) -> Int8?
fn (static: Int8) wrapping_from(x: Int) -> Int8
```

### Int16

```
fn (static: Int16) try(x: Int) -> Int16?
fn (static: Int16) wrapping_from(x: Int) -> Int16
```

### Int32

```
fn (static: Int32) try(x: Int) -> Int32?
fn (static: Int32) wrapping_from(x: Int) -> Int32
```

### Int64

```
fn (static: Int64) try(x: Int) -> Int64?
```

### UInt8

```
fn (static: UInt8) try(x: Int) -> UInt8?
fn (static: UInt8) wrapping_from(x: Int) -> UInt8
```

### UInt16

```
fn (static: UInt16) try(x: Int) -> UInt16?
fn (static: UInt16) wrapping_from(x: Int) -> UInt16
```

### UInt32

```
fn (static: UInt32) try(x: Int) -> UInt32?
fn (static: UInt32) wrapping_from(x: Int) -> UInt32
```

### UInt64

```
fn (static: UInt64) try(x: Int) -> UInt64?
fn (static: UInt64) wrapping_from(x: Int) -> UInt64
fn (static: UInt64) from_bits(b: Int) -> UInt64
fn (u: UInt64) to_bits() -> Int
```

### Float32

```
fn (f: Float32) to_bits() -> UInt32
fn (static: Float32) from_bits(b: UInt32) -> Float32
```

### Rune

```
fn (static: Rune) try(x: Int) -> Rune?
fn (r: Rune) is_digit() -> Bool
fn (r: Rune) is_letter() -> Bool
fn (r: Rune) is_space() -> Bool
fn (r: Rune) is_upper() -> Bool
fn (r: Rune) is_lower() -> Bool
fn (r: Rune) is_title() -> Bool
fn (r: Rune) is_punct() -> Bool
fn (r: Rune) is_control() -> Bool
fn (r: Rune) is_graphic() -> Bool
fn (r: Rune) is_mark() -> Bool
fn (r: Rune) is_symbol() -> Bool
fn (r: Rune) is_number() -> Bool
fn (r: Rune) to_lower() -> Rune
fn (r: Rune) to_upper() -> Rune
fn (r: Rune) to_title() -> Rune
fn (r: Rune) utf8_len() -> Int
fn (r: Rune) is_print() -> Bool
fn (r: Rune) quote() -> Str
```

### Str

```
fn (s: Str) len() -> Int
fn (s: Str) rune_count() -> Int
fn (s: Str) is_empty() -> Bool
fn (s: Str) contains(sub: Str) -> Bool
fn (s: Str) starts_with(p: Str) -> Bool
fn (s: Str) ends_with(p: Str) -> Bool
fn (s: Str) equal_fold(other: Str) -> Bool
fn (s: Str) find(sub: Str) -> Int?
fn (s: Str) rfind(sub: Str) -> Int?
fn (s: Str) count(sub: Str) -> Int
fn (s: Str) find_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx
fn (s: Str) rfind_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx
fn (s: Str) contains_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (s: Str) find_any(chars: Str) -> Int?
fn (s: Str) rfind_any(chars: Str) -> Int?
fn (s: Str) contains_any(chars: Str) -> Bool
fn (s: Str) contains_rune(r: Rune) -> Bool
fn (s: Str) split_n(sep: Str, n: Int) -> List[Str]
fn (s: Str) split_after(sep: Str) -> List[Str]
fn (s: Str) split_after_n(sep: Str, n: Int) -> List[Str]
fn (s: Str) cut(sep: Str) -> (Str, Str)?
fn (s: Str) cut_prefix(prefix: Str) -> Str?
fn (s: Str) cut_suffix(suffix: Str) -> Str?
fn (s: Str) split(sep: Str) -> List[Str]
fn (s: Str) fields() -> List[Str]
fn (s: Str) split_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> List[Str] ! E uses fx
fn (s: Str) lines() -> List[Str]
fn (s: Str) trim() -> Str
fn (s: Str) trim_start() -> Str
fn (s: Str) trim_end() -> Str
fn (s: Str) trim_prefix(p: Str) -> Str
fn (s: Str) trim_suffix(p: Str) -> Str
fn (s: Str) trim_chars(chars: Str) -> Str
fn (s: Str) trim_start_chars(chars: Str) -> Str
fn (s: Str) trim_end_chars(chars: Str) -> Str
fn (s: Str) trim_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Str ! E uses fx
fn (s: Str) trim_start_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Str ! E uses fx
fn (s: Str) trim_end_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Str ! E uses fx
fn (s: Str) to_lower() -> Str
fn (s: Str) to_upper() -> Str
fn (s: Str) map_runes[E, fx: Effects](f: fn(Rune) -> Rune? ! E uses fx) -> Str ! E uses fx
fn (s: Str) replace(old: Str, new: Str) -> Str
fn (s: Str) replace_n(old: Str, new: Str, n: Int) -> Str
fn (s: Str) repeat(n: Int) -> Str
fn (s: Str) pad_left(width: Int, fill: Str = " ") -> Str
fn (s: Str) pad_right(width: Int, fill: Str = " ") -> Str
fn (s: Str) center(width: Int, fill: Str = " ") -> Str
fn (s: Str) clone() -> Str
fn (s: Str) runes() -> List[Rune]
fn (s: Str) bytes() -> Bytes
fn (s: Str) quote() -> Str
fn (s: Str) unquote() -> Str?
```

### Bytes

```
fn (b: Bytes) len() -> Int
fn (b: Bytes) is_empty() -> Bool
fn (b: Bytes) to_str() -> Str?
fn (b: Bytes) to_str_lossy() -> Str
fn (b: Bytes) to_valid_utf8(replacement: Str) -> Str
fn (b: Bytes) to_hex() -> Str
fn (b: Bytes) clone() -> Bytes
fn (b: Bytes) to_base64() -> Str
fn (static: Bytes) from_hex(s: Str) -> Bytes?
fn (static: Bytes) from_base64(s: Str) -> Bytes?
fn (b: Bytes) rune_count() -> Int
fn (b: Bytes) decode_rune(i: Int) -> (Rune, Int)
fn (b: Bytes) quote() -> Str
fn (b: Bytes) find(sub: Bytes) -> Int?
fn (b: Bytes) rfind(sub: Bytes) -> Int?
fn (b: Bytes) contains(sub: Bytes) -> Bool
fn (b: Bytes) starts_with(p: Bytes) -> Bool
fn (b: Bytes) ends_with(p: Bytes) -> Bool
fn (b: Bytes) count(sub: Bytes) -> Int
fn (b: Bytes) find_any(chars: Str) -> Int?
fn (b: Bytes) rfind_any(chars: Str) -> Int?
fn (b: Bytes) contains_any(chars: Str) -> Bool
fn (b: Bytes) contains_rune(r: Rune) -> Bool
fn (b: Bytes) find_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx
fn (b: Bytes) rfind_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Int? ! E uses fx
fn (b: Bytes) contains_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (b: Bytes) split(sep: Bytes) -> List[Bytes]
fn (b: Bytes) split_n(sep: Bytes, n: Int) -> List[Bytes]
fn (b: Bytes) split_after(sep: Bytes) -> List[Bytes]
fn (b: Bytes) split_after_n(sep: Bytes, n: Int) -> List[Bytes]
fn (b: Bytes) cut(sep: Bytes) -> (Bytes, Bytes)?
fn (b: Bytes) cut_prefix(prefix: Bytes) -> Bytes?
fn (b: Bytes) cut_suffix(suffix: Bytes) -> Bytes?
fn (b: Bytes) fields() -> List[Bytes]
fn (b: Bytes) split_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> List[Bytes] ! E uses fx
fn (b: Bytes) trim() -> Bytes
fn (b: Bytes) trim_prefix(p: Bytes) -> Bytes
fn (b: Bytes) trim_suffix(p: Bytes) -> Bytes
fn (b: Bytes) trim_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bytes ! E uses fx
fn (b: Bytes) trim_start_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bytes ! E uses fx
fn (b: Bytes) trim_end_where[E, fx: Effects](f: fn(Rune) -> Bool ! E uses fx) -> Bytes ! E uses fx
fn (b: Bytes) replace(old: Bytes, new: Bytes) -> Bytes
fn (b: Bytes) replace_n(old: Bytes, new: Bytes, n: Int) -> Bytes
fn (b: Bytes) to_lower() -> Bytes
fn (b: Bytes) to_upper() -> Bytes
fn (b: Bytes) map_runes[E, fx: Effects](f: fn(Rune) -> Rune? ! E uses fx) -> Bytes ! E uses fx
fn (b: Bytes) equal_fold(other: Bytes) -> Bool
fn (b: Bytes) repeat(n: Int) -> Bytes
fn (b: Bytes) indexed() -> List[(Int, UInt8)]
fn (static: Bytes) zeroed(n: Int) -> Bytes
```

### List

```
fn (static: List[T]) with_capacity(n: Int) -> List[T]
fn (xs: List[T]) len() -> Int
fn (xs: List[T]) is_empty() -> Bool
fn (xs: List[T]) get(i: Int) -> T?
fn (xs: List[T]) first() -> T?
fn (xs: List[T]) last() -> T?
fn (xs: List[T]) contains(x: T) -> Bool
fn (xs: List[T]) index_of(x: T) -> Int?
fn (xs: List[T]) indexed() -> List[(Int, T)]
fn (xs: List[T]) map[U, E, fx: Effects](f: fn(T) -> U ! E uses fx) -> List[U] ! E uses fx
fn (xs: List[T]) filter[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> List[T] ! E uses fx
fn (xs: List[T]) filter_map[U, E, fx: Effects](f: fn(T) -> U? ! E uses fx) -> List[U] ! E uses fx
fn (xs: List[T]) fold[A, E, fx: Effects](init: A, f: fn(A, T) -> A ! E uses fx) -> A ! E uses fx
fn (xs: List[T]) find[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> T? ! E uses fx
fn (xs: List[T]) any[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (xs: List[T]) all[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (xs: List[T]) sort() -> List[T]
fn (xs: List[T]) sort_by[K: Ord, E, fx: Effects](key: fn(T) -> K ! E uses fx) -> List[T] ! E uses fx
fn (var xs: List[T]) sort_in_place()
fn (var xs: List[T]) sort_in_place_by[K: Ord, E, fx: Effects](key: fn(T) -> K ! E uses fx) -> () ! E uses fx
fn (xs: List[T]) group_by[K: Hash, E, fx: Effects](key: fn(T) -> K ! E uses fx) -> List[(K, List[T])] ! E uses fx
fn (xs: List[T]) reverse() -> List[T]
fn (xs: List[T]) to_set() -> Set[T]
fn (xs: List[Str]) join(sep: Str) -> Str
fn (var xs: List[T]) push(x: T)
fn (var xs: List[T]) pop() -> T?
fn (var xs: List[T]) insert(i: Int, x: T)
fn (var xs: List[T]) remove_at(i: Int) -> T
fn (var xs: List[T]) set(i: Int, x: T)
fn (var xs: List[T]) heap_push(x: T)
fn (var xs: List[T]) heap_pop() -> T?
fn (var xs: List[T]) heap_remove(i: Int) -> T
fn (var xs: List[T]) heap_fix(i: Int)
fn (var xs: List[T]) heapify()
```

### Map

```
fn (m: Map[K, V]) len() -> Int
fn (m: Map[K, V]) is_empty() -> Bool
fn (m: Map[K, V]) get(k: K) -> V?
fn (m: Map[K, V]) get_or_fault(k: K, reason: Str) -> V
fn (m: Map[K, V]) contains(k: K) -> Bool
fn (m: Map[K, V]) keys() -> List[K]
fn (m: Map[K, V]) values() -> List[V]
fn (m: Map[K, V]) entries() -> List[(K, V)]
fn (m: Map[K, V]) filter[E, fx: Effects](f: fn(K, V) -> Bool ! E uses fx) -> Map[K, V] ! E uses fx
fn (m: Map[K, V]) map[U, E, fx: Effects](f: fn(K, V) -> U ! E uses fx) -> Map[K, U] ! E uses fx
fn (m: Map[K, V]) any[E, fx: Effects](f: fn(K, V) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (m: Map[K, V]) all[E, fx: Effects](f: fn(K, V) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (var m: Map[K, V]) put(k: K, v: V)
fn (var m: Map[K, V]) remove(k: K) -> V?
fn (var m: Map[K, V]) update[R, E, fx: Effects](k: K, init: V, f: fn(var V) -> R ! E uses fx) -> R ! E uses fx
```

### Set

```
fn (s: Set[T]) len() -> Int
fn (s: Set[T]) is_empty() -> Bool
fn (s: Set[T]) contains(x: T) -> Bool
fn (s: Set[T]) union(o: Set[T]) -> Set[T]
fn (s: Set[T]) intersect(o: Set[T]) -> Set[T]
fn (s: Set[T]) difference(o: Set[T]) -> Set[T]
fn (s: Set[T]) is_subset(o: Set[T]) -> Bool
fn (s: Set[T]) intersects(o: Set[T]) -> Bool
fn (s: Set[T]) to_list() -> List[T]
fn (s: Set[T]) any[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (s: Set[T]) all[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> Bool ! E uses fx
fn (s: Set[T]) filter[E, fx: Effects](f: fn(T) -> Bool ! E uses fx) -> Set[T] ! E uses fx
fn (s: Set[T]) map[U, E, fx: Effects](f: fn(T) -> U ! E uses fx) -> Set[U] ! E uses fx
fn (var s: Set[T]) add(x: T)
fn (var s: Set[T]) remove(x: T) -> Bool
```

### Option

```
fn (o: Option[T]) is_some() -> Bool
fn (o: Option[T]) is_none() -> Bool
fn (o: Option[T]) or_err[E](e: E) -> Result[T, E]
fn (o: Option[T]) map[U, fx: Effects](f: fn(T) -> U uses fx) -> U? uses fx
fn (o: Option[T]) and_then[U, fx: Effects](f: fn(T) -> U? uses fx) -> U? uses fx
fn (o: Option[T]) or_fault(msg: Str) -> T
```

### Result

```
fn (r: Result[T, E]) is_ok() -> Bool
fn (r: Result[T, E]) is_err() -> Bool
fn (r: Result[T, E]) map[U, fx: Effects](f: fn(T) -> U uses fx) -> Result[U, E] uses fx
fn (r: Result[T, E]) map_err[F, fx: Effects](f: fn(E) -> F uses fx) -> Result[T, F] uses fx
fn (r: Result[T, E]) ok() -> T?
fn (r: Result[T, E]) err() -> E?
fn (r: Result[T, E]) or_fault(msg: Str) -> T
fn (r: Result[T, E]) ignore_err(reason: Str) -> T?
```

### Duration

```
fn (static: Duration) from_secs(s: Float) -> Duration
fn (static: Duration) from_secs_int(s: Int) -> Duration
fn (static: Duration) from_millis(ms: Int) -> Duration
fn (static: Duration) from_nanos(ns: Int) -> Duration
fn (static: Duration) parse(s: Str) -> Duration ! DurationErr
fn (d: Duration) seconds() -> Float
fn (d: Duration) millis() -> Int
fn (d: Duration) nanos() -> Int
fn (d: Duration) scale(f: Float) -> Duration
fn (d: Duration) ceil_seconds() -> Int
fn (d: Duration) abs() -> Duration
fn (d: Duration) round(m: Duration) -> Duration
fn (d: Duration) truncate(m: Duration) -> Duration
```

### DurationErr

```
fn (e: DurationErr) display() -> Str
```

### Time

```
fn (static: Time) UNIX_EPOCH() -> Time
fn (static: Time) from_unix(secs: Int) -> Time
fn (static: Time) from_unix_millis(ms: Int) -> Time
fn (static: Time) parse_rfc3339(s: Str) -> Time?
fn (static: Time) rfc3339_offset(s: Str) -> Duration?
fn (t: Time) unix() -> Int
fn (t: Time) unix_millis() -> Int
fn (t: Time) format_rfc3339() -> Str
```

### Chan

```
fn (static: Chan[T]) new(cap: Int = 0) -> Chan[T]
fn (c: Chan[T]) send(x: T)
fn (c: Chan[T]) recv() -> T?
fn (c: Chan[T]) try_send(x: T) -> Bool
fn (c: Chan[T]) try_recv() -> T?
fn (c: Chan[T]) close()
fn (c: Chan[T]) len() -> Int
```

### Mutex

```
fn (static: Mutex[T]) new(v: T) -> Mutex[T]
fn (m: Mutex[T]) with[R, E, fx: Effects](f: fn(var T) -> R ! E uses fx) -> R ! E uses fx
fn (m: Mutex[T]) with_read[R, E, fx: Effects](f: fn(T) -> R ! E uses fx) -> R ! E uses fx
fn (m: Mutex[T]) reset(v: T)
```

### Atomic

```
fn (static: Atomic[T]) new(v: T) -> Atomic[T]
fn (a: Atomic[T]) get() -> T
fn (a: Atomic[T]) set(v: T)
fn (a: Atomic[Int]) add(n: Int) -> Int
fn (a: Atomic[Int]) max(n: Int) -> Int
fn (a: Atomic[T]) swap(v: T) -> T
fn (a: Atomic[T]) compare_and_swap(old: T, new: T) -> Bool
```

### Scope

```
fn (s: Scope) spawn[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> Task[T, E] uses fx
fn (s: Scope) spawn_background[T, E, fx: Effects](f: fn() -> T ! E uses fx) -> Task[T, E] uses fx
fn (s: Scope) cancel()
fn (s: Scope) group() -> task.Group
```

### Task

```
fn (t: Task[T, E]) await() -> T ! E
```

### Buf

```
fn (static: Buf[T]) new() -> Buf[T]
fn (static: Buf[T]) with_capacity(n: Int) -> Buf[T]
fn (var b: Buf[T]) push(x: T)
fn (b: Buf[T]) len() -> Int
fn (b: Buf[T]) freeze() -> List[T]
```

### StrBuilder

```
fn (static: StrBuilder) new() -> StrBuilder
fn (var b: StrBuilder) write(s: Str)
fn (b: StrBuilder) len() -> Int
fn (b: StrBuilder) freeze() -> Str
```

### BytesBuilder

```
fn (static: BytesBuilder) new() -> BytesBuilder
fn (static: BytesBuilder) with_capacity(n: Int) -> BytesBuilder
fn (var b: BytesBuilder) write(data: Bytes)
fn (var b: BytesBuilder) write_byte(x: UInt8)
fn (var b: BytesBuilder) write_str(s: Str)
fn (var b: BytesBuilder) write_rune(r: Rune)
fn (var b: BytesBuilder) put_le(at: Int, v: Int, n: Int)
fn (var b: BytesBuilder) put_be(at: Int, v: Int, n: Int)
fn (var b: BytesBuilder) write_le(v: Int, n: Int)
fn (var b: BytesBuilder) write_be(v: Int, n: Int)
fn (b: BytesBuilder) len() -> Int
fn (b: BytesBuilder) freeze() -> Bytes
```

### ByteBuffer

```
fn (static: ByteBuffer) new() -> ByteBuffer
fn (static: ByteBuffer) with_capacity(n: Int) -> ByteBuffer
fn (var b: ByteBuffer) write(data: Bytes)
fn (b: ByteBuffer) len() -> Int
fn (b: ByteBuffer) peek(n: Int) -> Bytes
fn (var b: ByteBuffer) next(n: Int) -> Bytes
fn (var b: ByteBuffer) read_until(delim: UInt8) -> Bytes?
fn (var b: ByteBuffer) discard(n: Int)
fn (b: ByteBuffer) find(sub: Bytes) -> Int?
```

### Secret

```
fn (static: Secret[T]) new(v: T) -> Secret[T]
fn (s: Secret[T]) reveal() -> T
```

### FsErr

```
fn (e: FsErr) display() -> Str
```

### Fs

```
fn sub(self, dir: Str) -> Fs
fn cwd(self) -> Fs
fn cwd_path(self) -> Str ! FsErr
fn read_only(self) -> Fs
fn write_only(self) -> Fs
fn case_sensitive(self) -> Bool
fn stat(self, path: Str) -> FileInfo? ! FsErr uses fs.read
fn exists(self, path: Str) -> Bool ! FsErr uses fs.read
fn disk_usage(self, path: Str) -> DiskUsage ! FsErr uses fs.read
fn read(self, path: Str) -> Bytes ! FsErr uses fs.read
fn read_text(self, path: Str) -> Str ! FsErr uses fs.read
fn read_dir(self, dir: Str) -> List[FileInfo] ! FsErr uses fs.read
fn walk(self, dir: Str) -> List[FileInfo] ! FsErr uses fs.read
fn write(self, path: Str, data: Bytes) -> () ! FsErr uses fs.write
fn create_temp(self, dir: Str, prefix: Str) -> Str ! FsErr uses fs.write
fn rename(self, from: Str, to: Str) -> () ! FsErr uses fs.write
fn remove(self, path: Str) -> () ! FsErr uses fs.write
fn remove_all(self, path: Str) -> () ! FsErr uses fs.write
fn create_dir_all(self, dir: Str) -> () ! FsErr uses fs.write
fn set_modified(self, path: Str, t: Time) -> () ! FsErr uses fs.write
fn open(self, path: Str) -> File ! FsErr uses fs.read
fn create(self, path: Str) -> File ! FsErr uses fs.write
fn open_append(self, path: Str) -> File ! FsErr uses fs.write
fn open_rw(self, path: Str) -> File ! FsErr uses fs.write
fn create_new(self, path: Str) -> File ! FsErr uses fs.write
fn append(self, path: Str, data: Bytes) -> () ! FsErr uses fs.write
fn copy(self, from: Str, to: Str) -> () ! FsErr uses fs
fn mode(self, path: Str) -> Int ! FsErr uses fs.read
fn set_mode(self, path: Str, mode: Int) -> () ! FsErr uses fs.write
fn symlink(self, target: Str, link: Str) -> () ! FsErr uses fs.write
fn read_link(self, path: Str) -> Str? ! FsErr uses fs.read
fn glob(self, pattern: Str) -> List[Str] ! FsErr uses fs.read
fn lock(self, path: Str) -> FileLock ! FsErr uses fs.write
fn try_lock(self, path: Str) -> FileLock? ! FsErr uses fs.write
```

### DirEntry

```
fn (e: DirEntry) info(fs: Fs) -> FileInfo ! FsErr uses fs.read
```

### File

```
fn (f: File) read(max: Int) -> Bytes? ! FsErr uses fs.read
fn (f: File) read_at(offset: Int, max: Int) -> Bytes? ! FsErr uses fs.read
fn (f: File) read_line() -> Str? ! FsErr uses fs.read
fn (f: File) write(data: Bytes) -> () ! FsErr uses fs.write
fn (f: File) write_at(offset: Int, data: Bytes) -> () ! FsErr uses fs.write
fn (f: File) truncate(size: Int) -> () ! FsErr uses fs.write
fn (f: File) stat() -> FileInfo ! FsErr uses fs.read
fn (f: File) sync() -> () ! FsErr uses fs.write
fn (f: File) close() -> () ! FsErr
```

### FileLock

```
fn (l: FileLock) unlock()
```

### NetErr

```
fn (e: NetErr) display() -> Str
```

### Listener

```
fn (l: Listener) addr() -> Str
fn (l: Listener) accept() -> ServerConn ! NetErr uses net.listen
fn (l: Listener) close()
```

### Conn

```
fn (c: Conn) read(max: Int) -> Bytes? ! NetErr uses net.dial
fn (c: Conn) write(data: Bytes) -> () ! NetErr uses net.dial
fn (c: Conn) close()
fn (c: Conn) set_deadline(clock: Clock, t: Time?) uses clock
fn (c: Conn) set_read_deadline(clock: Clock, t: Time?) uses clock
fn (c: Conn) set_write_deadline(clock: Clock, t: Time?) uses clock
```

### ServerConn

```
fn (c: ServerConn) read(max: Int) -> Bytes? ! NetErr uses net.listen
fn (c: ServerConn) write(data: Bytes) -> () ! NetErr uses net.listen
fn (c: ServerConn) close()
fn (c: ServerConn) set_deadline(clock: Clock, t: Time?) uses clock
fn (c: ServerConn) set_read_deadline(clock: Clock, t: Time?) uses clock
fn (c: ServerConn) set_write_deadline(clock: Clock, t: Time?) uses clock
```

### Stream

```
fn read(self, max: Int) -> Bytes? ! NetErr uses net
fn write(self, data: Bytes) -> () ! NetErr uses net
fn close(self)
```

### NetConn

```
fn read(self, max: Int) -> Bytes? ! NetErr uses net
fn write(self, data: Bytes) -> () ! NetErr uses net
fn close(self)
fn set_deadline(self, clock: Clock, t: Time?) uses clock
fn set_read_deadline(self, clock: Clock, t: Time?) uses clock
fn set_write_deadline(self, clock: Clock, t: Time?) uses clock
```

### Net

```
fn listen(self, addr: Str) -> Listener ! NetErr uses net.listen
fn dial(self, addr: Str) -> Conn ! NetErr uses net.dial
fn dial_timeout(self, addr: Str, d: Duration) -> Conn ! NetErr uses net.dial
fn lookup_host(self, host: Str) -> List[Str] ! NetErr uses net.dial
fn interfaces(self) -> List[NetInterface] ! NetErr uses net.listen
fn only(self, hosts: List[Str]) -> Net
```

### Clock

```
fn now(self) -> Time uses clock
fn sleep(self, d: Duration) uses clock
fn sleep_or_cancel(self, d: Duration) -> () ! ClockErr uses clock
fn since(self, t: Time) -> Duration uses clock
```

### Rand

```
fn int(self, lo: Int, hi: Int) -> Int uses rand
fn float(self) -> Float uses rand
fn bool(self) -> Bool uses rand
fn bytes(self, n: Int) -> Bytes uses rand
fn shuffle[T](self, xs: List[T]) -> List[T] uses rand
```

### ClockErr

```
fn (e: ClockErr) display() -> Str
```

### EnvErr

```
fn (e: EnvErr) display() -> Str
```

### Env

```
fn get(self, name: Str) -> Str? uses env
fn vars(self) -> Map[Str, Str] uses env
fn args(self) -> List[Str] uses env
fn program(self) -> Str uses env
fn home(self) -> Str ! EnvErr uses env
fn only(self, names: List[Str]) -> Env
fn config_dir(self) -> Str ! EnvErr uses env
fn cache_dir(self) -> Str ! EnvErr uses env
fn hostname(self) -> Str uses env
```

### IoErr

```
fn (e: IoErr) display() -> Str
```

### Stdio

```
fn print(self, text: Str) uses stdio
fn println(self, text: Str) uses stdio
fn eprint(self, text: Str) uses stdio
fn eprintln(self, text: Str) uses stdio
fn write(self, data: Bytes) -> () ! IoErr uses stdio
fn read_line(self) -> Str? ! IoErr uses stdio
fn read_all(self) -> Str ! IoErr uses stdio
fn lines(self) -> List[Str] ! IoErr uses stdio
fn stdin_is_tty(self) -> Bool uses stdio
fn stdout_is_tty(self) -> Bool uses stdio
fn stderr_is_tty(self) -> Bool uses stdio
fn width(self) -> Int? uses stdio
fn read_secret(self, prompt: Str) -> Str? ! IoErr uses stdio
fn read_bytes(self, max: Int) -> Bytes? ! IoErr uses stdio
fn read_all_bytes(self) -> Bytes ! IoErr uses stdio
```

### ProcErr

```
fn (e: ProcErr) display() -> Str
```

### Proc

```
fn exit(self, code: Int) -> Never uses proc
fn set_grace(self, d: Duration) uses proc
fn pid(self) -> Int uses proc
fn ppid(self) -> Int uses proc
fn usage(self) -> ProcUsage uses proc
fn children_usage(self) -> ProcUsage uses proc
fn run(self, cmd: Str, args: List[Str]) -> ProcOutput ! ProcErr uses proc
fn only(self, cmds: List[Str]) -> Proc
fn cmd(self, name: Str, args: List[Str] = []) -> Cmd
fn pipeline(self, cmds: List[Cmd]) -> ProcOutput ! ProcErr uses proc
fn kill(self, pid: Int, signal: Str = "TERM") -> () ! ProcErr uses proc
fn notify(self, signals: List[Signal]) -> Chan[Signal] uses proc
fn stop_notify(self, c: Chan[Signal]) uses proc
fn request_stop(self) uses proc
```

### Cmd

```
fn (c: Cmd) dir(path: Str) -> Cmd
fn (c: Cmd) env(e: Env) -> Cmd uses env
fn (c: Cmd) set_env(name: Str, value: Str) -> Cmd
fn (c: Cmd) stdin(data: Bytes) -> Cmd
fn (c: Cmd) inherit() -> Cmd
fn (c: Cmd) merge_stderr() -> Cmd
fn (c: Cmd) output() -> ProcOutput ! ProcErr uses proc
fn (c: Cmd) run() -> Int ! ProcErr uses proc
fn (c: Cmd) spawn() -> Child ! ProcErr uses proc
```

### Child

```
fn (c: Child) pid() -> Int
fn (c: Child) write(data: Bytes) -> () ! ProcErr uses proc
fn (c: Child) close_stdin()
fn (c: Child) read(max: Int) -> Bytes? ! ProcErr uses proc
fn (c: Child) read_err(max: Int) -> Bytes? ! ProcErr uses proc
fn (c: Child) read_line() -> Str? ! ProcErr uses proc
fn (c: Child) wait() -> Int ! ProcErr uses proc
fn (c: Child) wait_usage() -> (Int, ProcUsage) ! ProcErr uses proc
fn (c: Child) kill()
```

### Functions

```
fn fault(msg: Str) -> Never
fn dbg[T](x: T) -> T
fn create_dir_all_mode(fs: Fs, dir: Str, mode: Int) -> () ! FsErr uses fs
fn walk_dir[fx: Effects](fs: Fs, dir: Str, visit: fn(FileInfo) -> WalkStep uses fx) -> List[FileInfo] ! FsErr uses (fs.read, fx)
fn read_dir_entries(fs: Fs, dir: Str) -> List[DirEntry] ! FsErr uses fs.read
fn walk_entries(fs: Fs, dir: Str) -> List[DirEntry] ! FsErr uses fs.read
fn walk_dir_entries[fx: Effects](fs: Fs, dir: Str, visit: fn(DirEntry) -> WalkStep uses fx) -> List[DirEntry] ! FsErr uses (fs.read, fx)
fn read_text_or(fs: Fs, path: Str, default: Str) -> Str ! FsErr uses fs.read
fn read_or(fs: Fs, path: Str, default: Bytes) -> Bytes ! FsErr uses fs.read
fn read_mapped(fs: Fs, path: Str) -> Bytes ! FsErr uses fs.read
fn write_atomic(fs: Fs, path: Str, data: Bytes) -> () ! FsErr uses fs
```

<!-- END prelude index -->

## 35. std/websocket

WebSockets (RFC 6455, decision 0313) with gorilla/websocket's behavior
(v1.5), both sides, on every backend.

```
struct Message { kind: Kind, data: Bytes }   // a text message's data is valid UTF-8
enum Kind { Text, Binary }
enum WsErr {
  Closed(code: Int, reason: Str)    // the peer's close frame (1005: it had no code), answered
  Protocol(msg: Str)                // a frame broke the protocol: close 1002 sent (1007 for text not UTF-8); or `close sent`
  TooBig(limit: Int)                // a message over max_message: close 1009 sent
  Net(err: NetErr)
  Handshake(status: Int, msg: Str)  // the server refused the upgrade (status 0: no answer, or not a ws:// URL)
}
fn (e: WsErr) display() -> Str      // gorilla's texts: `websocket: close 1001 (going away): bye`
struct WsConn {}
fn (c: WsConn) read() -> Message ! WsErr       // a whole message; pings answered (or on_ping), pongs to on_pong
fn (c: WsConn) write(m: Message) -> () ! WsErr // one frame
fn (c: WsConn) write_text(s: Str) -> () ! WsErr
fn (c: WsConn) ping(data: Bytes = b"") -> () ! WsErr
fn (c: WsConn) close(code: Int = 1000, reason: Str = "") -> () ! WsErr   // the close frame, then the connection
fn (c: WsConn) subprotocol() -> Str
fn (c: WsConn) set_deadline(clock: Clock, t: Time?) uses clock         // decision 0430: gorilla's SetReadDeadline and SetWriteDeadline
fn (c: WsConn) set_read_deadline(clock: Clock, t: Time?) uses clock
fn (c: WsConn) set_write_deadline(clock: Clock, t: Time?) uses clock
fn (c: WsConn) on_ping[fx: Effects](f: fn(Bytes) -> () uses fx) uses fx   // gorilla's SetPingHandler: replaces the automatic pong
fn (c: WsConn) on_pong[fx: Effects](f: fn(Bytes) -> () uses fx) uses fx   // gorilla's SetPongHandler
fn (c: WsConn) pong(data: Bytes = b"") -> () ! WsErr
pub fn upgrade[fx: Effects](req: http.Request, f: fn(WsConn) -> () uses fx, protocols: List[Str] = [], origins: List[Str] = [], max_message: Int = 1048576) -> http.Response uses fx
pub fn dial(net: Net, url: Str, headers: Map[Str, Str] = {}, protocols: List[Str] = [], max_message: Int = 1048576, roots: Bytes = b"") -> WsConn ! WsErr uses net.dial
```

- `upgrade` is called in a handler and its response returned. It checks
  the request as gorilla's Upgrader: `Connection: upgrade` and `Upgrade:
  websocket` tokens (else 400), GET (405), `Sec-WebSocket-Version: 13`
  (400), the `Origin` (403), and a key of 16 bytes in base64 (400); a
  refusal is the status text with `Sec-Websocket-Version: 13`. The
  `Origin` passes when absent, when its host name is the request's
  (`req.url.host`; ports are not compared, a Request keeps none), or when
  `origins` lists its host (with or without the port) or `"*"`. The
  subprotocol is the first of `protocols` that the client offers.
- An accepted upgrade is a 101 response that hands the connection over
  (std.md 4.3): the server writes gorilla's 101 (with
  `Sec-WebSocket-Accept` and the subprotocol), runs `f` with the
  connection, and closes it when `f` returns. HTTP/1.1 only. Until `f`
  returns the connection is a request in flight: a stop waits for it
  (std 4.4, decision 0429) and does not close it as idle, so `f` watches
  `task.stopping()` and closes with 1001 (going away). A handler that
  waits in `read()` does that from a background task; its read then ends
  when the peer answers the close:

  ```
  websocket.upgrade(req, fn(c) {
    scope s {
      s.spawn_background(fn() {
        let _ = task.stopping().recv()
        c.close(code: 1001, reason: "server stopping").ignore_err("stopping")
      })
      while true {
        match c.read() {
          Ok(m) => handle(m)
          Err(_) => break
        }
      }
    }
  })
  ```
  `Router.call` returns the 101 without running `f`.
- Frames as gorilla reads them: a client's frames must be masked and a
  server's not; reserved bits, unknown opcodes, a control frame over 125
  bytes or fragmented, and continuations out of order are `Protocol` with
  gorilla's texts joined (`RSV1 set, bad MASK`) after a close 1002; a
  close frame's code must be valid and its reason UTF-8. A text message
  that is not UTF-8 is close 1007 (gorilla does not check). A read error
  sticks. The end of the stream is `Closed(1006, "unexpected EOF")`.
- A write is one final frame (masked with a fresh key from a client)
  under a lock, so one task may read while another writes; writes after
  this side's close fail with `Protocol("close sent")`. `close` sends the
  close frame unless one was sent (also when answering the peer's), then
  closes the connection; the peer's answering close is not awaited.
- `dial` takes `ws://host[:port]/path?query` or `wss://` (TLS through
  std/tls's client, no ALPN, the host name verified against `roots`, a
  PEM bundle, when given, else the system's; a TLS or roots failure is
  `Net`), sends gorilla's handshake with a random key,
  the protocols, and `headers`, and fails with `Handshake(status,
  "websocket: bad handshake")` unless the answer is a 101 with the
  upgrade tokens and the right `Sec-WebSocket-Accept`.
- Deadlines (decision 0430) are gorilla's. A read or write past its
  deadline fails with `Net(Timeout)`, and so does one whose deadline has
  already passed, at once. The error sticks for that direction: after a
  timeout a frame may be read or sent in part, so later reads (or
  writes) fail the same way, as gorilla's. `None` clears a deadline. A
  client's deadlines are its connection's (`Conn` or `TlsConn`); a
  server's apply from the next wait of the handed over connection, so
  one set while a read or write is waiting does not move that wait (Go
  moves it).
- `on_ping` and `on_pong` run `f` with each ping's or pong's data inside
  `read`, which holds its lock: `f` must not call `read`. `on_ping`
  replaces the automatic pong (send one with `pong`). A later call
  replaces `f`. Both are dropped when a read fails for good or the
  connection closes, so a hook that captures its `WsConn` does not keep
  it alive. A keepalive moves the read deadline in `on_pong` while
  another task pings:

  ```xo
  c.set_read_deadline(clock, Some(clock.now() + 60s))
  c.on_pong(fn(_data) { c.set_read_deadline(clock, Some(clock.now() + 60s)) })
  scope s {
    s.spawn_background(fn() {
      while c.ping() is Ok(_) {
        clock.sleep(30s)
      }
    })
    while let Ok(m) = c.read() {
      // handle m
    }
  }
  ```
- Not provided: permessage-deflate, a message written in pieces, ping
  timers (use `ping` and a clock, as above), WebSockets over HTTP/2.


## 36. std/debug

Live inspection of a running server (decision 0325). Nothing is mounted
by default: a program mounts the router under a prefix of its own and
gives it an `auth` function.

```xo
fn router(clock: Clock, auth: fn(http.Request) -> Bool) -> http.Router uses clock
fn heap_start(rate: Int = 0) -> Bool
fn mem_budget(bytes: Int)
fn mem_label(text: Str)
fn mem_mapped() -> Int
fn mem_peak() -> Int
fn mem_total() -> Int
fn mem_trim()
```

- `GET /profile?seconds=N&hz=H`: a CPU profile of the next N seconds
  (default 30, 1 to 300) at H samples per CPU second (default 100, 1 to
  1000). The native backends answer decision 0240's container
  (`Content-Type: application/x-xo-profile`; `xo prof FILE` converts a
  saved answer to pprof files and a text summary, and `xo prof URL`
  fetches and converts it); the Go backend answers
  runtime/pprof's profile (`X-Profile-Format: pprof`) and ignores `hz`.
  One profile runs at a time: a request during one is 409. 501 where
  profiles cannot be taken (windows, wasm, a native build without
  `--profile-support`).
- `GET /heap`: the sampled heap profile in the same container; 409 until
  the sampler runs on the native backends (`XO_PROF=heap` at start, or
  `heap_start(rate)`; rate 0 is 512 KiB, Go's MemProfileRate; once
  started it runs for the life of the process). The Go backend samples
  always (`heap_start` returns false).
- `GET /sched`: the scheduler's counters as `name value` lines (native:
  the `XO_RTSTATS` counts, zero until they are on; `?on=1` turns them on;
  and `heap_live`, the heap blocks allocated and not yet freed, when the
  heap counts from the start: `XO_LEAKCHECK`, `XO_HEAPSTATS` or
  `XO_RTSTATS` set, decision 0520. Go: goroutines, gomaxprocs, threads, heap_inuse_bytes, gc_count).
- A request for which `auth` answers false gets 403; the unmounted
  router costs nothing.
- A live profile running when the program exits is dropped, with no file
  (the `XO_PROF` file profile is separate: it still writes at exit).
- `XO_PROF_HEAP_AT=TEXT` (native backends, with `XO_PROF=heap`): the
  first `mem_label` call whose text ends with TEXT writes the heap
  samples in use at that moment to `XO_PROF_HEAP_OUT`
  (`xo-heap-at.xoraw` by default, `%p` the pid; never overwritten),
  which `xo prof` converts like any heap profile (decision 0352).
- The memory budget (decision 0349, native backends): `mem_budget(bytes)`
  stops the program with status 70 when the bytes its heap has mapped
  from the system pass `bytes` (0: none), writing `fatal error: memory
  budget exceeded: N MB mapped, more than XO_MAX_MEM (M MB)` and
  `  while: LABEL`, the last `mem_label(text)` (at most 255 bytes).
  `XO_MAX_MEM` (bytes, or with K, M, G; `off`) sets the budget at start.
  `mem_mapped()` is the count the budget compares (chunks and records as
  mapped, large blocks as handed out, not the room a growing block
  reserves), `mem_peak()` its high-water mark, `mem_total()` the
  machine's physical memory (macOS, Linux and Windows; 0 on WASI and the Go backend). xo, when
  XO_MAX_MEM is not set, sets three quarters of physical memory, at least
  16 GiB (decision 0349's default; none where physical memory is not
  read). The Go backend ignores the
  budget and its label, and both counts are 0 there.
- `mem_trim()` (decisions 0448 and 0505, native backends): gives the
  system the pages of the free heap blocks of 16 KiB and over of every
  worker thread (each thread's free lists and spare magazines; a task's
  frees land on whichever thread ran it) and of the central lists'
  magazines, each once until the block is
  used and freed again, and on every target unmaps the cached large
  blocks of 1 MiB and over; for a program that knows its live memory just
  fell, as xo does after its phases that free the most. The calling thread
  trims its own blocks, wakes the idle workers to trim theirs, and waits up
  to about 20 ms for them; a worker busy in a task trims at its next
  switch, which may come after `mem_trim` returns. The pages come back on
  the next use (a page fault each), and the next such large block maps
  anew. Nothing on the Go backend, whose runtime's scavenger does this
  itself. Without a call, a program with worker threads gives back the
  same blocks once they stay free long enough (decision 0533, Linux and
  darwin): about every 30 s an idle worker marks the free blocks of 16
  KiB and over it finds, and a block still free and marked at the next
  pass, 30 to 60 s unused, goes back; so a server's memory falls after a
  burst ends, and blocks a busy program reuses keep their pages.

## 37. std/metrics

Counters, gauges and histograms with label values, and their Prometheus
text exposition (format 0.0.4), with no dependency (decision 0325). A
registry is a value that `main` makes and passes on; there is no
program wide default.

```xo
fn registry() -> Registry
fn latency_buckets() -> List[Float]      // 0.005 to 10 seconds
fn (r: Registry) counter(name: Str, help: Str, labels: List[Str] = []) -> Counter
fn (r: Registry) gauge(name: Str, help: Str, labels: List[Str] = []) -> Gauge
fn (r: Registry) histogram(name: Str, help: Str, labels: List[Str] = [], buckets: List[Float] = latency_buckets()) -> Histogram
fn (c: Counter) inc(values: List[Str] = [])
fn (c: Counter) add(n: Float, values: List[Str] = [])
fn (g: Gauge) set(v: Float, values: List[Str] = [])
fn (g: Gauge) add(v: Float, values: List[Str] = [])
fn (h: Histogram) observe(v: Float, values: List[Str] = [])
fn (r: Registry) summary(name: Str, help: Str, labels: List[Str] = [], objectives: List[(Float, Float)] = [], max_age: Duration = 10m, age_buckets: Int = 5) -> Summary
fn (s: Summary) observe(v: Float, values: List[Str] = [])
fn (r: Registry) gauge_func(name: Str, help: Str, f: fn() -> Float)
fn (r: Registry) counter_func(name: Str, help: Str, f: fn() -> Float)
fn (r: Registry) collector(name: Str, help: Str, kind: Kind, labels: List[Str], f: fn() -> List[Sample])
struct Sample { values: List[Str], value: Float }
enum Kind { Counter, Gauge }
fn (c: Counter) remove(values: List[Str]) -> Bool   // also Gauge, Histogram, Summary
fn (c: Counter) reset()                             // also Gauge, Histogram, Summary
fn (c: Counter) child(values: List[Str] = []) -> CounterChild      // also Gauge, Histogram, Summary
fn (c: CounterChild) inc()
fn (c: CounterChild) add(n: Float)
fn (g: GaugeChild) set(v: Float)
fn (g: GaugeChild) add(v: Float)
fn (h: HistogramChild) observe(v: Float)                           // also SummaryChild
fn (r: Registry) text(now: Time? = None) -> Str
fn (r: Registry) handler() -> fn(http.Request) -> http.Response
fn (r: Registry) handler_at(clock: Clock) -> fn(http.Request) -> http.Response uses clock
fn (r: Registry) middleware(clock: Clock, label: fn(http.Request) -> Str) -> fn(http.Request, http.Next) -> http.Response uses clock
```

- A name is a Prometheus name (letters, digits, `_` and `:`, not
  starting with a digit); a label name has no `:` and is not `le` or
  `__`-prefixed. A bad name, a second instrument of the same name, a
  value list of the wrong length, a counter decreasing, or buckets that
  do not increase is a fault: programmer errors found at start.
- A series is the label values of one call (one per label name, in
  order). Calls are safe from any task: a series is found under a read
  lock and updated with atomics; the write lock is taken only to make a
  series the first time.
- A child (decision 0501, client_golang's WithLabelValues) is a held
  series: `child(values)` finds or makes it (a wrong count is the same fault
  as `inc`), and `inc`, `add`, `set` and `observe` on it take no label list,
  key, lock or lookup. It shares the series' cells, is safe from any task,
  and past the series limit counts into a series nothing exposes (the drop
  is counted). `remove` and `reset` detach a series: a child made before
  keeps counting into the detached series, so take a new child after a
  remove.
- An instrument holds at most 10,000 series: later new ones are dropped
  and counted in `xo_metrics_dropped_total`, which `text()` always ends
  with.
- `reg.middleware(clock, label)` is a middleware for `router.wrap` that
  counts requests in `http_requests_total{method,route,status}`, times
  them in `http_request_seconds{route}` and tracks `http_in_flight`;
  `label` maps a request to the (bounded) route label.
- `text()` writes `# HELP` and `# TYPE` per instrument in the order they
  were made, its series sorted by label values, a histogram as
  cumulative `_bucket{le="..."}` (then `le="+Inf"`), `_sum` and `_count`,
  integral values without a fraction, and label values escaped (`\\`,
  `\"`, `\n`). `handler()` answers it as
  `text/plain; version=0.0.4; charset=utf-8`.
- Summaries (decision 0411, Prometheus client_golang's Summary): each
  objective `(quantile, error)` is a `name{...,quantile="Q"}` series,
  computed by CKMS targeted quantiles (beorn7/perks' stream, ported: below
  500 observations the exact sorted value), then `_sum` and `_count`. The
  quantiles cover the last `max_age`, kept in `age_buckets` streams that
  every observation enters; the oldest is reset when its time runs out.
  The time is a scrape's, not the observation's, so `observe` stays pure:
  `text(now)` and `handler_at(clock)` age the buckets, while `text()`
  and `handler()` never do (a summary scraped without a time keeps every
  observation). A summary with no objectives is `_sum` and `_count` only,
  as Go's default.
- `gauge_func`, `counter_func` and `collector` read their values when
  `text` runs (Go's GaugeFunc, CounterFunc and Collector): a collector's
  `f` returns one Sample per series (values one per label), sorted by
  label values in the output. `remove(values)` deletes a series and says
  whether there was one (DeleteLabelValues); `reset()` deletes every
  series.

## 38. std/trace

Request ids and a bounded ring of spans (decision 0325), with no
exporter: a program's handler reads the ring.

```xo
struct Span { id: Str, name: Str, start_ms: Int, micros: Int, status: Int } derive Json
fn ring(cap: Int = 1000) -> Ring
fn (r: Ring) middleware(rand: Rand, clock: Clock) -> fn(http.Request, http.Next) -> http.Response
fn id(req: http.Request) -> Str
fn (r: Ring) record(id: Str, name: Str, start: Time, d: Duration)
fn (r: Ring) spans(min: Duration, n: Int) -> List[Span]
fn (r: Ring) handler(auth: fn(http.Request) -> Bool) -> fn(http.Request) -> http.Response
```

- The middleware takes the id from the `X-Request-Id` header when it is 1
  to 64 printable characters without spaces, else makes a random 64 bit
  one (16 hex digits); the request carries it (`id(req)`), the response
  gets the `X-Request-Id` header, and a `request` span (the request's id,
  start in Unix milliseconds, duration in microseconds, response status)
  is recorded.
- `record` adds a span of the program's own (status 0): time the work
  with the clock and record it with the request's id.
- The ring keeps the last `cap` spans; `spans(min, n)` returns the spans
  of at least `min` newest first, at most `n` (all when n <= 0).
  `handler(auth)` answers them as JSON (`?slow=MILLIS&n=N`, defaults 0 and
  100); 403 when `auth` answers false.

## 39. std/html

Status: implemented on every backend (decision 0408, 2026-10-29): Go's
html package, run by the Go backend itself (`std/rt/html.go`) and ported
natively (`std/native/html.xo`, its entity table generated from Go's
entity.go); Go's results on its
escape_test.go cases (`html_test.xo`). Pure.

```
pub fn escape_string(s: Str) -> Str      // html.EscapeString: < > & ' " as &lt; &gt; &amp; &#39; &#34;
pub fn unescape_string(s: Str) -> Str    // html.UnescapeString: named (legacy names without ; too), decimal and hex references
```

- `unescape_string(escape_string(s)) == s` always; the converse does
  not hold (`&amp` unescapes, nothing escapes to it).
- Numeric references in 0x80 to 0x9F become what browsers substitute
  (`&#x87;` is U+2021); 0, surrogates and code points past U+10FFFF
  become U+FFFD; `&#` and `&#x` with no digits stay as they are.
- html/template's contextual autoescaping is std/template's html mode
  (section 30, decision 0408), which uses this module for attribute
  values.

## 40. std/tabwriter

Go's text/tabwriter (decision 0410): elastic tabstops. Text is split into
cells at tabs, and the columns of a block of consecutive lines are
padded to the width of their widest cell. Pure: bytes in, bytes out.

```xo
const FILTER_HTML: Int = 1
const STRIP_ESCAPE: Int = 2
const ALIGN_RIGHT: Int = 4
const DISCARD_EMPTY_COLUMNS: Int = 8
const TAB_INDENT: Int = 16
const DEBUG: Int = 32
const ESCAPE: Int = 255
fn writer(min_width: Int, tab_width: Int, padding: Int, pad_char: Int = 32, flags: Int = 0) -> TabWriter
fn (var w: TabWriter) write(data: Bytes)
fn (var w: TabWriter) flush() -> Bytes
```

- `writer` is Go's `NewWriter(output, minwidth, tabwidth, padding,
  padchar, flags)` without the output (`flush` returns it): `min_width`
  is the minimal cell width including padding, `tab_width` the width of
  a tab, `padding` what is added to the text's width, `pad_char` the byte
  padded with (9 pads with tabs, which are then never right aligned),
  `flags` the flags added together. A negative number is a fault.
- A cell ends at `\t` or `\v`; a line at `\n` or `\f`. The text of the last
  cell of a line is not part of a column (Go's rule); a `\f` or a line of
  one cell ends the block, so a table is a run of lines with tabs.
- Widths count runes, not bytes; with `FILTER_HTML` an HTML tag counts
  as zero width and an entity as one rune; text between two `ESCAPE`
  bytes passes through as one piece (without them with `STRIP_ESCAPE`).
- `TabWriter` is move only. `write` takes bytes in any pieces; `flush`
  returns the formatted text since the last flush and starts again.
  Output of lines that Go would write as they complete is held until
  `flush`.

## 41. std/x509

X.509 certificates (RFC 5280), Go's crypto/x509: parsing, and verifying
chains as std/tls does for every server. Pure: `verify` takes its roots
and its time; only `system_roots` reads the platform.

```xo
struct Name {
  country: List[Str], organization: List[Str], organizational_unit: List[Str],
  locality: List[Str], province: List[Str], street_address: List[Str],
  postal_code: List[Str], serial_number: Str, common_name: Str,
  raw: Bytes                                // the Name's DER
}
fn (n: Name) display() -> Str               // Go's Name.String(): "CN=a.example,O=A,C=US"
fn (n: Name) attributes() -> List[(Str, Str)]  // every (OID, value), in order
struct Extension { oid: Str, critical: Bool, value: Bytes }
enum KeyUsage { DigitalSignature, ContentCommitment, KeyEncipherment, DataEncipherment,
  KeyAgreement, CertSign, CrlSign, EncipherOnly, DecipherOnly }
enum ExtKeyUsage { Any, ServerAuth, ClientAuth, CodeSigning, EmailProtection,
  TimeStamping, OcspSigning, Other(oid: Str) }
struct Certificate {
  raw: Bytes, version: Int, serial: Bytes, signature_algorithm: Str,
  issuer: Name, subject: Name, not_before: Time, not_after: Time,
  public_key: crypto.PublicKey?, dns_names: List[Str], ip_addresses: List[netip.Addr],
  email_addresses: List[Str], uris: List[Str], is_ca: Bool, max_path_len: Int?,
  key_usage: List[KeyUsage], ext_key_usage: List[ExtKeyUsage],
  subject_key_id: Bytes, authority_key_id: Bytes, extensions: List[Extension]
}
struct CertErr { msg: Str }
fn (e: CertErr) display() -> Str
fn parse(der: Bytes) -> Certificate ! CertErr
fn parse_pem(pem: Bytes) -> List[Certificate] ! CertErr
struct Pkcs12 { key: crypto.PrivateKey, certificate: Certificate, ca: List[Certificate] }
fn parse_pkcs12(data: Bytes, password: Str) -> Pkcs12 ! CertErr
struct VerifyOptions {
  roots: List[Certificate], intermediates: List[Certificate] = [],
  dns_name: Str = "", at: Time, key_usages: List[ExtKeyUsage] = []   // [] is [ServerAuth]
}
fn (c: Certificate) verify(opts: VerifyOptions) -> List[List[Certificate]] ! CertErr
fn (c: Certificate) verify_hostname(host: Str) -> () ! CertErr
fn (c: Certificate) check_signature_from(parent: Certificate) -> () ! CertErr
fn system_roots(fs: Fs) -> List[Certificate] ! CertErr uses fs
fn (c: Certificate) pem() -> Bytes

struct Template {
  serial: Bytes = b"", subject: Name, not_before: Time, not_after: Time,
  dns_names: List[Str] = [], ip_addresses: List[netip.Addr] = [],
  email_addresses: List[Str] = [], uris: List[Str] = [],
  is_ca: Bool = false, max_path_len: Int? = None,
  key_usage: List[KeyUsage] = [], ext_key_usage: List[ExtKeyUsage] = [],
  subject_key_id: Bytes = b"", extra_extensions: List[Extension] = []
}
fn create_certificate(r: Rand, t: Template, public_key: crypto.PublicKey,
  parent: Certificate?, key: crypto.PrivateKey) -> Certificate ! CertErr uses rand
fn self_signed(r: Rand, hosts: List[Str], now: Time, days: Int = 30) -> (Bytes, Bytes) ! CertErr uses rand

struct CertificateRequest {
  raw: Bytes, version: Int, signature_algorithm: Str, subject: Name,
  public_key: crypto.PublicKey?, dns_names: List[Str], ip_addresses: List[netip.Addr],
  email_addresses: List[Str], uris: List[Str], extensions: List[Extension]
}
struct RequestTemplate {
  subject: Name, dns_names: List[Str] = [], ip_addresses: List[netip.Addr] = [],
  email_addresses: List[Str] = [], uris: List[Str] = [], extra_extensions: List[Extension] = []
}
fn create_request(r: Rand, t: RequestTemplate, key: crypto.PrivateKey) -> CertificateRequest ! CertErr uses rand
fn parse_request(der: Bytes) -> CertificateRequest ! CertErr
fn (q: CertificateRequest) check_signature() -> () ! CertErr
fn (q: CertificateRequest) pem() -> Bytes

struct RevokedCertificate { serial: Bytes, revoked_at: Time, reason: Int, extensions: List[Extension] }
struct RevocationList {
  raw: Bytes, signature_algorithm: Str, issuer: Name, number: Bytes,
  this_update: Time, next_update: Time?, revoked: List[RevokedCertificate],
  authority_key_id: Bytes, extensions: List[Extension]
}
fn parse_revocation_list(der: Bytes) -> RevocationList ! CertErr
fn (l: RevocationList) check_signature_from(issuer: Certificate) -> () ! CertErr
```
`parse_pkcs12` reads a PKCS #12 file (RFC 7292, a `.pfx` or `.p12`) as golang.org/x/crypto/pkcs12 does: a v3 file with a
SHA-1 MAC, `pbeWithSHAAnd3-KeyTripleDES-CBC` and `pbeWithSHAAnd40BitRC2-CBC` encryption (the only algorithms Go reads; a
PBES2 or AES file is `pkcs12: algorithm 1.2.840.113549.1.5.13 is not supported`), DER encoded. The password is the text
the file was made with (`""` also opens a file made with no password, as Go does); a wrong one is `pkcs12: decryption
password incorrect`. Deviations from Go's `Decode`, which accepts exactly one certificate and one key: any number of
certificates and bags are read, `ca` holds those other than the leaf, the leaf is the certificate whose `localKeyID`
names the key (else the first), and an unencrypted key bag is read too (on the native backends only). The corpus
(`internal/stdtest/testdata/corpus/pkcs12`) holds Go's three test files and two made with OpenSSL, each with the right
and a wrong password, truncated and bit-flipped; Go's results are the expected values.

```
// a development certificate for a local HTTPS server
let (cert, key) = x509.self_signed(os.rand, ["localhost", "127.0.0.1"], os.clock.now())?
let srv = http.server(listener, routes).with_tls([http.tls_cert(cert, key)?])
```

```
fn check(os: Os, pem: Bytes, roots: Bytes) -> () ! Error uses clock {
  let chain = x509.parse_pem(pem)?
  let leaf = chain[0]
  os.stdio.println("${leaf.subject.display()} until ${leaf.not_after}")
  let _chains = leaf.verify(x509.VerifyOptions{roots: x509.parse_pem(roots)?,
    intermediates: chain[1..chain.len()], dns_name: "api.example.com", at: os.clock.now()})?
}
```

- `parse` is Go's ParseCertificate, with its checks and its error texts
  (`CertErr("x509: malformed certificate")`, ...). `parse_pem` parses
  each CERTIFICATE block of a PEM text in order (other blocks are
  skipped) and fails at the first that does not parse.
- Fields are Go's: `serial` is the serial number's magnitude, big endian;
  `signature_algorithm` is Go's `SignatureAlgorithm.String()`
  (`"SHA256-RSA"`, `"ECDSA-SHA256"`, `"SHA256-RSAPSS"`, `"Ed25519"`, `"0"`
  for one Go does not know); `public_key` is None for a key std/crypto
  has none for (DSA, X25519, an unknown algorithm); `max_path_len` is None
  without a path length constraint; `key_usage` and `ext_key_usage` list
  what the extensions allow, extended usages in the extension's order;
  `extensions` is every extension in order, the value the DER inside its
  OCTET STRING. A time outside what a `Time` holds (1677 to 2262) is
  clamped to its nearest end. A Name's lists hold every value of their attribute,
  `common_name` and `serial_number` the last; `attributes()` is every
  attribute, the ones without a field included.
- `verify` is Go's Certificate.Verify with the given roots: every chain
  from `c` through `intermediates` to one of `roots` whose certificates
  are valid at `at`, allow one of `key_usages` (`Any` accepts any) and
  meet the name constraints and policies (empty `key_usages` is
  `[ServerAuth]`, as Go's nil), leaf first; with `dns_name`
  the leaf must be valid for it (`verify_hostname`). The error is Go's
  text (`"x509: certificate signed by unknown authority"`, `"x509:
  certificate has expired or is not yet valid: current time ... is
  after ..."`, ...). `check_signature_from` is Go's CheckSignatureFrom.
- `create_certificate` is Go's CreateCertificate: the TBS certificate
  of `t` for `public_key`, issued by `parent` (self-signed when None)
  and signed with `key`, in Go's encoding, so an Ed25519 or RSA
  certificate is byte for byte Go's. The signature algorithm is Go's
  default for the key (SHA256-RSA; ECDSA-SHA256, -SHA384, -SHA512 by
  curve; Ed25519; a P-224 key is `CertErr("x509: unknown elliptic
  curve")`). Its extensions, in Go's order: key usage (critical), extended
  key usage, basic constraints (critical; made when `is_ca` or
  `max_path_len` is set), subject key id (on a CA, the first 20 bytes of
  the SHA-256 of the key when `subject_key_id` is empty), authority key id
  (the parent's subject key id), the SAN (critical when the subject is
  empty); then `extra_extensions`, one of which replaces a made one with
  its OID. An empty `serial` is 20 random bytes with the top bit clear. A
  subject with `raw` set is used as it is (a parsed certificate's).
  Errors are Go's texts: a non-ASCII DNS name, e-mail or URI, a
  `max_path_len` on a non-CA, a key that is not the parent's.
- `self_signed` is for development and tests only: a fresh ECDSA P-256
  key and a self-signed certificate for `hosts` (IP addresses go to
  `ip_addresses`, the rest to `dns_names`), organization "Xo
  Development", digital signature and server authentication, valid from
  `now` for `days`; it returns the certificate's PEM and the key's PKCS
  #8 PEM. `pem()` is a certificate's (or request's) PEM block.
- `create_request` and `parse_request` are Go's CreateCertificateRequest
  and ParseCertificateRequest (PKCS #10): the names go into a requested
  SAN extension; `check_signature` checks the request's own signature.
  A request whose DER does not parse is `CertErr("x509: malformed
  certificate request")` where Go reports encoding/asn1's text.
- `parse_revocation_list` is Go's ParseRevocationList (RFC 5280 5):
  each revoked serial with its time, reason code and extensions, the CRL
  number and the authority key id; `check_signature_from(issuer)` is Go's
  RevocationList.CheckSignatureFrom. Creating CRLs, OCSP and PKCS #12 are
  not in std (Go keeps the latter two in x/crypto).
- `system_roots(fs)` is the roots std/tls verifies servers against when
  given none (27): Linux's CA bundle and directories (`SSL_CERT_FILE`,
  `SSL_CERT_DIR`), Windows's ROOT store, and on macOS the system's anchor
  certificates (SecTrustCopyAnchorCertificates; TLS there asks the
  Security framework, which also applies user and admin trust settings
  that a list cannot hold). Certificates that do not parse are left out.
- Tests: the corpus `x509create` makes every template both in Xo and in
  Go (equal bytes for Ed25519 and RSA, equal fields for ECDSA), and
  parses Go's requests and CRLs and mutated copies against Go's fields
  and errors. The corpus `x509` (internal/stdtest) parses every certificate of
  Go's crypto/x509 and crypto/tls testdata, a fifth of the NIST PKITS
  certificates, verify_test.go's and mutated copies, comparing every
  field and error text with Go's, and runs verify_test.go's verifyTests
  against Go's chains and errors, on every backend.
- Backends: native (std/native/x509.xo over the TLS stack's parser and
  verifier, std/native/llrt/x509cert.xo and x509verify.xo) and Go
  (crypto/x509).

## 42. std/rate

A token bucket rate limiter, golang.org/x/time/rate's `Limiter`: `limit`
tokens a second fill a bucket of `burst` tokens, which starts full; an
event takes n tokens. The limiter holds the `Clock` it reads, given at
`new`, so a test's virtual clock (`os.clock` in a test) makes it
deterministic. A `Limiter` is a handle: copies share the bucket, and
tasks may use it at once.

```xo
const INF: Float                                     // allows every event
fn new(limit: Float, burst: Int, clock: Clock) -> Limiter
fn every(interval: Duration) -> Float                // one token per interval
fn (l: Limiter) limit() -> Float
fn (l: Limiter) burst() -> Int
fn (l: Limiter) tokens() -> Float uses clock
fn (l: Limiter) allow() -> Bool uses clock
fn (l: Limiter) allow_n(n: Int) -> Bool uses clock
fn (l: Limiter) reserve() -> Reservation uses clock
fn (l: Limiter) reserve_n(n: Int) -> Reservation uses clock
fn (l: Limiter) wait() -> () ! RateErr uses clock
fn (l: Limiter) wait_n(n: Int) -> () ! RateErr uses clock
fn (l: Limiter) set_limit(limit: Float) uses clock
fn (l: Limiter) set_burst(burst: Int) uses clock
fn (r: Reservation) ok() -> Bool
fn (r: Reservation) delay() -> Duration uses clock
fn (r: Reservation) cancel() uses clock
enum RateErr { Exceeds(n: Int, burst: Int), Canceled }
```

- `allow_n` takes n tokens when the bucket has them and answers whether
  it did; `reserve_n` always takes them (the bucket may go negative) and
  says when the event may act: `delay()` is 0 when now is fine. A
  reservation of more than the burst is not `ok` and its delay is the
  longest Duration. `cancel` gives back the tokens later reservations
  have not counted on (Go's CancelAt).
- `wait_n` sleeps on the limiter's clock until n events may happen; it
  fails with `Exceeds` when n is over the burst (the limit not `INF`)
  and with `Canceled` when the task is cancelled while it waits, the
  tokens going back. Display: `rate: Wait(n=4) exceeds limiter's burst
  3`, `rate: canceled`.
- `set_limit` and `set_burst` keep the tokens so far. A limit of 0
  allows the burst once and then nothing; `INF` allows everything,
  whatever the burst. `every(d)` is `INF` for d of 0 or less.
- Answers are x/time/rate's to the bit (a random differential on the
  virtual clock, 2026-11-04): tokens use Go's `Duration.Seconds`, and a
  wait of forever acts at Xo's last Time (Go's Time reaches further)
  while `delay()` counts down from it as Go's does; nothing overflows.
- Go's API without a Context: Go's `WaitN(ctx, n)` deadline check is the
  task's cancellation; `AllowN(t, n)` and the other `*At` forms are the
  clock's time (a test sets it).

## 43. std/backoff

Exponential backoff with jitter and a retry loop, cenkalti/backoff v4's
`ExponentialBackOff` and `Retry`. The Clock and Rand are given per call,
so a test's virtual clock and a seeded or scripted Rand make it
deterministic.

```xo
struct Backoff {
  initial: Duration = 500ms
  multiplier: Float = 1.5
  randomization: Float = 0.5     // d drawn from [d - r d, d + r d]
  max_interval: Duration = 60s
  max_elapsed: Duration = 15m    // 0s: no limit
  max_tries: Int = 0             // calls of op; 0: no limit
}
fn (b: Backoff) delay(attempt: Int, rand: Rand) -> Duration uses rand
fn retry[T, E, fx: Effects](b: Backoff, clock: Clock, rand: Rand, op: fn() -> T ! E uses fx, retryable: fn(E) -> Bool) -> T ! E uses (clock, rand, fx)
fn retry_notify[T, E, fx: Effects, rx: Effects, nx: Effects](b: Backoff, clock: Clock, rand: Rand, op: fn() -> T ! E uses fx, retryable: fn(E) -> Bool uses rx, notify: fn(E, Duration) -> () uses nx) -> T ! E uses (clock, rand, fx, rx, nx)
fn always[E]() -> fn(E) -> Bool
```

- `delay(k, rand)` is the delay before retry k (0 first): `initial`
  multiplied k times by `multiplier`, truncated to whole nanoseconds at
  each step and capped at `max_interval`, then randomized as Go's
  getRandomValueFromInterval with one `rand.float()`.
- `retry` calls op; on `Err(e)` it answers `Err(e)` when `retryable(e)`
  is false, when `max_tries` calls were made, or when sleeping the next
  delay would pass `max_elapsed` since the first call; otherwise it
  sleeps that delay on clock (a cancellation point) and calls op again.
  `retryable` is required: `backoff.always()` retries every error, the
  same as cenkalti's default (its Permanent errors are `retryable`
  answering false).
- `retry_notify` is `retry` with cenkalti's RetryNotify: before each
  sleep it calls `notify(e, d)` with the error and the delay, to log or
  count retries; an error it answers is not notified. `retryable` and
  `notify` may have effects of their own (a log, a metric): each
  callback has its own effect variable.

## 44. std/crc32

Status: implemented on every backend (2026-10-29): Go's hash/crc32, run
by the Go backend itself (`std/rt/crc32.go`) and ported natively
(`std/native/crc32.xo`: IEEE on std/native/llrt/flate.xo's slicing-by-8
with arm64's crc32x instruction, Castagnoli on slicing-by-8 in
software); Go's values on its crc32_test.go inputs (corpus `crc32`). Pure.

```xo
enum Poly { Ieee, Castagnoli }
pub fn checksum(data: Bytes, poly: Poly = Poly.Ieee) -> Int             // ChecksumIEEE, Checksum(data, MakeTable(Castagnoli))
pub fn update(crc: Int, data: Bytes, poly: Poly = Poly.Ieee) -> Int     // Update
```

- A checksum is 0 to 2^32 - 1. `update(checksum(a), b) == checksum(a +
  b)` and `update(0, d) == checksum(d)`, so a stream is checked a piece
  at a time.
- IEEE is the polynomial of gzip, zip, PNG and Ethernet; Castagnoli
  (CRC-32C) that of iSCSI, SCTP, ext4, Btrfs and many storage formats.
  Go's Koopman table is not offered.

## 45. std/fnv

Status: implemented on every backend (2026-10-29): Go's hash/fnv, run by
the Go backend itself (`std/rt/fnv.go`) and ported natively
(`std/native/fnv.xo`); Go's values on its fnv_test.go inputs (corpus
`fnv`). Pure.

```xo
pub fn sum32(data: Bytes) -> Int          // New32: FNV-1, 0 to 2^32 - 1
pub fn sum32a(data: Bytes) -> Int         // New32a: FNV-1a
pub fn sum64(data: Bytes) -> Int          // New64: the Int's bits are Go's uint64
pub fn sum64a(data: Bytes) -> Int         // New64a
pub fn sum128(data: Bytes) -> Bytes       // New128: 16 bytes, big endian
pub fn sum128a(data: Bytes) -> Bytes      // New128a
pub fn update32(sum: Int, data: Bytes) -> Int
pub fn update32a(sum: Int, data: Bytes) -> Int
pub fn update64(sum: Int, data: Bytes) -> Int
pub fn update64a(sum: Int, data: Bytes) -> Int
pub fn update128(sum: Bytes, data: Bytes) -> Bytes    // faults unless sum is 16 bytes
pub fn update128a(sum: Bytes, data: Bytes) -> Bytes
```

- `updateN(sumN(a), b) == sumN(a + b)`: hashing a piece at a time, as
  Go's Write on one hash.
- FNV is fast and not cryptographic: for hash tables, sharding and
  checksums of trusted data, not where an attacker chooses the input
  (use std/crypto's digest there).
## 46. std/dns

DNS record lookups (decision 0417), Go's `net.LookupMX`, `LookupTXT`,
`LookupSRV` and `LookupAddr`. Free functions over a Net, `uses
net.dial` as `Net.lookup_host`: an attenuated Net (`only`) refuses a
name it does not admit with `Denied` (for `lookup_srv` the name checked
is `name`, for `lookup_addr` the address).

```xo
struct Mx { host: Str, pref: Int }
struct Srv { target: Str, port: Int, priority: Int, weight: Int }
struct Resolver { server: Str = "" }   // "": the system's servers
fn lookup_mx(n: Net, name: Str) -> List[Mx] ! NetErr uses net.dial
fn lookup_txt(n: Net, name: Str) -> List[Str] ! NetErr uses net.dial
fn lookup_srv(n: Net, service: Str, proto: Str, name: Str) -> List[Srv] ! NetErr uses net.dial
fn lookup_addr(n: Net, ip: Str) -> List[Str] ! NetErr uses net.dial
// and the same four as methods of Resolver
```

- The free functions are `Resolver{}`'s. A Resolver with `server`
  ("127.0.0.1:5353", a numeric host and a port) sends every query to that
  server only, as Go's `Resolver{PreferGo: true, Dial: ...}`; otherwise
  the servers, search list and options of /etc/resolv.conf apply, read on
  every lookup.
- Names come back rooted ("mx1.example.com."), with their case as the
  server sent it. `lookup_mx` orders by preference, equal preferences
  shuffled; `lookup_srv` asks for `_service._proto.name` (`name` alone
  when service and proto are both ""), orders by priority and shuffles
  each priority's records by weight (RFC 2782); `lookup_txt` joins each
  record's character strings; `lookup_addr` asks for the PTR records of
  the address's in-addr.arpa or ip6.arpa name, after /etc/hosts (its
  names lowercased and rooted).
- A record whose name is not a domain name (Go's isDomainName), or a TXT
  record that is not UTF-8, is left out: the call is `Ok` with the
  valid records when there is at least one, and `Err(Io)` ("DNS response
  contained records which contain invalid names") when every record was
  left out. Go returns the valid records together with that error.
- Errors are `Io` with Go's DNSError text: "lookup NAME on SERVER: no
  such host" (NXDOMAIN, or no record of the type), "... server
  misbehaving" (SERVFAIL, or no server answered), "lookup IP:
  unrecognized address" for an address that does not parse. A FakeNet
  (without `xo test --real`) has no DNS: every lookup is "no such host".
- The native backends use Xo's own DNS client (std/native/llrt/dns.xo,
  the one `lookup_host` uses on Linux) on Linux and macOS: UDP with TCP
  when an answer is truncated. macOS's scoped resolvers (a VPN's split
  DNS in `scutil --dns`) are not consulted, as Go's own resolver does not
  consult them. On Windows the same client asks the DNS servers of the
  network adapters that are up (GetAdaptersAddresses) and reads the system
  directory's drivers\etc\hosts, as Go's pure Go resolver does there,
  unlike Go's default on Windows (the system resolver): Windows' name
  resolution policies (NRPT, a VPN's split DNS), its DNS over HTTPS
  setting and its cache are not consulted (decision 0427).
  The Go backend uses Go's resolver on every OS.

## 47. std/deque

A double ended queue on a ring buffer, as a Go slice ring or
`container/list` used as a queue. A std module rather than the prelude:
prelude type names cannot be redeclared (core 1.4), so a program's own
`Deque` keeps compiling. Pure.

```xo
struct Deque[T] {}
pub fn new[T](cap: Int = 0) -> Deque[T]
fn (var d: Deque[T]) push_back(x: T)
fn (var d: Deque[T]) push_front(x: T)
fn (var d: Deque[T]) pop_front() -> T?          // None when empty
fn (var d: Deque[T]) pop_back() -> T?
fn (d: Deque[T]) front() -> T?
fn (d: Deque[T]) back() -> T?
fn (d: Deque[T]) get(i: Int) -> T?              // from the front; None out of range
fn (d: Deque[T]) len() -> Int
fn (d: Deque[T]) is_empty() -> Bool
fn (d: Deque[T]) to_list() -> List[T]           // front to back
```

- Both ends are O(1), amortized: a full ring doubles (at least 8
  slots), copying its elements once. A popped element is released at
  once.
- A `Deque` is a value, as `List`: a copy shares the ring until one of
  them is written, and that write copies the ring once (O(n)). Pass it
  by `var` or keep it in one place to avoid the copy.
- There is no `for x in d` (core 3.5 iterates the built in collections
  only): `for x in d.to_list()`.

## 48. std/png

Go's image/png over whole buffers: `decode`, `decode_config` and `encode`
(decision 0466). One `Image` struct holds Go's seven concrete image types
with their `Pix` layouts, so the pixel bytes are what Go's decoder returns
and what Go's encoder takes. Pure; decode and encode go through std/compress.

```xo
enum Model { Gray, Gray16, Paletted, RGBA, NRGBA, RGBA64, NRGBA64 } derive Eq, Display
struct Image { width: Int, height: Int, model: Model, pix: Bytes, palette: List[Int] = [] }
struct Config { width: Int, height: Int, model: Model }
enum Compression { Default, None, Speed, Best } derive Eq
enum PngErr { Format(msg: Str), Unsupported(msg: Str), Truncated, Flate(msg: Str) }
fn (e: PngErr) display() -> Str
fn image(width: Int, height: Int, model: Model) -> Image
fn (m: Model) bytes_per_pixel() -> Int
fn (img: Image) at(x: Int, y: Int) -> Int          // 0xRRGGBBAA, not premultiplied
fn decode(data: Bytes, max_pixels: Int = 100000000) -> Image ! PngErr
fn decode_config(data: Bytes) -> Config ! PngErr
fn encode(img: Image, level: Compression = Compression.Default) -> Bytes ! PngErr
```

- `decode` returns the type Go's `png.Decode` does: gray is Gray (8 bit or
  less, scaled to 8) or Gray16; palette color types are Paletted (a palette
  grows to the largest index in the data, new entries opaque black); opaque
  truecolor is RGBA or RGBA64; truecolor or gray with alpha or a tRNS chunk
  is NRGBA or NRGBA64. Adam7 interlacing is read. Chunk order, lengths and
  CRC-32 checks are Go's, in Go's order.
- `PngErr` carries Go's errors: `Format` and `Unsupported` with the text after
  `png: invalid format: ` or `png: unsupported feature: ` (`display` gives the
  full text), `Truncated` for `unexpected EOF`, and `Flate` for the zlib
  reader's own errors. A header over `max_pixels` pixels is `Format("image too
  large")`, refused before anything is allocated (Go allocates from the
  header). Where a file has more than one fault in its pixel data stream the
  error that comes first can differ from Go's (Go inflates as it reads rows;
  Xo inflates a buffer and reads again for the rows before an error); the file
  is refused either way, and every error outside the stream is Go's exactly.
- `encode` writes Go's bytes at every level: the filter chosen per row by the
  sum of absolute differences (none for Paletted or `Compression.None`), 8 or
  16 bit truecolor or gray, 1, 2, 4 or 8 bit palettes with a tRNS chunk for
  palette alpha, truecolor with alpha only when a pixel is not opaque, IDAT
  chunks of 32,768 bytes, and the zlib stream at the same level
  (`Speed` is zlib 1, `Best` 9). It faults when `pix` is not the size the
  model, width and height say.
- Not ported: the `image.Image` interface and other color models, `png`'s
  `EncoderBufferPool`, interlaced encode (Go has none).

## 49. std/jpeg

Go's image/jpeg decoder and encoder over whole buffers: `decode`,
`decode_config` (decision 0467) and `encode` (decision 0510). Baseline and progressive JPEG, every chroma subsampling
ratio Go supports, restart intervals, Adobe CMYK, YCCK and RGB, and Go
1.26's integer IDCT, so the decoded planes are the bytes Go's decoder
returns. One `Image` struct holds Go's four decoded image types. Pure.

```xo
enum Model { Gray, YCbCr, CMYK, RGBA } derive Eq, Display
enum Subsample { S444, S440, S422, S420, S411, S410 } derive Eq, Display
struct Image { width: Int, height: Int, model: Model, subsample: Subsample = Subsample.S444,
               pix: Bytes = b"", y: Bytes = b"", cb: Bytes = b"", cr: Bytes = b"" }
struct Config { width: Int, height: Int, model: Model }
enum JpegErr { Format(msg: Str), Unsupported(msg: Str), Truncated, Encode(msg: Str) }
fn (e: JpegErr) display() -> Str
fn (img: Image) chroma_width() -> Int
fn (img: Image) chroma_height() -> Int
fn (img: Image) to_rgba() -> Bytes
fn decode(data: Bytes, max_pixels: Int = 100000000) -> Image ! JpegErr
fn decode_config(data: Bytes) -> Config ! JpegErr
fn encode(img: Image, quality: Int = 75) -> Bytes ! JpegErr
```

- `decode` returns the type Go's `jpeg.Decode` does: Gray for one component,
  YCbCr for three (with the chroma `subsample` ratio), CMYK for four
  components (Adobe transform 0 or 2, the YCCK case, converted as Go does),
  and RGBA for an RGB JPEG (an Adobe marker that says RGB, or components named
  R, G and B). Rows are tight (no stride padding): a plane has exactly
  `width * height` bytes (chroma planes `chroma_width() * chroma_height()`).
- `to_rgba` converts as Go's `draw.Draw` does (color.YCbCrToRGB,
  color.CMYKToRGB, gray with alpha 255).
- `JpegErr` carries Go's errors: `Format` and `Unsupported` with the text after
  `invalid JPEG format: ` or `unsupported JPEG feature: ` (`display` gives the
  full text) and `Truncated` for `unexpected EOF`. A frame header asking for
  more than `max_pixels` pixels is `Format("image too large")`, refused before
  anything is allocated (Go allocates from the header).
- `encode` is Go's `jpeg.Encode` with `Options{Quality: quality}`, byte for
  byte: baseline JPEG with the standard Huffman tables, quality scaled
  quantization tables, Go's integer forward DCT, one component for Gray and
  4:2:0 for every other input (a YCbCr of any `subsample` is resampled as Go
  does; RGBA ignores alpha, as Go's `image.RGBA` fast path; CMYK goes through
  Go's generic path). `quality` is clamped to 1..100. A width or height of
  65,536 or more is `Encode("jpeg: image is too large to encode")`; an
  `Image` whose `pix` or planes do not match its size is a fault, as in
  `png.encode`. A zero size image gives what Go gives.
- Not ported: the `image.Image` interface, a streaming encoder, and the
  `Reader` interface (whole buffers only).

## 50. std/cbor

RFC 8949 CBOR over a `Value` tree (decision 0468): encode with the preferred
serialization, decode definite and indefinite lengths with limits, the
diagnostic notation of section 8, and the JSON bridge of section 6. Pure.

```xo
enum Value { Uint(n: UInt64), Nint(n: UInt64), ByteString(b: Bytes), TextString(s: Str),
             Array(items: List[Value]), Map(pairs: List[(Value, Value)]), Tag(num: UInt64, v: Value),
             Simple(n: Int), Bool(b: Bool), Null, Undefined, Float(f: Float) }
enum CborErr { Truncated, Syntax(msg: Str), Invalid(msg: Str), Limit(msg: Str) }
fn (e: CborErr) display() -> Str
fn from_int(n: Int) -> Value
fn (v: Value) to_int() -> Int?
fn (v: Value) get(key: Str) -> Value?
fn encode(v: Value, deterministic: Bool = false) -> Bytes ! CborErr
fn decode(data: Bytes, max_depth: Int = 128, reject_dup_keys: Bool = false) -> Value ! CborErr
fn decode_prefix(data: Bytes, max_depth: Int = 128, reject_dup_keys: Bool = false) -> (Value, Int) ! CborErr
fn well_formed(data: Bytes, max_depth: Int = 128) -> Bool
fn (v: Value) diagnostic() -> Str
fn (v: Value) to_json() -> json.Value ! CborErr
fn from_json(j: json.Value) -> Value
```

- The model holds the whole format: `Uint(n)` is n and `Nint(n)` is -1-n, each
  to 64 bits, so every integer CBOR can write is a value; a map keeps its pairs
  in order with any key; `Simple(n)` is a simple value other than false, true,
  null and undefined (0 to 19, 32 to 255).
- `encode` writes the preferred serialization: shortest integer heads, the
  shortest of half, single and double that holds a float (NaN as `f97e00`,
  infinities as half), definite lengths. With `deterministic` the pairs of
  every map are sorted bytewise by the encoding of the key (RFC 8949 4.2.1).
  `Simple(n)` outside 0 to 19 and 32 to 255 is `Invalid`.
- `decode` reads one item and refuses data after it (`Invalid`); `decode_prefix`
  returns the number of bytes used (a CBOR sequence, RFC 8742). Indefinite
  length strings, arrays and maps are read into definite values. `Truncated`
  is data that ends inside an item, a length past the data included (nothing is
  allocated from a length); `Syntax` is what is not well formed (additional
  information 28 to 30, a break outside an indefinite item, a wrong or
  indefinite chunk, a map with an odd count, a two byte simple value below 32);
  `Invalid` is text that is not UTF-8 (each chunk of an indefinite text string
  is checked) and, with `reject_dup_keys`, a repeated map key (keys compare by
  their deterministic encoding; a NaN key is never a repeat); `Limit` is nesting
  past `max_depth` (arrays, maps and chains of tags; one tag does not count).
- `well_formed` is RFC 8949 section 5.3.1: no UTF-8 or key checks.
- `diagnostic` is section 8 notation, with a bignum (tag 2 or 3 on a byte
  string) written as its number and floats as ES6 number text with a decimal
  point (`1.0`, `1.0e+300`, `NaN`, `Infinity`).
- `to_json` follows section 6.2: byte strings as base64url without padding,
  tags dropped, `Undefined` as null, `Simple(n)` as the text `simple(n)`, map
  keys of text, integer, float, bool and null kinds as their text; NaN,
  infinity and other keys are `Invalid`. `from_json` follows 6.1.
- `derive Cbor` (decision 0527, core 2.9) gives a struct typed `marshal[T]`,
  `unmarshal[T]` and `unmarshal_prefix[T]`, matching fxamacker/cbor/v2's
  `Marshal` and `Unmarshal` (differential corpus `testdata/corpus/cbor`,
  2,000 typed cases):
  - Fields: Str, Int, Float, Bool, Bytes, Time, Duration, `cbor.Value`, an
    Option, a List, a `Map[Str, V]` of those, or a struct that derives Cbor.
    Key: the field's `cbor "key,options"` string, else its `as` string, else
    its name; options `omitempty`, `omitzero` (the same here), `keyasint`;
    `-` skips the field; `cbor ",toarray"` on a struct's first field writes the struct as an
    array in field order and reads an array of exactly that many items.
    `derive Cbor(case:)` is not built yet.
  - `marshal` writes a map of the fields (keys sorted when `deterministic`),
    Float in the shortest form, Time as integer seconds or, with a fraction,
    a float (fxamacker's TimeUnixDynamic), Duration as integer nanoseconds,
    None as null. Its generated code writes the encoding directly, with no
    `Value` tree (decision 0559), through std/cbor's helpers for generated
    code: `put_head(out, major, n)`, `put_int`, `put_float`, `put_text`,
    `put_bytes` (each appends one item to a `BytesBuilder`), `put_value(out,
    v, deterministic)` (a `Value` field, as `encode` writes it) and
    `det_keys(keys)` (text keys in the deterministic order).
  - `unmarshal` matches a text key exactly, then ignoring case, and the first
    key to reach a field wins; a `keyasint` field also takes its number as
    text. Null leaves a field alone; tags are transparent after fxamacker's
    checks of tags 0 to 3 (the tag 2 and 3 big integers read into Int when
    they fit); an array of small integers reads as Bytes; an integer reads as
    a Float. The first type mismatch is kept (`CborErr.Type`) and decoding
    goes on; `strict: true` also makes a key no field names an error.
- Not ported: streaming, the date and time tag helpers.

## 51. std/crc64

Status: implemented on every backend (2026-11-03): Go's hash/crc64, run
by the Go backend itself (`std/rt/crc64.go`) and ported natively
(`std/native/crc64.xo`: slicing-by-8 over Go's tables); Go's values on
its golden strings and on pseudo random buffers of 0 to 1,500,000 bytes
(corpus `crc64`). Pure.

```xo
enum Poly { Iso, Ecma, Nvme }  // Nvme: CRC-64/NVME, 0x9a6c9329ac4bc9b5 reversed (S3's default checksum); check value of "123456789" 0xae8b14860a799888
pub fn checksum(data: Bytes, poly: Poly = Poly.Iso) -> Int             // Checksum(data, MakeTable(poly))
pub fn update(crc: Int, data: Bytes, poly: Poly = Poly.Iso) -> Int     // Update
```

- A checksum is 64 bits: the Int's bits are Go's uint64, so one past 2^63
  is negative (as `fnv.sum64`). `update(checksum(a), b) == checksum(a +
  b)` and `update(0, d) == checksum(d)`.
- ISO is the polynomial of Redis and HDLC; ECMA that of xz.

## 52. std/adler32

Status: implemented on every backend (2026-11-03): Go's hash/adler32, run
by the Go backend itself (`std/rt/adler32.go`) and ported natively
(`std/native/adler32.xo`); Go's values on its golden strings and on
pseudo random buffers (corpus `adler32`). Pure.

```xo
pub fn checksum(data: Bytes) -> Int          // Checksum: 0 to 2^32 - 1, 1 for empty data
pub fn update(sum: Int, data: Bytes) -> Int  // Write on a digest at that state
```

- `update(checksum(a), b) == checksum(a + b)` and `update(1, d) ==
  checksum(d)`.
- Adler-32 is the checksum of zlib (RFC 1950). It is weak on short inputs:
  for detecting corruption, not for hashing.

## 55. std/bufio

Status: implemented on every backend (2026-11-03, decision 0471): `std/native/bufio.xo` on the native
backends and `std/rt/bufio.go` on the Go backend (one algorithm in both), checked against Go's
bufio.Reader (ReadBytes, Peek, Read) and io.ReadFull on 700 scripted data and chunking cases
(`internal/stdtest/testdata/corpus/bufio`, generated by `go run ./scripts/stdcorpus bufio`).

```xo
enum BufErr { Net(err: NetErr), TooLong, UnexpectedEof }   // displays the stream's text, `bufio: record too long`, `bufio: unexpected EOF`
fn reader(s: Stream, size: Int = 4096) -> Reader           // size at least 16; Reader is move only
fn (var r: Reader) read(max: Int) -> Bytes? ! NetErr uses net            // up to max bytes; None at the end
fn (var r: Reader) read_until(delim: UInt8, limit: Int = 1048576) -> Bytes? ! BufErr uses net
fn (var r: Reader) read_line(limit: Int = 1048576) -> Str? ! BufErr uses net
fn (var r: Reader) read_exact(n: Int) -> Bytes ! BufErr uses net
fn (var r: Reader) peek(n: Int) -> Bytes ! BufErr uses net
fn (r: Reader) buffered() -> Int
```

- Any `Stream` (a Conn, a ServerConn, a TLS connection, a test's fake) is read; the Reader keeps the chunk
  it last read and a record that lies inside one chunk is a view of it, so a line costs no copy; a record
  that spans chunks is gathered once.
- `read_until` returns the bytes up to and including `delim` (Go's ReadBytes); at the end of the stream
  the bytes left, if any, are the last record, and None means none were left. When `limit` bytes arrive
  without `delim` it fails with `TooLong`: the chunks of the record already gathered are dropped and the
  current chunk stays buffered, so a caller closes the connection.
- `read_line` is `read_until('\n')` with the "\n" or "\r\n" cut, invalid UTF-8 as U+FFFD for each
  invalid byte; a last line without an ending is a line.
- `read_exact(n)` is io.ReadFull: `UnexpectedEof` when the stream ends first (what was read is lost); n 0
  is empty. `peek(n)` does not consume, and returns fewer than n only at the end of the stream.
- `read(0)`, `peek(0)` and `read_exact(0)` return empty without reading.

## 54. std/charset

Status: implemented on every backend (2026-11-03, decision 0470): one source, `std/native/charset.xo`,
that the Go backend compiles (std/go_compiled.txt, decision 0312); golang.org/x/text v0.43.0's UTF-16,
Latin-1 and Windows-1252 and Go's unicode/utf16, on hand picked and seeded random inputs
(`internal/stdtest/testdata/corpus/charset`, generated by `go run ./scripts/stdcorpus charset` from
scripts/stdcorpus/charsetoracle). Pure.

```xo
enum Charset { Utf8, Utf16Le, Utf16Be, Utf16Bom, Latin1, Windows1252 }   // derive Eq, Display
struct CharsetErr { offset: Int, msg: Str }       // displays `charset: MSG at byte OFFSET`
pub fn decode(data: Bytes, cs: Charset, lossy: Bool = false) -> Str ! CharsetErr
pub fn encode(text: Str, cs: Charset) -> Bytes ! CharsetErr
pub fn lookup(label: Str) -> Charset?
pub fn utf16_units(text: Str) -> List[Int]          // Go's utf16.Encode
pub fn from_utf16_units(units: List[Int]) -> Str    // Go's utf16.Decode
```

- `Utf16Le` and `Utf16Be` read and write no byte order mark (a U+FEFF stays a character); `Utf16Bom`
  reads FF FE or FE FF and drops it, little endian when there is none, and writes FF FE and little
  endian: Excel's "Unicode Text". `Latin1` is ISO 8859-1; `Windows1252` the Encoding Standard's table,
  with the five unassigned bytes (0x81, 0x8D, 0x8F, 0x90, 0x9D) as U+0081, U+008D, U+008F, U+0090 and
  U+009D, in both directions (x/text decodes them to U+FFFD and refuses to encode them).
- `decode` fails with the byte offset of the first invalid input (`invalid UTF-8`, `odd number of
  UTF-16 bytes`, `unpaired UTF-16 surrogate`); `lossy: true` writes U+FFFD for it: one for each
  invalid UTF-8 byte (Go's range over a string), one for each unpaired UTF-16 surrogate (Go's
  utf16.Decode; x/text gives one U+FFFD for a lone low surrogate and the surrogate after it) and one
  for an odd last byte. The single byte charsets accept every byte.
- `encode` fails with the byte offset in `text` of a character the charset cannot hold
  (`character U+20AC is not in latin1`).
- `lookup` reads the Encoding Standard's labels, ASCII case and surrounding space ignored: "utf-8",
  "utf8"; "utf-16" (`Utf16Bom`), "utf-16le", "ucs-2", "unicode" (`Utf16Le`), "utf-16be";
  "windows-1252", "cp1252", "iso-8859-1", "latin1", "ascii" and the other labels browsers read as
  windows-1252 (all `Windows1252`, so a "latin1" label gets the printable characters of 0x80 to 0x9F, as
  in a browser; ask for `Latin1` by name for the strict ISO table). None for any other label.

## 56. std/heap

Status: implemented on every backend (2026-11-03, decision 0472, Accepted by the owner 2026-11-03): Go's container/heap algorithm
(heap.go: Init, Push, Pop, Remove, Fix) in `std/native/heap.xo` and, for the Go backend, `std/rt/heap.go`, checked against
container/heap on random operation sequences, the whole heap array after every operation
(`internal/stdtest/testdata/corpus/heap`, generated by `go run ./scripts/stdcorpus heap`).

```xo
fn new[T](less: fn(T, T) -> Bool) -> Heap[T]                        // less(a, b): a comes out before b
fn tracked[T](less: fn(T, T) -> Bool, moved: fn(T, Int)) -> Heap[T]  // moved(x, i): x landed at position i
fn from_list[T](items: List[T], less: fn(T, T) -> Bool) -> Heap[T]   // heap.Init, O(n)
fn (var h: Heap[T]) push(x: T)           // O(log n)
fn (var h: Heap[T]) pop() -> T?          // the least; None when empty
fn (h: Heap[T]) peek() -> T?
fn (h: Heap[T]) get(i: Int) -> T?        // by heap position; None out of range
fn (var h: Heap[T]) remove(i: Int) -> T? // heap.Remove; None out of range
fn (var h: Heap[T]) fix(i: Int, x: T)    // replace the element at i, restore the order; a fault out of range
fn (h: Heap[T]) len() -> Int
fn (h: Heap[T]) is_empty() -> Bool
fn (h: Heap[T]) to_list() -> List[T]     // heap order, not sorted
```

- A heap is a value as `Deque` is: a copy shares its elements until one is written. It holds closures, so it is
  not `Eq`, `Hash` or `Json`. Equal elements come out in no particular order.
- `moved` is called for each element that lands at a position: a push (its position, then each sift step), the
  two elements of every swap, the element a `remove` or `pop` moves into the gap, and `fix`'s element. A caller
  keeps `Map[Key, Int]` of positions to `remove` a cancelled job or `fix` a rescheduled one.
- The fields `_items`, `_less` and `_moved` of the declaration exist so the Go backend copies a heap's list as
  Xo's value semantics require; use the methods.

## 57. std/otp

Status: implemented on every backend (2026-11-03, decision 0473, Accepted by the owner 2026-11-03): one pure source,
`std/native/otp.xo`, that the Go backend compiles (std/go_compiled.txt, decision 0312), checked against an
independent Go implementation on crypto/hmac and net/url, and the RFC 4226 appendix D and RFC 6238 appendix B
vectors (`internal/stdtest/testdata/corpus/otp`, generated by `go run ./scripts/stdcorpus otp`).

```xo
fn hotp(secret: Bytes, counter: Int, digits: Int = 6, algo: crypto.Hash = Sha1) -> Str                  // RFC 4226
fn totp(secret: Bytes, unix: Int, period: Int = 30, digits: Int = 6, algo: crypto.Hash = Sha1) -> Str  // RFC 6238
fn verify(secret: Bytes, code: Str, unix: Int, period: Int = 30, digits: Int = 6, algo: crypto.Hash = Sha1, skew: Int = 1) -> Int?
fn generate_secret(r: Rand, len: Int = 20) -> Bytes uses rand
fn uri(issuer: Str, account: Str, secret: Bytes, period: Int = 30, digits: Int = 6, algo: crypto.Hash = Sha1) -> Str
```

- `digits` is 6, 7 or 8 and `algo` Sha1, Sha256 or Sha512; anything else, a negative counter or unix time, or a
  period under 1 is a fault. The code is zero padded to `digits`.
- `verify` returns the time step (the hashed counter) whose code matches, among the step of `unix` and `skew` steps
  either side, or None (decision 0473, owner 2026-11-03). Every candidate is compared in constant time (no early
  exit); a code that is not `digits` ASCII digits is None, never a fault. It does not remember a used code: the
  caller stores the last step it accepted and rejects a step that is not later (RFC 6238 section 5.2).
- `uri` is `otpauth://totp/ISSUER:ACCOUNT?algorithm=SHA1&digits=6&issuer=ISSUER&period=30&secret=BASE32`: the label
  parts path escaped, the parameters sorted and query escaped (`url.Values.encode`), the secret unpadded base32,
  as pquerna/otp's Key.URL.

## 53. std/bcrypt

Status: implemented on every backend (2026-11-03): one source, `std/native/bcrypt.xo`, a port of
golang.org/x/crypto v0.58.0's bcrypt and blowfish packages that the Go backend compiles
(std/go_compiled.txt, decision 0312); Go's hashes, errors and texts, on hashes Go made (seeded
salts, costs 4 to 6) and on malformed ones (`internal/stdtest/testdata/corpus/bcrypt`, generated by
`go run ./scripts/stdcorpus bcrypt` from scripts/stdcorpus/bcryptoracle). Needs `rand`.

```xo
const MIN_COST: Int = 4        // Go's MinCost
const MAX_COST: Int = 31       // Go's MaxCost
const DEFAULT_COST: Int = 10   // Go's DefaultCost
enum BcryptErr { Mismatch, HashTooShort, VersionTooNew(version: Int), InvalidPrefix(byte: Int),
                 InvalidCost(cost: Int), PasswordTooLong, Corrupt(msg: Str) }
pub fn hash(password: Bytes, rand: Rand, cost: Int = 10) -> Str ! BcryptErr uses rand  // GenerateFromPassword
pub fn compare(hashed: Str, password: Bytes) -> () ! BcryptErr                       // CompareHashAndPassword
pub fn cost(hashed: Str) -> Int ! BcryptErr                                          // Cost
```

- A hash is 60 characters: `$2a$`, the cost as two digits, `$`, a 22 character salt, a 31 character
  hash (bcrypt's own base 64 alphabet, no padding). `compare` also reads hashes of other systems in
  the `$2$` and `$2a$`/`$2b$`/`$2y$` forms Go reads (any minor letter; major version 3 and up is
  `VersionTooNew`).
- Only the first 72 bytes of a password count (bcrypt's key limit): `compare` ignores the rest, `hash`
  refuses a longer one with `PasswordTooLong`. A cost below MIN_COST is DEFAULT_COST (Go's rule),
  above MAX_COST `InvalidCost`. Each step of cost doubles the work: cost 10 is tens of milliseconds.
- `compare` takes the same time wherever the hashes differ. Its error texts are Go's
  (`crypto/bcrypt: hashedPassword is not the hash of the given password`, ...); `Corrupt` carries Go's
  strconv or base64 error text for a cost or salt that is not valid.
## 58. std/cron

Status: implemented on every backend (2026-11-03, decision 0474, Accepted by the owner 2026-11-03): `std/native/cron.xo` on
the native backends (a port of robfig/cron v3.0.1's parser.go and spec.go) and `std/rt/cron.go` on the Go backend
(robfig's code on Go's time), checked against robfig/cron itself on 400+ seeded specs, zones and start times
including daylight saving changes (`internal/stdtest/testdata/corpus/cron`, generated by `go run ./scripts/stdcorpus cron`
from scripts/stdcorpus/cronoracle).

```xo
struct CronErr { msg: Str }                  // displays robfig/cron's text, e.g. `expected exactly 5 fields, found 3: [* * *]`
fn parse(spec: Str, zone: time.Zone = time.utc()) -> Schedule ! CronErr
fn (s: Schedule) next(after: Time) -> Time?   // strictly after, whole seconds; None when none within 5 years
fn (s: Schedule) zone() -> time.Zone
```

- The spec is robfig's ParseStandard: five fields (minute 0-59, hour 0-23, day of month 1-31, month 1-12 or
  jan-dec, day of week 0-6 or sun-sat), each a comma list of `*`, `?`, a number or name, `a-b`, `a-b/n`, `*/n`,
  `a/n`; or `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`, `@midnight`, `@hourly`, `@every DURATION`
  (at least a second, rounded down to whole seconds); led by `TZ=Name ` or `CRON_TZ=Name ` for the schedule's zone
  (else `zone`, UTC by default; there is no "local" zone). Both day of month and day of week restricted: either
  matches; either starred: both must.
- `next` is robfig's SpecSchedule.Next loop for loop, so a time in a daylight saving gap or overlap, and midnight
  that does not exist (Sao Paulo), behave as in robfig, on Go's reading of such a civil time (the offset Go's
  `time.Date` meets first).
- The error text of an `@every` duration is Xo's `Duration.parse` text, not Go's `time.ParseDuration`.
- Go backend: a `TZ=` name needs the zone data in the program: name the zone with a constant
  `time.zone("Europe/Berlin")` somewhere, or build with `--tzdata=system` (the native backends embed the database
  because std/cron looks zones up by name).

## 59. std/cookiejar

Status: implemented on every backend (2026-11-03, decision 0475, Accepted by the owner 2026-11-03): `std/native/cookiejar.xo` on the native backends and, through `std/go_compiled.txt`, the Go backend; a port of go1.26.1's net/http/cookiejar (jar.go) with the clock a parameter, checked against the real package on 100 seeded sequences of SetCookies and Cookies (`internal/stdtest/testdata/corpus/cookiejar`, generated by `go run ./scripts/stdcorpus cookiejar`).

```xo
struct Jar {}                                  // a handle: copies share the cookies; tasks may use it at once
fn new() -> Jar
fn (j: Jar) set_cookies(url: http.Url, cookies: List[http.Cookie], now: Time)
fn (j: Jar) cookies(url: http.Url, now: Time) -> List[http.Cookie]    // name, value, quoted only
fn (j: Jar) clear()
```

- Cookies are kept per registrable domain key (the last two labels of the host; an IP address is its own key), by domain, path and name. `set_cookies` takes the Domain and Path attributes as RFC 6265 section 5.3 and Go: a Domain that is not the host or a parent of it is ignored, an IP host takes a Domain only if equal to it (then host only), a Max-Age below 0 or an Expires not after `now` deletes the cookie, Max-Age wins over Expires, a Path that does not start with `/` is the default path of the URL.
- `cookies` sends a cookie whose domain matches (a host cookie only its host), whose path matches (RFC 6265 section 5.1.4) and, if it is Secure, over https, or to `localhost` and loopback addresses over http (as Go 1.26 does). Cookies past their expiry are dropped. They come in Go's order: the greater path first (compared as strings), then the earlier created.
- A scheme other than http and https stores and sends nothing. A host that is not ASCII is ignored (there is no IDNA yet, decision 0476). No public suffix list is consulted (Go's default): a cookie for `com` or `co.uk` is accepted. `http.Url` holds the port apart; an IPv6 host keeps its brackets unless the Url has a port, as Go's `net/url` and `canonicalHost` do. The jar is not saved to a file and is not wired into `http.Client` (a later change).

## 60. std/norm

Status: implemented on every backend (2026-11-03, decision 0476, std/norm accepted by the owner 2026-11-03; std/idna waits): `std/native/norm.xo` on the native backends and, through `std/go_compiled.txt`, the Go backend. The tables (Unicode 15.0.0, the version of golang.org/x/text v0.43.0's unicode/norm that go1.26 builds) are generated into the native source by `scripts/gennorm`; the corpus (`internal/stdtest/testdata/corpus/norm`, `go run ./scripts/stdcorpus norm`) holds x/text's result for every rune a form changes and for 700 seeded strings of decomposable runes, combining marks, Hangul and ASCII.

```xo
enum Form { Nfc, Nfd, Nfkc, Nfkd }
fn unicode_version() -> Str                    // "15.0.0"
fn normalize(s: Str, f: Form) -> Str
fn is_normalized(s: Str, f: Form) -> Bool
```

- `normalize` decomposes every rune (canonically, or by compatibility for Nfkc and Nfkd; Hangul syllables by arithmetic), puts each run of combining marks in canonical order (stable by combining class) and, for Nfc and Nfkc, composes (the primary composites, none of the full composition exclusions; a mark blocked by one of an equal or higher class stays separate), as UAX #15. Pure ASCII is returned as it is.
- `is_normalized(s, f)` is `normalize(s, f) == s` (x/text's quick check avoids building the result; here it does not yet).
- The tables are 61 KB of read only byte records (decompositions as runs, combining classes, composition pairs and small block indexes) and importing the module adds 66 KB to a program, 34 percent less than x/text/unicode/norm does to a Go program (bench/results/std-idna-size-2026-11-03.md); a program that does not import it pays nothing. Not included: `Form.Append`, the streaming `Reader` and `Writer`, `Iter`, `FirstBoundary` and `Properties` (a later change if a user needs them), and IDNA (decision 0476, waits for the owner).


## 61. std/idna

Status: implemented on every backend (2026-11-03, decision 0476, std/idna accepted by the owner 2026-11-03): `std/native/idna.xo` on the native backends and, through `std/go_compiled.txt`, the Go backend. A port of golang.org/x/net v0.60.0's idna (idna.go, punycode.go) and of the Bidi rule of golang.org/x/text's secure/bidirule; the tables (Unicode 15.0.0, the same version as std/norm) are generated by `go run ./scripts/genidna NETDIR TEXTVERSION std/native/idna.xo` from x/net's own trie, one range per run of runes with equal category, joining type, attributes, Bidi class and mapping. The corpus (`internal/stdtest/testdata/corpus/idna`, `go run ./scripts/stdcorpus idna`) holds x/net's answer, with the error's status code, for about 1,000 names (hand picked, seeded random mixes of scripts, joiners, hyphens, dots and punycode labels, and x/net's punycode test vectors) on each of the four profiles, ToASCII and ToUnicode.

```xo
struct IdnaErr { code: Str, msg: Str }            // code: P1, V2, V3, C, B, A3, A4, ...
enum Profile { Lookup, Registration, Display, Punycode }
fn unicode_version() -> Str                       // "15.0.0"
fn to_ascii(domain: Str, p: Profile = Profile.Lookup) -> Str ! IdnaErr
fn to_unicode(domain: Str, p: Profile = Profile.Lookup) -> Str ! IdnaErr
```

- `Lookup` is `idna.Lookup` (UTS 46 mapping, STD3 rules, hyphen, joiner and Bidi checks, non transitional), `Display` is `idna.Display`, `Registration` is `idna.Registration` (no mapping, DNS length limits) and `Punycode` is `idna.Punycode` (labels only, what x/net's `idna.ToASCII` and `ToUnicode` use). The profile options (`Transitional`, `RemoveLeadingDots`, `BidiRule`, ...) are not exposed; the four profiles are x/net's own.
- `to_ascii` maps the name (`Bücher.DE` to `bucher.de`, `ß` stays), validates each label and turns each label that is not ASCII into `xn--` and its punycode; `to_unicode` decodes each `xn--` label and validates it. Both fail with the first status x/net reports (`msg` is its text, `code` the UTS 46 code); x/net also returns the partly converted name with the error, which is not kept here.
- A name x/net rejects is rejected for the same reason on the same input: the corpus compares the code, not only the failure.
- The tables are 52 KB of ranges (7,380, 7 bytes each, binary searched), 6.5 KB of mappings and 0.5 KB of infos; importing the module adds 167 KB to a program, 28 percent less than x/net/idna does to a Go program (bench/results/std-idna-size-2026-11-03.md). No domain name is looked up or resolved; `std/url` and `http` are unchanged: call `idna.to_ascii` on a host before building a `Url`.

## 62. std/jwt

Status: implemented on every backend (2026-11-03, decision 0486, Accepted by the owner 2026-11-03): one pure source,
`std/native/jwt.xo`, that the Go backend compiles (std/go_compiled.txt, decision 0312), on std/crypto and std/json.
JSON Web Signature compact tokens (RFC 7515, RFC 7519) with HS256/384/512, RS*, PS*, ES256/384/512 and EdDSA; no JWE
for now. Checked against github.com/golang-jwt/jwt/v5 v5.3.1 (`internal/stdtest/testdata/corpus/jwt`, generated by
`go run ./scripts/stdcorpus jwt` from scripts/stdcorpus/jwtoracle): the same tokens byte for byte for the algorithms
with deterministic signatures (except that a claim text holding `<`, `>` or `&` is written as it is, where Go's
encoding/json writes `\u003c`, `\u003e` and `\u0026`: both are valid JSON and verify alike), and the same error category for about 400 tokens and option sets.

```xo
enum Alg { HS256, HS384, HS512, RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, EdDSA }
enum Key { Hmac(secret: Secret[Bytes]), Public(key: crypto.PublicKey), Private(key: crypto.PrivateKey) }
struct Header { alg: Alg, kid: Str?, typ: Str?, extra: Map[Str, Str] = {}, x5c: List[Str] = [] }
struct Options { algs: List[Alg], now: Time, leeway: Duration = 0s, issuer: Str? = None, audience: Str? = None,
                 subject: Str? = None, require_exp: Bool = true, check_iat: Bool = false,
                 max_age: Duration? = None, max_size: Int = 16384 }
enum JwtErr { Malformed(msg: Str), Alg(msg: Str), Signature, Expired(at: Time), NotYetValid(at: Time), Issuer,
              Audience, Subject, Claim(msg: Str), Key(msg: Str) }
fn (e: JwtErr) display() -> Str
fn sign[T: Json](r: Rand, alg: Alg, key: Key, claims: T, kid: Str? = None, header: Map[Str, Str] = {}, x5c: List[Str] = []) -> Str ! JwtErr uses rand
fn verify_with[T: Json](token: Str, key: Key, opts: Options) -> T ! JwtErr
fn verify[T: Json](token: Str, keys: fn(Header) -> Key ! JwtErr, opts: Options) -> T ! JwtErr
fn peek_header(token: Str) -> Header ! JwtErr
fn jwks(set: Str) -> Map[Str, Key] ! JwtErr
```
`sign`'s `header` adds string fields to the protected header (`x5t#S256`, `jku`) and `x5c` the certificate chain
(standard base64 DER, leaf first); they are written after `alg`, `kid` and `typ`, ordered by name (`x5c` among them),
and a `header` name of `alg`, `kid`, `typ` or `crit` is `Malformed`. `Header.extra` and `Header.x5c` give the string
fields and the chain of a parsed token (`peek_header`, the `verify` key function); an `x5c` that is not a list of strings
is `Malformed`. A call without them writes the header it always did.

- There is no `none` algorithm and no default for `Options.algs`: a token whose `alg` is not in the list is
  `Alg`, and a key that does not fit the algorithm (an HMAC secret for RS256, an RSA key for HS256, a P-384
  key for ES256) is `Alg` too, so an RSA public key cannot be used as an HMAC secret.
- The order is: size (`max_size`), three segments, header (base64url without padding or line breaks, a JSON
  object, `crit` refused), `alg` allowed, key, signature, then the claims. No claim is looked at before the
  signature holds.
- Claims follow golang-jwt v5: `exp` is valid while `now < exp + leeway`, `nbf` while `now >= nbf - leeway`,
  `iat` only with `check_iat`; a missing `exp` is `Claim` unless `require_exp` is false; `aud` is a string or a
  list; `exp`, `nbf` and `iat` are numbers truncated to whole seconds. A claim of the wrong type is `Claim`.
- `sign` writes the header `{"alg":...,"kid":...,"typ":"JWT"}` (keys in that order, as golang-jwt) and the
  claims with `json.encode`; ECDSA signatures are the fixed-width `r || s` of RFC 7518 and RSA-PSS uses a salt
  as long as the hash.
- `jwks` reads an RFC 7517 key set (RSA, EC and OKP public keys) into keys by `kid`, skipping `use: enc`.

## 63. std/yaml

Status: implemented on every backend (2026-11-05, decision 0512, Accepted by the owner 2026-11-05): one pure source,
`std/native/yaml.xo` (about 5,000 lines), that the Go backend compiles (std/go_compiled.txt, decision 0312), a port of
gopkg.in/yaml.v3 v3.0.1 (reader, scanner, parser, composer, resolver, decoder, emitter, encoder), deviations included.
Checked against yaml.v3 (`internal/stdtest/testdata/corpus/yaml` and `yamlfuzz`, generated by `go run ./scripts/stdcorpus yaml`
and `yamlfuzz` from scripts/stdcorpus/yamloracle): the same values, the same error texts and the same Marshal bytes on 466
documents and 8,000 fuzz documents.

```xo
struct YamlErr { line: Int, msg: Str }
fn (e: YamlErr) display() -> Str
fn decode[T: Yaml](text: Str, strict: Bool = false) -> T ! YamlErr
fn decode_all[T: Yaml](text: Str, strict: Bool = false) -> List[T] ! YamlErr
fn encode_value[T: Yaml](v: T, indent: Int = 4) -> Str ! YamlErr
fn encode_all[T: Yaml](docs: List[T], indent: Int = 4) -> Str ! YamlErr
enum NodeKind { Document, Sequence, Mapping, Scalar, Alias }
struct Node { kind: NodeKind, style: Int, tag: Str, value: Str, anchor: Str,
              head_comment: Str, line_comment: Str, foot_comment: Str, alias: Node?,
              content: List[Node], line: Int, column: Int }
fn parse(text: Str) -> Node ! YamlErr
fn parse_all(text: Str) -> List[Node] ! YamlErr
fn encode_node(n: Node, indent: Int = 4) -> Str ! YamlErr
fn to_json(text: Str) -> Str ! YamlErr
```

- `derive Yaml` (structs only, no type parameters) maps Str, Int, Float, Bool, Duration, Bytes, Time, `time.Civil`, `yaml.Node`, an Option, a List, a Map[Str, V] of those, or a
  struct that derives Yaml (`XO0425` for another type). A field's key is its `yaml "name,options"` string, else its `as` string
  (shared with Json), else its name. Options are `omitempty` (also in the shared `as "name,omitempty"`), `flow` (only in
  `yaml "name,flow"`) and `inline` (a field that is a struct deriving Yaml, read and written in the enclosing mapping); the name
  `-` leaves the field out.
- Decoding follows yaml.v3: errors are collected as `unmarshal errors:` lines in document order, a null item of a List is
  dropped and a null Map value stores the zero value, explicit tags (`!!str`) are checked, a duplicate key in a mapping is an
  error at the later line, merge keys (`<<`) put the own entries first and a merged entry never replaces one, aliases are
  limited by yaml.v3's expansion ratio, nesting by 10000 levels, and only the first document is read by `decode`.
  `strict` refuses a key no field takes (`line N: field K not found in type T`, yaml.v3's KnownFields).
- Encoding follows yaml.v3's Marshal: keys of a Map sorted as yaml.v3 sorts them, strings quoted where yaml.v3 quotes them
  (`"n"`, `"yes"`, `"1"`), indent 4 by default (decision 0512) and 2 to 9 with `indent`.
- A `yaml.Node` field holds the node of its key as it is (an alias stays an Alias node, as in yaml.v3, which stops at the Node type
  before its alias handling), comments included, and is written back through `encode_node`'s rules; a field the document lacks is a
  null scalar (yaml.v3's zero Node has kind 0 and is written as null).
- A `Time` field decodes a timestamp scalar (yaml.v3's layouts `2001-12-14`, `2001-12-14 21:59:43.10`, `2001-12-14t21:59:43.10-05:00`)
  to its instant, which equals yaml.v3's `t.UnixNano()`; any other scalar must be RFC 3339 (`Time.UnmarshalText`), else the
  error `parsing time "x" as "2006-01-02T15:04:05Z07:00": cannot parse` (the rest of Go's layout error text is not
  reproduced), and a mapping decodes into nothing, as for yaml.v3's struct. A `Time` is an instant with no zone (decision
  0094) and spans 1677-09-21 to 2262-04-11, so a timestamp outside that range is `cannot unmarshal !!timestamp ... into
  time.Time`, and a `Time` is written as RFC 3339 in UTC (`2001-12-15T02:59:43.1Z`), where yaml.v3 keeps the zone it read. A
  `time.Civil` field keeps the offset (`+05:30`) and writes it back as yaml.v3 does. The zero of both is 1970-01-01T00:00:00Z
  (yaml.v3's is year 1), so a field the document lacks differs.
- A Duration is a string in `Duration.parse` syntax (`1h30m`), written as Go's Duration.String (`1h30m0s`); yaml.v3 refuses an
  int or a float there (`cannot unmarshal !!int `5` into time.Duration`), and so does Xo. Bytes is, as yaml.v3's `[]byte`, a
  sequence of ints 0 to 255 (a string or `!!binary` value is an error, `into []uint8`), written as such (`[]` when empty).
- `parse` and `parse_all` give yaml.v3's Node trees (a Document node holds the root; a scalar's `tag` is the resolved tag such
  as `!!int`; `style` has yaml.v3's bits `TAGGED`, `DOUBLE_QUOTED`, `SINGLE_QUOTED`, `LITERAL`, `FOLDED`, `FLOW`; `line` and
  `column` count from 1); the trees equal yaml.v3's on 466 documents. A Node carries its head, line and foot
  comments (`head_comment`, `line_comment`, `foot_comment`) as yaml.v3 associates them (the scanner's comment folding, the parser's
  stem and document comments, the composer's foot comment moves): the `#` stays in the text, the lines of one comment are joined
  by a line feed, an empty string is no comment. `alias` of an Alias node is the node it names.
  `parse_all` is all or nothing: on an error it returns the error, where yaml.v3's Decoder has returned the earlier documents.
- `encode_node` writes a Node as yaml.v3's Marshal of a `yaml.Node` does (a Document node as is, another node as a document
  of its own; a tag is left out when the text resolves to it, except `!!merge`; styles and anchors are kept); equal to yaml.v3 on
  all 466 documents (a text with no document, which yaml.v3 writes as null for a zero Node, is not compared), comments
  included: the head, line and foot comments of a Node are written where yaml.v3 writes them.
- `to_json` is the first document as JSON text, as yaml.v3 decoded into an `any` and written by encoding/json: keys sorted, aliases
  and merge keys resolved, an empty document `null`, an error for NaN and the infinities. Deviations: `<`, `>` and `&` are
  written as they are (Go writes `\u003c`, `\u003e`, `\u0026`; both are valid JSON), a timestamp is its text, and compared
  documents leave out non-string keys, integers above the int64 range and invalid UTF-8.
- Not yet (later stages): Time and Node fields, inline Maps.

## 64. std/amqp

AMQP 1.0 (ISO/IEC 19464) on a port of github.com/Azure/go-amqp v1.7.0 (MIT; decision 0521). Stage 1: the type
system and its encoding. Connections, sessions, links and the CBS helper follow in stages 2 to 4. Pure: no
capability, one source for every backend (std/go_compiled.txt).

```
enum Value { Null, Bool(v), UByte(v), UShort(v), UInt(v), ULong(v), Byte(v), Short(v), Int(v), Long(v),
             Float(v: Float32), Double(v: Float), Decimal32(bits), Decimal64(bits), Decimal128(bits: Bytes),
             Char(v: Rune), Timestamp(ms: Int), Uuid(bytes: Bytes), Binary(v: Bytes), Str(v: Str),
             Symbol(v: Str), List(items), Map(pairs: List[(Value, Value)]), Array(items),
             Described(descriptor: Value, value: Value) }
enum AmqpErr { Decode(msg: Str), Encode(msg: Str) }
pub fn encode(v: Value) -> Bytes ! AmqpErr
pub fn decode(b: Bytes) -> (Value, Int) ! AmqpErr   // the value at the start of b and the bytes it took
```

- `encode` writes go-amqp's constructors: `smalluint`/`uint0` and `smallulong`/`ulong0` for unsigned values below
  256, `smallint`/`smalllong` for signed values in -128 to 127, `str8`/`vbin8`/`sym8` up to 255 bytes and the
  32 bit forms above, `list0` for an empty list, always `list32` and `map32` otherwise (go-amqp never writes
  `list8` or `map8`). An array holds one element type and one constructor: the narrow form when every integer
  fits it (`arrayUint32` with all elements below 256 is `smalluint`), a variable width form by its longest
  element, an array of maps always with the `array32` header and `map32` bodies. An empty array, an array of
  mixed types, of nulls, of lists or of arrays, a uuid or decimal128 that is not 16 bytes, and invalid UTF-8
  are `Encode` errors.
- `decode` accepts every constructor of the specification (the narrow and wide forms, `list8`, `map8`, `array8`,
  zero width values, decimals, chars, described values), keeps a map's pairs in wire order and duplicate keys,
  and refuses with a `Decode` error, never a fault: an unknown constructor, a length past the data, a compound
  whose size or count disagrees with its elements, an odd map count, invalid UTF-8 in a string or symbol, a
  boolean other than 0 or 1, and nesting deeper than 64 levels. Deviations from go-amqp's `ReadAny`: it refuses
  decimals, chars and described values outside its typed composites, and decodes a symbol as a string; here they
  decode to their own `Value`s.
- The corpus (`internal/stdtest/testdata/corpus/amqp`) is go-amqp's encoder run on 148 values and its decoder's
  verdict on each, checked on every backend: Xo decodes each of go-amqp's encodings to the same value and
  encodes it back to the same bytes, and refuses the truncations go-amqp refuses.

### 64.1 Frames and performatives (stage 2a)

```
enum Role { Sender, Receiver }
enum SenderSettleMode { Unsettled, Settled, Mixed }
enum ReceiverSettleMode { First, Second }
struct WireError { condition: Str, description: Str = "", info: List[(Str, Value)] = [] }
enum DeliveryState { Received(section_number: UInt32, section_offset: UInt64), Accepted, Rejected(error: WireError?),
                     Released, Modified(delivery_failed: Bool, undeliverable_here: Bool, annotations: List[(Value, Value)]) }
struct Source { address, durable, expiry_policy, timeout, dynamic, dynamic_node_properties, distribution_mode,
                filter, default_outcome, outcomes, capabilities }                 // every field defaulted
struct Target { address, durable, expiry_policy, timeout, dynamic, dynamic_node_properties, capabilities }
struct Open, Begin, Attach, Flow, Transfer, Disposition, Detach, End, Close,
       SaslMechanisms, SaslInit, SaslChallenge, SaslResponse, SaslOutcome   // one field per performative field
enum FrameBody { Open(v), Begin(v), Attach(v), Flow(v), Transfer(v), Disposition(v), Detach(v), End(v), Close(v),
                 SaslMechanisms(v), SaslInit(v), SaslChallenge(v), SaslResponse(v), SaslOutcome(v), Empty }
struct Frame { channel: UInt16, body: FrameBody }
pub fn encode_frame(f: Frame) -> Bytes ! AmqpErr
pub fn decode_frame(b: Bytes, max_size: Int = 4294967295) -> (Frame, Int) ! AmqpErr
```

- A frame is the 8 byte header (size, data offset 2, type 0 for AMQP and 1 for SASL, channel) and a described
  list; a `Transfer`'s `payload` follows the list. `encode_frame` writes go-amqp's bytes: trailing null fields
  trimmed, an empty list as `list0`, defaults omitted (`max_frame_size` 2^32-1, `channel_max` 65535,
  `handle_max` 2^32-1, zero `idle_timeout_ms`), a receiver's `Attach` without `initial_delivery_count`, one
  symbol for a one element symbol list and an array otherwise.
- Fields that hold an unsigned wire value with a default (`durable`, `timeout`, `max_frame_size`, `channel_max`,
  `handle_max`, `initial_delivery_count`, `max_message_size`, `idle_timeout_ms`) are `Int` and must be in the
  wire type's range, or `encode_frame` returns an `Encode` error. (A `UInt32` field with a literal default would
  need an Int to UInt32 conversion in the default that the native backends lack, XO1201.)
- `decode_frame` refuses with a `Decode` error: a short header, a data offset below 2 or past the size, a size
  past the data or past `max_size`, an unknown frame type, a SASL frame with no body, a performative that does
  not belong to the frame type, an unknown performative or delivery state, and a field of the wrong type.
  Unknown extra fields past the performative's last are ignored.
- The corpus captures the frames go-amqp's client writes (SASL init, open, begin, attach as a sender and as a
  receiver, transfer, flow, disposition, detach, end, close) against a fake peer over `net.Pipe`; Xo decodes each
  and encodes it back to the same bytes.

### 64.2 The connection (stage 2b-i)

```
enum Sasl { None, Plain(user: Str, password: Str), Anonymous }
struct ConnOptions { container_id: Str = "", hostname: Str = "", max_frame_size: Int = 65536, channel_max: Int = 65535,
                     idle_timeout_ms: Int = 0, properties: List[(Str, Value)] = [], sasl: Sasl = Sasl.None,
                     tls_roots: tls.Roots? = None, tls_verify: tls.Verify = tls.Verify.Full }
struct Connection { st: Mutex[ConnState] }   // a handle (copies share the connection); peer_container_id(), peer_max_frame_size(), peer_channel_max(), peer_idle_timeout_ms(), peer_properties()
pub fn dial(net: Net, addr: Str, opts: ConnOptions = ConnOptions{}) -> Connection ! AmqpErr uses net.dial
fn (c: Connection) send(f: Frame) -> () ! AmqpErr uses net.dial
fn (c: Connection) recv() -> Frame ! AmqpErr uses net.dial
fn (c: Connection) begin(opts: SessionOptions = SessionOptions{}) -> Session ! AmqpErr uses net.dial
fn (c: Connection) end_session(s: Session, error: WireError? = None) -> () ! AmqpErr uses net.dial
fn (c: Connection) close(error: WireError? = None) -> () ! AmqpErr uses net.dial
fn (c: Connection) abort()
```

- `dial` takes `host:port`, `amqp://host[:port]` or `amqps://host[:port]` (ports 5672 and 5671; `[::1]:5671` for
  IPv6). `amqps` runs TLS first (`tls.client` with `tls_roots` and `tls_verify`). With a SASL layer it sends the
  `AMQP\x03\x01\x00\x00` header, reads the mechanisms, sends the init (`PLAIN` is `\0user\0password`, `ANONYMOUS`
  is empty) and needs an outcome with code 0 (`Auth(code, text)` otherwise; a mechanism the server does not offer
  is `Auth(-1, ...)`); then the `AMQP\x00\x01\x00\x00` header and the two Opens. The advertised idle timeout is
  half of `idle_timeout_ms`, as go-amqp does. `container_id` defaults to `xo-amqp`.
- The connection is a handle over a Mutex (the Go backend's go_compiled modules take no `var` receivers); every call holds the lock while it works, so concurrent tasks take turns. It is meant to be driven by one task: `recv` returns the next frame (queued ones first), `begin` and
  `end_session` read until their reply and queue frames for other channels. A peer Close or End with an error is
  `Peer(error)`; a frame past `max_frame_size` or out of turn is `Protocol`, a transport failure is `Net`.
  `close` sends Close and waits for the peer's Close (a peer error in it is `Peer`).
- Timeouts and heartbeats (stage 2b-ii; they need `ConnOptions.clock`, and every call then also `uses clock`): `timeout_ms` bounds the wait for the peer's next bytes inside one call (`Net("i/o timeout")`, the connection stays usable); the Open advertises `idle_timeout_ms / 2`, the connection fails with `Net("idle timeout: ...")` when the peer sends nothing for `idle_timeout_ms`, and a call that waits sends an empty frame at half the peer's idle timeout. `dial` also bounds the connect by `timeout_ms` (`Net("i/o timeout")`), and `receiver.receive(timeout_ms)` gives up after that many milliseconds in total without a delivery (`Net("i/o timeout")`; the link stays usable). `c.keepalive(g)` spawns a task in the task group `g` that does the same while no call is running (and reads and applies arriving frames); it ends when `g` is cancelled or the connection closes, and a failure it finds is returned by later calls. Without `keepalive` the calling task reads while it waits, holding the lock, so tasks take turns. With `keepalive` that task becomes the connection's reader (decision 0521, "Reader task"): it reads the transport without the lock, applies each frame under it, and wakes the calls waiting for a frame; each blocked call (`receive`, `send` waiting for credit, window or outcome, `new_sender`/`new_receiver`, link and session close, `begin`, `end_session`, `close`) waits on its own channel outside the lock with its own deadline (`receive(timeout_ms)`), so a receive that waits does not stop a send, a lock renewal or another receiver on the same connection. A send holds its session's send turn from its first frame to its last, so deliveries stay in order. `recv` (raw frames) is not available while the reader task runs. Calls stay cancellation points while they wait.

### 64.3 Messages (stage 2c)

```
struct Header { durable: Bool = false, priority: Int = 4, ttl_ms: Int = 0, first_acquirer: Bool = false, delivery_count: Int = 0 }
struct Properties { message_id: Value = Value.Null, user_id: Bytes = b"", to: Str? = None, subject: Str? = None, reply_to: Str? = None,
                    correlation_id: Value = Value.Null, content_type: Str? = None, content_encoding: Str? = None,
                    absolute_expiry_ms: Int? = None, creation_ms: Int? = None, group_id: Str? = None,
                    group_sequence: Int? = None, reply_to_group_id: Str? = None }
enum Body { Empty, Data(sections: List[Bytes]), Sequence(sections: List[List[Value]]), Value(v: Value) }
struct Message { header: Header? = None, delivery_annotations: List[(Value, Value)]? = None, annotations: List[(Value, Value)]? = None,
                 properties: Properties? = None, app_properties: List[(Str, Value)]? = None, body: Body = Body.Empty,
                 footer: List[(Value, Value)]? = None }
pub fn encode_message(m: Message) -> Bytes ! AmqpErr
pub fn decode_message(b: Bytes) -> Message ! AmqpErr
```

- `encode_message` writes go-amqp's `Message.MarshalBinary` bytes: the sections in the order header, delivery
  annotations, annotations, properties, application properties, body, footer; a `None` section is not written and
  a `Some` one is, even when empty; a header always has its durable field, and omits priority 4, zero ttl and
  delivery count and a false first-acquirer; a properties section omits absent fields and trailing nulls.
- `decode_message` accepts the sections in any order and refuses a repeated header, properties or body kind mix, a
  section that is not described, and an unknown section code.
- The corpus (`MESSAGES`) is go-amqp's encoder on 22 messages and its decoder's values; Xo decodes each to the same
  message and encodes it back to the same bytes.

### 64.4 Links (stage 3)

```
struct SenderOptions { name: Str = "", settle_mode: SenderSettleMode? = None, source: Str = "", max_message_size: Int = 0,
                       properties: List[(Str, Value)] = [], capabilities: List[Str] = [], target_capabilities: List[Str] = [],
                       desired_capabilities: List[Str] = [] }
struct ReceiverOptions { name: Str = "", credit: Int = 1, settle_mode: ReceiverSettleMode? = None, target: Str = "",
                         filters: List[(Str, Value)] = [], max_message_size: Int = 0, properties: List[(Str, Value)] = [],
                         capabilities: List[Str] = [], source_capabilities: List[Str] = [], desired_capabilities: List[Str] = [] }
struct Sender { conn: Connection, id: Int, name: Str, target: Str }       // handles over the connection
struct Receiver { conn: Connection, id: Int, name: Str, source: Str }
struct Delivery { message: Message, id: Int, tag: Bytes, settled: Bool, link: Int }
fn (c: Connection) new_sender(s: Session, target: Str, opts: SenderOptions = SenderOptions{}) -> Sender ! AmqpErr uses net.dial
fn (c: Connection) new_receiver(s: Session, source: Str, opts: ReceiverOptions = ReceiverOptions{}) -> Receiver ! AmqpErr uses net.dial
fn (x: Sender) send(m: Message, settled: Bool = false) -> DeliveryState? ! AmqpErr uses net.dial
fn (x: Sender) close(error: WireError? = None) -> () ! AmqpErr uses net.dial
fn (r: Receiver) receive() -> Delivery ! AmqpErr uses net.dial
fn (r: Receiver) accept(d: Delivery) / reject(d, error) / release(d) / modify(d, delivery_failed, undeliverable_here)
fn (r: Receiver) close(error: WireError? = None) -> () ! AmqpErr uses net.dial
```

- Links share the connection's single read loop: any call that waits (credit, window, an outcome, a delivery) reads frames
  and applies them to every session and link, so a sender and a receiver can be used from one task in turn. A background
  reader task is the optional keeper of 64.2.
- `send` waits for link credit (the peer's Flow: delivery-count + link-credit - ours) and the session's remote incoming
  window, splits the message into transfer frames no larger than the peer's `max_frame_size` (the delivery id and tag only
  on the first, `more` on all but the last), counts the window per frame, and returns the peer's terminal outcome
  (`Accepted`, `Rejected`, `Released`, `Modified`) once its Disposition settles the delivery; `settled: true` sends
  pre-settled and returns `None`. A message larger than the peer's `max_message_size` is an `Encode` error.
- `receive` tops the credit up to `ReceiverOptions.credit` (a Flow) when it falls to half, assembles multi-frame
  deliveries, and returns a `Delivery`; an aborted delivery is dropped; `accept`, `reject`, `release` and `modify` send a
  settling Disposition (nothing for a delivery the sender pre-settled). The session's incoming window is restored with a
  Flow when it falls below half.
- Capabilities: a sender's `capabilities` are its Source's and `target_capabilities` its Target's; a receiver's
  `capabilities` are its Target's and `source_capabilities` its Source's; `desired_capabilities` (either) is the Attach
  frame's desired-capabilities, the link's own (Event Hubs' `com.microsoft:georeplication`).
- `send(m, settled, message_format)`: `message_format` is the Transfer's message-format (0 for an AMQP message; Azure
  Service Bus batches are one transfer of format 0x80013700, 2147563264, whose data sections are encoded messages).
- Manual credit: `ReceiverOptions.credit: 0` grants nothing and `receive` never tops up; `r.issue_credit(n)` raises the link
  credit by `n` (a Flow); `r.drain(timeout_ms)` sends a Flow with `drain` set, waits for the peer's answering Flow and returns
  every delivery that arrived before it, leaving the link with no credit (Go's ReceiveMessages: credit, collect, drain).
- Settle mode Second (`ReceiverOptions.settle_mode`): `accept`, `reject`, `release` and `modify` send the outcome unsettled
  and wait for the peer's settling Disposition; a Rejected state with an error is returned as `Peer(error)` (Service Bus's
  `com.microsoft:message-lock-lost`). `modify(d, delivery_failed, undeliverable_here, annotations)` carries message
  annotations (symbol keys), Go's properties to modify.
- `remote_attach()` on a `Sender` or `Receiver` is the peer's Attach for the link (None before it arrives): its source
  filters (Service Bus echoes `com.microsoft:session-filter` with the chosen session), target and properties
  (`com.microsoft:locked-until-utc`).
- A peer Detach (with an error: `Peer(error)`) or End makes later calls on the link fail; `close` sends Detach and waits
  for the peer's.

### 64.5 Claims based security (stage 4)

```
fn cbs_put_token(c: Connection, s: Session, audience: Str, token: Str, token_type: Str = "servicebus.windows.net:sastoken", expiry_ms: Int = 0) -> () ! AmqpErr uses net.dial
```

Azure's token authorization: a request message (`operation: put-token`, `type`, `name: audience`, optional
`expiration`, the token as the body, `to: $cbs`, `reply-to: cbs`) sent on a sender to `$cbs`, the answer read on a receiver
from `$cbs` with target `cbs`; a `status-code` outside 200 to 299 is `Peer` with the condition
`amqp:unauthorized-access`. The test is an in-process peer; the smoke test against a real namespace is run by the owner
only (decision 0521).

### 64.6 The test broker (std/amqptest)

```
use std/amqptest
fn amqptest.new(clock: Clock) -> Broker
fn (b: Broker) serve(g: task.Group, l: Listener) uses (net.listen, clock)
fn (b: Broker) queue(name: Str, opts: QueueOptions = QueueOptions{})      // QueueOptions{sessions, lock_ms, max_delivery_count}
fn (b: Broker) topic(name: Str)
fn (b: Broker) event_hub(name: Str, partitions: Int, consumer_groups: List[Str] = ["$Default"])
fn (b: Broker) event_hub_geo_factor(hub: Str, factor: Int)
fn (b: Broker) events(hub: Str, partition: Int) -> List[amqp.Message]
fn (b: Broker) subscription(topic: Str, name: Str, opts: QueueOptions = QueueOptions{})
fn (b: Broker) put(queue: Str, m: amqp.Message)
fn (b: Broker) ready(queue: Str) -> List[amqp.Message]
fn (b: Broker) locked(queue: Str) -> Int
fn (b: Broker) dead_letters(queue: Str) -> List[amqp.Message]
fn (b: Broker) scheduled(queue: Str) -> List[amqp.Message]
fn (b: Broker) deferred(queue: Str) -> List[amqp.Message]
fn (b: Broker) tokens() -> List[(Str, Str)]
fn (b: Broker) attaches() -> List[(Str, Str, List[Str])]
fn (b: Broker) reject_token(token: Str)
fn (b: Broker) require_token(on: Bool)
fn (b: Broker) session_state(queue: Str, session: Str) -> Bytes
fn (b: Broker) fail_next(op: Str, status: Int, condition: Str, description: Str)
fn (b: Broker) fail(op: Str, status: Int, condition: Str, description: Str)
fn (b: Broker) clear_failures()
fn (b: Broker) slow_management(ms: Int)
fn (b: Broker) detach_links(queue: Str, error: amqp.WireError)
fn (b: Broker) disconnect_all(error: amqp.WireError? = None)
```

An in-process AMQP 1.0 broker that behaves like Azure Service Bus for what a client library uses, for tests
(decision 0521; Go's `net/http/httptest` is the model). `serve` accepts connections on any `Listener` (the test world's
in memory listeners, or a loopback one) and serves each in a task of `g`; clients dial it with `amqp.dial`. It speaks
SASL (any mechanism is accepted) or none.

- Entities are queues, topics and subscriptions (`topic/Subscriptions/name`); a link to an undeclared entity is refused with
  `amqp:not-found`, a send to a topic is copied to every subscription, and `put` creates a queue it does not know.
- `$cbs`: a `put-token` request is answered with `status-code` 202 (401 and `amqp:unauthorized-access` for a token given to
  `reject_token`); a later put-token for the same audience replaces the earlier one (the client's refresh). `tokens()` lists
  the accepted `(audience, token)` pairs. With `require_token(true)` a link to an entity is refused with
  `amqp:unauthorized-access` unless an accepted, unexpired token's audience ends with the entity's name.
- Sending: a transfer is stored (sequence number and enqueue time are added as `x-opt-sequence-number` and
  `x-opt-enqueued-time`) and answered `Accepted` unless settled by the sender; a session enabled queue refuses a message
  without a session id (`properties.group_id`).
- Receiving is peek lock: a delivery is locked for `lock_ms`, its delivery tag is the 16 byte lock token (also
  `x-opt-lock-token`, with `x-opt-locked-until`), and the header's delivery count is the number of earlier deliveries.
  `Accepted` completes; `Released` and `Modified` give the message back (`Modified` with `undeliverable-here` defers it);
  `Rejected` dead letters it; past `max_delivery_count` a message is dead lettered with `MaxDeliveryCountExceeded`. A
  disposition for a lock that ran out or is not the connection's is answered `Rejected` with
  `com.microsoft:message-lock-lost`. A receiver that asks for settled transfers (`ReceiverOptions.sender_settle_mode`)
  deletes on receive. Both receiver settle modes are answered. `<queue>/$DeadLetterQueue` delivers the dead letters
  (removing them as it sends).
- `<entity>/$management` answers (reply `statusCode`, `statusDescription`, `errorCondition`, correlation id = the request's
  message id) `renew-lock`, `peek-message`, `schedule-message` (the message carries `x-opt-scheduled-enqueue-time`),
  `cancel-scheduled-message`, `receive-by-sequence-number` (deferred messages), `update-disposition`
  (`completed`, `abandoned`, `suspended`, `defered`), `renew-session-lock`, `get-session-state`, `set-session-state` and `get-message-sessions`; any
  other operation is 501.
- Sessions: a receiver on a session enabled queue gives the `com.microsoft:session-filter` (a session id, or null for the
  next available one); the broker locks that session (or the first with a deliverable message) to the connection, echoes
  the chosen id in the attach reply's filter and adds `com.microsoft:locked-until-utc` to the reply's properties; a session
  locked by another connection, or none available, is refused with `com.microsoft:timeout`.
- Batches and drain: a transfer with message format 0x80013700 is a batch, each data section of its body one encoded message,
  and all are stored under one `Accepted`. A flow with credit makes the broker send up to the credit; a flow with `drain` set
  gets what is deliverable, then a flow from the link with `drain` and `link-credit` 0 whose delivery count has advanced by
  the credit left.
- Event Hubs: `event_hub(name, partitions, groups)` declares a hub whose partitions are "0", "1", .... A send to `hub` picks a
  partition by `x-opt-partition-key` (a fixed hash), else round robin; a send to `hub/Partitions/id` goes to that one. Each
  stored event gets the annotations `x-opt-sequence-number` (Long, from 0 per partition), `x-opt-offset` (Str, increasing) and
  `x-opt-enqueued-time` (Timestamp); the partition key stays. A receiver at `hub/ConsumerGroups/cg/Partitions/id` has its own
  position and gets pre-settled deliveries (reading removes nothing), starting where its `apache.org:selector-filter:string`
  says (`amqp.annotation.x-opt-offset > '-1'` or `'@latest'` or `'N'`, `>=`, `x-opt-sequence-number`, `x-opt-enqueued-time`);
  events sent later keep flowing. The attach property `com.microsoft:epoch` makes a receiver the owner: a higher epoch
  detaches the holders with a lower one with `amqp:link:stolen`, and a lower epoch (or none) against a higher holder is refused
  with `amqp:link:stolen`. An unknown partition is `com.microsoft:argument-out-of-range`, an unknown hub or group
  `amqp:not-found`. The namespace `$management` link answers operation READ with `type` `com.microsoft:eventhub`
  (name, created_at, partition_count, partition_ids, georeplication_factor: a Long, 1 unless `event_hub_geo_factor` set
  it; above 1 means geo-replication, as Event Hubs v2 reads it) and `com.microsoft:partition` (name, partition, begin_sequence_number,
  last_enqueued_sequence_number, last_enqueued_offset, last_enqueued_time_utc, is_partition_empty), reply keys `status-code`
  and `status-description`. `events` lists a partition's events; `detach_links` takes a hub name or a receiver path.
- `attaches` lists every link a client attached, in order: its address (a sender's target, a receiver's source), the
  client's role (`"sender"` or `"receiver"`) and the Attach's desired capabilities (`com.microsoft:georeplication`).
- Faults: `fail_next`/`fail` make the operation `attach`, `transfer`, `disposition`, `put-token` or a management operation
  (its name without `com.microsoft:`) fail with `condition`, `status` and `description` (an attach, transfer or
  disposition is refused or rejected with the condition; a management reply carries `status` and the condition);
  `slow_management(ms)` delays management replies; `detach_links` detaches every link to an entity with an error;
  `disconnect_all` closes every connection, with the error in a Close frame when given. Both take effect before any frame a
  client sends after the call returns is handled.
- Time: locks, scheduled messages, session locks and token expiry use the clock given to `new`; an expired lock is noticed
  when the next operation looks at the message, not by a timer.

The client side of a refused attach: `new_sender` and `new_receiver` return the link's `Peer` error when the answering
Attach has no target (sender) or no source (receiver) and a Detach with an error follows (decision 0521).

## 65. std/protobuf

The Protocol Buffers wire format and the runtime of `derive Proto` (decision 0530; core 2.9), with Go's
`google.golang.org/protobuf` (protowire, proto) as the reference. Pure.

```xo
enum ProtoErr { Truncated, Overflow, BadTag, BadWire(wire: Int), EndGroup, Recursion, BadLength, Utf8 }
fn marshal[T: Proto](v: T) -> Bytes                     // rewritten to the generated proto_marshal__T
fn unmarshal[T: Proto](data: Bytes) -> T ! ProtoErr     // as proto.Unmarshal: scalars last wins, messages merge
fn append_varint(b: Bytes, v: UInt64) -> Bytes          // the wire functions of protowire, by the same names
fn append_tag(b: Bytes, num: Int, wire: Int) -> Bytes
fn append_fixed32(b: Bytes, v: UInt32) -> Bytes
fn append_fixed64(b: Bytes, v: UInt64) -> Bytes
fn append_bytes(b: Bytes, data: Bytes) -> Bytes
fn size_varint(v: UInt64) -> Int
fn encode_zigzag(v: Int) -> UInt64
fn decode_zigzag(v: UInt64) -> Int
fn read_varint(b: Bytes, at: Int) -> (UInt64, Int) ! ProtoErr
fn read_tag(b: Bytes, at: Int) -> (Int, Int, Int) ! ProtoErr
fn read_fixed32(b: Bytes, at: Int) -> (UInt32, Int) ! ProtoErr
fn read_fixed64(b: Bytes, at: Int) -> (UInt64, Int) ! ProtoErr
fn read_bytes(b: Bytes, at: Int) -> (Bytes, Int) ! ProtoErr
fn skip_field(b: Bytes, at: Int, num: Int, wire: Int) -> Int ! ProtoErr   // groups to MAX_GROUP_DEPTH
```

- Reads return the value and the offset after it. A field number above 536870911 is `BadTag` in the generated
  reader; `read_tag` itself accepts what protowire accepts.
- A struct with `derive Proto` writes its fields in field number order, then oneofs, then unknown fields. A plain
  nested struct is always written (use an Option for presence); a scalar equal to its zero value is omitted.
- Not yet: proto2, well known types (Timestamp, Duration, Any, Struct), JSON mapping, the `.proto` generator.

## 66. std/sqlite

A SQLite driver for std/sql's `open_driver` (decision 0534) over the SQLite library (3.54.1, public domain),
compiled from the amalgamation vendored in std/native/sqlite and linked through `use c`: native backends only
(`XO1201` on the Go backend; wasm has no C). Stage 1: the driver on a checkout's C compiler; the release's
prebuilt object per target follows.

```
struct SqliteDriver {}
struct Config { path: Str, uri: Bool, keys: List[Str], busy_ms: Int, pragmas: List[Str] }
fn driver(fs: Fs) -> sql.Driver[fs]                     // performs fs (decision 0534)
fn config(dsn: Str) -> Config ! sql.SqlErr               // the DSN's parts, as connect reads them
fn version() -> Str uses net.dial                       // "3.54.1"
```

```
let db = sql.open_driver(os.net, sqlite.driver(os.fs), "file:app.db?_busy_timeout=5000&_journal_mode=wal")?
```

- DSN as mattn/go-sqlite3 reads it: `:memory:`, a path, or `file:PATH?k=v&...`. SQLite's own URI keys
  (`mode=ro|rw|rwc|memory`, `cache=shared|private`) pass to SQLite; the driver's keys are `_busy_timeout`
  (`_timeout`, milliseconds, default 5000), `_journal_mode` (`_journal`: DELETE, TRUNCATE, PERSIST, MEMORY,
  WAL, OFF), `_foreign_keys` (`_fk`) and `_synchronous` (`_sync`), run as PRAGMAs after the open. An unknown
  `_` key or a bad value is `Param` (mattn ignores an unknown key).
- The file is opened through the driver's Fs: a sub Fs (`Fs.sub`) roots the path, and a path that is absolute
  or holds `..` is `Conn`; a read only Fs opens it read only; a write only Fs is `Conn`. SQLite keeps its
  journal and WAL files beside the database.
- Placeholders bind by position: `$1` and `?1` name argument 1, a bare `?` is its position among the
  placeholders; a named one (`:a`, `@a`) is `Param`, as is a count that does not match the arguments.
- Values: an INTEGER column's value is `Integer`, REAL `Real`, TEXT `Text` (invalid UTF-8 replaced by U+FFFD),
  BLOB `Blob`, NULL `Null`. `Flag` binds as 0 or 1, `At` as RFC 3339 text, `JsonText` as text; `Array` is
  `Param`.
- Errors: a UNIQUE or PRIMARY KEY violation is `Constraint(code: "23505")`, FOREIGN KEY "23503", NOT NULL
  "23502", CHECK "23514" (the memory driver's and Postgres's codes, with SQLite's message); a missing table is
  `NoTable`, a parse error `Syntax`, anything else `Server(code: the extended result code's name, such as
  "SQLITE_BUSY", msg)`. After an I/O error or a corrupt file the connection is dropped by the pool.
- A text of several statements runs them in order; arguments bind to the last, whose rows and changes are
  the result. `exec`'s `last_insert_id` is `sqlite3_last_insert_rowid`.
- Transactions are SQLite's (BEGIN, COMMIT, ROLLBACK; serializable); a read only transaction runs with
  `PRAGMA query_only` until it ends. `copy_from` runs one prepared INSERT per row.
- Effects: each call is a database call, `uses net.dial` as std/sql's Db names every driver's work; the files
  are reached through the Fs given to `driver` (decision 0534 records this for the owner).

## 67. std/timego

Go's time layouts (`2006-01-02T15:04:05Z07:00`) for code ported from Go (decision 0537); the core time API keeps
its named fields (decision 0087). Go's own layout scanner (`nextStdChunk`), formatter and parser run over core time's
civil functions, with Go's rules. Pure. Reference: Go's `time.Format` and `time.Parse`.

```xo
struct Layout { text: Str, ... }                          // a Go layout scanned once; use it in hot code
fn layout(go_layout: Str) -> Layout                       // every string is a layout: text that is no element is literal
fn (l: Layout) format(t: Time, z: time.Zone) -> Str
fn (l: Layout) parse(s: Str, z: time.Zone) -> Time ! time.ParseErr
fn format(t: Time, z: time.Zone, go_layout: Str) -> Str
fn parse(s: Str, go_layout: Str, z: time.Zone) -> Time ! time.ParseErr
const ANSIC, UNIX_DATE, RUBY_DATE, RFC822, RFC822Z, RFC850, RFC1123, RFC1123Z, RFC3339, RFC3339_NANO, KITCHEN,
      STAMP, STAMP_MILLI, STAMP_MICRO, STAMP_NANO, DATE_TIME, DATE_ONLY, TIME_ONLY: Str
```

- Every element of Go's layout language: `2006 06 01 1 Jan January 02 2 _2 002 __2 Mon Monday 15 03 3 04 4 05 5 PM pm
  MST Z07:00 Z0700 Z07 Z070000 Z07:00:00 -07:00 -0700 -07 -070000 -07:00:00`, and `.000`, `,000`, `.999` runs (a fraction
  is an element only when no digit follows it, as in Go).
- Format is Go's: `Z` for a zero offset in the `Z...` elements (`-07:00` prints `+00:00`), a zone without an abbreviation
  prints `-hhmm` for `MST`, `.999` drops trailing zeros and the separator when the fraction is zero.
- Parse is Go's `time.ParseInLocation` with `z` as the zone of a text without an offset: `15` and the other numeric
  fields accept one digit where Go does, the weekday text is not checked against the date, a fraction after the seconds is
  accepted when the layout has no fractional element, offsets accept hour 24 and minute and second 60 as Go does, a zone
  name is the zone's own offset when `z` had that abbreviation at that instant and otherwise a fixed offset (`GMT+3` reads
  its hour, any other unknown name is zero), `Z` is accepted for the `Z...` elements, the day of year and the month and
  day must agree. The result is an instant: the zone name or offset of the text is not kept.
- Differences from Go: a `Time` spans 1677 to 2262, so a text outside it, and a text without a year (Go reads year 0),
  is a `ParseErr`; a civil time in a daylight saving gap or overlap is the earlier one (Go leaves it unspecified); a
  zone name `z` does not know is never looked up among all of `z`'s zones, only at that instant; `ParseErr` text is
  core's, not Go's `parsing time ...` wording.
- `layout` plus `RFC3339` and `RFC3339_NANO` texts take Go's fast path in `parse`.

## 68. std/grpc

gRPC over std/http's HTTP/2 (decision 0535): the wire protocol of grpc-go, interoperable with it in both directions
(TestGrpcInterop). Messages are `Bytes`; a message type that derives `Proto` (std/protobuf) is marshalled by the caller,
or by the stubs `xo proto gen` writes. A server runs on `http.server` with HTTP/2 (`http.Protocol.UnencryptedHttp2`, or
TLS and ALPN `h2`); a client is an `http.Client`. Reference: grpc-go's wire behavior (PROTOCOL-HTTP2.md).

```xo
enum Code { Ok, Canceled, Unknown, InvalidArgument, DeadlineExceeded, NotFound, AlreadyExists, PermissionDenied,
            ResourceExhausted, FailedPrecondition, Aborted, OutOfRange, Unimplemented, Internal, Unavailable, DataLoss,
            Unauthenticated }
fn (c: Code) number() -> Int
fn code_from(n: Int) -> Code?
struct Status { code: Code, message: Str, details: Bytes }      // the error of every call; displays as grpc-go's text
// the wire pieces
fn frame(payload: Bytes, compressed: Bool = false) -> Bytes
fn unframe(data: Bytes, at: Int, max_len: Int = 4194304) -> (Bool, Bytes, Int) ! FrameErr
fn encode_timeout(ns: Int) -> Str
fn decode_timeout(s: Str) -> Int?
fn encode_message(s: Str) -> Str
fn decode_message(s: Str) -> Str
fn encode_message_bytes(b: Bytes) -> Str
fn decode_message_bytes(s: Str) -> Bytes
fn (c: Client) with_gzip() -> Client          // sends gzip compressed messages (`grpc-encoding: gzip`); compressed messages from the peer are always read; a server answers compressed when the request was
fn bin_encode(data: Bytes) -> Str            // a `-bin` metadata value: unpadded standard base64
fn bin_decode(value: Str) -> Bytes?          // padded or not; None when it is not base64
// the server
struct Call { method: Str, metadata: Map[Str, Str], timeout: Duration? }
fn server(clock: Clock) -> Server
fn (var s: Server) unary(method: Str, f: fn(Call, Bytes) -> Bytes ! Status)
fn (var s: Server) stream(method: Str, f: fn(Call, Recv, Send) -> () ! Status)
fn (s: Server) router() -> http.Router
fn (r: Recv) next() -> Bytes? ! Status
fn (s: Send) send(msg: Bytes) -> () ! Status
fn (s: Send) set_trailer(name: Str, value: Str)
// the client
fn client(c: http.Client, base: Str) -> Client
fn (c: Client) unary(method: Str, req: Bytes, metadata = {}, timeout: Duration? = None) -> Bytes ! Status
fn (c: Client) call[R](method: Str, metadata: Map[Str, Str], timeout: Duration?, f: fn(ClientCall) -> R ! Status) -> R ! Status
fn (c: ClientCall) send(msg: Bytes) -> () ! Status
fn (c: ClientCall) close_send()
fn (c: ClientCall) recv() -> Bytes? ! Status
```

- One call is one HTTP/2 stream: `application/grpc`, `te: trailers`, 5 byte frames, the status in the `grpc-status` and
  `grpc-message` trailers (percent encoded as grpc-go does, any byte), `grpc-timeout` as grpc-go encodes it (at most 8
  digits, the unit that keeps them). A method nobody registered, and a path no service has, answer `Unimplemented` in
  the trailers of a 200 response with grpc-go's texts; a request without `application/grpc` is 415.
- `unary` is `stream` with one message each way. A handler reads `Recv.next` until None (the client finished sending)
  and writes with `Send.send`; returning Ok ends the call with OK, an error with its status. Server streaming,
  client streaming and bidirectional calls differ only in how the handler and the `ClientCall` use them.
- A deadline: the client sends its `timeout` as `grpc-timeout` and stops waiting at it (`DeadlineExceeded`); the
  server cancels a handler still running at the deadline the client sent (the task unwinds at its next blocking
  call; `defer` blocks still run) and ends the call with `DeadlineExceeded`.
- A message above `max_recv` (4 MiB, `with_max_recv`) is `ResourceExhausted`; a bad frame, a stream cut inside a
  message, or a missing status is `Internal`; a connection that cannot be made is `Unavailable`; a non gRPC response
  maps its HTTP status to a code as grpc-go does.
- Not yet: compression (`grpc-encoding`), binary metadata (`-bin`), response headers and trailers other than the
  status, `grpc-status-details-bin`, the typed stubs of `xo proto gen`, a TLS example.

## 69. std/otlp

OTLP export of std/trace spans and std/metrics registries (decision 0539): the opentelemetry-proto 1.x messages
`ExportTraceServiceRequest` and `ExportMetricsServiceRequest` written straight onto std/protobuf's builder (no message
tree), the body of OTLP/HTTP's `POST /v1/traces` and `/v1/metrics` with `Content-Type: application/x-protobuf`. The bytes
are checked against protobuf-go marshaling the same messages (testdata/corpus/otlp).

```xo
struct Resource { service: Str, attributes: List[(Str, Str)] = [], scope: Str = "std/trace", version: Str = "" }
fn encode_traces(res: Resource, spans: List[trace.Span]) -> Bytes
fn encode_metrics(res: Resource, reg: metrics.Registry, start_ms: Int, now_ms: Int) -> Bytes
enum ExportErr { Transport(msg: Str), Rejected(status: Int, body: Str), GaveUp(status: Int, tries: Int) }
fn (e: ExportErr) display() -> Str
struct Exporter { client: http.Client, endpoint: Str, resource: Resource, headers: Map[Str, Str] = {}, backoff: backoff.Backoff = backoff.Backoff{max_tries: 5}, clock: Clock, rand: Rand }
fn exporter(client: http.Client, endpoint: Str, res: Resource, clock: Clock, rand: Rand) -> Exporter
fn (e: Exporter) with_headers(h: Map[Str, Str]) -> Exporter
fn (e: Exporter) with_backoff(b: backoff.Backoff) -> Exporter
fn (e: Exporter) send_traces(spans: List[trace.Span]) -> () ! ExportErr uses net.dial, clock, rand
fn (e: Exporter) send_metrics(reg: metrics.Registry, start_ms: Int) -> () ! ExportErr uses net.dial, clock, rand
```

`Exporter` posts `{endpoint}/v1/traces` and `/v1/metrics` as `application/x-protobuf` over OTLP/HTTP. 429, 502, 503 and 504 (and a
transport failure) are retried on the schedule of `backoff`, waiting at least the `Retry-After` seconds the collector asks for;
any other non 2xx status is `Rejected` and not retried, as the OTLP specification says; retries that run out are `GaveUp`.
`send_traces` of an empty list sends nothing. (The HTTP date form of `Retry-After` and partial success answers are not read yet.)

`encode_traces`: one ResourceSpans and one ScopeSpans. A span's trace id is its request id when that is 32 hex digits,
else the first 16 bytes of the SHA-256 of the id; the `request` span of an id is a server span (the id's root), its other
spans are internal spans whose parent is that root; span ids are the first 8 bytes of a SHA-256 of the span's request id,
name and start, so a retried export names the same spans. A `request` span's status becomes the attribute
`http.response.status_code` and the span status (500 and above is an error).

`encode_metrics`: a counter is a monotonic cumulative Sum, a gauge a Gauge, a histogram a cumulative Histogram (bucket
counts per bucket, with the explicit bounds), a summary a Summary with its count and sum; labels are string attributes.

## 70. std/protoparse

A parser for `.proto` files and a generator of `derive Proto` structs, for `xo proto gen` (decision 0530). Pure.

```xo
struct ParseErr { line: Int, col: Int, msg: Str }              // 1 based; displays as `line:col: msg`
fn tokens(src: Str) -> List[Token] ! ParseErr
fn parse(src: Str) -> File ! ParseErr
fn gen(f: File) -> Str ! ParseErr
// File { syntax, package, imports, messages: List[Message], enums: List[Enum], services: List[Service] }
// Message { name, fields: List[Field], oneofs: List[Str], nested: List[Message], enums: List[Enum] }
// Field { name, num, typ, key, repeated, optional, required, oneof }   Rpc { name, req, resp, client_stream, server_stream }
```

- The lexer reads identifiers (dots inside), integers (a `0x` prefix is lexed), floats, string literals (escapes decoded)
  and single symbol characters; `//` and `/* */` comments are dropped.
- `parse` reads proto3 and proto2: `package`, `import`, messages with fields, `repeated`, `optional`, `map<K, V>`,
  `oneof`, nested messages and enums, top level enums, `service` with `rpc` and `stream`. Options, `reserved`,
  `extensions`, field options (`default` included) and `extend` blocks are read and dropped. In a proto2 file every
  field outside a oneof needs a label (`required`, `optional` or `repeated`, else an error), and `required` and
  `group` are accepted: a group is a nested message plus a field of that type named in lower case. A proto3 file
  rejects `required` and `group`. `gen` maps `required` to a plain field and proto2 `optional` to an Option;
  proto2 repeated numeric fields are unpacked on the wire but `derive Proto` writes them packed (the reader takes
  both), so a generated proto2 message is not byte identical to protoc's output for those fields.
- `gen` writes Xo source: a nested type `Inner` of `Outer` is `OuterInner`, a oneof is an enum held in an optional
  field marked `proto "oneof"`, an enum is an open `Int` enum with a catch-all variant. Services are not generated
  here (the stubs of std/grpc come with `xo proto gen`).

## 71. std/htmltok

The HTML5 tokenizer of golang.org/x/net/html (decision 0548; token.go and the unescape of escape.go, BSD license): a text
in memory becomes a stream of tokens with Go's rules for raw text elements, comments, doctypes, attributes, entities
and the buffer limit. There is no tree builder. The token streams are checked against x/net v0.62.0's Tokenizer
(testdata/corpus/htmltok: curated and seeded random inputs in plain, AllowCDATA and fragment modes, and with buffer
limits). The entity tables are shared with std/html through `std/htmlent` (`entity_of(name)` and `entity2_of(name)`, "" for none;
internal, use std/html for escaping).

```xo
enum TokenType { Error, Text, StartTag, EndTag, SelfClosingTag, Comment, Doctype }
struct Attribute { key: Str, val: Str }
struct Token { typ: TokenType, data: Str, attrs: List[Attribute] }
struct Tokenizer {}
fn tokenizer(text: Str) -> Tokenizer
fn tokenizer_fragment(text: Str, context_tag: Str) -> Tokenizer
fn (var z: Tokenizer) next() -> TokenType
fn (var z: Tokenizer) token() -> Token
fn (z: Tokenizer) raw() -> Str
fn (z: Tokenizer) err() -> Str?
fn (var z: Tokenizer) allow_cdata(allow: Bool)
fn (var z: Tokenizer) next_is_not_raw_text()
fn (var z: Tokenizer) set_max_buf(n: Int)
fn (z: Tokenizer) buffered() -> Str
```

`next` is Go's Next: the type of the next token, `Error` at the end (`err` is `EOF`) or past the limit set by
`set_max_buf` (`max buffer exceeded`). `token` is Go's Token and consumes the token's data, so call it once per `next`:
a tag's `data` is its lower cased name (NUL as U+FFFD), a text, comment or doctype's is unescaped with newlines
converted; `attrs` keep the order, lower case the keys, unescape the values (a legacy reference without its semicolon
stays literal in a value when an `=` or a letter follows, as the HTML5 attribute rule says) and drop empty and duplicate
keys. `raw` is the unmodified text of the current token: the raw texts partition the input up to the Error token.
`script`, `style`, `textarea`, `title`, `iframe`, `noembed`, `noframes`, `noscript`, `plaintext` and `xmp` make the next
token raw text (RCDATA for `textarea` and `title`: entities are still read); `next_is_not_raw_text` and
`allow_cdata` are the caller's tools for foreign content, as in Go. Differences from Go: the input is a `Str` (valid
UTF-8, held in memory) rather than an `io.Reader`, so there is no read error, and `tag_name`, `tag_attr` and `text`
(the allocation free accessors) are not offered: `token` is the one accessor.

## 72. std/protodesc

The descriptors of protobuf's `google/protobuf/descriptor.proto` as Xo values (decision 0546 item 1): `FileDesc` with
`MessageDesc`, `FieldDesc`, `EnumDesc`, `EnumValueDesc`, `ServiceDesc`, `MethodDesc`, `OneofDesc` and the range types, the
`LABEL_*` and `TYPE_*` constants, and a hand written wire codec over std/protobuf (derive Proto does not expand in std,
decision 0549, which converts this module to generated code). Options messages are kept as their wire bytes and unknown
fields of a `FieldDesc` as theirs, so a descriptor made by protoc or protobuf-go decodes and encodes back to the same
bytes (checked on descriptor.proto, the well known types and grpc-go's health, reflection and test files: testdata/corpus/
protodesc, byte for byte against protobuf-go).

```xo
fn encode(f: FileDesc) -> Bytes
fn decode(data: Bytes) -> FileDesc ! protobuf.ProtoErr
```

`encode` writes fields in field number order, the order protobuf-go's Deterministic marshaling uses; a proto2 file has an
empty `syntax`. Unknown fields other than a `FieldDesc`'s are dropped.

## 73. std/protowkt

The well known types of `google/protobuf/*.proto` as Xo values with hand written codecs (decision 0546 item 2):
`Timestamp`, `Duration`, `Empty`, `FieldMask`, `Any`, the wrappers `DoubleValue`, `FloatValue`, `Int64Value`,
`UInt64Value`, `Int32Value`, `UInt32Value`, `BoolValue`, `StringValue`, `BytesValue`, and `Struct`, `Value`, `ListValue`
(`Value` is an enum: `Unset`, `Null`, `Number`, `Text`, `Flag`, `Object`, `Array`). Each type has
`encode_NAME(m) -> Bytes` and `decode_NAME(b) -> NAME ! protobuf.ProtoErr` (`encode_timestamp`, `decode_field_mask`,
`encode_value`, ...). Encoding is proto3: a zero scalar is not written, a `Value`'s member is, and a `Struct`'s entries
are written in key order (protobuf-go's Deterministic marshaling); decoding keeps the last value of a field and skips
unknown ones. Checked against protobuf-go's types marshaled for fixed and seeded random values (testdata/corpus/protowkt,
byte for byte, and the decoded values). Decision 0549 converts the module to generated code.

## 74. std/protojson

Protocol Buffers' canonical JSON mapping (decision 0546 item 2) as protobuf-go's `encoding/protojson`, driven by descriptors
(std/protodesc) instead of generated code: a message is converted between its wire bytes and compact JSON text.

```xo
struct JsonErr { text: Str }
struct Opts { proto_names: Bool = false, emit_defaults: Bool = false, enum_numbers: Bool = false, discard_unknown: Bool = false }
fn registry(files: List[protodesc.FileDesc]) -> Registry
fn registry_from_set(set: Bytes) -> Registry ! protobuf.ProtoErr
fn marshal(r: Registry, name: Str, wire: Bytes, o: Opts = Opts{}) -> Str ! JsonErr
fn unmarshal(r: Registry, name: Str, json: Str, o: Opts = Opts{}) -> Bytes ! JsonErr
```

`marshal` writes fields in descriptor order with lowerCamelCase `json_name`s (`proto_names` writes the proto names), 64 bit
integers as strings, bytes as standard base64, enums by name (`enum_numbers` writes numbers), `NaN` and `Infinity` as
strings, maps with sorted keys, only the set member of a oneof, and nothing for an unset or default field unless
`emit_defaults` (implicit presence scalars, `[]` and `{}`). `Timestamp` and `Duration` are strings, `FieldMask` one
comma separated string, `Struct`, `Value` and `ListValue` plain JSON, the wrappers their value, `Any` its message with a
leading `@type` (the type must be in the registry). `unmarshal` accepts both names, numbers or strings for integers (integer
valued forms such as `1e2` and `1.0`), base64 with or without padding in the standard or URL alphabet, enum names or
numbers, rejects unknown fields (skipped with `discard_unknown`), duplicate fields and a second member of a oneof, treats
`null` as unset (except for `Value` and `NullValue`), and writes the wire fields in protobuf-go's order (fields by number,
then oneofs), maps in key order. Checked against protobuf-go's `protojson` (testdata/corpus/protojson).
