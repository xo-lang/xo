# 0116: `use c` on the native backends (decision 0093 stage 2)

Status: Accepted (2026-10-10, made while implementing decision 0093
stage 2 at the owner's request). Implemented 2026-10-10.

## Context

Decision 0093 settled the shape of a foreign function interface: `use go`
on the Go backend, `use c "header.h"` with bindings generated from a C
header on the native backends, the effect `ffi`, boundary values only,
and recording at the boundary. Stage 1 (recording `use go` calls) is
done. Stage 2 needed the concrete choices: the syntax of the line, how a
library to link is named, how a header is read, the type mapping, how
the code generators call C without learning C's calling conventions,
and what of recording can exist while native builds do not record at
all (decision 0083).

## Decision

- **Syntax.** `use c "<header>" { link "<x>" } [as name] [uses none |
  uses e1, e2]`, all on the `use` line (core 4.5, 15). The header is a
  path relative to the module directory, else a header on the C include
  path. Each `link` is a C source, object, or archive relative to the
  module directory (it has `/` or an extension), or a library name
  (`link "z"` is `-lz`). Keeping the link on the line, not in xo.mod,
  puts everything an agent needs to read a call's origin in one place;
  a header used by several files repeats the line, as `use go` does.
- **Effects.** Every binding function declares `uses ffi`; a `uses`
  clause on the line replaces it with the labels a human vouches for
  (`uses none` for pure functions such as checksums), in the spirit of
  the effect ceilings of decision 0086. An unknown label is `XO1104`.
- **Reading headers.** clang's JSON AST dump (`clang -fsyntax-only
  -Xclang -ast-dump=json`) of a one line file that includes the header.
  clang is already required by the native backends, sees through macros
  (zlib's `ZEXTERN ... OF((...))`) and typedefs, and reports
  `desugaredQualType`. clang leaves out a location's file when it
  repeats the previous one, so the dump is decoded with its key order and
  the elision replayed to learn which file a declaration is in; only
  functions declared in the named header bind, while typedefs and
  records come from every file it includes.
- **Adapters.** `internal/cbind` writes, beside the declaration file, a C
  adapter per header: one function `xo_c_<name>_<hash>_<fn>` per binding
  with the native runtime's call convention (decision 0068: words,
  doubles, Str and Bytes as bytes and length, a Str result as a runtime
  string and its length through a pointer). The SSA builder lowers a
  call to `rt.<that name>`, so neither code generator learns C's ABI
  for narrow integers, floats, or structs; the adapter does the
  conversions in C. The native build compiles the adapters and the
  `link` sources with the program (`internal/build/cffi.go`) and puts
  `-l` flags after the objects.
- **Type mapping (the subset).** `int` Int32, `unsigned` UInt32, `long`
  and `long long` Int, unsigned 64 bit UInt64, `short` and `char` the
  sized integers, `float` Float32, `double` Float, `_Bool` Bool, enums
  Int32. `const char *` parameter: Str, copied and NUL terminated;
  `const char *` result: Str, copied, NULL as "". A const byte pointer
  followed by an integer parameter whose name reads as a length (`n`,
  `len`, `size`, `count`): one Str (`const char *`) or Bytes parameter,
  not copied (C must not keep it). A struct of scalars by value, in or
  out: an Xo struct, copied field by field (a struct result is stored in
  a thread local slot by the adapter and read back through one getter
  per field, since the runtime convention returns one word). A pointer
  to a struct the header declares but does not define: an opaque handle
  (a word, never reference counted; a result is `T?`, None for NULL;
  not comparable, decision 0112). Everything else is skipped with a
  reason (`XO1105` when used).
- **Go backend.** `use c` is `XO1201` (impossible) from the checker
  whenever the check is not native, so `xo check` without `--no-gc`
  reports it, mirroring `use go` under `--no-gc`. The description of
  XO1201 is unchanged (a construct not available on the other backend
  family); the spec table lists the checker as a reporter too.
- **Record and replay.** Native builds do not record (decision 0083),
  so nothing is recorded yet. Each declaration carries the replay mode
  decision 0093 gives it (`/// Replay: by value`, or `a C handle
  crosses: re executed`) so the native recorder, when it exists, has the
  table it needs: by value calls become `ffi.call(<name>, <hash>)`
  effects with their results, handle calls are re executed and marked.
- **Codes.** `XO1104` (header cannot be bound) and `XO1105` (name skipped)
  are new, beside the Go interop codes XO1101 and XO1102, whose texts
  name Go.

## Consequences

- Native programs can call C libraries; tiny.h (internal/cbind
  testdata) and the system zlib run on arm64 and LLVM with identical
  output and no live allocation at exit (internal/llvm TestUseC,
  TestUseCZlib).
- A program with `use c` does not `xo check` without `--no-gc`, and
  editor tooling that checks for the Go backend shows XO1201 on the line.
- A build that links a module's own C files is not cached by the native
  binary cache (its inputs lie outside the build directory); one that
  only links system libraries is.
- A C crash ends the process; handles are freed only by the library's
  own functions.

## Left (next steps, ranked by value)

1. Native recording at the adapter: a `rt.ffi_enter`/`ffi_value` pair
   around each by value call once native `--record` exists (decision
   0083 lists it as not available), so `xo replay` answers C calls.
2. Output parameters and buffers: `T *out` as an extra result, and
   `(char *buf, size_t cap)` as a Bytes result with a declared capacity
   rule.
3. Pointers to defined structs (zlib's `z_stream`, `gzFile`): a handle
   that Xo allocates and frees through the adapter.
4. Errors: a binding annotation (xo.mod line) that maps a negative or
   nonzero return to a failure (`T ! CErr`).
5. Lists, tuples, and nested structs at the boundary; constants
   (`#define`, enum members) as Xo constants; callbacks (function
   pointers).
6. A binding cache keyed on header contents across processes (today the
   header is read once per process).

## Alternatives

- `link` lines in xo.mod: keeps the `use` line short, but separates the
  library from the import an agent reads, and a module would need a key
  to tie a link line to its header.
- Parsing headers with a hand written C parser: no clang dependency,
  but macros and system headers make it unreliable; clang is already
  required by both native backends.
- Calling C functions directly from generated code: needs each backend
  to implement the C ABI for every scalar width, floats, and structs
  (by value structs differ per platform), where the adapter gets it
  from the C compiler.
