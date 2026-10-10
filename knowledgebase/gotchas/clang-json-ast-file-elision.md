# clang's JSON AST dump leaves out repeated file names

Status: Observed (2026-10-10, internal/cbind on tiny.h and zlib.h)

Symptom: in `clang -Xclang -ast-dump=json`, only the first declaration of
a header carries `"loc": {"file": "tiny.h", ...}`; the following ones have
a `loc` with `offset`, `line`, and `col` but no `file`. Decoding the dump
into Go maps and reading `loc.file` per declaration loses which header a
declaration is in, so a binding would either bind nothing or bind every
function of every system header the header includes.

Cause: clang's JSONNodeDumper writes a location's `file` (and `line`)
only when it differs from the previously written location, in output
order, including locations nested in ranges and in `spellingLoc` and
`expansionLoc` of macro expansions (zlib's `ZEXTERN` declarations). An
`includedFrom` object also has a `file` key that is not a location.

Fix: internal/cbind decodes the dump with `json.Decoder.Token` into
objects that keep key order (`clang.go` `obj`) and replays the elision:
it walks every location in order, remembers the last `file`, skips
`includedFrom`, and gives each top level declaration the file current
after its own `loc` (the expansion's file for a macro). Typedefs and
records are read from every file; only functions in the named header
bind.
