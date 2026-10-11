# 0164: Formats: CSV, TOML, gzip/tar/zip, regexp

Status: Accepted, implemented (2026-10-11). `std/csv` and `std/toml` on
every backend; `std/regexp` and `std/archive` on the Go backend, XO1201
(planned) on native builds. Spec: std.md 16 to 19.

## Context

Command line tools read and write a handful of formats that Xo could
only reach through `use go` (which native builds cannot compile and
which costs a vouch in a dependency, decision 0156): CSV exports, TOML
configuration files, compressed archives, and regular expressions for
searching and validating text. Each should be a std module that works
without capabilities (pure functions over `Str` and `Bytes`; files come
from `Fs`).

## Decision

1. **`std/csv`**: `parse(text, sep = ',') -> List[List[Str]]`,
   `parse_maps` (header row to maps), `format(rows, sep)`; RFC 4180
   with Go's leniencies that matter (variable field counts, skipped
   empty lines, `\r\n` in quotes read as `\n`) and errors with line and
   column. Our own parser (not `encoding/csv`) in Go and the same in Xo
   for the native backends, so both give byte for byte the same records,
   text, and errors (golden `csv_basic` on Go, LLVM, arm64).
2. **`std/regexp`**: a `Regex` handle from `compile` (Result) or `must`
   (faults: for patterns written in the program), with `is_match`,
   `find`, `find_index`, `find_all`, `captures` (`List[Str?]`, None for
   groups that did not take part), `named`, `replace_all` (`$1`,
   `${name}`), `replace_all_literal`, `split`, and `quote`. RE2 (Go's
   `regexp`): linear time, so untrusted patterns and inputs are safe.
3. **`std/archive`**: one module for gzip, tar, and zip, in memory:
   `gzip`/`gunzip`, `tar`/`untar`, `zip`/`unzip` over `List[Entry]`
   (`path`, `data`, `mode`, `is_dir`, `modified`), plus `file` and `dir`
   constructors. Entries whose paths are absolute or climb out of the
   root are refused when writing and when reading (zip slip), as are
   links and devices: what `untar` returns can be written with
   `Fs.write` without checking. `gzip` output depends only on the data.
4. **`std/toml`**: `decode[T: Json](text) -> T ! DecodeErr` and
   `to_json(text)`. A TOML 1.0 document is mapped onto the JSON value
   model and decoded by the existing `derive Json` rules, so
   configuration structs need no new derive, defaults and `Duration`
   text work as in JSON, and one set of decoding rules is learned.
   Dates become RFC 3339 text (decodable into `Time` when they have an
   offset); `inf` and `nan` are errors. The parser is our own (no TOML
   package in Go's standard library; std/rt stays standard library
   only), tested against the TOML 1.0 cases in `std/rt/toml_test.go`.
   `DecodeErr` keeps syntax and type errors apart.
5. **Native backends**: CSV and TOML are ported to Xo
   (`std/native/csv.xo`, `std/native/toml.xo`; TOML's JSON text is
   decoded by the native JSON codec), checked against the Go backend by
   the goldens `csv_basic` and `toml_basic` (the latter generated from
   the cases of `std/rt/toml_test.go`). Regexp and archives report
   XO1201 (planned). The plans: archives need deflate, which
   means vendoring a C inflate/deflate (zlib or miniz) into std/llrt like
   Mbed TLS (decision 0105); regexp needs an RE2-equivalent engine (a
   port of Go's `regexp` to Xo or C), since the platform's `regcomp`
   has other syntax and backtracking.
6. A std parameter default may now be a rune literal (`sep: Rune =
   ','`), in internal/gen's std default lowering.

## Consequences

- Tools read configs with `toml.decode[Config](os.fs.read_text(p)?)?`
  and CSV with `csv.parse_maps`, and validate input with `regexp`,
  without `use go` and without vouches.
- `std/regexp` is a module name; programs that bound Go's `regexp` with
  `use go "regexp"` keep working but must alias one of them to import
  both.
- Native builds of programs using regexp or archives fail with XO1201
  until the plans above land; the Go backend has them all.

## Alternatives

- `encoding/csv` on the Go backend: its error texts and edge cases
  (`LazyQuotes`, `FieldsPerRecord`) would have to be reproduced in the
  native port exactly; owning a 150 line parser is simpler.
- A `derive Toml`: duplicates the JSON rules; mapping to the JSON model
  reuses them and their tests.
- A dynamic `toml.Value` tree: useful for tools that edit TOML; `to_json`
  plus `json.decode` into maps covers reading; writing TOML (`encode`)
  is left for later.
- Separate `std/gzip`, `std/tar`, `std/zip`: three tiny modules; one
  `std/archive` is easier to find.
- POSIX `regcomp` natively: different syntax (no `\d`, no non-greedy)
  and exponential worst cases; the same program would behave
  differently per backend.
