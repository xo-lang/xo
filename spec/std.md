# Xo Standard Library and Prelude

Status: draft 0.8 (2026-10-09; the same draft number as `spec/core.md`, decision 0089)
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
`Bytes` does not (use `to_str()`). `Json` is implemented by `derive Json` and by
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
fn (s: Str) bytes() -> Bytes

fn (b: Bytes) len() -> Int                   // b[i] is the byte at i, a UInt8
fn (b: Bytes) is_empty() -> Bool             // `for x in b` yields each byte (core 3.5)
fn (b: Bytes) to_str() -> Str?               // None if not valid UTF-8
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
in its own array with a scratch under one list, where `sort` builds a
whole second list.

Higher order methods (`map`, `filter`, ...) pass through the callback's effects
and errors through effect variables (core 6.2); `map` with a failing callback
returns `List[U] ! E`.

`List[T].with_capacity(n)` (also `List.with_capacity(n)` where the type is
known) is an empty list, equal to `[]`, whose first `n` pushes append
without growing it; `n` below 0 counts as 0. Capacity is not observable
(decision 0170).

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
fn (s: Set[T]) to_list() -> List[T]
fn (s: Set[T]) is_empty() -> Bool
fn (s: Set[T]) any(f: fn(T) -> Bool) -> Bool   // also all
fn (s: Set[T]) filter(f: fn(T) -> Bool) -> Set[T]
fn (s: Set[T]) map[U](f: fn(T) -> U) -> Set[U]
fn ([var] s: Set[T]) add(x: T)
fn ([var] s: Set[T]) remove(x: T) -> Bool
```

A `Set` has no literal of its own: build one from a list, and an empty
one from an empty list whose element type the context gives
(`let seen: Set[Str] = [].to_set()`, `S{tags: [].to_set()}`); `[]`
alone is a `List`, not a `Set`.

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
fn (d: Duration) seconds() -> Float
fn (d: Duration) millis() -> Int             // truncated
fn (d: Duration) nanos() -> Int
fn (d: Duration) scale(f: Float) -> Duration // rounds to the nearest nanosecond
fn (d: Duration) ceil_seconds() -> Int
fn (d: Duration) abs() -> Duration           // durations may be negative

struct DurationErr { input: Str }  // displays as `invalid duration "1x": expected numbers with units ...`

Time.UNIX_EPOCH: Time
Time.from_unix(secs: Int) -> Time
Time.from_unix_millis(ms: Int) -> Time
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

Secret[T].new(v: T) -> Secret[T]
fn (s: Secret[T]) reveal() -> T
```

`BytesBuilder.put_le` and `put_be` (decision 0399) overwrite the `n` bytes
`at..at + n` of what has been written, in place (Go's
`binary.LittleEndian.PutUint32` on a slice): the low `n` bytes of `v`, lowest
first (`put_le`) or highest first (`put_be`), the rest of `v` dropped. `n` is
1, 2, 4 or 8 and the range must lie within `len()`; any other `n`, or a range
outside, is a fault. The length does not change and nothing is copied.

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

fn (f: Fs) sub(dir: Str) -> Fs                               // lazy: dir need not exist yet
fn (f: Fs) cwd() -> Fs                                       // sub capability at the working directory
fn (f: Fs) cwd_path() -> Str ! FsErr                         // the working directory's absolute path, from the OS
fn (f: Fs) read_only() -> Fs
fn (f: Fs) write_only() -> Fs                // reads fail with Denied
fn (f: Fs) case_sensitive() -> Bool                          // of the underlying filesystem
fn (f: Fs) stat(path: Str) -> FileInfo? ! FsErr  uses fs.read // None when missing
fn (f: Fs) exists(path: Str) -> Bool ! FsErr     uses fs.read
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

Handles, links, globs, and locks (decision 0163; the Go backend and
MemFs, and native builds on the real filesystem, decision 0172; a native
test binary's MemFs has all of them, as on the Go backend, since
2026-10-15):

```
fn (f: Fs) open(path: Str) -> File ! FsErr        uses fs.read   // read as a stream
fn (f: Fs) create(path: Str) -> File ! FsErr      uses fs.write  // write; created or truncated
fn (f: Fs) open_append(path: Str) -> File ! FsErr uses fs.write  // write at the end; created if missing
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
fn (f: File) write(data: Bytes) -> () ! FsErr     uses fs.write
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
  most 1 MiB), `None` at the end. Operations after `close` are `Io(path,
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

interface Stream {                                 // either kind of connection
  fn read(self, max: Int) -> Bytes? ! NetErr uses net
  fn write(self, data: Bytes) -> () ! NetErr uses net
  fn close(self)
}
```

A connection's reads and writes perform the effect of how it was made
(decision 0280): a `Conn` from `dial` is `net.dial`, a `ServerConn` from
`accept` is `net.listen`. So a client library declares `uses net.dial`
alone though it reads and writes, and a function given only a
`ServerConn` cannot dial. Both are handles with the same behavior;
`Stream` takes either, under `net`.

TCP connections (decision 0071): `accept` waits for the next connection;
`read` returns at most `max` bytes (at most 1 MiB; fewer when fewer are
there) and `None` at the end of the stream; `write` returns when every
byte is written. All three are cancellation points that report
cancellation as `Canceled` (core 7.2). `close` is idempotent and makes a
waiting `accept`, `read`, or `write` on the same listener or connection
fail with `Io`. A refused `dial` is `Refused`. `dial` gives up after 30
seconds with `Io` (`dial tcp ADDR: i/o timeout`); `dial_timeout(addr, d)`
gives up after `d`, over the lookup and the connect, with `Timeout`
(decision 0406).

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
struct ProcOutput { code: Int, stdout: Bytes, stderr: Bytes }
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
took over Interrupt shuts down the same way:

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
  peer_certificates: List[x509.Certificate]   // the client's certificates, leaf first: empty, the server asks for none
}
fn (r: Request) raw_query() -> Str            // Go's URL.RawQuery
fn (r: Request) escaped_path() -> Str         // Go's URL.EscapedPath
pub fn redirect(req: Request, url: Str, status: Int) -> Response   // Go's Redirect: Location resolved against the path, an HTML body on GET
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
    // a response with status 101 made by std/websocket's upgrade hands its
    // connection over: the server writes nothing itself (decision 0313)
fn (b: BodyReader) read(max: Int = 32768) -> Bytes? ! IoErr   // a handle_stream body's next bytes, None at its end
fn (b: BodyReader) read_all() -> Bytes ! IoErr                // the rest of it
fn (b: BodyReader) multipart(req: Request) -> PartReader ! multipart.MultipartErr   // the body's multipart/form-data parts as they arrive (decision 0310)
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
fn (s: Server) with_max_header_bytes(n: Int) -> Server           // Go's MaxHeaderBytes (plus 4096 of slack; 1 MB when 0): a longer head is 431
fn (s: Server) with_max_body(n: Int) -> Server                   // the body read before a handler (64 MB when 0): a longer one is 413
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
fn (r: Request) with_timeout(d: Duration) -> Request       // decision 0404
fn (r: Request) with_idle_timeout(d: Duration) -> Request  // decision 0404
fn (r: Request) with_body_reader(b: BodyReader) -> Request // decision 0404

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
struct TlsCert {}                                  // handle: a certificate chain with its private key
pub fn tls_cert(cert_pem: Bytes, key_pem: Bytes) -> TlsCert ! TlsErr
struct TlsErr { msg: Str }
fn (s: Server) with_tls(certs: List[TlsCert]) -> Server
fn (c: Client) with_roots(pem: Bytes) -> Client ! TlsErr
```

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
  Client certificates are not requested.
- HTTP/2 (decision 0121): over TLS a server offers `h2` and `http/1.1`
  by ALPN and prefers `h2`, and the client offers both; plain `http` is
  always HTTP/1.1 (no h2c). Nothing in the API changes, and every backend
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
  not yet valid: ...`), a server answering plain HTTP is `Protocol("...:
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
pub fn indent(text: Str, prefix: Str = "", indent: Str = "  ") -> Str ! DecodeErr      // Go's Indent over a text
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
| newtype (`type Email struct(Str)`) | its underlying value's JSON (core 2.8) |
| `T?` field | missing key or `null` decodes to `None`; `None` encodes as `null` |
| field with a default | missing key decodes to the default |
| other missing field | `DecodeErr` |
| `Patch[T]` field | missing key is `Absent`, `null` is `Null`, value is `Value(v)` |
| unknown keys | ignored |
| `Int`, `Float`, `Bool`, `Str` | number, number, boolean, string |
| `Int8`, `Int16`, `Int32`, `UInt8`, `UInt16`, `UInt32`, `UInt64`, `Float32` | number; decoding refuses a number out of the type's range (`expected integer` for a signed type, `expected non negative integer` for an unsigned one, `cannot unmarshal number 1e39 into Float32` for a Float32 past its largest value, Go's words with Xo's type), on every backend |
| `List[T]`, `Set[T]` | array |
| `Map[Str, V]` | object |
| enum unit variant | string `"Variant"` |
| enum variant with fields | object `{"Variant": {fields}}` |
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
fn (n: FakeNet) fail(pattern: Str, err: http.ClientErr)       // every matching request, until clear_failures
fn (n: FakeNet) fail_next(pattern: Str, err: http.ClientErr)  // the next matching request only
fn (n: FakeNet) clear_failures()
fn (n: FakeNet) requests() -> List[http.Request]     // every request seen, including failed ones
```

- `http.client(os.net)` calls registered handlers in process. Unhandled
  requests fail with `ClientErr.Connect("no fake handler for GET ...")`.
- `os.net.listen(":0")` gives an in memory listener, so a real `http.server`
  can be tested end to end with `http.client(os.net)`. Its `addr()` is
  `127.0.0.1:49152`, then `:49153`, ... A request to a listener whose
  `serve` has not started yet waits until it starts, like a connection in
  a real socket's backlog (a test that never serves it reports a deadlock
  naming the wait); after `serve` returns, requests fail with
  `ClientErr.Connect`.
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
fn (d: Db) begin() -> Tx ! SqlErr uses net.dial
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
interface Driver {
  fn connect(self, net: Net, dsn: Str) -> DriverConn ! SqlErr uses net.dial
}
interface DriverConn {
  fn query(self, q: Str, args: List[SqlValue]) -> DriverRows ! SqlErr uses net.dial
  fn exec(self, q: Str, args: List[SqlValue]) -> ExecResult ! SqlErr uses net.dial
  fn begin(self) -> () ! SqlErr uses net.dial      // also commit, rollback, ping
  fn close(self)
  fn is_broken(self) -> Bool                       // dropped from the pool when true
}
struct DriverRows { columns: List[Str], rows: List[List[SqlValue]] }

pub fn open_driver(net: Net, d: Driver, dsn: Str, max_open: Int = 10, max_idle: Int = 2,
                   max_lifetime: Duration = 30m) -> Db ! SqlErr uses net.dial
```

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
  intrinsic (std/native/llrt/cpu.xo `sha256_block`), not API.
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
pub fn generate_key(r: Rand, alg: KeyAlg) -> PrivateKey ! KeyErr uses rand
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
  `sign_pss` with another kind of key is `KeyErr`. `generate_key(r, Rsa)`
  is `KeyErr` until prime generation lands (import RSA keys from PEM).
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
  dropped, so `r`'s later values match. Keys from `generate_key`, RSA
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
zero), `{frac:3}` exactly three (1 to 9); `{offset}` `Z` or `+02:00`,
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
  default to 1970-01-01 and 00:00:00. A parsed weekday must match the
  date. Without `{offset}`, `{zone}` picks the instant whose abbreviation
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
               commands: List[Command] = [], version: Str = "") -> Command

pub fn bool_flag(name: Str, help: Str, short: Str = "", env: Str = "") -> Flag
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
pub fn valid_comment(r: Rune) -> Bool       // a usable `comment`, `'\0'` (none) included
```

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
```

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
  the archive, not an entry, and `untar` skips it (2026-10-12).
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
  between date and time and lower case `t` and `z` become `T` and `Z`).
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
  errors with their line.
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
  integer, and a map's keys come in its JSON order (Go sorts them);
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
struct TlsConn {}                                  // handle
fn client(c: Conn, host: Str, roots: Roots? = None, verify: Verify = Verify.Full, alpn: List[Str] = []) -> TlsConn ! NetErr uses net.dial
fn (t: TlsConn) read(max: Int) -> Bytes? ! NetErr  uses net.dial
fn (t: TlsConn) write(data: Bytes) -> () ! NetErr  uses net.dial
fn (t: TlsConn) close()
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
struct Addr { bits: Int, ip: Bytes, zone: Str } derive Ord   // 32, 128, or 0; 16 bytes; an IPv6 zone
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
  in a prefix). An IPv4 address is kept as `::ffff:a.b.c.d` in `ip`, with
  `bits` 32; `==` and the order are Go's (`Compare`: the bit length, the
  bytes, then the zone).
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
463 declarations.

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
```

### Int16

```
fn (static: Int16) try(x: Int) -> Int16?
```

### Int32

```
fn (static: Int32) try(x: Int) -> Int32?
```

### Int64

```
fn (static: Int64) try(x: Int) -> Int64?
```

### UInt8

```
fn (static: UInt8) try(x: Int) -> UInt8?
```

### UInt16

```
fn (static: UInt16) try(x: Int) -> UInt16?
```

### UInt32

```
fn (static: UInt32) try(x: Int) -> UInt32?
```

### UInt64

```
fn (static: UInt64) try(x: Int) -> UInt64?
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
fn (b: BytesBuilder) len() -> Int
fn (b: BytesBuilder) freeze() -> Bytes
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

### File

```
fn (f: File) read(max: Int) -> Bytes? ! FsErr uses fs.read
fn (f: File) read_line() -> Str? ! FsErr uses fs.read
fn (f: File) write(data: Bytes) -> () ! FsErr uses fs.write
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
```

### ServerConn

```
fn (c: ServerConn) read(max: Int) -> Bytes? ! NetErr uses net.listen
fn (c: ServerConn) write(data: Bytes) -> () ! NetErr uses net.listen
fn (c: ServerConn) close()
```

### Stream

```
fn read(self, max: Int) -> Bytes? ! NetErr uses net
fn write(self, data: Bytes) -> () ! NetErr uses net
fn close(self)
```

### Net

```
fn listen(self, addr: Str) -> Listener ! NetErr uses net.listen
fn dial(self, addr: Str) -> Conn ! NetErr uses net.dial
fn dial_timeout(self, addr: Str, d: Duration) -> Conn ! NetErr uses net.dial
fn lookup_host(self, host: Str) -> List[Str] ! NetErr uses net.dial
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
fn (c: Child) kill()
```

### Functions

```
fn fault(msg: Str) -> Never
fn dbg[T](x: T) -> T
fn create_dir_all_mode(fs: Fs, dir: Str, mode: Int) -> () ! FsErr uses fs
fn walk_dir[fx: Effects](fs: Fs, dir: Str, visit: fn(FileInfo) -> WalkStep uses fx) -> List[FileInfo] ! FsErr uses (fs.read, fx)
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
fn (c: WsConn) read() -> Message ! WsErr       // a whole message; pings answered, pongs skipped
fn (c: WsConn) write(m: Message) -> () ! WsErr // one frame
fn (c: WsConn) write_text(s: Str) -> () ! WsErr
fn (c: WsConn) ping(data: Bytes = b"") -> () ! WsErr
fn (c: WsConn) close(code: Int = 1000, reason: Str = "") -> () ! WsErr   // the close frame, then the connection
fn (c: WsConn) subprotocol() -> Str
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
- Not provided: permessage-deflate, a message written in pieces, ping
  timers (use `ping` and a clock), WebSockets over HTTP/2.


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
  the `XO_RTSTATS` counts, zero until they are on; `?on=1` turns them on.
  Go: goroutines, gomaxprocs, threads, heap_inuse_bytes, gc_count).
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
