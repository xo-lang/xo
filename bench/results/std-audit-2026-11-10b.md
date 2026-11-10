# Std function level audit, second pass, 2026-11-10

Follows std-audit-2026-11-10.md (same go.txt and xo.txt listings, origin/main of
that morning). Tool: scripts/stdaudit.

Commands: `go run ./scripts/stdaudit -report -go-file go.txt -xo-file xo.txt -map scripts/stdaudit/map.txt > rep.txt`;
`go run ./scripts/stdaudit -rank rep.txt -status gap DIRS...` over the pinned modules of
scripts/stdaudit/corpus.txt and over GOROOT/src (go1.26.1) without cmd, internal and vendor.

## What changed in the tool

- The three column "not planned" rows of std-coverage.md (reflect, unsafe; go/*;
  debug/*; runtime/*) were read with the status in the wrong column, so their
  ~3,000 symbols counted as unmapped; `debug/* (dwarf, elf, ...)` cells are now
  expanded too.
- go.txt listed 288 symbols twice (a package in two table rows, net/http
  server and client); `packages` lists each package once.
- map.txt takes wildcards: `pkg.*` and `pkg.Type.*`, applied only to a
  symbol no exact line, doc comment or seed names.
- A key mapped twice is an error (49 keys were, the last one silently
  winning); the first line of each was kept.
- `-rank -status gap` ranks the verified gaps (default: unmapped).

## Counts

| Status | First pass | Second pass |
|---|---|---|
| doc-named | 221 | 196 |
| seeded | 177 | 154 |
| mapped | 196 | 1217 |
| gap | 59 | 533 |
| skipped (not planned) | 2743 | 7853 |
| unmapped | 7708 | 876 |

Total 10829 (11117 before the duplicates were removed). The gap count includes
whole packages that std-coverage.md marks planned or internal only (crypto/hpke,
crypto/mlkem, encoding/asn1, net/textproto, hash/maphash, image/color, os/user):
each of their symbols is a gap row.

## Top 40 gaps by use

Files using the symbol, pinned modules / stdlib. Method counts are an upper
bound (the receiver's type is not resolved). Order: twice the module files
plus the stdlib files.

| Go symbol | Files (pinned modules / stdlib) | Gap |
|---|---|---|
| `unicode/utf8.RuneSelf` (const) | 0 / 36 | no RuneSelf const |
| `math.MaxUint16` (const) | 15 / 4 | see math.MaxInt32 |
| `io.Copy` (func) | 5 / 18 | no copy between two streams (Stream/File have read and write, no Copy helper; check std/prelude.Stream before filling) |
| `math.MaxInt32` (const) | 6 / 11 | only INT_MAX/INT_MIN (64 bit) exist; no MaxInt32/MaxInt16/MaxInt8/MaxUint16/MaxUint32 and Min* consts |
| `maps.Copy` (func) | 6 / 4 | no Map.extend |
| `math.Inf` (func) | 5 / 6 | no Float infinity value (Float.is_inf tests one) |
| `strings.NewReplacer` (func) | 5 / 5 | no Str replacer (one pass over several old/new pairs; Str.replace per pair) |
| `time.Time.Nanosecond` (method) | 6 / 3 (upper bound) | no Time.nanosecond; Time has unix and unix_millis only (check std/time) |
| `math/bits.Add64` (func) | 0 / 14 | Int.wrapping_add has no carry out (llrt cpu.adc is internal) |
| `math.MinInt32` (const) | 5 / 3 | see math.MaxInt32 |
| `time.Time.UnixNano` (method) | 3 / 7 (upper bound) | Time.unix and unix_millis only (check std/time) |
| `hash.Cloner` (type) | 0 / 12 | Hasher clone? check std/crypto.Hasher |
| `io.SeekStart` (const) | 0 / 12 | File.read_at/write_at exist; no seek cursor (check std/prelude.File) |
| `math/bits.Sub64` (func) | 0 / 12 | no borrow out (llrt cpu.borrow is internal) |
| `net/http.CrossOriginProtection.Handler` (method) | 6 / 0 (upper bound) | no cross origin protection middleware (Go 1.25 CrossOriginProtection) |
| `net/textproto.MIMEHeader.Get` (method) | 0 / 12 (upper bound) | header canonicalization lives inside std/http; no public textproto reader |
| `slices.Clone` (func) | 2 / 8 | no List.clone |
| `math/bits.Len` (func) | 1 / 9 | Int has leading_zeros; no bit length |
| `net/textproto.TrimString` (func) | 0 / 11 | header canonicalization lives inside std/http; no public textproto reader |
| `math.MaxInt16` (const) | 4 / 2 | see math.MaxInt32 |
| `math.MaxInt8` (const) | 4 / 2 | see math.MaxInt32 |
| `math.MaxUint32` (const) | 3 / 4 | see math.MaxInt32 |
| `math.MinInt16` (const) | 4 / 2 | see math.MaxInt32 |
| `maps.Clone` (func) | 1 / 7 | no Map.clone |
| `net/netip.AddrPort.IsValid` (method) | 3 / 3 (upper bound) | no AddrPort type (host and port are text in Net.dial) |
| `math.NaN` (func) | 2 / 4 | no Float NaN value (Float.is_nan tests one) |
| `math/big.Float.Int64` (method) | 3 / 2 (upper bound) | no big.Float (arbitrary precision floats) |
| `net/netip.Addr.Zone` (method) | 0 / 8 (upper bound) | no IPv6 zones |
| `net/netip.AddrPort.Addr` (method) | 2 / 4 (upper bound) | no AddrPort type (host and port are text in Net.dial) |
| `sort.Search` (func) | 1 / 6 | no List.binary_search |
| `unicode.MaxRune` (const) | 0 / 8 | no Rune.MAX constant (0x10FFFF); Rune.try rejects past it |
| `unicode.SimpleFold` (func) | 0 / 8 | no Rune.simple_fold (Str.equal_fold exists for strings) |
| `math.MaxFloat64` (const) | 2 / 3 | no Float limit consts |
| `math.MaxUint8` (const) | 3 / 1 | no small integer limit consts |
| `net.JoinHostPort` (func) | 1 / 5 | no join_host_port (IPv6 brackets by hand) |
| `net/textproto.CanonicalMIMEHeaderKey` (func) | 2 / 3 | header canonicalization lives inside std/http; no public textproto reader |
| `slices.Concat` (func) | 2 / 3 | no List.concat (use +) |
| `encoding/asn1.ObjectIdentifier` (type) | 0 / 6 | ASN.1 DER is inside std/native/llrt/asn1.xo (X.509), no public API |
| `image/color.Palette` (type) | 0 / 6 | std/image has the color conversions as functions but no Color model values per type (std-coverage.md partial, 0578 step 1) |
| `image/color.RGBA64` (type) | 0 / 6 | std/image has the color conversions as functions but no Color model values per type (std-coverage.md partial, 0578 step 1) |

## Correction found

- std-coverage.md listed net/http/httputil as "none, planned" while
  std/http has `reverse_proxy` and `ReverseProxy`; the row now says partial
  (DumpRequest and the rest are missing).

## Next

The 876 unmapped rows are mostly packages whose rows say done (archive,
compress, crypto, encoding/*, mime, net/smtp, sort, path/filepath, os/exec):
each needs a symbol by symbol line, which is where a done row hiding a miss
would show.
