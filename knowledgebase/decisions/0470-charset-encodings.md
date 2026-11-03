# 0470: std/charset, text encodings (UTF-16, Windows-1252, Latin-1) (proposal)

Status: Accepted (owner, 2026-11-03)

## Context

The 2026-10-30 backend survey (std-coverage.md, rank 4) found a CSV, TSV and XLSX import job blocked on
text encodings: Excel's "Unicode Text" export is UTF-16LE with a byte order mark, and legacy exports are
Windows-1252. Xo's `Str` is UTF-8 only; `Bytes.to_str` rejects other input and `read_text` is lossy. Go's
standard library has `unicode/utf16` (code unit conversion, no byte decoding) and leaves charsets to
golang.org/x/text (`encoding/unicode`, `encoding/charmap`, `encoding/htmlindex`), which is outside Go's
std. `unicode/utf16` is still a "planned" row in std-coverage.md (no user API needed so far).

## Options

1. `std/charset`, a decode and encode pair over a closed `Charset` enum:

   ```xo
   enum Charset { Utf8, Utf16Le, Utf16Be, Utf16Bom, Latin1, Windows1252 }
   struct CharsetErr { offset: Int, msg: Str }
   pub fn decode(data: Bytes, cs: Charset, lossy: Bool = false) -> Str ! CharsetErr
   pub fn encode(text: Str, cs: Charset) -> Bytes ! CharsetErr
   pub fn lookup(label: Str) -> Charset?     // WHATWG labels: "utf-16le", "latin1", "iso-8859-1", "cp1252"...
   pub fn utf16_units(text: Str) -> List[Int]        // Go's utf16.Encode
   pub fn from_utf16_units(units: List[Int]) -> Str  // Go's utf16.Decode (lone surrogates to U+FFFD)
   ```

   `Utf16Bom` decodes by the byte order mark (little endian when none, as x/text's UseBOM with a
   little endian default, which is Excel's case) and encodes little endian with a BOM. `Utf16Le` and
   `Utf16Be` take no BOM and keep one that is present (as x/text's IgnoreBOM). Decoding fails with the byte
   offset of the first invalid input (an odd trailing byte, a lone surrogate); `lossy: true` writes U+FFFD
   instead (x/text's replacement behavior). Windows-1252 follows the WHATWG table, where the five bytes
   Microsoft leaves unassigned (0x81, 0x8D, 0x8F, 0x90, 0x9D) decode to U+0081, U+008D, U+008F, U+0090,
   U+009D, and `lookup("iso-8859-1")` and `lookup("ascii")` return Windows-1252 as browsers do (Latin-1 is
   reachable by its own name only when asked for as `Latin1`). Encoding fails with the offset of a
   character the charset cannot hold.
   Cost: one std module, a native port (about 150 lines: two table lookups and the UTF-16 pairing), the Go
   backend over x/text's charmap tables (vendored in std/rt) or hand tables, spec section, std/changes,
   corpus from x/text's own tests and the WHATWG index, a timing row against Go.
2. A `Charset` as a trait with user implementations (more tables later): open, but every decode goes
   through a dynamic call, and Windows-1252 and UTF-16 cover the surveyed need.
3. Only `unicode/utf16` (Go's real std package): closes the coverage row but not the survey gap (it
   converts code units, not bytes).
4. Decode inside readers (`csv.reader(file, charset: ...)`, `File.read_text(charset)`): composes worse
   than a Bytes to Str function that every reader can take after `read`; can be added over option 1.

## Recommendation

Option 1, with option 4's reader parameters left for a follow up. It closes the unicode/utf16 row and the
survey's rank 4. More charsets (ISO-8859-x, Shift JIS, GBK) are table work for a later ADR once a service
needs them; the enum is the extension point (adding a variant is not breaking for callers that match with
a default).

## Consequences

New public API (hence this ADR). One new std module, `std/charset`; spec/std.md section 53; the enum and
function names are the commitment. The Go backend carries a table for Windows-1252 only. The lossy flag
makes the failure mode explicit rather than the `read_text` style silent loss.

## Alternatives

Listed above (2, 3, 4). Also rejected: putting the functions on `Bytes` and `Str` (`b.decode(cs)`), which
would grow the prelude for an occasional need; and a whole x/text port (htmlindex, transform chains), too
large for the need.
